package app

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// countingQuotaServer answers like Google and counts the calls; summaryStatus
// fails the summary when set, and date is sent as the Date header.
type countingQuotaServer struct {
	loads, summaries atomic.Int32
	summaryStatus    atomic.Int32
	date             time.Time
	reset            string
}

func (c *countingQuotaServer) start(t *testing.T) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if !c.date.IsZero() {
			w.Header().Set("Date", c.date.UTC().Format(http.TimeFormat))
		}
		switch r.URL.Path {
		case "/loadCodeAssist":
			c.loads.Add(1)
			fmt.Fprint(w, `{"cloudaicompanionProject":"p","paidTier":{"id":"g1-pro-tier"}}`)
		case "/retrieveUserQuotaSummary":
			c.summaries.Add(1)
			if status := c.summaryStatus.Load(); status != 0 {
				w.WriteHeader(int(status))
				fmt.Fprint(w, `{}`)
				return
			}
			fmt.Fprintf(w, `{"groups":[{"displayName":"Gemini","buckets":[{"bucketId":"gemini-weekly","window":"weekly","remainingFraction":0.4,"resetTime":%q}]},{"displayName":"Claude and GPT models","buckets":[{"bucketId":"3p-weekly","window":"weekly","remainingFraction":0.5,"resetTime":%q}]}]}`, c.reset, c.reset)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server
}

func countingQuota(t *testing.T, c *countingQuotaServer) (*QuotaService, Account) {
	t.Helper()
	if c.reset == "" {
		c.reset = "2030-01-01T00:00:00Z"
	}
	server := c.start(t)
	httpService := NewHTTPService(io.Discard)
	httpService.oauthURL = server.URL + "/oauth"
	httpService.cloudAPI = server.URL + "/"
	account := Account{"email": "user@example.com", "token_data": tokenBlob(t, "user@example.com", true, "r", time.Now().Add(time.Hour))}
	return NewQuotaService(httpService, nil), account
}

func TestARepeatReadSkipsTheProjectLookup(t *testing.T) {
	c := &countingQuotaServer{}
	quota, account := countingQuota(t, c)
	for i := 0; i < 3; i++ {
		if _, err := quota.Fetch(context.Background(), account); err != nil {
			t.Fatal(err)
		}
	}
	if c.loads.Load() != 1 || c.summaries.Load() != 3 {
		t.Fatalf("three reads made %d project lookups and %d summaries, want 1 and 3", c.loads.Load(), c.summaries.Load())
	}
}

func TestTheProjectIsLookedUpAgainAfterAFailureOrFifteenMinutes(t *testing.T) {
	c := &countingQuotaServer{}
	quota, account := countingQuota(t, c)
	now := time.Now()
	quota.now = func() time.Time { return now }
	if _, err := quota.Fetch(context.Background(), account); err != nil {
		t.Fatal(err)
	}
	c.summaryStatus.Store(500)
	if _, err := quota.Fetch(context.Background(), account); err == nil {
		t.Fatal("a failed summary must fail the read")
	}
	c.summaryStatus.Store(0)
	if _, err := quota.Fetch(context.Background(), account); err != nil {
		t.Fatal(err)
	}
	if c.loads.Load() != 2 {
		t.Fatalf("after a failure: %d project lookups, want 2", c.loads.Load())
	}
	now = now.Add(quotaProjectMemo + time.Second)
	if _, err := quota.Fetch(context.Background(), account); err != nil {
		t.Fatal(err)
	}
	if c.loads.Load() != 3 {
		t.Fatalf("after %s: %d project lookups, want 3", quotaProjectMemo, c.loads.Load())
	}
}

func TestResetTimesFollowTheLocalClockWhenItIsOff(t *testing.T) {
	reset := time.Now().Add(48 * time.Hour).UTC().Truncate(time.Second)
	// Google's clock is an hour ahead of this machine's.
	c := &countingQuotaServer{date: time.Now().Add(time.Hour), reset: reset.Format(time.RFC3339)}
	quota, account := countingQuota(t, c)
	snapshot, err := quota.Fetch(context.Background(), account)
	if err != nil {
		t.Fatal(err)
	}
	bucket := getMap(getSlice(getMap(getSlice(snapshot["groups"])[0])["buckets"])[0])
	got, _ := parseUTC(getString(bucket, "reset_at"))
	if diff := got.Sub(reset.Add(-time.Hour)); diff < -2*time.Second || diff > 2*time.Second {
		t.Fatalf("reset_at = %s, want about %s (the reset on this machine's clock)", got, reset.Add(-time.Hour))
	}
	// A clock that agrees leaves the times alone.
	c2 := &countingQuotaServer{date: time.Now(), reset: reset.Format(time.RFC3339)}
	quota2, account2 := countingQuota(t, c2)
	snapshot2, _ := quota2.Fetch(context.Background(), account2)
	if got := getString(getMap(getSlice(getMap(getSlice(snapshot2["groups"])[0])["buckets"])[0]), "reset_at"); got != reset.Format(time.RFC3339) {
		t.Fatalf("reset_at = %s, want %s unchanged", got, reset.Format(time.RFC3339))
	}
}

func TestRefreshEmailsReadsOnlyThoseAccountsAndBacksOffARefusal(t *testing.T) {
	c := &countingQuotaServer{}
	quota, first := countingQuota(t, c)
	quota.store = NewStore(testPaths(t))
	second := Account{"email": "other@example.com", "token_data": tokenBlob(t, "other@example.com", true, "r", time.Now().Add(time.Hour))}
	accounts := NewAccounts()
	accounts.Set("user@example.com", first)
	accounts.Set("other@example.com", second)
	if errs := quota.RefreshEmails(context.Background(), accounts, []string{"user@example.com"}); len(errs) != 0 {
		t.Fatalf("errors: %v", errs)
	}
	if c.summaries.Load() != 1 || getMap(accounts.ByEmail["other@example.com"]["quota_snapshot"]) != nil {
		t.Fatalf("%d summaries; the other account read: %v", c.summaries.Load(), accounts.ByEmail["other@example.com"]["quota_snapshot"] != nil)
	}
	c.summaryStatus.Store(429)
	quota.RefreshEmails(context.Background(), accounts, []string{"other@example.com"})
	if until := quota.BackoffUntil("other@example.com"); !until.After(time.Now()) {
		t.Fatalf("a 429 did not start a back-off: %s", until)
	}
	if until := quota.BackoffUntil("user@example.com"); !until.IsZero() {
		t.Fatalf("the other account backs off too: %s", until)
	}
}

// A timer that fires as a snapshot turns a minute old must not skip it for a
// whole round (it used to, leaving quota up to two minutes old).
func TestASnapshotJustShortOfAMinuteIsDue(t *testing.T) {
	c := &countingQuotaServer{}
	quota, account := countingQuota(t, c)
	quota.store = NewStore(testPaths(t))
	account["quota_snapshot"] = scheduledAccount(time.Now().Add(-57*time.Second), 0.5, 0.5, time.Now().Add(48*time.Hour))["quota_snapshot"]
	accounts := NewAccounts()
	accounts.Set("user@example.com", account)
	quota.Refresh(context.Background(), accounts, false, nil)
	if c.summaries.Load() != 1 {
		t.Fatalf("a 57-second-old snapshot was skipped (%d reads)", c.summaries.Load())
	}
	quota.Refresh(context.Background(), accounts, false, nil)
	if c.summaries.Load() != 1 {
		t.Fatalf("a fresh snapshot was read again (%d reads)", c.summaries.Load())
	}
}
