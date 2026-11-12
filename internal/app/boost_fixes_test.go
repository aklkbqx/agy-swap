package app

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestVerifyUpdateBinaryAcceptsGoInstallBuild(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX shell script test")
	}
	dir := t.TempDir()
	bin := filepath.Join(dir, "agy-swap")
	script := "#!/bin/sh\necho 'agy-swap v2.22.2-go-install'\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := verifyUpdateBinary(context.Background(), bin, "2.22.2"); err != nil {
		t.Fatalf("expected verifyUpdateBinary to accept -go-install suffix, got: %v", err)
	}
}

func TestExtendedCommandParsingWithFlagsAndHelp(t *testing.T) {
	if !isExtendedCommand([]string{"--no-color", "profile", "list"}) {
		t.Fatal("expected --no-color profile list to be recognized as extended command")
	}
	if !isExtendedCommand([]string{"--json", "doctor"}) {
		t.Fatal("expected --json doctor to be recognized as extended command")
	}
	if isExtendedCommand([]string{"--switch-to", "user@example.com"}) {
		t.Fatal("expected --switch-to user@example.com not to be recognized as extended command")
	}
	_, opts, _, err := parseExtended([]string{"profile", "--help"})
	if err != nil {
		t.Fatalf("expected parseExtended to accept --help, got err: %v", err)
	}
	if !opts.Help {
		t.Fatal("expected opts.Help to be true")
	}

	f := newCLIFixture(t)
	var out bytes.Buffer
	f.a.Out = &out
	code := f.a.Run(context.Background(), []string{"profile", "--help"})
	if code != 0 {
		t.Fatalf("expected exit code 0 for profile --help, got %d", code)
	}
	if !strings.Contains(strings.ToLower(out.String()), "usage:") {
		t.Fatalf("expected usage output for profile --help, got: %q", out.String())
	}
}

func TestCmdLimitsRefreshOnlyFetchesQuotaOnce(t *testing.T) {
	f := newCLIFixture(t)
	var fetchCalls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/loadCodeAssist":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"cloudaicompanionProject":"p","paidTier":{"id":"g1-pro-tier"}}`))
		case "/retrieveUserQuotaSummary":
			atomic.AddInt32(&fetchCalls, 1)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"groups":[{"displayName":"Gemini","buckets":[{"bucketId":"gemini-weekly","window":"weekly","remainingFraction":0.8,"resetTime":"2030-01-01T00:00:00Z"}]}]}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	f.a.http.oauthURL = srv.URL + "/oauth"
	f.a.http.cloudAPI = srv.URL + "/"
	f.a.quota = NewQuotaService(f.a.http, f.a.store)
	f.a.quota.vault = f.a.vault

	accounts, err := f.a.store.Load(true)
	if err != nil {
		t.Fatal(err)
	}
	email := "once@example.com"
	rawToken := tokenBlob(t, email, true, "ref-1", time.Now().Add(time.Hour))
	ref := accountSecretRef(email)
	f.a.vault.Set(context.Background(), ref, rawToken)
	accounts.Set(email, Account{
		"email":       email,
		"name":        "Once User",
		"secret_ref":  ref,
		"token_hash":  hashToken(rawToken),
		"project_id":  "p",
		"last_active": 1,
	})
	if err := f.a.store.Save(accounts); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	f.a.Out = &out
	code := f.a.Run(context.Background(), []string{"limits", "--refresh"})
	if code != 0 {
		t.Fatalf("limits --refresh returned %d", code)
	}
	if got := atomic.LoadInt32(&fetchCalls); got != 1 {
		t.Fatalf("expected 1 quota summary fetch call for limits --refresh, got %d", got)
	}
}

func TestTUIBackupEncryptExportShortcutAndFooter(t *testing.T) {
	a, state := listApp()
	state.view = tuiViewBackup
	c := &tuiController{app: a, state: state, ctx: context.Background, finish: func() {}}
	c.handleKeyEvent("e", "e", func() {})
	if state.form == nil || state.form.Kind != "backup-export" {
		t.Fatalf("expected pressing 'e' in Backup view to open backup-export form, got %#v", state.form)
	}
	var encryptedVal string
	for _, field := range state.form.Fields {
		if field.Key == "encrypted" {
			encryptedVal = field.Value
		}
	}
	if encryptedVal != "on" {
		t.Fatalf("expected encrypted field to default to 'on' for 'e' shortcut, got %q", encryptedVal)
	}
	state.mode = tuiBrowse
	footer := stripEscapes(strings.Join(a.tuiFooterLines(state, 100), "\n"))
	if !strings.Contains(footer, "Encrypt export") {
		t.Fatalf("expected Backup footer to mention Encrypt export, got %q", footer)
	}
}

