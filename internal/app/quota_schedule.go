package app

import (
	"errors"
	"time"
)

// The TUI reads each account's quota on its own schedule instead of all at
// once every minute: the account Antigravity is using often, the others less,
// an account whose shared pools are used up when they reset, and any account
// right after one of its pools resets.

const (
	quotaActiveEvery = 20 * time.Second
	quotaOtherEvery  = 2 * time.Minute
	// quotaResetSlack is how long after a reset Google is asked again, so the
	// answer already shows the fresh pool.
	quotaResetSlack = 3 * time.Second
	quotaBackoffMax = 10 * time.Minute
)

// quotaDueAt is when an account's quota should next be read: zero for at once.
// tried is the last read that failed (zero for none) and backoffUntil the end
// of a back-off after Google refused.
func quotaDueAt(account Account, active bool, tried, backoffUntil time.Time) time.Time {
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
	if !errors.As(err, &cloud) {
		return false
	}
	return cloud.Status == 429 || cloud.Status >= 500
}
