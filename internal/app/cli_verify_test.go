package app

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// withCloud points the fixture's quota checks at a Google stand-in that
// answers loadCodeAssist with verify=true as an account that needs verification.
func (f *cliFixture) withCloud(t *testing.T, verify bool) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case verify && strings.HasSuffix(r.URL.Path, "loadCodeAssist"):
			w.WriteHeader(http.StatusForbidden)
			fmt.Fprint(w, `{"error":{"details":[{"reason":"VALIDATION_REQUIRED","metadata":{"validation_url":"https://accounts.google.com/v","uiMessage":"Verify your account to continue."}}]}}`)
		case strings.HasSuffix(r.URL.Path, "loadCodeAssist"):
			fmt.Fprint(w, `{"cloudaicompanionProject":"p","currentTier":{"id":"free-tier"}}`)
		default:
			fmt.Fprint(w, `{"groups":[{"buckets":[{"bucketId":"gemini-weekly","displayName":"Weekly","window":"weekly","remainingFraction":0.5,"resetTime":"2030-01-01T00:00:00Z"}]}]}`)
		}
	}))
	t.Cleanup(server.Close)
	f.a.http.cloudAPI = server.URL + "/"
	f.a.quota = NewQuotaService(f.a.http, f.a.store)
	f.a.quota.SetVault(f.vault)
}

func (f *cliFixture) markVerify(t *testing.T, email, link string) {
	t.Helper()
	accounts, err := f.a.store.Load(false)
	if err != nil {
		t.Fatal(err)
	}
	accounts.ByEmail[email]["eligibility"] = (&eligibilityError{Message: "Verify", URL: link}).record(time.Now())
	if err := f.a.store.Save(accounts); err != nil {
		t.Fatal(err)
	}
}

func TestVerifyOpensTheStoredPageAndPrintsIt(t *testing.T) {
	f := newCLIFixture(t, "v@example.invalid")
	f.markVerify(t, "v@example.invalid", "https://accounts.google.com/v")
	var opened string
	f.a.openURL = func(_ context.Context, link string) error { opened = link; return nil }
	if code := f.run("", "verify", "v@example.invalid"); code != 0 || opened != "https://accounts.google.com/v" {
		t.Fatalf("code=%d opened=%q out=%s err=%s", code, opened, f.out, f.errOut)
	}
	if !strings.Contains(f.out.String(), "\nhttps://accounts.google.com/v\n") {
		t.Fatalf("link not printed on its own line:\n%s", f.out)
	}
	if strings.Contains(f.out.String(), "Press r") || !strings.Contains(f.out.String(), "agy-swap limits --refresh") {
		t.Fatalf("the shell has no r key:\n%s", f.out)
	}
}

func TestVerifyWithoutAnOpenerPrintsTheLinkAndFails(t *testing.T) {
	f := newCLIFixture(t, "v@example.invalid")
	f.markVerify(t, "v@example.invalid", "https://accounts.google.com/v")
	f.a.openURL = func(context.Context, string) error { return exec.ErrNotFound }
	if code := f.run("", "verify", "v@example.invalid"); code != 1 || !strings.Contains(f.out.String(), "https://accounts.google.com/v") {
		t.Fatalf("code=%d out=%s", code, f.out)
	}
}

func TestVerifyChecksAnAccountWithoutARecord(t *testing.T) {
	f := newCLIFixture(t, "ok@example.invalid")
	f.withCloud(t, false)
	f.a.openURL = func(context.Context, string) error { t.Fatal("nothing to open"); return nil }
	if code := f.run("", "verify", "ok@example.invalid"); code != 0 || !strings.Contains(f.out.String(), "no verification needed") {
		t.Fatalf("code=%d out=%s err=%s", code, f.out, f.errOut)
	}
	g := newCLIFixture(t, "v@example.invalid")
	g.withCloud(t, true)
	var opened string
	g.a.openURL = func(_ context.Context, link string) error { opened = link; return nil }
	if code := g.run("", "verify", "v@example.invalid"); code != 0 || opened != "https://accounts.google.com/v" {
		t.Fatalf("code=%d opened=%q out=%s err=%s", code, opened, g.out, g.errOut)
	}
}

