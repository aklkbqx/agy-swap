package app

import (
	"errors"
	"hash/fnv"
	"time"
)

// The TUI reads each account's quota on its own schedule instead of all at
// once every minute: in safe mode (default) inactive accounts are never polled
// in the background and the active account is refreshed passively on activity;
// balanced and live modes opt into periodic polling.

const (
	quotaActiveEvery   = 20 * time.Second
	quotaOtherEvery    = 2 * time.Minute
	quotaSafeActiveCap = 2 * time.Minute
	quotaBalancedAct   = 60 * time.Second
	quotaBalancedOther = 10 * time.Minute
	// quotaResetSlack is how long after a reset Google is asked again, so the
	// answer already shows the fresh pool.
	quotaResetSlack = 3 * time.Second
	quotaBackoffMax = 10 * time.Minute
)

// safeResetJitter returns a deterministic 15–45s per-account jitter so multiple
// accounts whose reset_at aligns never wake up and poll simultaneously.
func safeResetJitter(email string) time.Duration {
	h := fnv.New32a()
	_, _ = h.Write([]byte(email))
	return 15*time.Second + time.Duration(h.Sum32()%31)*time.Second
}

// quotaDueAt is when an account's quota should next be read in live mode: zero for at once.
// tried is the last read that failed (zero for none) and backoffUntil the end
// of a back-off after Google refused.
func quotaDueAt(account Account, active bool, tried, backoffUntil time.Time) time.Time {
	return quotaDueAtForMode(account, active, tried, backoffUntil, "live")
}

// quotaDueAtForMode computes the next scheduled quota poll according to
// settings.Quota.PollMode ("safe", "balanced", or "live").
func quotaDueAtForMode(account Account, active bool, tried, backoffUntil time.Time, mode string) time.Time {
	if _, ineligible := accountEligibility(account); ineligible {
		// Never poll VERIFY or SUSPENDED accounts automatically in the background.
		return time.Date(9999, 1, 1, 0, 0, 0, 0, time.UTC)
	}
	switch mode {
	case "safe":
		// Passive-first safe mode: idle accounts never poll in the background,
		// except once after an exhausted pool's reset_at + per-account jitter.
		observed, err := parseUTC(getString(getMap(account["quota_snapshot"]), "observed_at"))
		if err != nil {
			// Active account without any snapshot yet may be read once on demand/activity;
			// inactive accounts wait for explicit manual refresh or lazy selection.
			if !active {
				return time.Date(9999, 1, 1, 0, 0, 0, 0, time.UTC)
			}
			var due time.Time
			if !tried.IsZero() {
				due = tried.Add(quotaSafeActiveCap)
			}
			if backoffUntil.After(due) {
				due = backoffUntil
			}
			return due
		}
		var due time.Time
		if wait, _ := quotaWait(account, observed, ""); wait > 0 {
			due = observed.Add(wait + safeResetJitter(getString(account, "email")))
		} else {
			// Healthy account: stay passive until activity or manual refresh.
			return time.Date(9999, 1, 1, 0, 0, 0, 0, time.UTC)
		}
		if !tried.IsZero() && tried.Add(quotaSafeActiveCap).After(due) {
			due = tried.Add(quotaSafeActiveCap)
		}
		if backoffUntil.After(due) {
			due = backoffUntil
		}
		return due
	case "balanced":
		every := quotaBalancedOther
		if active {
			every = quotaBalancedAct
		}
		observed, err := parseUTC(getString(getMap(account["quota_snapshot"]), "observed_at"))
		var due time.Time
		if err == nil {
			due = observed.Add(every)
			if wait, _ := quotaWait(account, observed, ""); wait > 0 {
				due = observed.Add(wait + safeResetJitter(getString(account, "email")))
			}
			if reset, ok := nextQuotaReset(account, observed); ok && reset.Add(safeResetJitter(getString(account, "email"))).Before(due) {
				due = reset.Add(safeResetJitter(getString(account, "email")))
			}
		}
		if !tried.IsZero() && tried.Add(every).After(due) {
			due = tried.Add(every)
		}
		if backoffUntil.After(due) {
			due = backoffUntil
		}
		return due
	default:
		every := quotaOtherEvery
		if active {
			every = quotaActiveEvery
		}
		observed, err := parseUTC(getString(getMap(account["quota_snapshot"]), "observed_at"))
		var due time.Time
		if err == nil {
			due = observed.Add(every)
			if wait, _ := quotaWait(account, observed, ""); wait > 0 {
				// The shared pools are used up: nothing changes before they reset.
				due = observed.Add(wait + quotaResetSlack)
			}
			if reset, ok := nextQuotaReset(account, observed); ok && reset.Add(quotaResetSlack).Before(due) {
				due = reset.Add(quotaResetSlack)
			}
		}
		if !tried.IsZero() && tried.Add(every).After(due) {
			due = tried.Add(every) // a failed read is not retried at once
		}
		if backoffUntil.After(due) {
			due = backoffUntil
		}
		return due
	}
}

// nextQuotaReset is the earliest pool reset after the snapshot was taken.
func nextQuotaReset(account Account, observed time.Time) (time.Time, bool) {
	var earliest time.Time
	for _, rawGroup := range quotaGroups(account) {
		for _, rawBucket := range getSlice(getMap(rawGroup)["buckets"]) {
			reset, err := parseUTC(getString(getMap(rawBucket), "reset_at"))
			if err != nil || !reset.After(observed) {
				continue
			}
			if earliest.IsZero() || reset.Before(earliest) {
				earliest = reset
			}
		}
	}
	return earliest, !earliest.IsZero()
}

// quotaBackoff is one account's back-off after Google refused a read: the
// wait doubles from the active interval up to ten minutes, with jitter so
// accounts do not retry together, and a good read clears it.
type quotaBackoff struct {
	wait time.Duration
}

func (b *quotaBackoff) fail(now time.Time, random func() float64) time.Time {
	switch {
	case b.wait == 0:
		b.wait = quotaActiveEvery
	case b.wait < quotaBackoffMax:
		b.wait = min(2*b.wait, quotaBackoffMax)
	}
	// Jitter takes up to a quarter off, never adding past the cap.
	jitter := time.Duration(float64(b.wait/4) * random())
	if b.wait == quotaBackoffMax {
		jitter = 0
	}
	return now.Add(b.wait - jitter)
}

func (b *quotaBackoff) succeed() { b.wait = 0 }

// backsOff says Google refused for load (429) or failed (5xx), which more
// requests would not fix.
func backsOff(err error) bool {
	var cloud *cloudError
	if errors.As(err, &cloud) {
		return cloud.Status == 429 || cloud.Status >= 500
	}
	var httpErr *httpStatusError
	if errors.As(err, &httpErr) {
		return httpErr.Status == 429 || httpErr.Status >= 500
	}
	return false
}
