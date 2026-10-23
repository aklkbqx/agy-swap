package app

import (
	"testing"
	"time"
)

func scheduledAccount(observed time.Time, gemini, thirdParty float64, reset time.Time) Account {
	bucket := func(id string, f float64) map[string]any {
		return map[string]any{"id": id, "name": "Weekly", "window": "weekly", "remaining_fraction": f, "reset_at": isoTime(reset)}
	}
	return Account{"email": "user@example.com", "quota_snapshot": map[string]any{
		"observed_at": isoTime(observed), "tier": map[string]any{"id": "t", "name": "T"},
		"groups": []any{
			map[string]any{"id": "gemini", "name": "Gemini", "buckets": []any{bucket("gemini-weekly", gemini)}},
			map[string]any{"id": "third_party", "name": "Claude and GPT", "buckets": []any{bucket("3p-weekly", thirdParty)}},
		},
	}}
}

func TestTheActiveAccountIsDueEveryTwentySecondsAndOthersEveryTwoMinutes(t *testing.T) {
	now := time.Date(2030, 1, 1, 12, 0, 0, 0, time.UTC)
	account := scheduledAccount(now, 0.5, 0.5, now.Add(48*time.Hour))
	if due := quotaDueAt(account, true, time.Time{}, time.Time{}); !due.Equal(now.Add(quotaActiveEvery)) {
		t.Fatalf("active due %s, want %s", due, now.Add(quotaActiveEvery))
	}
	if due := quotaDueAt(account, false, time.Time{}, time.Time{}); !due.Equal(now.Add(quotaOtherEvery)) {
		t.Fatalf("other due %s, want %s", due, now.Add(quotaOtherEvery))
	}
	if quotaActiveEvery != 20*time.Second || quotaOtherEvery != 2*time.Minute {
		t.Fatalf("intervals %s / %s", quotaActiveEvery, quotaOtherEvery)
	}
}

func TestAnAccountWithoutQuotaIsDueAtOnce(t *testing.T) {
	if due := quotaDueAt(Account{"email": "new@example.com"}, false, time.Time{}, time.Time{}); !due.IsZero() {
		t.Fatalf("an account never read is due %s, want now", due)
	}
}

func TestAUsedUpAccountWaitsForItsResetThenRefreshesRightAfter(t *testing.T) {
	now := time.Date(2030, 1, 1, 12, 0, 0, 0, time.UTC)
	reset := now.Add(3 * time.Hour)
	account := scheduledAccount(now, 0, 0, reset)
	if due := quotaDueAt(account, false, time.Time{}, time.Time{}); !due.Equal(reset.Add(quotaResetSlack)) {
		t.Fatalf("used-up account due %s, want just after its reset %s", due, reset)
	}
	// A reset sooner than the usual interval brings the next read forward.
	soon := scheduledAccount(now, 0.5, 0.5, now.Add(30*time.Second))
	if due := quotaDueAt(soon, false, time.Time{}, time.Time{}); !due.Equal(now.Add(30*time.Second + quotaResetSlack)) {
		t.Fatalf("a reset in 30 s made the account due %s", due)
	}
}

func TestFailedReadsAndBackOffDelayTheNextRead(t *testing.T) {
	now := time.Date(2030, 1, 1, 12, 0, 0, 0, time.UTC)
	stale := scheduledAccount(now.Add(-time.Hour), 0.5, 0.5, now.Add(48*time.Hour))
	tried := now.Add(-5 * time.Second)
	if due := quotaDueAt(stale, true, tried, time.Time{}); !due.Equal(tried.Add(quotaActiveEvery)) {
		t.Fatalf("a read that just failed made the account due %s, want %s", due, tried.Add(quotaActiveEvery))
	}
	until := now.Add(4 * time.Minute)
	if due := quotaDueAt(stale, true, time.Time{}, until); !due.Equal(until) {
		t.Fatalf("back-off until %s gave due %s", until, due)
	}
}

func TestBackOffDoublesWithJitterUpToTenMinutes(t *testing.T) {
	var b quotaBackoff
	now := time.Date(2030, 1, 1, 12, 0, 0, 0, time.UTC)
	last := time.Duration(0)
	for i := 0; i < 8; i++ {
		until := b.fail(now, func() float64 { return 0.5 })
		wait := until.Sub(now)
		if wait < last || wait > quotaBackoffMax {
			t.Fatalf("step %d waits %s after %s", i, wait, last)
		}
		last = wait
	}
	if last != quotaBackoffMax {
		t.Fatalf("back-off settles at %s, want %s", last, quotaBackoffMax)
	}
	b.succeed()
	if until := b.fail(now, func() float64 { return 0.5 }); until.Sub(now) > 2*quotaActiveEvery {
		t.Fatalf("a success did not reset the back-off: %s", until.Sub(now))
	}
}

func TestOnlyRateLimitsAndServerErrorsBackOff(t *testing.T) {
	for status, want := range map[int]bool{429: true, 500: true, 503: true, 401: false, 403: false, 404: false} {
		if got := backsOff(&cloudError{Method: "retrieveUserQuotaSummary", Status: status}); got != want {
			t.Errorf("HTTP %d backs off = %v, want %v", status, got, want)
		}
	}
}