func TestVerifyNeedsAKnownAccount(t *testing.T) {
	f := newCLIFixture(t, "v@example.invalid")
	if code := f.run("", "verify", "nobody@example.invalid"); code != 1 || !strings.Contains(f.errOut.String(), "not found") {
		t.Fatalf("code=%d err=%s", code, f.errOut)
	}
	if code := f.run("", "verify"); code == 0 {
		t.Fatal("verify without an account must fail")
	}
}

func TestAddWarnsWhenTheNewAccountNeedsVerification(t *testing.T) {
	f := newCLIFixture(t)
	f.withCloud(t, true)
	f.userInfo = "new@example.invalid"
	f.a.openURL = func(context.Context, string) error {
		t.Fatal("a non-interactive add must not open a browser")
		return nil
	}
	token := tokenBlob(t, "new@example.invalid", true, "refresh-new", time.Now().Add(time.Hour))
	if code := f.run(token, "add", "--token", "-"); code != 0 {
		t.Fatalf("code=%d out=%s err=%s", code, f.out, f.errOut)
	}
	out := f.out.String()
	if !strings.Contains(out, "needs Google verification") || !strings.Contains(out, "\nhttps://accounts.google.com/v\n") {
		t.Fatalf("out = %s", out)
	}
	accounts, _ := f.a.store.Load(false)
	if _, ok := accountEligibility(accounts.ByEmail["new@example.invalid"]); !ok {
		t.Fatal("the verification record was not stored")
	}
}

func TestAddOffersToOpenTheVerificationPage(t *testing.T) {
	for _, answer := range []string{"\n", "n\n"} {
		f := newCLIFixture(t, "v@example.invalid")
		f.withCloud(t, true)
		f.a.stdinTTY = true
		var opened string
		f.a.openURL = func(_ context.Context, link string) error { opened = link; return nil }
		f.a.In = strings.NewReader(answer)
		f.a.lineReader = nil
		_, _ = f.a.checkNewAccount(context.Background(), "v@example.invalid")
		if want := answer == "\n"; (opened != "") != want {
			t.Fatalf("answer %q: opened=%q out=%s", answer, opened, f.out)
		}
	}
}

func TestListSaysAnAccountNeedsVerification(t *testing.T) {
	account := Account{"email": "v@example.invalid", "eligibility": (&eligibilityError{Message: "Verify"}).record(time.Now())}
	if got := accountStatus(account, palette{}, time.Now()); !strings.Contains(got, "Verification required") || !strings.Contains(got, "agy-swap verify v@example.invalid") {
		t.Fatalf("status = %q", got)
	}
}

func TestCompletionOffersVerify(t *testing.T) {
	for _, shell := range []string{"bash", "zsh", "fish", "powershell"} {
		f := newCLIFixture(t)
		if code := f.run("", "completion", shell); code != 0 || !strings.Contains(f.out.String(), "verify") {
			t.Fatalf("%s: code=%d out=%s err=%s", shell, code, f.out, f.errOut)
		}
	}
}

func TestVerifyRefreshesStoredRecordBeforeOpening(t *testing.T) {
	// Case A: Account was previously marked VERIFY, then owner completed verification in browser.
	f := newCLIFixture(t, "v@example.invalid")
	f.withCloud(t, false)
	f.markVerify(t, "v@example.invalid", "https://accounts.google.com/old-expired")
	f.a.openURL = func(context.Context, string) error {
		t.Fatal("must not open stale verification link once Google says the account works")
		return nil
	}
	if code := f.run("", "verify", "v@example.invalid"); code != 0 || !strings.Contains(f.out.String(), "no verification needed") {
		t.Fatalf("code=%d out=%s err=%s", code, f.out, f.errOut)
	}
	accounts, _ := f.a.store.Load(false)
	if _, ok := accountEligibility(accounts.ByEmail["v@example.invalid"]); ok {
		t.Fatal("stored eligibility record should be cleared after live verify check passes")
	}

	// Case B: Account still needs verification and Google issues a fresh token URL.
	g := newCLIFixture(t, "still@example.invalid")
	g.withCloud(t, true)
	g.markVerify(t, "still@example.invalid", "https://accounts.google.com/stale-token")
	var opened string
	g.a.openURL = func(_ context.Context, link string) error { opened = link; return nil }
	if code := g.run("", "verify", "still@example.invalid"); code != 0 || opened != "https://accounts.google.com/v" {
		t.Fatalf("expected fresh URL https://accounts.google.com/v, got code=%d opened=%q", code, opened)
	}
}

