package app

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestValidateAgyDownloadURLRequiresHTTPSExceptLoopback(t *testing.T) {
	if err := validateAgyDownloadURL("https://storage.googleapis.com/agy/agy.tar.gz", defaultAgyUpdaterBaseURL); err != nil {
		t.Fatalf("valid https URL rejected: %v", err)
	}
	if err := validateAgyDownloadURL("http://evil.example.com/agy.tar.gz", defaultAgyUpdaterBaseURL); err == nil {
		t.Fatal("non-HTTPS remote download URL was accepted")
	}
	if err := validateAgyDownloadURL("http://127.0.0.1:8080/agy.tar.gz", "https://storage.googleapis.com"); err == nil {
		t.Fatal("downgrade from HTTPS base URL to HTTP loopback URL was accepted")
	}
	if err := validateAgyDownloadURL("http://127.0.0.1:8080/agy.tar.gz", "http://127.0.0.1:8080"); err != nil {
		t.Fatalf("loopback test server HTTP URL rejected: %v", err)
	}
}

func TestHTTPServiceInsecureTLSOnlyRetriesTLSErrors(t *testing.T) {
	t.Setenv("AGY_SWAP_INSECURE_TLS", "1")
	var warnBuf strings.Builder
	h := NewHTTPService(&warnBuf)
	// Point at a closed local listener so Dial fails with connection refused (NOT a TLS error).
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	closedURL := srv.URL
	srv.Close()

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, closedURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := h.do(req)
	if resp != nil {
		_ = resp.Body.Close()
	}
	if err == nil {
		t.Fatal("expected connection error on closed server")
	}
	if warnBuf.Len() != 0 {
		t.Fatalf("AGY_SWAP_INSECURE_TLS retried a non-TLS error and emitted warning: %q", warnBuf.String())
	}
}

func TestResolveHostViaDoHFallsBackToAAAAWhenNoARecords(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/dns-json")
		switch r.URL.Query().Get("type") {
		case "A":
			fmt.Fprint(w, `{"Status":0,"Answer":[]}`)
		case "AAAA":
			fmt.Fprint(w, `{"Status":0,"Answer":[{"type":28,"data":"2606:4700:4700::1111"}]}`)
		default:
			w.WriteHeader(http.StatusBadRequest)
		}
	}))
	defer srv.Close()
	t.Setenv("AGY_SWAP_DOH_URL", srv.URL+"/dns-query")

	ips, err := resolveHostViaDoH(context.Background(), srv.Client(), "example.com")
	if err != nil {
		t.Fatalf("resolveHostViaDoH AAAA fallback failed: %v", err)
	}
	if len(ips) != 1 || ips[0] != "2606:4700:4700::1111" {
		t.Fatalf("unexpected DoH AAAA result: %v", ips)
	}
}

func TestBackupImportDeletesReplacedVaultSecrets(t *testing.T) {
	f := newCLIFixture(t, "alpha@example.com")
	vault := fakeAccountVault{}
	f.a.vault = vault

	oldRef := accountSecretRef("alpha@example.com") + ":legacyold"
	vault[oldRef] = f.tokens["alpha@example.com"]
	accounts, err := f.a.store.Load(false)
	if err != nil {
		t.Fatal(err)
	}
	acc := accounts.ByEmail["alpha@example.com"]
	acc["secret_ref"] = oldRef
	delete(acc, "token_data")
	accounts.Set("alpha@example.com", acc)
	if err := f.a.store.Save(accounts); err != nil {
		t.Fatal(err)
	}

	backupPath := filepath.Join(t.TempDir(), "backup.json")
	if code := f.run("", "backup", "export", backupPath, "--include-secrets", "--passphrase", "secret-pass"); code != 0 {
		t.Fatalf("backup export failed: %s", f.errOut.String())
	}
	if code := f.run("", "backup", "import", backupPath, "--passphrase", "secret-pass"); code != 0 {
		t.Fatalf("backup import failed: %s", f.errOut.String())
	}
	if _, stillPresent := vault[oldRef]; stillPresent {
		t.Fatalf("replaced vault ref %q was leaked after backup import; vault=%v", oldRef, vault)
	}
	if len(vault) != 1 {
		t.Fatalf("expected exactly 1 live vault entry after import, got %d: %v", len(vault), vault)
	}
}

