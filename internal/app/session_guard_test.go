//go:build !windows

package app

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestCredentialsApplyAndClearSkipSecurityAndMaintainKeyringBypassMarker(t *testing.T) {
	calls := securityStub(t, 0)
	app, _, _ := newSessionGuardTestApp(t)

	token := sampleTokenWithRefresh("at-1", "rt-1", time.Now().Add(time.Hour))
	if !app.credentials.Apply(context.Background(), token, "alpha@example.com") {
		t.Fatal("Apply failed")
	}
	if got := strings.Join(calls(), ""); got != "" {
		t.Fatalf("expected 0 /usr/bin/security calls during Apply, got %q", got)
	}

	wantMarkers := []string{
		filepath.Join(filepath.Dir(app.paths.OAuthToken), "cache", "antigravity-keyring-unavailable"),
		filepath.Join(filepath.Dir(app.paths.IDEOAuthToken), "cache", "antigravity-keyring-unavailable"),
		filepath.Join(filepath.Dir(app.paths.JetskiToken), "cache", "antigravity-keyring-unavailable"),
	}
	for _, markerPath := range wantMarkers {
		info, err := os.Stat(markerPath)
		if err != nil {
			t.Fatalf("expected keyring bypass marker at %s: %v", markerPath, err)
		}
		if time.Until(info.ModTime()) < 365*24*time.Hour {
			t.Fatalf("expected marker mtime far in the future at %s, got %v", markerPath, info.ModTime())
		}
	}

	if got := app.credentials.Current(context.Background()); got != token {
		t.Fatalf("Current() = %q, want %q", got, token)
	}
	if got := strings.Join(calls(), ""); got != "" {
		t.Fatalf("expected 0 /usr/bin/security calls during Current, got %q", got)
	}

	if !app.credentials.Clear(context.Background()) {
		t.Fatal("Clear failed")
	}
	if got := strings.Join(calls(), ""); got != "" {
		t.Fatalf("expected 0 /usr/bin/security calls during Clear, got %q", got)
	}
	if got := app.credentials.Current(context.Background()); got != "" {
		t.Fatalf("expected empty Current() after Clear, got %q", got)
	}
	for _, markerPath := range wantMarkers {
		info, err := os.Stat(markerPath)
		if err != nil {
			t.Fatalf("expected keyring bypass marker to remain after Clear at %s: %v", markerPath, err)
		}
		if time.Until(info.ModTime()) < 365*24*time.Hour {
			t.Fatalf("expected marker mtime far in the future after Clear at %s, got %v", markerPath, info.ModTime())
		}
	}
}

