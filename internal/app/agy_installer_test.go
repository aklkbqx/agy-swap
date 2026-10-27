package app

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func buildTestAgyTarGz(t *testing.T, payload []byte) ([]byte, string) {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	hdr := &tar.Header{
		Name: "antigravity",
		Mode: 0o755,
		Size: int64(len(payload)),
	}
	if err := tw.WriteHeader(hdr); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(payload); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	raw := buf.Bytes()
	sum := sha512.Sum512(raw)
	return raw, hex.EncodeToString(sum[:])
}

func TestInstallAgyDownloadsVerifiesAndExtractsTarGz(t *testing.T) {
	app, out, errOut := newSessionGuardTestApp(t)
	fakeBinary := append([]byte{0xcf, 0xfa, 0xed, 0xfe}, []byte("antigravity-cli-1.4.7")...)
	archiveBytes, digest := buildTestAgyTarGz(t, fakeBinary)

	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/manifests/"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]string{
				"version": "1.4.7",
				"url":     srv.URL + "/downloads/cli.tar.gz",
				"sha512":  digest,
			})
		case r.URL.Path == "/downloads/cli.tar.gz":
			w.Header().Set("Content-Type", "application/octet-stream")
			_, _ = w.Write(archiveBytes)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	t.Setenv("AGY_SWAP_AGY_INSTALL_URL", srv.URL)
	customDir := filepath.Join(app.paths.Home, "custom-bin")
	code := app.Run(context.Background(), []string{"install-agy", "--dir", customDir})
	if code != 0 {
		t.Fatalf("install-agy exit = %d, out=%s, err=%s", code, out.String(), errOut.String())
	}

	targetName := "agy"
	if runtime.GOOS == "windows" {
		targetName = "agy.exe"
	}
	installedPath := filepath.Join(customDir, targetName)
	data, err := os.ReadFile(installedPath)
	if err != nil {
		t.Fatalf("expected installed binary at %s: %v", installedPath, err)
	}
	if runtime.GOOS == "windows" {
		if !bytes.Equal(data, fakeBinary) {
			t.Fatalf("installed binary mismatch on windows")
		}
	} else {
		// On Unix, customDir/agy is wrapped by session guard and the real binary is preserved alongside or in libexec
		if !strings.Contains(string(data), "--sync-session") {
			t.Fatalf("expected customDir/agy to be wrapped with --sync-session, got:\n%s", string(data))
		}
	}

	if got := app.resolveAgyExecutable(); got == "" {
		t.Fatalf("expected resolveAgyExecutable to locate newly installed agy in %s", customDir)
	}
}

func TestInstallAgyRejectsChecksumMismatch(t *testing.T) {
	app, _, errOut := newSessionGuardTestApp(t)
	fakeBinary := []byte("corrupted-payload")
	archiveBytes, _ := buildTestAgyTarGz(t, fakeBinary)

	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/manifests/"):
			_ = json.NewEncoder(w).Encode(map[string]string{
				"version": "1.4.7",
				"url":     srv.URL + "/downloads/cli.tar.gz",
				"sha512":  strings.Repeat("a", 128),
			})
		case r.URL.Path == "/downloads/cli.tar.gz":
			_, _ = w.Write(archiveBytes)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	t.Setenv("AGY_SWAP_AGY_INSTALL_URL", srv.URL)
	customDir := filepath.Join(app.paths.Home, "bad-bin")
	code := app.Run(context.Background(), []string{"install-agy", "--dir", customDir})
	if code == 0 {
		t.Fatalf("expected checksum mismatch to fail")
	}
	if !strings.Contains(strings.ToLower(errOut.String()), "checksum") {
		t.Fatalf("expected checksum error message, got: %s", errOut.String())
	}
	if _, err := os.Stat(filepath.Join(customDir, "agy")); !os.IsNotExist(err) {
		t.Fatalf("corrupted binary should not be installed")
	}
}

