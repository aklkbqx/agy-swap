package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestBuildAgyLauncherScriptDoesNotMatchPromptContainingHub(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX launcher wrapper test")
	}
	dir := t.TempDir()
	logPath := filepath.Join(dir, "calls.log")
	realBin := filepath.Join(dir, "agy.real")
	realScript := fmt.Sprintf("#!/bin/sh\necho \"ARGS:$*\" >> %q\nexit 0\n", logPath)
	if err := os.WriteFile(realBin, []byte(realScript), 0o755); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(dir, "reload-marker")
	wrapperPath := filepath.Join(dir, "agy")
	script := buildAgyLauncherScriptForPaths(realBin, "/nonexistent/agy-swap", marker, false)
	if err := os.WriteFile(wrapperPath, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	// Passing a prompt that mentions "--hub" inside one argument must NOT enter hub mode.
	cmd := exec.Command(wrapperPath, "explain --hub mode")
	cmd.Env = append(os.Environ(), "AGY_SWAP_SKIP_SYNC=1")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("wrapper prompt failed: %v\n%s", err, out)
	}
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(string(data)); got != "ARGS:explain --hub mode" {
		t.Fatalf("unexpected real binary invocation: %q", got)
	}
	if strings.Contains(script, `case " $* "`) {
		t.Fatal("wrapper script still uses unsafe `case \" $* \"` matching for --hub")
	}
}

func TestEnsureAgyLauncherGuardRollsBackBinaryWhenWrapperWriteFails(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX launcher wrapper test")
	}
	f := newCLIFixture(t)
	home := f.a.paths.Home
	binDir := filepath.Join(home, ".local", "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	agyBin := filepath.Join(binDir, "agy")
	// Minimal Mach-O 64-bit magic payload so isNativeExecutable returns true
	nativePayload := []byte{0xcf, 0xfa, 0xed, 0xfe, 0x01, 0x02, 0x03, 0x04}
	if err := os.WriteFile(agyBin, nativePayload, 0o755); err != nil {
		t.Fatal(err)
	}

	origWriter := writeLauncherWrapper
	writeLauncherWrapper = func(string, []byte) error {
		return errors.New("simulated wrapper write failure")
	}
	t.Cleanup(func() { writeLauncherWrapper = origWriter })

	f.a.ensureAgyLauncherGuardForPath(agyBin)

	got, err := os.ReadFile(agyBin)
	if err != nil {
		t.Fatalf("native agy binary was lost after wrapper write failure: %v", err)
	}
	if string(got) != string(nativePayload) {
		t.Fatalf("restored agy binary contents mismatch: got %x want %x", got, nativePayload)
	}
}

func TestReconcileActiveSessionRollsBackVaultOnStoreSaveFailure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("chmod read-only dir test")
	}
	f := newCLIFixture(t, "alpha@example.com")
	f.a.paths.SessionLock = filepath.Join(t.TempDir(), "session.lock")
	accounts, err := f.a.store.Load(false)
	if err != nil {
		t.Fatal(err)
	}
	oldTok := tokenBlob(t, "alpha@example.com", true, "refresh-alpha", time.Now().UTC().Add(30*time.Minute))
	newTok := tokenBlob(t, "alpha@example.com", true, "refresh-alpha", time.Now().UTC().Add(3*time.Hour))
	vault := fakeAccountVault{}
	f.a.vault = vault

	acc := accounts.ByEmail["alpha@example.com"]
	if _, ok := f.a.saveAccountSecret(context.Background(), acc, oldTok); !ok {
		t.Fatal("failed to seed vault secret")
	}
	accounts.Set("alpha@example.com", acc)
	if err := f.a.store.Save(accounts); err != nil {
		t.Fatal(err)
	}
	if !f.a.credentials.writeOAuthFiles(newTok, "alpha@example.com") {
		t.Fatal("failed to write OAuth disk token")
	}

	// Force store.Save to fail on backup write by placing a non-empty directory at AccountsBackup.
	if err := os.RemoveAll(f.a.paths.AccountsBackup); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(f.a.paths.AccountsBackup, "blocker"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(f.a.paths.AccountsBackup) })

	f.a.reconcileActiveSession(context.Background())

	ref := accountSecretRef("alpha@example.com")
	if got := vault[ref]; got != oldTok {
		t.Fatalf("vault secret was mutated even though store.Save failed; got %q want original token", got)
	}
}