func TestTUISubviewEscReturnsToDashboardAndSettingsBindingKey(t *testing.T) {
	a, state := listApp()
	c := &tuiController{app: a, state: state, ctx: context.Background, finish: func() {}}
	for _, v := range []tuiView{tuiViewProfiles, tuiViewHistory, tuiViewSettings, tuiViewDoctor, tuiViewBackup} {
		state.view = v
		quit := c.handleKeyEvent("\x1b", "esc", func() {})
		if quit {
			t.Fatalf("expected Esc in view %v not to quit TUI", v)
		}
		if state.view != tuiViewDashboard {
			t.Fatalf("expected Esc in view %v to return to Dashboard, got %v", v, state.view)
		}
	}

	// In Settings, 'b' should return to Dashboard and 'f' should open the binding form.
	state.view = tuiViewSettings
	c.handleKeyEvent("b", "b", func() {})
	if state.view != tuiViewDashboard {
		t.Fatalf("expected 'b' in Settings view to return to Dashboard, got %v", state.view)
	}
	if state.form != nil {
		t.Fatalf("expected 'b' in Settings view not to open form, got %#v", state.form)
	}

	state.view = tuiViewSettings
	c.handleKeyEvent("f", "f", func() {})
	if state.form == nil || state.form.Kind != "binding" {
		t.Fatalf("expected 'f' in Settings view to open binding form, got %#v", state.form)
	}
}

func TestEnsureAgyLauncherGuardPreservesSymlinkAndWrapperTarget(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX wrapper test")
	}
	f := newCLIFixture(t)
	realDir := t.TempDir()
	realBin := filepath.Join(realDir, "agy-native")
	nativeBytes := append([]byte{0xcf, 0xfa, 0xed, 0xfe}, []byte("native-payload")...)
	if err := os.WriteFile(realBin, nativeBytes, 0o755); err != nil {
		t.Fatal(err)
	}

	customDir := t.TempDir()
	customBin := filepath.Join(customDir, "agy")
	if err := os.Symlink(realBin, customBin); err != nil {
		t.Fatal(err)
	}

	if resolved, evalErr := filepath.EvalSymlinks(realBin); evalErr == nil && resolved != "" {
		realBin = resolved
	}
	f.a.ensureAgyLauncherGuardForPath(customBin)
	firstWrapper, err := os.ReadFile(customBin)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(firstWrapper), `real_command="`+realBin+`"`) {
		t.Fatalf("expected wrapper to point to %q, got:\n%s", realBin, string(firstWrapper))
	}

	legacyWrapper := strings.ReplaceAll(string(firstWrapper), "reload_marker=", "legacy_marker=")
	if writeErr := os.WriteFile(customBin, []byte(legacyWrapper), 0o755); writeErr != nil {
		t.Fatal(writeErr)
	}
	if resolved, evalErr := filepath.EvalSymlinks(realBin); evalErr == nil && resolved != "" {
		realBin = resolved
	}
	f.a.ensureAgyLauncherGuardForPath(customBin)
	upgradedWrapper, err := os.ReadFile(customBin)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(upgradedWrapper), `real_command="`+realBin+`"`) {
		t.Fatalf("expected upgraded wrapper to preserve real_command=%q, got:\n%s", realBin, string(upgradedWrapper))
	}
}