func TestResolveAgyExecutableFindsCustomAndFallbackPaths(t *testing.T) {
	app, _, _ := newSessionGuardTestApp(t)
	t.Setenv("PATH", "")

	// 1. Fallback ~/.local/bin/agy when PATH is empty
	localBin := filepath.Join(app.paths.Home, ".local", "bin")
	if err := os.MkdirAll(localBin, 0o755); err != nil {
		t.Fatal(err)
	}
	exeName := "agy"
	if runtime.GOOS == "windows" {
		exeName = "agy.exe"
	}
	fallbackAgy := filepath.Join(localBin, exeName)
	if err := os.WriteFile(fallbackAgy, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got := app.resolveAgyExecutable(); got != fallbackAgy {
		t.Fatalf("resolveAgyExecutable fallback = %q, want %q", got, fallbackAgy)
	}

	// 2. Target setting overrides fallback
	targetDir := filepath.Join(app.paths.Home, "target-bin")
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		t.Fatal(err)
	}
	targetAgy := filepath.Join(targetDir, exeName)
	if err := os.WriteFile(targetAgy, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	settings, err := app.loadSettings()
	if err != nil {
		t.Fatal(err)
	}
	settings.Targets["agy"] = TargetConfig{Command: targetAgy, Enabled: true}
	if err := app.store.SaveSettings(settings); err != nil {
		t.Fatal(err)
	}
	if got := app.resolveAgyExecutable(); got != targetAgy {
		t.Fatalf("resolveAgyExecutable target = %q, want %q", got, targetAgy)
	}

	// 3. AGY_SWAP_AGY_BIN env var overrides everything
	envDir := filepath.Join(app.paths.Home, "env-bin")
	if err := os.MkdirAll(envDir, 0o755); err != nil {
		t.Fatal(err)
	}
	envAgy := filepath.Join(envDir, exeName)
	if err := os.WriteFile(envAgy, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AGY_SWAP_AGY_BIN", envAgy)
	if got := app.resolveAgyExecutable(); got != envAgy {
		t.Fatalf("resolveAgyExecutable env = %q, want %q", got, envAgy)
	}
}

func TestEnsureAgyLauncherGuardSupportsCustomPathsAndSymlinks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix shell wrapper test")
	}
	app, _, _ := newSessionGuardTestApp(t)

	// Case 1: ~/.local/bin/agy is a relative symlink to ../opt/agy/bin/agy
	optDir := filepath.Join(app.paths.Home, ".local", "opt", "agy", "bin")
	binDir := filepath.Join(app.paths.Home, ".local", "bin")
	if err := os.MkdirAll(optDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	realTarget := filepath.Join(optDir, "agy")
	fakeMachO := append([]byte{0xcf, 0xfa, 0xed, 0xfe}, []byte("real-opt-agy")...)
	if err := os.WriteFile(realTarget, fakeMachO, 0o755); err != nil {
		t.Fatal(err)
	}
	symlinkPath := filepath.Join(binDir, "agy")
	if err := os.Symlink("../opt/agy/bin/agy", symlinkPath); err != nil {
		t.Fatal(err)
	}

	app.ensureAgyLauncherGuard()

	// The real target in optDir must remain intact and executable
	targetBytes, err := os.ReadFile(realTarget)
	if err != nil || !bytes.Equal(targetBytes, fakeMachO) {
		t.Fatalf("real target was corrupted or moved: %v", err)
	}
	// symlinkPath should now be a wrapper script pointing to the resolved realTarget
	wrapperBytes, err := os.ReadFile(symlinkPath)
	if err != nil {
		t.Fatal(err)
	}
	wrapper := string(wrapperBytes)
	if !strings.Contains(wrapper, realTarget) || !strings.Contains(wrapper, "--sync-session") {
		t.Fatalf("expected wrapper at %s to reference resolved %s and --sync-session, got:\n%s", symlinkPath, realTarget, wrapper)
	}
}

func TestAccountMigrateDistinguishesAlreadyMigratedAccounts(t *testing.T) {
	app, out, errOut := newSessionGuardTestApp(t)
	ctx := context.Background()
	tok := sampleTokenWithRefresh("acc-1", "ref-1", time.Now().Add(time.Hour))
	acc := newAccount("alpha@example.com", "Alpha", tok)
	if _, ok := app.saveAccountSecret(ctx, acc, tok); !ok {
		t.Fatal("saveAccountSecret failed")
	}
	accounts := NewAccounts()
	accounts.Set("alpha@example.com", acc)
	if err := app.store.Save(accounts); err != nil {
		t.Fatal(err)
	}

	code := app.Run(ctx, []string{"account", "migrate", "--force"})
	if code != 0 {
		t.Fatalf("account migrate exit=%d err=%s", code, errOut.String())
	}
	if !strings.Contains(out.String(), "0 remain in legacy storage") {
		t.Fatalf("expected 0 remain in legacy storage for already-migrated account, got: %s", out.String())
	}
}

type headlessPreparedBackend struct {
	preparedSessionBackend
}

func (b *headlessPreparedBackend) NoLogonSession() bool { return true }

func TestWindowsHeadlessSessionStoreFallback(t *testing.T) {
	paths := testPaths(t)
	backend := &headlessPreparedBackend{preparedSessionBackend: preparedSessionBackend{fail: true}}
	c := NewCredentials(paths)
	c.backend = backend
	raw := sessionTestPayload()
	if !c.Apply(context.Background(), raw, "user@example.invalid") {
		t.Fatal("expected headless logon session fallback to succeed via file session store")
	}
	if _, err := os.Stat(paths.OAuthToken); err != nil {
		t.Fatalf("expected OAuthToken file written on headless fallback: %v", err)
	}
	if _, err := os.Stat(c.keyringMarkerPath()); err != nil {
		t.Fatalf("expected keyring bypass marker written on headless fallback: %v", err)
	}
}