func TestReconcileActiveSessionHarvestsRefreshAndRestoresClobberedActiveAccount(t *testing.T) {
	calls := securityStub(t, 0)
	app, out, errOut := newSessionGuardTestApp(t)
	ctx := context.Background()

	alphaOld := sampleTokenWithRefresh("at-alpha-old", "rt-alpha", time.Now().Add(time.Hour))
	betaActive := sampleTokenWithRefresh("at-beta-active", "rt-beta", time.Now().Add(2*time.Hour))

	accounts := NewAccounts()
	alphaAcc := Account{"email": "alpha@example.com", "name": "Alpha", "added_at": "2026-03-01T00:00:00Z", "last_used": "2026-03-01T00:00:00Z"}
	app.saveAccountSecret(ctx, alphaAcc, alphaOld)
	accounts.Set("alpha@example.com", alphaAcc)

	betaAcc := Account{"email": "beta@example.com", "name": "Beta", "added_at": "2026-03-01T00:00:00Z", "last_used": "2026-03-02T00:00:00Z"}
	app.saveAccountSecret(ctx, betaAcc, betaActive)
	accounts.Set("beta@example.com", betaAcc)

	if err := app.store.Save(accounts); err != nil {
		t.Fatal(err)
	}

	if code := app.cmdSwitch(ctx, cliArgs{account: "beta@example.com"}); code != 0 {
		t.Fatalf("cmdSwitch(beta) exited %d", code)
	}

	// Simulate a background agy process (running as alpha@example.com in another folder)
	// refreshing its OAuth token after 1 hour and writing raw JSON to antigravity-oauth-token.
	alphaRefreshedExpiry := time.Now().Add(3 * time.Hour)
	alphaRefreshedRaw := sampleRawDiskTokenWithRefresh("at-alpha-refreshed", "rt-alpha", alphaRefreshedExpiry)
	alphaRefreshed := sampleTokenWithRefresh("at-alpha-refreshed", "rt-alpha", alphaRefreshedExpiry)
	if err := os.WriteFile(app.paths.OAuthToken, alphaRefreshedRaw, 0o600); err != nil {
		t.Fatal(err)
	}

	if code := app.Run(ctx, []string{"--sync-session"}); code != 0 {
		t.Fatalf("--sync-session exited %d: out=%s err=%s", code, out.String(), errOut.String())
	}

	// 1. Active OAuth token on disk must be restored to beta@example.com.
	if got := app.credentials.readOAuthToken(); got != betaActive {
		t.Fatalf("OAuthToken after sync = %q, want betaActive %q", got, betaActive)
	}
	if got := app.credentials.StoredActiveEmail(); got != "beta@example.com" {
		t.Fatalf("StoredActiveEmail = %q, want beta@example.com", got)
	}

	// 2. alpha@example.com's refreshed token must not be lost — it should be harvested into the vault.
	harvestedAlpha, ok := app.vault.Get(ctx, accountSecretRef("alpha@example.com"))
	if !ok {
		t.Fatal("vault.Get(alpha) returned false")
	}
	if harvestedAlpha != alphaRefreshed {
		t.Fatalf("vault token for alpha = %q, want refreshed %q", harvestedAlpha, alphaRefreshed)
	}
	if got := strings.Join(calls(), ""); got != "" {
		t.Fatalf("expected 0 /usr/bin/security calls, got %q", got)
	}
}

func TestActiveAccountInPlaceTokenRefreshRecognizedAndHarvested(t *testing.T) {
	_ = securityStub(t, 0)
	app, _, _ := newSessionGuardTestApp(t)
	ctx := context.Background()

	betaInitial := sampleTokenWithRefresh("at-beta-1", "rt-beta", time.Now().Add(time.Hour))
	accounts := NewAccounts()
	betaAcc := Account{"email": "beta@example.com", "name": "Beta", "added_at": "2026-03-01T00:00:00Z", "last_used": "2026-03-02T00:00:00Z"}
	app.saveAccountSecret(ctx, betaAcc, betaInitial)
	accounts.Set("beta@example.com", betaAcc)
	if err := app.store.Save(accounts); err != nil {
		t.Fatal(err)
	}
	if code := app.cmdSwitch(ctx, cliArgs{account: "beta@example.com"}); code != 0 {
		t.Fatalf("cmdSwitch(beta) exited %d", code)
	}

	// Active agy session refreshes beta's access_token in place.
	betaRefreshedExpiry := time.Now().Add(2 * time.Hour)
	betaRefreshedRaw := sampleRawDiskTokenWithRefresh("at-beta-2", "rt-beta", betaRefreshedExpiry)
	betaRefreshed := sampleTokenWithRefresh("at-beta-2", "rt-beta", betaRefreshedExpiry)
	if err := os.WriteFile(app.paths.OAuthToken, betaRefreshedRaw, 0o600); err != nil {
		t.Fatal(err)
	}

	reloaded, err := app.store.Load(false)
	if err != nil {
		t.Fatal(err)
	}
	if got := localActiveEmail(reloaded, app.credentials.Current(ctx)); got != "beta@example.com" {
		t.Fatalf("localActiveEmail after in-place refresh = %q, want beta@example.com", got)
	}
	if got := app.activeEmail(ctx, reloaded, app.credentials.Current(ctx)); got != "beta@example.com" {
		t.Fatalf("activeEmail after in-place refresh = %q, want beta@example.com", got)
	}

	app.reconcileActiveSession(ctx)

	if got := app.credentials.readOAuthToken(); got != betaRefreshed {
		t.Fatalf("OAuthToken should keep refreshed beta token, got %q", got)
	}
	vaultBeta, ok := app.vault.Get(ctx, accountSecretRef("beta@example.com"))
	if !ok {
		t.Fatal("vault.Get(beta) returned false")
	}
	if vaultBeta != betaRefreshed {
		t.Fatalf("vault token for beta = %q, want refreshed %q", vaultBeta, betaRefreshed)
	}
}

