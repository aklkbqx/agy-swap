package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func TestCredentialsShouldReloadIDEHubsDecoupledFromKeyringMirror(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX IDE hub reload test")
	}
	realHome, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(realHome) == "" {
		t.Skip("no user home directory")
	}
	t.Setenv("AGY_SWAP_MIRROR_OS_KEYRING", "0")
	t.Setenv("AGY_SWAP_SKIP_SYNC", "0")
	t.Setenv("AGY_SWAP_SKIP_HUB_RELOAD", "")

	c := &Credentials{paths: Paths{Home: realHome}, backend: osCredentialBackend{}}
	if c.shouldMirrorOSKeyring() {
		t.Fatal("expected shouldMirrorOSKeyring=false when AGY_SWAP_MIRROR_OS_KEYRING=0")
	}
	if !c.shouldReloadIDEHubsForBinary("/usr/local/bin/agy-swap") {
		t.Fatal("expected IDE hub reload to remain enabled when AGY_SWAP_MIRROR_OS_KEYRING=0")
	}
	t.Setenv("AGY_SWAP_SKIP_HUB_RELOAD", "1")
	if c.shouldReloadIDEHubsForBinary("/usr/local/bin/agy-swap") {
		t.Fatal("expected AGY_SWAP_SKIP_HUB_RELOAD=1 to disable hub reload")
	}
}

func TestAddLoginFlowDoesNotHoldSessionLockDuringPrompt(t *testing.T) {
	f := newCLIFixture(t)
	f.a.loginCommand = func(ctx context.Context) *exec.Cmd {
		return exec.CommandContext(ctx, "true")
	}
	var out lockedBuffer
	pr, pw := io.Pipe()
	f.a.In = pr
	f.a.Out = &out
	f.a.lineReader = nil

	done := make(chan int, 1)
	go func() {
		done <- f.a.addLoginFlow(context.Background())
	}()

	// Wait until the login prompt is shown (meaning addLoginFlow is blocked on user input).
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(out.String(), "Press Enter when ready") {
			break
		}
		time.Sleep(15 * time.Millisecond)
	}
	if !strings.Contains(out.String(), "Press Enter when ready") {
		_ = pw.Close()
		t.Fatalf("did not reach login prompt: %q", out.String())
	}

	// SessionLock must NOT be held while waiting at the prompt.
	lock, err := tryAcquireFileLock(f.a.paths.SessionLock)
	if err != nil {
		_ = pw.Close()
		<-done
		t.Fatalf("SessionLock was held while waiting at addLoginFlow prompt: %v", err)
	}
	_ = lock.Close()

	_, _ = pw.Write([]byte("q\n"))
	_ = pw.Close()
	if code := <-done; code != 0 {
		t.Fatalf("addLoginFlow cancel returned %d", code)
	}
}

func TestAddLoginFlowRestoresMirroredKeychainOnCancel(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("file session + mirrored keyring test")
	}
	f := newCLIFixture(t)
	t.Setenv("AGY_SWAP_SESSION_STORE", "file")
	t.Setenv("AGY_SWAP_MIRROR_OS_KEYRING", "1")
	origToken := tokenBlob(t, "orig@example.com", true, "refresh-orig", time.Now().Add(time.Hour))
	f.backend.token = origToken
	_ = f.a.credentials.writeOAuthFiles(origToken, "orig@example.com")

	// Force addLoginFlow to fail at launching agy after clearUnlocked ran so restore() executes.
	f.a.loginCommand = func(ctx context.Context) *exec.Cmd {
		return exec.CommandContext(ctx, "/nonexistent/agy-binary-for-rollback-test")
	}
	f.a.In = strings.NewReader("n\n\n") // decline saving active session, then press Enter to launch login
	f.a.lineReader = nil

	if code := f.a.addLoginFlow(context.Background()); code == 0 {
		t.Fatal("expected addLoginFlow to fail when login binary does not exist")
	}
	if f.backend.token != origToken {
		t.Fatalf("mirrored OS keyring token was lost on addLoginFlow rollback: got %q", f.backend.token)
	}

	// Explicitly exercise fileSession=true + mirrorKeyring=true rollback path (macOS/Linux default).
	f.backend.token = ""
	snap, err := snapshotFiles(f.a.credentials.sessionFilePaths(true)...)
	if err != nil {
		t.Fatal(err)
	}
	f.a.restoreSessionBackup(context.Background(), true, true, origToken, snap)
	if f.backend.token != origToken {
		t.Fatalf("restoreSessionBackup(fileSession=true, mirrorKeyring=true) did not restore mirrored keyring token: got %q", f.backend.token)
	}
}

