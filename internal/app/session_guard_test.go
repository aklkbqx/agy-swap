//go:build !windows

package app

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func newSessionGuardTestApp(t *testing.T) (*Application, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	paths := testPaths(t)
	var out, errOut bytes.Buffer
	in := bytes.NewBuffer(nil)
	credentials := NewCredentials(paths)
	credentials.backend = osCredentialBackend{}
	app := &Application{
		Version:     "2.17.1",
		In:          in,
		Out:         &out,
		Err:         &errOut,
		lineReader:  bufio.NewReader(in),
		paths:       paths,
		store:       NewStore(paths),
		credentials: credentials,
		vault:       NewFileAccountVault(filepath.Join(paths.ConfigDir, "vault.json")),
		http:        NewHTTPService(&errOut),
		p:           makePalette(false),
	}
	return app, &out, &errOut
}

func sampleRawDiskTokenWithRefresh(accessToken, refreshToken string, expiry time.Time) []byte {
	payload := map[string]any{
		"token": map[string]any{
			"access_token":  accessToken,
			"refresh_token": refreshToken,
			"token_type":    "Bearer",
			"expiry":        expiry.UTC().Format(time.RFC3339),
		},
	}
	raw, _ := json.Marshal(payload)
	return raw
}

func sampleTokenWithRefresh(accessToken, refreshToken string, expiry time.Time) string {
	raw := sampleRawDiskTokenWithRefresh(accessToken, refreshToken, expiry)
	return "go-keyring-base64:" + base64.StdEncoding.EncodeToString(raw)
}

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

	markerPath := filepath.Join(filepath.Dir(app.paths.OAuthToken), "cache", "antigravity-keyring-unavailable")
	info, err := os.Stat(markerPath)
	if err != nil {
		t.Fatalf("expected keyring bypass marker at %s: %v", markerPath, err)
	}
	if time.Until(info.ModTime()) < 365*24*time.Hour {
		t.Fatalf("expected marker mtime far in the future, got %v", info.ModTime())
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
	info, err = os.Stat(markerPath)
	if err != nil {
		t.Fatalf("expected keyring bypass marker to remain after Clear: %v", err)
	}
	if time.Until(info.ModTime()) < 365*24*time.Hour {
		t.Fatalf("expected marker mtime far in the future after Clear, got %v", info.ModTime())
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