func TestReconcileActiveSessionHarvestsIDERefreshAndPropagatesAcrossSurfaces(t *testing.T) {
	calls := securityStub(t, 0)
	app, _, _ := newSessionGuardTestApp(t)
	ctx := context.Background()

	alphaOld := sampleTokenWithRefresh("at-alpha-old", "rt-alpha", time.Now().Add(time.Hour))
	betaInitial := sampleTokenWithRefresh("at-beta-1", "rt-beta", time.Now().Add(time.Hour))

	accounts := NewAccounts()
	alphaAcc := Account{"email": "alpha@example.com", "name": "Alpha", "added_at": "2026-03-01T00:00:00Z", "last_used": "2026-03-01T00:00:00Z"}
	app.saveAccountSecret(ctx, alphaAcc, alphaOld)
	accounts.Set("alpha@example.com", alphaAcc)

	betaAcc := Account{"email": "beta@example.com", "name": "Beta", "added_at": "2026-03-01T00:00:00Z", "last_used": "2026-03-02T00:00:00Z"}
	app.saveAccountSecret(ctx, betaAcc, betaInitial)
	accounts.Set("beta@example.com", betaAcc)

	if err := app.store.Save(accounts); err != nil {
		t.Fatal(err)
	}
	if code := app.cmdSwitch(ctx, cliArgs{account: "beta@example.com"}); code != 0 {
		t.Fatalf("cmdSwitch(beta) exited %d", code)
	}

	// Case A: Antigravity IDE (agy --hub --app_data_dir=antigravity) refreshes beta's token in IDEOAuthToken
	// while OAuthToken and JetskiToken still hold betaInitial.
	betaIDERefreshedExpiry := time.Now().Add(3 * time.Hour)
	betaIDERefreshedRaw := sampleRawDiskTokenWithRefresh("at-beta-ide-refreshed", "rt-beta", betaIDERefreshedExpiry)
	betaIDERefreshed := sampleTokenWithRefresh("at-beta-ide-refreshed", "rt-beta", betaIDERefreshedExpiry)
	if err := os.WriteFile(app.paths.IDEOAuthToken, betaIDERefreshedRaw, 0o600); err != nil {
		t.Fatal(err)
	}

	app.reconcileActiveSession(ctx)

	vaultBeta, ok := app.vault.Get(ctx, accountSecretRef("beta@example.com"))
	if !ok || vaultBeta != betaIDERefreshed {
		t.Fatalf("vault token for beta = %q, want %q", vaultBeta, betaIDERefreshed)
	}
	for _, surfacePath := range []string{app.paths.OAuthToken, app.paths.IDEOAuthToken, app.paths.JetskiToken} {
		if got := readOAuthTokenFromFile(surfacePath); got != betaIDERefreshed {
			t.Fatalf("surface %s token = %q, want propagated %q", surfacePath, got, betaIDERefreshed)
		}
	}

	// Case B: A stale IDE Hub process writes alpha's refreshed token into IDEOAuthToken while beta is active.
	alphaRefreshedExpiry := time.Now().Add(4 * time.Hour)
	alphaRefreshedRaw := sampleRawDiskTokenWithRefresh("at-alpha-ide-refreshed", "rt-alpha", alphaRefreshedExpiry)
	alphaRefreshed := sampleTokenWithRefresh("at-alpha-ide-refreshed", "rt-alpha", alphaRefreshedExpiry)
	if err := os.WriteFile(app.paths.IDEOAuthToken, alphaRefreshedRaw, 0o600); err != nil {
		t.Fatal(err)
	}

	app.reconcileActiveSession(ctx)

	vaultAlpha, ok := app.vault.Get(ctx, accountSecretRef("alpha@example.com"))
	if !ok || vaultAlpha != alphaRefreshed {
		t.Fatalf("vault token for alpha = %q, want harvested %q", vaultAlpha, alphaRefreshed)
	}
	for _, surfacePath := range []string{app.paths.OAuthToken, app.paths.IDEOAuthToken, app.paths.JetskiToken} {
		if got := readOAuthTokenFromFile(surfacePath); got != betaIDERefreshed {
			t.Fatalf("surface %s after clobber restore = %q, want %q", surfacePath, got, betaIDERefreshed)
		}
	}
	if got := strings.Join(calls(), ""); got != "" {
		t.Fatalf("expected 0 /usr/bin/security calls, got %q", got)
	}
}