func TestCleanBindingPathAndResolveBindingCanonicalizeSymlinks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX symlink test")
	}
	base := t.TempDir()
	realDir := filepath.Join(base, "real-project")
	if err := os.MkdirAll(realDir, 0o755); err != nil {
		t.Fatal(err)
	}
	linkDir := filepath.Join(base, "link-project")
	if err := os.Symlink(realDir, linkDir); err != nil {
		t.Fatal(err)
	}

	settings := defaultSettings()
	settings.Bindings = []Binding{{Path: linkDir, Profile: "work", Mode: "prompt"}}

	// Both cleanBindingPath(linkDir) and resolveBinding(settings, realDir/sub) must match!
	if got, want := cleanBindingPath(linkDir), cleanBindingPath(realDir); got != want {
		t.Fatalf("cleanBindingPath did not canonicalize symlink: link=%q real=%q", got, want)
	}
	sub := filepath.Join(realDir, "nested", "pkg")
	binding := resolveBinding(settings, sub)
	if binding.Profile != "work" {
		t.Fatalf("resolveBinding(%q) = %#v, want profile=work", sub, binding)
	}
}

func TestProfileRemovePrunesOrphanBindings(t *testing.T) {
	f := newCLIFixture(t, "alpha@example.com", "beta@example.com")
	if code := f.run("", "profile", "set", "work", "alpha@example.com"); code != 0 {
		t.Fatalf("profile set work failed: %s", f.errOut.String())
	}
	if code := f.run("", "profile", "set", "personal", "beta@example.com"); code != 0 {
		t.Fatalf("profile set personal failed: %s", f.errOut.String())
	}
	dirWork := t.TempDir()
	dirPersonal := t.TempDir()
	if code := f.run("", "bind", "set", dirWork, "work"); code != 0 {
		t.Fatalf("bind set work failed: %s", f.errOut.String())
	}
	if code := f.run("", "bind", "set", dirPersonal, "personal"); code != 0 {
		t.Fatalf("bind set personal failed: %s", f.errOut.String())
	}
	if code := f.run("", "profile", "remove", "work"); code != 0 {
		t.Fatalf("profile remove work failed: %s", f.errOut.String())
	}
	settings, err := f.a.loadSettings()
	if err != nil {
		t.Fatal(err)
	}
	if len(settings.Bindings) != 1 || settings.Bindings[0].Profile != "personal" {
		t.Fatalf("expected orphaned 'work' binding to be pruned, got %#v", settings.Bindings)
	}
}

func TestQuotaRefreshConflictReMergesSnapshotsAndOmitsFakeStorageKey(t *testing.T) {
	f := newCLIFixture(t, "alpha@example.com")
	// Set up mock Cloud API that simulates a concurrent write to accounts.json during Fetch!
	firstFetch := true
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, ":loadCodeAssist"):
			if firstFetch {
				firstFetch = false
				// Simulate another process updating accounts.json concurrently to trigger errStoreConflict on Save!
				concurrent, loadErr := f.a.store.Load(false)
				if loadErr == nil {
					concurrent.ByEmail["alpha@example.com"]["name"] = "Concurrent Name"
					_ = f.a.store.Save(concurrent)
				}
			}
			fmt.Fprint(w, `{"cloudaicompanionProject":"proj-1","currentTier":{"id":"free-tier"}}`)
		case strings.HasSuffix(r.URL.Path, ":retrieveUserQuotaSummary"):
			reset := time.Now().UTC().Add(3 * time.Hour).Format(time.RFC3339)
			fmt.Fprintf(w, `{"groups":[{"groupId":"gemini","displayName":"Gemini models","buckets":[{"bucketId":"g-5h","displayName":"5h","window":"5h","remainingFraction":0.85,"resetTime":%q}]}]}`, reset)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	httpSvc := NewHTTPService(io.Discard)
	httpSvc.cloudAPI = server.URL + "/v1internal:"
	q := NewQuotaService(httpSvc, f.a.store)
	q.SetVault(failSetVault{AccountVault: f.vault})

	accounts, err := f.a.store.Load(false)
	if err != nil {
		t.Fatal(err)
	}
	errs := q.Refresh(context.Background(), accounts, true, nil)
	if _, hasFakeStorage := errs["storage"]; hasFakeStorage {
		t.Fatalf("errorsByEmail should not contain fake 'storage' key: %#v", errs)
	}
	if storeErr, hasStore := errs["store"]; hasStore {
		t.Fatalf("expected conflict retry to merge snapshot cleanly, got store error: %s", storeErr)
	}
	reloaded, err := f.a.store.Load(false)
	if err != nil {
		t.Fatal(err)
	}
	acc := reloaded.ByEmail["alpha@example.com"]
	if getString(acc, "name") != "Concurrent Name" {
		t.Fatalf("concurrent update was lost during merge: name=%q", getString(acc, "name"))
	}
	if getMap(acc["quota_snapshot"]) == nil {
		t.Fatal("freshly fetched quota_snapshot was lost on errStoreConflict")
	}
}