func TestTUIPoolWideBodyScrollsDetailPaneWhenOverflowing(t *testing.T) {
	p := makePalette(false)
	a := &Application{p: p, renderClock: func() time.Time { return poolNow }}
	accounts := NewAccounts()
	email := "dev@example.com"
	acc := poolAccount(
		poolGroup("gemini", "Gemini models",
			poolBucket("weekly", 0.09, 54*time.Hour),
			poolBucket("5h", 0.40, 2*time.Hour),
		),
		poolGroup("third_party", "Claude and GPT models",
			poolBucket("weekly", 0.25, 36*time.Hour),
			poolBucket("5h", 0.50, 3*time.Hour),
		),
		poolGroup("argon", "Gemini 4 Argon",
			poolBucket("weekly", 0.60, 72*time.Hour),
		),
	)
	acc["email"] = email
	acc["name"] = "Developer"
	accounts.Set(email, acc)
	state := newTUIState(accounts, "")
	state.selectedEmail = email
	g := newTUIGeometry(100, 15) // wide split layout, tight height (bodyRows=6) so 3-pool detail overflows
	plan := tuiLayoutPlanFor(g.frameWidth+tuiRails, g.frameHeight)

	topLines := a.tuiPoolWideBody(state, g, plan)
	sc := state.scrollState("detail")
	if sc.total <= sc.view {
		t.Fatalf("expected wide-mode detail scroll overflow (total > view) for bodyRows=%d, got total=%d view=%d", g.bodyRows, sc.total, sc.view)
	}
	if !state.scrollBy("detail", sc.total-sc.view) {
		t.Fatalf("expected scrollBy(detail) to succeed when total=%d > view=%d", sc.total, sc.view)
	}

	bottomLines := a.tuiPoolWideBody(state, g, plan)
	if strings.Join(topLines, "\n") == strings.Join(bottomLines, "\n") {
		t.Fatal("scrolling detail pane in wide mode did not change rendered lines")
	}
}

func TestTUIPoolWideBodyDividerHoverDoesNotHighlightLeftScrollbar(t *testing.T) {
	p := makePalette(true)
	a := &Application{p: p}
	accounts := NewAccounts()
	for i := 0; i < 12; i++ {
		email := fmt.Sprintf("user%02d@example.com", i+1)
		accounts.Set(email, Account{"email": email, "name": fmt.Sprintf("User %02d", i+1)})
	}
	state := newTUIState(accounts, "")
	state.hover = tuiHit{kind: hitDivider}
	g := newTUIGeometry(100, 14)
	plan := tuiLayoutPlanFor(g.frameWidth+tuiRails, g.frameHeight)

	lines := a.tuiPoolWideBody(state, g, plan)
	// Find a body row that contains BOTH a scrollbar track bar on the left AND the divider in the middle.
	litDivider := p.Gray + "│"
	quietTrack := p.Track + "│"
	checked := false
	for _, line := range lines {
		if strings.Count(line, "│") >= 2 {
			firstQuiet := strings.Index(line, quietTrack)
			firstLit := strings.Index(line, litDivider)
			if firstLit < 0 {
				t.Fatalf("divider not highlighted with Gray on row: %q", line)
			}
			if firstQuiet >= 0 && firstLit < firstQuiet {
				t.Fatalf("left scrollbar track was highlighted instead of split divider: %q", line)
			}
			checked = true
		}
	}
	if !checked {
		t.Fatal("expected at least one wide-mode row with both left scrollbar and middle divider")
	}
}

func TestReadTerminalKeyPausedReturnsEOFOnClosedPipe(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = r.Close() }()
	_ = w.Close()

	key, readErr := readTerminalKeyPausedErr(r, nil)
	if !errors.Is(readErr, io.EOF) {
		t.Fatalf("expected io.EOF on closed pipe, got key=%q err=%v", key, readErr)
	}
}