func TestEnsureAgyLauncherGuardWrapsAndUpgradesAgyBinary(t *testing.T) {
	t.Run("wraps standalone agy binary into libexec and installs sync wrapper", func(t *testing.T) {
		app, _, _ := newSessionGuardTestApp(t)
		binDir := filepath.Join(app.paths.Home, ".local", "bin")
		if err := os.MkdirAll(binDir, 0o755); err != nil {
			t.Fatal(err)
		}
		agyBin := filepath.Join(binDir, "agy")
		fakeMachO := append([]byte{0xcf, 0xfa, 0xed, 0xfe}, []byte("fake-agy-binary")...)
		if err := os.WriteFile(agyBin, fakeMachO, 0o755); err != nil {
			t.Fatal(err)
		}

		app.ensureAgyLauncherGuard()

		libexecBin := filepath.Join(app.paths.Home, ".local", "libexec", "agy")
		movedBytes, err := os.ReadFile(libexecBin)
		if err != nil {
			t.Fatalf("expected original binary moved to %s: %v", libexecBin, err)
		}
		if !bytes.Equal(movedBytes, fakeMachO) {
			t.Fatalf("libexec binary bytes mismatch")
		}

		wrapperBytes, err := os.ReadFile(agyBin)
		if err != nil {
			t.Fatal(err)
		}
		wrapper := string(wrapperBytes)
		if !strings.Contains(wrapper, "--sync-session") || !strings.Contains(wrapper, "WSL_INTEROP") {
			t.Fatalf("expected wrapper to invoke --sync-session and export WSL_INTEROP, got:\n%s", wrapper)
		}
		if !strings.Contains(wrapper, `*" --hub "*)`) || !strings.Contains(wrapper, "hub_pid") || !strings.Contains(wrapper, `exec "$real_command" "$@"`) {
			t.Fatalf("expected wrapper to supervise --hub and exec real_command for --version, got:\n%s", wrapper)
		}
	})

	t.Run("wraps IDE managed ~/.gemini/bin/agy into .agy.real and updates agy.version.json sidecar", func(t *testing.T) {
		app, _, _ := newSessionGuardTestApp(t)
		ideBinDir := filepath.Join(app.paths.Home, ".gemini", "bin")
		if err := os.MkdirAll(ideBinDir, 0o755); err != nil {
			t.Fatal(err)
		}
		ideAgyBin := filepath.Join(ideBinDir, "agy")
		fakeMachO := append([]byte{0xcf, 0xfa, 0xed, 0xfe}, []byte("fake-ide-agy-binary-payload")...)
		if err := os.WriteFile(ideAgyBin, fakeMachO, 0o755); err != nil {
			t.Fatal(err)
		}
		sidecarPath := ideAgyBin + ".version.json"
		initialSidecar := `{"schemaVersion":1,"version":"1.4.12","sha512":"deadbeef","size":197254160,"mtimeMs":1700000000000,"ino":123456,"releaseBaseUrl":"https://antigravity-cli-auto-updater-974169037036.us-central1.run.app"}`
		if err := os.WriteFile(sidecarPath, []byte(initialSidecar), 0o600); err != nil {
			t.Fatal(err)
		}

		app.ensureAgyLauncherGuard()

		realBin := filepath.Join(ideBinDir, ".agy.real")
		movedBytes, err := os.ReadFile(realBin)
		if err != nil {
			t.Fatalf("expected IDE agy binary moved to %s: %v", realBin, err)
		}
		if !bytes.Equal(movedBytes, fakeMachO) {
			t.Fatalf("IDE .agy.real bytes mismatch")
		}

		wrapperInfo, err := os.Stat(ideAgyBin)
		if err != nil {
			t.Fatal(err)
		}
		sidecarBytes, err := os.ReadFile(sidecarPath)
		if err != nil {
			t.Fatal(err)
		}
		var sidecar map[string]any
		if err := json.Unmarshal(sidecarBytes, &sidecar); err != nil {
			t.Fatalf("unmarshal sidecar: %v", err)
		}
		if gotVersion := getString(sidecar, "version"); gotVersion != "1.4.12" {
			t.Fatalf("sidecar version = %q, want 1.4.12", gotVersion)
		}
		if gotSHA := getString(sidecar, "sha512"); gotSHA != "deadbeef" {
			t.Fatalf("sidecar sha512 = %q, want deadbeef", gotSHA)
		}
		if gotSize, _ := sidecar["size"].(float64); int64(gotSize) != wrapperInfo.Size() {
			t.Fatalf("sidecar size = %v, want wrapper size %d", sidecar["size"], wrapperInfo.Size())
		}
		if gotMtime, _ := sidecar["mtimeMs"].(float64); int64(gotMtime) != wrapperInfo.ModTime().UnixMilli() {
			t.Fatalf("sidecar mtimeMs = %v, want wrapper mtimeMs %d", sidecar["mtimeMs"], wrapperInfo.ModTime().UnixMilli())
		}
		if gotIno, _ := sidecar["ino"].(float64); int64(gotIno) != 0 {
			t.Fatalf("sidecar ino = %v, want 0", sidecar["ino"])
		}
	})

	t.Run("mirrors active token to OS keyring when AGY_SWAP_MIRROR_OS_KEYRING=1", func(t *testing.T) {
		if runtime.GOOS != "darwin" {
			t.Skip("darwin security stub test")
		}
		t.Setenv("AGY_SWAP_MIRROR_OS_KEYRING", "1")
		calls := securityStub(t, 0)
		app, _, _ := newSessionGuardTestApp(t)
		ctx := context.Background()

		token := sampleTokenWithRefresh("at-mirror-1", "rt-mirror-1", time.Now().Add(time.Hour))
		if !app.credentials.Apply(ctx, token, "mirror@example.com") {
			t.Fatal("Apply returned false")
		}
		recorded := strings.Join(calls(), "\n")
		if !strings.Contains(recorded, "add-generic-password") {
			t.Fatalf("expected Apply to mirror token to OS keyring via add-generic-password, got calls:\n%s", recorded)
		}
		if !app.credentials.Clear(ctx) {
			t.Fatal("Clear returned false")
		}
		recordedAfterClear := strings.Join(calls(), "\n")
		if !strings.Contains(recordedAfterClear, "delete-generic-password") {
			t.Fatalf("expected Clear to delete token from OS keyring via delete-generic-password, got calls:\n%s", recordedAfterClear)
		}
	})

	t.Run("updates Antigravity IDE state.vscdb agy.onboarding.lastLoginUsername on Apply", func(t *testing.T) {
		sqliteBin, lookErr := exec.LookPath("sqlite3")
		if lookErr != nil || sqliteBin == "" {
			t.Skip("sqlite3 not installed")
		}
		app, _, _ := newSessionGuardTestApp(t)
		dbPath := filepath.Join(app.paths.Home, "Library", "Application Support", "Antigravity IDE", "User", "globalStorage", "state.vscdb")
		if mkdirErr := os.MkdirAll(filepath.Dir(dbPath), 0o755); mkdirErr != nil {
			t.Fatal(mkdirErr)
		}
		initSQL := `CREATE TABLE ItemTable (key TEXT UNIQUE ON CONFLICT REPLACE, value BLOB);
INSERT INTO ItemTable (key, value) VALUES ('google.google-antigravity', '{"antigravity-storage":{"agy.onboarding.lastLoginUsername":"stale@example.com","agy.onboarding.lastLoginIsGcpTos":"false"}}');`
		if out, initErr := exec.Command(sqliteBin, dbPath, initSQL).CombinedOutput(); initErr != nil {
			t.Fatalf("init sqlite db: %v (%s)", initErr, string(out))
		}

		token := sampleTokenWithRefresh("at-vscdb-1", "rt-vscdb-1", time.Now().Add(time.Hour))
		if !app.credentials.Apply(context.Background(), token, "switched@example.com") {
			t.Fatal("Apply returned false")
		}

		out, queryErr := exec.Command(sqliteBin, dbPath, "SELECT value FROM ItemTable WHERE key = 'google.google-antigravity';").Output()
		if queryErr != nil {
			t.Fatalf("select state.vscdb: %v", queryErr)
		}
		if !strings.Contains(string(out), `"agy.onboarding.lastLoginUsername":"switched@example.com"`) {
			t.Fatalf("expected state.vscdb lastLoginUsername updated to switched@example.com, got %s", string(out))
		}
	})

	t.Run("upgrades legacy python3 wrapper while preserving AGY_NORMAL_MODE", func(t *testing.T) {
		app, _, _ := newSessionGuardTestApp(t)
		binDir := filepath.Join(app.paths.Home, ".local", "bin")
		libexecDir := filepath.Join(app.paths.Home, ".local", "libexec")
		if err := os.MkdirAll(binDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(libexecDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(libexecDir, "agy"), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
			t.Fatal(err)
		}
		legacyWrapper := `#!/bin/sh
real_command="$HOME/.local/libexec/agy"
if [ "${AGY_NORMAL_MODE:-0}" = "1" ]; then
  "$real_command" "$@"
else
  "$real_command" --dangerously-skip-permissions "$@"
fi
status=$?
python3 - <<'PY' >/dev/null 2>&1 || true
print("legacy broken sync")
PY
exit "$status"
`
		agyBin := filepath.Join(binDir, "agy")
		if err := os.WriteFile(agyBin, []byte(legacyWrapper), 0o755); err != nil {
			t.Fatal(err)
		}

		app.ensureAgyLauncherGuard()

		wrapperBytes, err := os.ReadFile(agyBin)
		if err != nil {
			t.Fatal(err)
		}
		wrapper := string(wrapperBytes)
		if strings.Contains(wrapper, "python3 -") {
			t.Fatalf("expected legacy python3 snippet to be removed, got:\n%s", wrapper)
		}
		if !strings.Contains(wrapper, "AGY_NORMAL_MODE") || !strings.Contains(wrapper, "--dangerously-skip-permissions") {
			t.Fatalf("expected AGY_NORMAL_MODE behavior to be preserved, got:\n%s", wrapper)
		}
		if !strings.Contains(wrapper, "--sync-session") || !strings.Contains(wrapper, "WSL_INTEROP") {
			t.Fatalf("expected upgraded wrapper to invoke --sync-session and export WSL_INTEROP, got:\n%s", wrapper)
		}
	})
}

func TestHubWrapperRestartsOnlyAfterSwitch(t *testing.T) {
	cases := []struct {
		name     string
		hub      string
		wantRuns int
		wantCode int
	}{
		{"exits normally", `exit 0`, 1, 0},
		{"crashes", `exit 3`, 1, 3},
		{"terminated without a switch", `kill -TERM $$`, 1, 143},
		{"switch reload", `if [ "$n" -eq 1 ]; then echo "$n" > "$HUB_MARKER"; kill -TERM $$; fi; exit 0`, 2, 0},
		{"reload loop is capped", `echo "$n" > "$HUB_MARKER"; kill -TERM $$`, 10, 143},
		{"repeated switches are not capped", `if [ "$n" -le 6 ]; then sleep 1.1; echo "$n" > "$HUB_MARKER"; kill -TERM $$; fi; exit 0`, 7, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			count := filepath.Join(dir, "runs")
			marker := filepath.Join(dir, "cache", "agy-swap-hub-reload")
			if err := os.MkdirAll(filepath.Dir(marker), 0o700); err != nil {
				t.Fatal(err)
			}
			hub := filepath.Join(dir, "hub")
			body := "#!/bin/sh\necho run >> \"$HUB_COUNT\"\nn=$(( $(wc -l < \"$HUB_COUNT\") ))\n" + tc.hub + "\n"
			if err := os.WriteFile(hub, []byte(body), 0o755); err != nil {
				t.Fatal(err)
			}
			wrapper := filepath.Join(dir, "agy")
			if err := os.WriteFile(wrapper, []byte(buildAgyLauncherScriptForPaths(hub, "", marker, false)), 0o755); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, "/bin/sh", wrapper, "--hub")
			cmd.Env = []string{"HOME=" + dir, "PATH=/usr/bin:/bin", "AGY_SWAP_SKIP_SYNC=1", "HUB_COUNT=" + count, "HUB_MARKER=" + marker}
			_ = cmd.Run()
			if ctx.Err() != nil {
				t.Fatal("wrapper did not exit within 20s")
			}
			raw, _ := os.ReadFile(count)
			runs := strings.Count(string(raw), "run")
			if runs != tc.wantRuns {
				t.Fatalf("hub ran %d times, want %d", runs, tc.wantRuns)
			}
			if code := cmd.ProcessState.ExitCode(); code != tc.wantCode {
				t.Fatalf("wrapper exit = %d, want %d", code, tc.wantCode)
			}
		})
	}
}