func TestDoctorChecksAgyWrapperAndVaultPermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permissions and wrapper test")
	}
	f := newCLIFixture(t, "alpha@example.com")
	// Write a vault.json with insecure 0644 permissions
	vaultPath := filepath.Join(f.a.paths.ConfigDir, "vault.json")
	if err := os.WriteFile(vaultPath, []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(vaultPath, 0o644); err != nil {
		t.Fatal(err)
	}
	// Write a broken launcher guard wrapper pointing to a missing real binary
	binDir := filepath.Join(f.a.paths.Home, ".local", "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	wrapper := buildAgyLauncherScriptForPaths(filepath.Join(binDir, "missing-real-agy"), "/usr/local/bin/agy-swap", "", false)
	if err := os.WriteFile(filepath.Join(binDir, "agy"), []byte(wrapper), 0o755); err != nil {
		t.Fatal(err)
	}

	_ = f.run("", "doctor", "--json")
	var env struct {
		Data struct {
			Checks []doctorCheck `json:"checks"`
		} `json:"data"`
	}
	if err := json.Unmarshal(f.out.Bytes(), &env); err != nil {
		t.Fatalf("invalid doctor JSON: %v\n%s", err, f.out.String())
	}
	byName := map[string]doctorCheck{}
	for _, c := range env.Data.Checks {
		byName[c.Name] = c
	}
	if c, ok := byName["vault_perms"]; !ok || c.Status != "warning" {
		t.Fatalf("expected vault_perms warning for 0644 vault.json, got %#v (ok=%v)", c, ok)
	}
	if c, ok := byName["launcher_guard"]; !ok || c.Status != "error" {
		t.Fatalf("expected launcher_guard error for dangling real_command, got %#v (ok=%v)", c, ok)
	}
	if _, ok := byName["agy_cli"]; !ok {
		t.Fatalf("expected agy_cli check in doctor output, got checks=%#v", byName)
	}
}

func TestTUIModalBlocksFooterChipsPaletteCtrlCAndLogoutConfirm(t *testing.T) {
	f := newCLIFixture(t, "alpha@example.com", "beta@example.com")
	accounts, _ := f.a.store.Load(false)
	state := newTUIState(accounts, "")
	state.mode = tuiForm
	state.hits = []tuiHit{{kind: hitChip, key: "q", top: 10, bottom: 11, left: 2, right: 8}}

	var pressed []string
	var lastTime time.Time
	var dragging bool
	var thumb string
	var grab, thumbTop, lastCol, lastRow int
	ptr := &tuiPointerState{
		dragging: &dragging, thumb: &thumb, grab: &grab, thumbTop: &thumbTop,
		lastTime: &lastTime, lastCol: &lastCol, lastRow: &lastRow,
		press: func(k string) { pressed = append(pressed, k) },
	}
	// Clicking footer chip while in tuiForm must NOT inject 'q' into the form.
	f.a.tuiPointer(state, "mouse:down:4:11", ptr)
	if len(pressed) != 0 {
		t.Fatalf("clicking footer chip in tuiForm leaked key presses: %v", pressed)
	}

	// Ctrl+C while palette is open must close palette cleanly.
	state.mode = tuiPalette
	ctrl := &tuiController{
		app:   f.a,
		state: state,
		ctx:   context.Background,
	}
	if quit := ctrl.handleKeyEvent("ctrl-c", "ctrl-c", func() {}); quit {
		t.Fatal("ctrl-c in palette should close palette, not quit TUI")
	}
	if state.mode != tuiBrowse {
		t.Fatalf("expected palette to close on ctrl-c, mode=%v", state.mode)
	}

	// Pressing 'l' on Dashboard opens confirmation dialog instead of logging out immediately.
	state.view = tuiViewDashboard
	state.mode = tuiBrowse
	ctrl.handleKeyEvent("l", "l", func() {})
	if state.mode != tuiDialogOpen || state.dialog == nil || state.dialog.Kind != "logout" {
		t.Fatalf("expected 'l' on dashboard to open logout confirmation dialog, mode=%v dialog=%#v", state.mode, state.dialog)
	}

	// Pressing 'l' or '1' on Profiles view must NOT open logout dialog or change selected account.
	state.mode = tuiBrowse
	state.dialog = nil
	state.view = tuiViewProfiles
	state.selectedEmail = "alpha@example.com"
	ctrl.handleKeyEvent("l", "l", func() {})
	if state.mode == tuiDialogOpen {
		t.Fatal("'l' should not open logout dialog from Profiles view")
	}
	ctrl.handleKeyEvent("2", "2", func() {})
	if state.selectedEmail != "alpha@example.com" {
		t.Fatalf("'2' changed dashboard selectedEmail while on Profiles view: %q", state.selectedEmail)
	}
}

func TestTUIProfilesHistoryScrollSearchTagsAliasesAndProfileRename(t *testing.T) {
	f := newCLIFixture(t, "alpha@example.com", "beta@example.com")
	accounts, _ := f.a.store.Load(false)
	settings := defaultSettings()
	settings.Tags["beta@example.com"] = []string{"production", "urgent"}
	settings.Aliases["work"] = "alpha@example.com"
	for i := 0; i < 15; i++ {
		name := fmt.Sprintf("prof-%02d", i)
		settings.Profiles[name] = Profile{Account: "alpha@example.com", Family: "gemini"}
	}
	if err := f.a.store.SaveSettings(settings); err != nil {
		t.Fatal(err)
	}

	state := newTUIState(accounts, "")
	state.settings = settings
	state.settingsLoaded = true

	// Search by tag
	state.search = "production"
	if got := state.visibleEmails(); len(got) != 1 || got[0] != "beta@example.com" {
		t.Fatalf("search by tag failed: got %v", got)
	}
	// Search by alias
	state.search = "work"
	if got := state.visibleEmails(); len(got) != 1 || got[0] != "alpha@example.com" {
		t.Fatalf("search by alias failed: got %v", got)
	}
	state.search = ""

	// Profiles view scrolls to show bottom item prof-14 when selected in small viewport height=6
	f.a.beginTUIView(state, tuiViewProfiles)
	state.profileIndex = len(state.profileNames) - 1
	profRows := f.a.tuiProfileViewRows(state, 80, 8)
	if !strings.Contains(strings.Join(profRows, "\n"), "prof-14") {
		t.Fatalf("Profiles view did not scroll to selected bottom profile prof-14:\n%s", strings.Join(profRows, "\n"))
	}

	// History view scrolls to show bottom item when selected in small viewport height=5
	state.history = make([]historyEvent, 20)
	for i := range state.history {
		state.history[i] = historyEvent{At: isoTime(time.Now().UTC()), Kind: "switch", Email: fmt.Sprintf("user%02d@example.com", i)}
	}
	state.historyIndex = 19
	histRows := f.a.tuiHistoryViewRows(state, 80, 5)
	if !strings.Contains(strings.Join(histRows, "\n"), "user19@example.com") {
		t.Fatalf("History view did not scroll to selected bottom event user19:\n%s", strings.Join(histRows, "\n"))
	}

	// Profile edit renaming prof-00 -> renamed-00 deletes old key and updates bindings
	settings.Bindings = []Binding{{Path: t.TempDir(), Profile: "prof-00", Mode: "prompt"}}
	if err := f.a.store.SaveSettings(settings); err != nil {
		t.Fatal(err)
	}
	f.a.beginTUIView(state, tuiViewProfiles)
	state.profileIndex = 0
	f.a.beginTUIForm(state, "profile-edit")
	state.form.Fields[0].Value = "renamed-00"
	if _, err := f.a.applyTUIForm(context.Background(), state); err != nil {
		t.Fatalf("applyTUIForm profile-edit rename failed: %v", err)
	}
	updated, err := f.a.loadSettings()
	if err != nil {
		t.Fatal(err)
	}
	if _, stillOld := updated.Profiles["prof-00"]; stillOld {
		t.Fatal("profile-edit rename left duplicate old profile 'prof-00'")
	}
	if _, hasNew := updated.Profiles["renamed-00"]; !hasNew {
		t.Fatal("profile-edit rename did not create 'renamed-00'")
	}
	if len(updated.Bindings) != 1 || updated.Bindings[0].Profile != "renamed-00" {
		t.Fatalf("profile-edit rename did not update binding profile reference: %#v", updated.Bindings)
	}
}

func TestDetectCompletionShellRespectsShellEnv(t *testing.T) {
	t.Setenv("SHELL", "/bin/zsh")
	if got := detectCompletionShell(); runtime.GOOS != "windows" && got != "zsh" {
		t.Fatalf("detectCompletionShell() with SHELL=/bin/zsh = %q, want zsh", got)
	}
	t.Setenv("SHELL", "/opt/homebrew/bin/fish")
	if got := detectCompletionShell(); runtime.GOOS != "windows" && got != "fish" {
		t.Fatalf("detectCompletionShell() with SHELL=.../fish = %q, want fish", got)
	}
}