func TestBacksOffRecognizesOAuth429And5xxErrors(t *testing.T) {
	for _, status := range []int{429, 500, 502, 503} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(status)
		}))
		httpSvc := NewHTTPService(io.Discard)
		httpSvc.oauthURL = srv.URL
		expired := tokenBlob(t, "u@example.com", true, "r-tok", time.Now().Add(-time.Hour))
		_, _, err := httpSvc.accessTokenData(context.Background(), expired)
		srv.Close()
		if err == nil {
			t.Fatalf("HTTP %d unexpectedly succeeded", status)
		}
		if !backsOff(err) {
			t.Fatalf("backsOff(%v) = false for OAuth HTTP %d, want true", err, status)
		}
		if !strings.Contains(err.Error(), fmt.Sprintf("OAuth refresh failed (HTTP %d)", status)) {
			t.Fatalf("unexpected OAuth error format: %q", err.Error())
		}
	}
	// HTTP 400/401 must remain auth invalidation, NOT backoff.
	srv401 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv401.Close()
	httpSvc := NewHTTPService(io.Discard)
	httpSvc.oauthURL = srv401.URL
	expired := tokenBlob(t, "u@example.com", true, "r-tok", time.Now().Add(-time.Hour))
	_, _, err401 := httpSvc.accessTokenData(context.Background(), expired)
	if backsOff(err401) {
		t.Fatalf("backsOff(%v) = true for HTTP 401, want false", err401)
	}
	if !authInvalidationError(err401) {
		t.Fatalf("authInvalidationError(%v) = false for HTTP 401, want true", err401)
	}
}

func TestMetricsPrometheusOutputIsDeterministicAndHasHeaders(t *testing.T) {
	f := newCLIFixture(t, "alpha@example.com")
	accounts, err := f.a.store.Load(false)
	if err != nil {
		t.Fatal(err)
	}
	acc := accounts.ByEmail["alpha@example.com"]
	acc["quota_snapshot"] = map[string]any{
		"observed_at": isoTime(time.Now().UTC()),
		"tier":        map[string]any{"id": "t", "name": "T"},
		"groups": []any{
			poolGroup("third_party", "Claude and GPT models", poolBucket("weekly", 0.5, 10*time.Hour)),
			poolGroup("gemini", "Gemini models", poolBucket("weekly", 0.8, 10*time.Hour)),
			poolGroup("argon", "Gemini 4 Argon", poolBucket("weekly", 0.9, 10*time.Hour)),
		},
	}
	if err := f.a.store.Save(accounts); err != nil {
		t.Fatal(err)
	}

	var first string
	for i := 0; i < 6; i++ {
		if code := f.run("", "metrics", "prometheus"); code != 0 {
			t.Fatalf("metrics prometheus failed: %s", f.errOut.String())
		}
		out := f.out.String()
		if i == 0 {
			first = out
			for _, want := range []string{
				"# HELP agy_swap_quota_remaining",
				"# TYPE agy_swap_quota_remaining gauge",
				"# HELP agy_swap_account_error",
				"# TYPE agy_swap_account_error gauge",
			} {
				if !strings.Contains(out, want) {
					t.Fatalf("metrics output missing %q:\n%s", want, out)
				}
			}
		} else if out != first {
			t.Fatalf("metrics prometheus output is non-deterministic:\n--- run 0 ---\n%s\n--- run %d ---\n%s", first, i, out)
		}
	}
}

func TestCompletionPowerShellIncludesUnbindAndHelpListsVerify(t *testing.T) {
	f := newCLIFixture(t)
	if code := f.run("", "completion", "powershell"); code != 0 {
		t.Fatalf("completion powershell failed: %s", f.errOut.String())
	}
	if !strings.Contains(f.out.String(), "'unbind'") {
		t.Fatalf("PowerShell completion missing 'unbind': %s", f.out.String())
	}

	if code := f.run("", "--help"); code != 0 {
		t.Fatalf("--help failed: %s", f.errOut.String())
	}
	if !strings.Contains(f.out.String(), "verify") {
		t.Fatalf("--help missing verify command:\n%s", f.out.String())
	}
}