func TestHubReloadMarkerPath(t *testing.T) {
	c := &Credentials{paths: Paths{OAuthToken: filepath.Join("home", ".gemini", "oauth_creds.json")}}
	want := filepath.Join("home", ".gemini", "cache", "agy-swap-hub-reload")
	if got := c.hubReloadMarkerPath(); got != want {
		t.Fatalf("hubReloadMarkerPath = %q, want %q", got, want)
	}
	if got := (*Credentials)(nil).hubReloadMarkerPath(); got != "" {
		t.Fatalf("nil credentials marker = %q, want empty", got)
	}
}

func TestReconcileActiveSessionRespectsUnsavedSignInAndSignOut(t *testing.T) {
	_ = securityStub(t, 0)
	ctx := context.Background()
	setup := func(t *testing.T) *Application {
		app, _, _ := newSessionGuardTestApp(t)
		beta := sampleTokenWithRefresh("at-beta", "rt-beta", time.Now().Add(time.Hour))
		accounts := NewAccounts()
		betaAcc := Account{"email": "beta@example.com", "name": "Beta", "added_at": "2026-03-01T00:00:00Z", "last_used": "2026-03-02T00:00:00Z"}
		app.saveAccountSecret(ctx, betaAcc, beta)
		accounts.Set("beta@example.com", betaAcc)
		if err := app.store.Save(accounts); err != nil {
			t.Fatal(err)
		}
		if code := app.cmdSwitch(ctx, cliArgs{account: "beta@example.com"}); code != 0 {
			t.Fatalf("cmdSwitch(beta) exited %d", code)
		}
		return app
	}

	t.Run("unsaved IDE sign-in is not overwritten", func(t *testing.T) {
		app := setup(t)
		gammaRaw := sampleRawDiskTokenWithRefresh("at-gamma", "rt-gamma", time.Now().Add(time.Hour))
		if err := os.WriteFile(app.paths.IDEOAuthToken, gammaRaw, 0o600); err != nil {
			t.Fatal(err)
		}
		app.reconcileActiveSession(ctx)
		got, _ := os.ReadFile(app.paths.IDEOAuthToken)
		if string(got) != string(gammaRaw) {
			t.Fatalf("IDE session for an unsaved account was overwritten: %s", got)
		}
	})

	t.Run("sign-out is not undone", func(t *testing.T) {
		app := setup(t)
		for _, p := range []string{app.paths.OAuthToken, app.paths.IDEOAuthToken, app.paths.JetskiToken} {
			if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
				t.Fatal(err)
			}
		}
		app.reconcileActiveSession(ctx)
		for _, p := range []string{app.paths.OAuthToken, app.paths.IDEOAuthToken, app.paths.JetskiToken} {
			if _, err := os.Stat(p); err == nil {
				t.Fatalf("%s was written back after sign-out", p)
			}
		}
	})

	t.Run("a file without a token is still repaired", func(t *testing.T) {
		app := setup(t)
		if err := os.WriteFile(app.paths.IDEOAuthToken, []byte("{}"), 0o600); err != nil {
			t.Fatal(err)
		}
		app.reconcileActiveSession(ctx)
		if !hasOAuthSession(readOAuthTokenFromFile(app.paths.IDEOAuthToken)) {
			t.Fatal("empty IDE session file was not repaired")
		}
	})
}