func TestCLIMetricsJSONAndExtendedMutationJSONOutputs(t *testing.T) {
	f := newCLIFixture(t, "alpha@example.com", "beta@example.com")

	assertJSONOK := func(args ...string) {
		t.Helper()
		code := f.run("", args...)
		if code != 0 {
			t.Fatalf("%v: exit=%d stderr=%q stdout=%q", args, code, f.errOut.String(), f.out.String())
		}
		var env map[string]any
		if err := json.Unmarshal(f.out.Bytes(), &env); err != nil {
			t.Fatalf("%v: invalid JSON stdout=%q err=%v", args, f.out.String(), err)
		}
		if ok, _ := env["ok"].(bool); !ok {
			t.Fatalf("%v: expected ok=true, got %#v", args, env)
		}
	}

	// metrics json without --json flag
	assertJSONOK("metrics", "json")

	// Set up resources then remove them with --json
	assertJSONOK("alias", "set", "work", "alpha@example.com", "--json")
	assertJSONOK("alias", "remove", "work", "--json")

	assertJSONOK("tag", "set", "alpha@example.com", "prod", "--json")
	assertJSONOK("tag", "remove", "alpha@example.com", "--json")

	// tag remove with missing account must fail
	if code := f.run("", "tag", "remove", "nobody@example.com"); code == 0 {
		t.Fatal("tag remove on missing account returned exit 0")
	}

	assertJSONOK("profile", "set", "work", "alpha@example.com", "--json")
	assertJSONOK("bind", "set", t.TempDir(), "work", "--json")
	bindDir := t.TempDir()
	assertJSONOK("bind", "set", bindDir, "work", "--json")
	assertJSONOK("bind", "remove", bindDir, "--json")
	assertJSONOK("profile", "remove", "work", "--json")

	assertJSONOK("target", "set", "custom", "sh", "--json")
	assertJSONOK("target", "remove", "custom", "--json")

	// target check on missing command under --json must exit non-zero matching non-JSON
	if code := f.run("", "target", "check", "definitely-nonexistent-binary-agy-swap-test", "--json"); code == 0 {
		t.Fatalf("target check missing --json exited 0, stdout=%q", f.out.String())
	}

	assertJSONOK("statusline", "install", "--json")
	assertJSONOK("statusline", "uninstall", "--json")

	exportPath := filepath.Join(t.TempDir(), "history.json")
	assertJSONOK("history", "export", exportPath, "--json")
	assertJSONOK("history", "clear", "--force", "--json")
}

func TestCLIRemoveAndLimitHandleAmbiguousYesAndForce(t *testing.T) {
	f := newCLIFixture(t, "dev-one@example.com", "dev-two@example.com", "solo@example.com")

	if code := f.run("", "remove", "dev"); code != 1 || !strings.Contains(f.errOut.String(), "dev-one@example.com") {
		t.Fatalf("remove ambiguous: code=%d err=%q", code, f.errOut.String())
	}
	if code := f.run("", "limit", "set", "dev", "1h"); code != 1 || !strings.Contains(f.errOut.String(), "dev-one@example.com") {
		t.Fatalf("limit set ambiguous: code=%d err=%q", code, f.errOut.String())
	}

	// "yes" must be accepted by remove confirmation
	if code := f.run("yes\n", "remove", "dev-one@example.com"); code != 0 {
		t.Fatalf("remove with 'yes': code=%d err=%q", code, f.errOut.String())
	}
	for _, e := range f.savedEmails(t) {
		if e == "dev-one@example.com" {
			t.Fatal("dev-one@example.com was not removed after typing 'yes'")
		}
	}

	// --force must remove without prompting even on empty stdin
	if code := f.run("", "remove", "solo@example.com", "--force"); code != 0 {
		t.Fatalf("remove --force: code=%d err=%q out=%q", code, f.errOut.String(), f.out.String())
	}
	for _, e := range f.savedEmails(t) {
		if e == "solo@example.com" {
			t.Fatal("solo@example.com was not removed with --force")
		}
	}
}

func TestBackupExportRejectsPassphraseWithoutIncludeSecrets(t *testing.T) {
	f := newCLIFixture(t, "alpha@example.com")
	outPath := filepath.Join(t.TempDir(), "backup.json")
	if code := f.run("", "backup", "export", outPath, "--passphrase", "mysecret"); code == 0 {
		t.Fatalf("backup export with --passphrase and no --include-secrets succeeded and wrote plaintext: %q", f.out.String())
	}
	if _, err := os.Stat(outPath); err == nil {
		t.Fatal("plaintext backup file should not have been created when --passphrase was passed without --include-secrets")
	}
}

func TestEligibilityClearsStaleVerifyWhenVaultEntryUnavailable(t *testing.T) {
	acc := Account{
		"email":      "v@example.invalid",
		"secret_ref": "account:v@example.invalid",
	}
	_, err := accountToken(context.Background(), acc, fakeAccountVault{})
	if err == nil {
		t.Fatal("expected error for missing vault entry")
	}
	if !authInvalidationError(err) {
		t.Fatalf("authInvalidationError(%q) = false, want true", err.Error())
	}
}