func TestSwitchWarnsWhenTargetAccountNeedsVerification(t *testing.T) {
	f := newCLIFixture(t, "ready@example.invalid", "v@example.invalid")
	f.markVerify(t, "v@example.invalid", "https://accounts.google.com/v")
	if code := f.run("", "switch", "v@example.invalid"); code != 0 {
		t.Fatalf("code=%d out=%s err=%s", code, f.out, f.errOut)
	}
	if !strings.Contains(f.out.String(), "needs Google verification") || !strings.Contains(f.out.String(), "agy-swap verify v@example.invalid") {
		t.Fatalf("expected verification warning on switch, got:\n%s", f.out)
	}
}

func TestDownstreamCommandsTreatVerifyAccountsAsUnreadyAndSuppressStaleBars(t *testing.T) {
	now := time.Now().UTC()
	freshSnapshot := map[string]any{
		"observed_at": isoTime(now),
		"tier":        map[string]any{"id": "free-tier", "name": "Free"},
		"groups": []any{
			map[string]any{
				"id":   "gemini",
				"name": "Gemini",
				"buckets": []any{
					map[string]any{"id": "gemini-weekly", "name": "Weekly", "window": "weekly", "remaining_fraction": 0.85, "reset_at": "2030-01-01T00:00:00Z"},
				},
			},
		},
	}

	f := newCLIFixture(t, "verify@example.invalid", "ready@example.invalid")
	accounts, _ := f.a.store.Load(false)
	accounts.ByEmail["verify@example.invalid"]["quota_snapshot"] = freshSnapshot
	accounts.ByEmail["verify@example.invalid"]["eligibility"] = (&eligibilityError{
		Reason:  "VALIDATION_REQUIRED",
		Message: "Verify to authenticate access",
		URL:     "https://accounts.google.com/v?secret=1",
	}).record(now)
	accounts.ByEmail["ready@example.invalid"]["quota_snapshot"] = freshSnapshot
	if err := f.a.store.Save(accounts); err != nil {
		t.Fatal(err)
	}

	// 1. rankAccounts must never mark verify@ Ready even with a <2m snapshot
	settings, _ := f.a.loadSettings()
	ranked := rankAccounts(accounts, settings, "verify@example.invalid", "", "", "")
	for _, item := range ranked {
		if item.Email == "verify@example.invalid" {
			if item.Ready || !strings.Contains(strings.Join(item.Reasons, ", "), "needs Google verification") {
				t.Fatalf("verify account ranked ready or missing reason: %#v", item)
			}
		}
	}

	// 2. accountSummary exposes sanitized eligibility without leaking the secret URL
	summary := accountSummary(accounts.ByEmail["verify@example.invalid"], false)
	eligSummary := getMap(summary["eligibility"])
	if getString(eligSummary, "state") != "verify_required" || eligSummary["has_url"] != true || getString(eligSummary, "url") != "" {
		t.Fatalf("unexpected accountSummary eligibility: %#v", eligSummary)
	}

	// 3. cmdList hides stale quota bars when account needs verification
	if code := f.run("", "list"); code != 0 {
		t.Fatalf("list failed: %d", code)
	}
	listOut := f.out.String()
	verifyIdx := strings.Index(listOut, "verify@example.invalid")
	readyIdx := strings.Index(listOut, "ready@example.invalid")
	if verifyIdx < 0 || readyIdx < 0 || strings.Contains(listOut[verifyIdx:readyIdx], "85%") {
		t.Fatalf("cmdList printed stale 85%% quota bar under verify account:\n%s", listOut)
	}
}