func TestInstallScriptsAndKeychainDarwinHardening(t *testing.T) {
	shBytes, err := os.ReadFile("../../install.sh")
	if err != nil {
		t.Fatal(err)
	}
	sh := string(shBytes)
	if !strings.Contains(sh, `AGY_SWAP_TARGET_DIR`) || !strings.Contains(sh, `AGY_SWAP_VERSION`) {
		t.Fatal("install.sh missing env override checks")
	}
	// Homebrew delegation must not delete ~/.local/bin/agy-swap before brew upgrade succeeds
	brewUpgradeIdx := strings.Index(sh, "brew upgrade aklkbqx/agy-swap/agy-swap")
	removeShadowIdx := strings.Index(sh, `rm -f "${HOME}/.local/bin/agy-swap"`)
	if brewUpgradeIdx < 0 || removeShadowIdx < 0 || removeShadowIdx < brewUpgradeIdx {
		t.Fatalf("install.sh removes ~/.local/bin/agy-swap before brew upgrade succeeds (remove=%d upgrade=%d)", removeShadowIdx, brewUpgradeIdx)
	}
	if !strings.Contains(sh, `rm -f "${TARGET_FILE}.bak"`) {
		t.Fatal("install.sh does not clean up .bak file after successful install")
	}

	psBytes, err := os.ReadFile("../../install.ps1")
	if err != nil {
		t.Fatal(err)
	}
	ps := string(psBytes)
	if strings.Contains(ps, "iex -Insecure") {
		t.Fatal("install.ps1 still recommends broken 'iex -Insecure' command")
	}
	if !strings.Contains(ps, "IsDefined([Net.SecurityProtocolType], 'Tls13')") {
		t.Fatal("install.ps1 does not guard Tls13 for PowerShell 5.1")
	}
	if !strings.Contains(ps, "$prevCertCallback") {
		t.Fatal("install.ps1 does not save/restore ServerCertificateValidationCallback in finally")
	}
	if !strings.Contains(ps, "^[a-f0-9]{64}$") {
		t.Fatal("install.ps1 does not validate 64-hex SHA-256 checksum format")
	}

	kcBytes, err := os.ReadFile("keychain_darwin.m")
	if err != nil {
		t.Fatal(err)
	}
	kc := string(kcBytes)
	delIdx := strings.Index(kc, "int deleteSecret(")
	if delIdx < 0 || !strings.Contains(kc[delIdx:], "interactionNotAllowed = YES") {
		t.Fatal("keychain_darwin.m deleteSecret missing LAContext interactionNotAllowed = YES")
	}
}

func TestSettingsPreservesExplicitZeroNotificationValuesAndFlagEquals(t *testing.T) {
	f := newCLIFixture(t, "alpha@example.com")
	if code := f.run("", "config", "set", "notifications.threshold", "0"); code != 0 {
		t.Fatalf("config set notifications.threshold 0 failed: %s", f.errOut.String())
	}
	if code := f.run("", "config", "set", "notifications.cooldown_seconds", "0"); code != 0 {
		t.Fatalf("config set notifications.cooldown_seconds 0 failed: %s", f.errOut.String())
	}
	settings, err := f.a.loadSettings()
	if err != nil {
		t.Fatal(err)
	}
	if settings.Notifications.Threshold != 0 || settings.Notifications.CooldownSeconds != 0 {
		t.Fatalf("explicit zero notification settings were overwritten by defaults: %#v", settings.Notifications)
	}

	// Verify --flag=value syntax works in both core CLI and extended commands.
	parsedCore, err := parseCLI([]string{"next", "--family=claude"})
	if err != nil || parsedCore.family != "claude" {
		t.Fatalf("parseCLI(--family=claude) = %#v, %v", parsedCore, err)
	}
	_, parsedExt, _, err := parseExtended([]string{"recommend", "--family=gemini", "--limit=5"})
	if err != nil || parsedExt.Family != "gemini" || parsedExt.Limit != 5 {
		t.Fatalf("parseExtended(--family=gemini --limit=5) = %#v, %v", parsedExt, err)
	}
}