func TestClockSkewAppliedToTokenExpiryAndGeminiModelGroup(t *testing.T) {
	// Case 1: local clock lags Google by +10m; token expires at localNow + 5m (already expired on Google's clock).
	refreshed := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/oauth"):
			refreshed = true
			fmt.Fprint(w, `{"access_token":"fresh-access","expires_in":3600}`)
		case strings.HasSuffix(r.URL.Path, ":loadCodeAssist"):
			w.Header().Set("Date", time.Now().UTC().Add(20*time.Minute).Format(http.TimeFormat))
			fmt.Fprint(w, `{"cloudaicompanionProject":"p","currentTier":{"id":"free-tier"}}`)
		case strings.HasSuffix(r.URL.Path, ":retrieveUserQuotaSummary"):
			w.Header().Set("Date", time.Now().UTC().Add(20*time.Minute).Format(http.TimeFormat))
			// Omit Gemini group so QuotaService falls back to fetchAvailableModels.
			fmt.Fprint(w, `{"groups":[{"groupId":"third_party","displayName":"Claude/GPT","buckets":[{"bucketId":"3p-5h","displayName":"5h","window":"5h","remainingFraction":0.8,"resetTime":"2030-01-01T00:00:00Z"}]}]}`)
		case strings.HasSuffix(r.URL.Path, ":fetchAvailableModels"):
			w.Header().Set("Date", time.Now().UTC().Add(20*time.Minute).Format(http.TimeFormat))
			// Reset is 5h20m from localNow (> 5h10m -> weekly on local clock),
			// which is 5h00m from Google's +20m clock (<= 5h10m -> 5h window).
			reset := time.Now().UTC().Add(5*time.Hour + 20*time.Minute).Format(time.RFC3339)
			fmt.Fprintf(w, `{"models":{"gemini-2.5-pro":{"quotaInfo":{"remainingFraction":0.75,"resetTime":%q}}}}`, reset)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	httpService := NewHTTPService(io.Discard)
	httpService.oauthURL = server.URL + "/oauth"
	httpService.cloudAPI = server.URL + "/v1internal:"
	httpService.skew.Store(int64(10 * time.Minute))

	blob := tokenBlob(t, "skew@example.com", true, "refresh-skew", time.Now().UTC().Add(5*time.Minute))
	access, _, err := httpService.accessTokenData(context.Background(), blob)
	if err != nil {
		t.Fatalf("accessTokenData: %v", err)
	}
	if !refreshed || access != "fresh-access" {
		t.Fatalf("expected OAuth refresh when token is expired on Google clock; refreshed=%v access=%q", refreshed, access)
	}

	// Case 2: Gemini model fallback classifies 5h window using Google clock before shiftQuotaResets.
	q := NewQuotaService(httpService, nil)
	snap, err := q.Fetch(context.Background(), Account{
		"email":      "skew@example.com",
		"token_data": tokenBlob(t, "skew@example.com", true, "refresh-skew", time.Now().UTC().Add(2*time.Hour)),
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	groups := getSlice(snap["groups"])
	var gemWindow string
	for _, raw := range groups {
		g := getMap(raw)
		if getString(g, "id") == "gemini" {
			buckets := getSlice(g["buckets"])
			if len(buckets) > 0 {
				gemWindow = getString(getMap(buckets[0]), "window")
			}
		}
	}
	if gemWindow != "5h" {
		t.Fatalf("expected Gemini fallback bucket window=5h under +20m clock skew, got %q", gemWindow)
	}
}

func TestForecastAndStatuslineIgnoreZeroResetTimestamp(t *testing.T) {
	f := newCLIFixture(t, "fresh@example.com")
	accounts, err := f.a.store.Load(false)
	if err != nil {
		t.Fatal(err)
	}
	acc := accounts.ByEmail["fresh@example.com"]
	acc["quota_schema"] = quotaSchema
	acc["quota_snapshot"] = map[string]any{
		"observed_at": isoTime(time.Now().UTC()),
		"tier":        map[string]any{"id": "t", "name": "T"},
		"groups": []any{
			map[string]any{
				"id":   "gemini",
				"name": "Gemini models",
				"buckets": []any{
					map[string]any{
						"id":                 "gemini-weekly",
						"name":               "Weekly",
						"window":             "weekly",
						"remaining_fraction": 1.0,
						"reset_at":           isoTime(time.Time{}),
					},
				},
			},
		},
	}
	if err := f.a.store.Save(accounts); err != nil {
		t.Fatal(err)
	}
	f.backend.token = f.tokens["fresh@example.com"]

	if code := f.run("", "forecast", "--json"); code != 0 {
		t.Fatalf("forecast --json: code=%d err=%q out=%q", code, f.errOut.String(), f.out.String())
	}
	out := f.out.String()
	if strings.Contains(out, "0001-01-01") {
		t.Fatalf("forecast emitted zero time 0001-01-01: %s", out)
	}
	if strings.Contains(out, `"stale"`) {
		t.Fatalf("forecast marked fresh 100%% account with no reset_at as stale: %s", out)
	}

	if code := f.run("", "statusline", "render"); code != 0 {
		t.Fatalf("statusline render: code=%d err=%q", code, f.errOut.String())
	}
	if strings.Contains(f.out.String(), "0001") || strings.Contains(f.out.String(), "reset") {
		t.Fatalf("statusline emitted bogus reset time for zero reset_at: %q", f.out.String())
	}
}