func TestEnsureAgyLauncherGuardUpgradesWrapperWithoutReloadMarker(t *testing.T) {
	app, _, _ := newSessionGuardTestApp(t)
	dir := filepath.Join(app.paths.Home, ".gemini", "bin")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	agyBin := filepath.Join(dir, "agy")
	// Shape of the wrapper written by earlier 2.21.0 builds: hub loop present,
	// no reload marker, restarts on every exit 0 or 143.
	old := "#!/bin/sh\n# Managed by agy-swap session guard\nreal_command=\"" + filepath.Join(dir, ".agy.real") + "\"\n" +
		"\"$swap_command\" --sync-session\nexport WSL_INTEROP=\"${WSL_INTEROP:-1}\"\nhub_pid=\"\"\n" +
		"if [ \"$status\" -ne 0 ] && [ \"$status\" -ne 143 ]; then exit \"$status\"; fi\n"
	if err := os.WriteFile(agyBin, []byte(old), 0o755); err != nil {
		t.Fatal(err)
	}
	app.ensureAgyLauncherGuardForPath(agyBin)
	got, _ := os.ReadFile(agyBin)
	if !strings.Contains(string(got), "reload_marker=") {
		t.Fatalf("wrapper without reload marker was not upgraded:\n%s", got)
	}
	if !strings.Contains(string(got), filepath.Join(dir, ".agy.real")) {
		t.Fatalf("upgraded wrapper lost the real agy path:\n%s", got)
	}
}