func TestQuotaFetchPersistsRefreshHashOnRotatedToken(t *testing.T) {
	f := newCLIFixture(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/oauth":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"new-access","refresh_token":"rotated-refresh","expires_in":3600}`))
		case "/loadCodeAssist":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"cloudaicompanionProject":"p","paidTier":{"id":"g1-pro-tier"}}`))
		case "/retrieveUserQuotaSummary":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"groups":[{"displayName":"Gemini","buckets":[{"bucketId":"gemini-weekly","window":"weekly","remainingFraction":0.9,"resetTime":"2030-01-01T00:00:00Z"}]}]}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	f.a.http.oauthURL = srv.URL + "/oauth"
	f.a.http.cloudAPI = srv.URL + "/"
	f.a.quota = NewQuotaService(f.a.http, f.a.store)
	f.a.quota.vault = f.a.vault

	email := "rotate@example.com"
	expiredToken := tokenBlob(t, email, true, "old-refresh", time.Now().Add(-time.Hour))
	ref := accountSecretRef(email)
	f.a.vault.Set(context.Background(), ref, expiredToken)
	accounts := NewAccounts()
	accounts.Set(email, Account{
		"email":        email,
		"secret_ref":   ref,
		"token_hash":   hashToken(expiredToken),
		"refresh_hash": hashToken("old-refresh"),
		"project_id":   "proj-1",
	})

	f.a.quota.Refresh(context.Background(), accounts, true, nil)
	gotHash := getString(accounts.ByEmail[email], "refresh_hash")
	wantHash := hashToken("rotated-refresh")
	if gotHash != wantHash {
		t.Fatalf("expected refresh_hash=%q after token rotation, got %q", wantHash, gotHash)
	}
}

func TestCmdHistoryStatsSortsByKindDeterministically(t *testing.T) {
	f := newCLIFixture(t)
	for _, kind := range []string{"zeta", "alpha", "beta"} {
		if err := f.a.appendHistory(kind, "user@example.com", nil); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 5; i++ {
		var out bytes.Buffer
		f.a.Out = &out
		code := f.a.cmdHistoryStats(extendedOptions{})
		if code != 0 {
			t.Fatalf("cmdHistoryStats returned %d", code)
		}
		text := out.String()
		aIdx := strings.Index(text, "alpha")
		bIdx := strings.Index(text, "beta")
		zIdx := strings.Index(text, "zeta")
		if aIdx < 0 || bIdx < 0 || zIdx < 0 || aIdx >= bIdx || bIdx >= zIdx {
			t.Fatalf("expected sorted byKind output (alpha < beta < zeta), got:\n%s", text)
		}
	}
}

func TestCmdLimitClearCommand(t *testing.T) {
	f := newCLIFixture(t)
	accounts, err := f.a.store.Load(true)
	if err != nil {
		t.Fatal(err)
	}
	email := "cooldown@example.com"
	accounts.Set(email, Account{"email": email, "name": "Cooldown User"})
	if saveErr := f.a.store.Save(accounts); saveErr != nil {
		t.Fatal(saveErr)
	}
	var out bytes.Buffer
	f.a.Out = &out
	if code := f.a.Run(context.Background(), []string{"limit", "set", email, "30m", "--group", "gemini"}); code != 0 {
		t.Fatalf("limit set failed with code %d", code)
	}
	if code := f.a.Run(context.Background(), []string{"limit", "clear", email, "--group", "gemini"}); code != 0 {
		t.Fatalf("limit clear failed with code %d", code)
	}
	loaded, err := f.a.store.Load(true)
	if err != nil {
		t.Fatal(err)
	}
	if limits := activeLimits(loaded.ByEmail[email], time.Now()); len(limits) != 0 {
		t.Fatalf("expected 0 active limits after limit clear, got %d", len(limits))
	}
}

type countingVault struct {
	AccountVault
	gets int32
}

func (c *countingVault) Get(ctx context.Context, ref string) (string, bool) {
	atomic.AddInt32(&c.gets, 1)
	return c.AccountVault.Get(ctx, ref)
}

func TestMatchAccountByTokenUsesRefreshHashWithoutVaultRead(t *testing.T) {
	f := newCLIFixture(t)
	cv := &countingVault{AccountVault: f.a.vault}
	f.a.vault = cv

	accounts := NewAccounts()
	for _, email := range []string{"one@example.com", "two@example.com"} {
		ref := accountSecretRef(email)
		tok, _ := json.Marshal(map[string]any{
			"access_token":  "acc-" + email,
			"refresh_token": "ref-" + email,
		})
		f.a.vault.Set(context.Background(), ref, string(tok))
		accounts.Set(email, Account{
			"email":        email,
			"secret_ref":   ref,
			"token_hash":   hashToken(string(tok)),
			"refresh_hash": hashToken("ref-" + email),
		})
	}
	liveTok, _ := json.Marshal(map[string]any{
		"access_token":  "rotated-access-token",
		"refresh_token": "ref-two@example.com",
	})
	matched := f.a.matchAccountByToken(context.Background(), accounts, string(liveTok))
	if matched != "two@example.com" {
		t.Fatalf("expected match two@example.com, got %q", matched)
	}
	if got := atomic.LoadInt32(&cv.gets); got != 0 {
		t.Fatalf("expected 0 vault.Get calls when refresh_hash is present, got %d", got)
	}
}

func TestTUIActivityDueDoesNotBlockWhilePollWalksSlowDisk(t *testing.T) {
	enteredList := make(chan struct{})
	releaseList := make(chan struct{})
	w := &tuiActivity{
		list: func() []string {
			close(enteredList)
			<-releaseList
			return nil
		},
	}
	now := time.Date(2030, 1, 1, 12, 0, 0, 0, time.UTC)
	pollDone := make(chan struct{})
	go func() {
		defer close(pollDone)
		w.poll(now)
	}()
	<-enteredList

	dueDone := make(chan bool, 1)
	go func() {
		dueDone <- w.due(now)
	}()
	select {
	case <-dueDone:
	case <-time.After(50 * time.Millisecond):
		close(releaseList)
		<-pollDone
		t.Fatal("tuiActivity.due blocked on mutex while poll was executing slow filesystem list()")
	}
	close(releaseList)
	<-pollDone
}

func TestHybridVaultCachesNegativeFallbackLookups(t *testing.T) {
	primary := newMemoryVault()
	fallbackBase := newMemoryVault()
	fallback := &countingVault{AccountVault: fallbackBase}
	h := &hybridVault{primary: primary, fallback: fallback}
	ctx := context.Background()

	ref := "account:missing@example.com"
	for i := 0; i < 10; i++ {
		if val, ok := h.Get(ctx, ref); ok || val != "" {
			t.Fatalf("expected missing secret on attempt %d, got %q", i, val)
		}
	}
	if got := atomic.LoadInt32(&fallback.gets); got != 1 {
		t.Fatalf("expected hybridVault to query fallback at most once for missing secret, got %d", got)
	}

	// If fallback later receives a value and Delete clears negative cache, it can be queried again.
	fallbackBase.Set(ctx, ref, "recovered-token")
	h.Delete(ctx, ref)
	fallbackBase.Set(ctx, ref, "recovered-token")
	if val, ok := h.Get(ctx, ref); !ok || val != "recovered-token" {
		t.Fatalf("expected recovered-token after cache reset, got %q (ok=%v)", val, ok)
	}
}

func TestSetAccountsWithVaultCacheSkipsMainThreadVaultLookups(t *testing.T) {
	f := newCLIFixture(t)
	cv := &countingVault{AccountVault: f.a.vault}
	f.a.vault = cv

	accounts := NewAccounts()
	email := "warm@example.com"
	ref := accountSecretRef(email)
	tok := tokenBlob(t, email, true, "ref-warm", time.Now().Add(time.Hour))
	f.a.vault.Set(context.Background(), ref, tok)
	accounts.Set(email, Account{
		"email":      email,
		"name":       "Warm User",
		"secret_ref": ref,
		"token_hash": hashToken(tok),
	})

	state := newTUIState(NewAccounts(), tok)
	state.vault = cv

	// Background goroutine warms the vault cache before sending tuiAccountsEvent.
	warmed := f.a.warmVaultCache(context.Background(), accounts)
	atomic.StoreInt32(&cv.gets, 0)

	// Main UI thread applies accounts with the pre-warmed cache and renders.
	state.setAccountsWithVaultCache(accounts, warmed)
	_ = f.a.tuiLines(state, 118, 30)
	if got := atomic.LoadInt32(&cv.gets); got != 0 {
		t.Fatalf("expected 0 vault.Get calls on main UI thread after setAccountsWithVaultCache, got %d", got)
	}
}

func TestCoalescePointerKeyEventDrainsQueuedMouseMovesWithoutDroppingKeys(t *testing.T) {
	events := make(chan tuiEvent, 16)
	events <- tuiKeyEvent{key: "mouse:move:2:3"}
	events <- tuiKeyEvent{key: "mouse:move:4:5"}
	events <- tuiKeyEvent{key: "mouse:move:8:9"}
	events <- tuiKeyEvent{key: "j"}

	got, staged := coalescePointerKeyEvent(tuiKeyEvent{key: "mouse:move:1:1"}, events)
	if got.key != "mouse:move:8:9" {
		t.Fatalf("coalesced mouse move = %q, want mouse:move:8:9", got.key)
	}
	stagedKey, ok := staged.(tuiKeyEvent)
	if !ok || stagedKey.key != "j" {
		t.Fatalf("staged event = %#v, want tuiKeyEvent{key: \"j\"}", staged)
	}
}