func TestEnsureAgyLauncherGuardReplacesWrapperAtomically(t *testing.T) {
	app, _, _ := newSessionGuardTestApp(t)
	dir := filepath.Join(app.paths.Home, ".gemini", "bin")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	agyBin := filepath.Join(dir, "agy")
	old := "#!/bin/sh\n# Managed by agy-swap session guard\nreal_command=\"" + filepath.Join(dir, ".agy.real") + "\"\n" +
		"\"$swap_command\" --sync-session\nexport WSL_INTEROP=\"${WSL_INTEROP:-1}\"\nhub_pid=\"\"\n" +
		"if [ \"$status\" -ne 0 ] && [ \"$status\" -ne 143 ]; then exit \"$status\"; fi\n"
	if err := os.WriteFile(agyBin, []byte(old), 0o755); err != nil {
		t.Fatal(err)
	}
	openBefore, err := os.Open(agyBin)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = openBefore.Close() }()

	app.ensureAgyLauncherGuardForPath(agyBin)

	stillOld, err := io.ReadAll(openBefore)
	if err != nil {
		t.Fatal(err)
	}
	if string(stillOld) != old {
		t.Fatalf("existing open file descriptor saw modified contents instead of atomic rename:\ngot:  %q\nwant: %q", string(stillOld), old)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".agy-swap-wrapper-") {
			t.Fatalf("leftover temporary wrapper file in %s: %s", dir, entry.Name())
		}
	}
}
