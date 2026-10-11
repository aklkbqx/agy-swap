package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// The test executable doubles as a real executable download fixture. Only the
// installer's external download boundary is substituted; hashing, version
// checks, installation, backup and cleanup run in the actual PowerShell script.
func TestMain(m *testing.M) {
	if version := os.Getenv("AGY_SWAP_INSTALLER_FIXTURE_VERSION"); version != "" {
		if len(os.Args) == 2 && os.Args[1] == "--version" {
			fmt.Printf("agy-swap v%s (installer-fixture)\n", version)
			if os.Getenv("AGY_SWAP_INSTALLER_FIXTURE_FAIL") == "1" {
				os.Exit(1)
			}
			os.Exit(0)
		}
		os.Exit(2)
	}
	os.Exit(m.Run())
}

func TestWindowsInstallerSavedFile(t *testing.T) {
	for _, tc := range []struct {
		name, checksum, version string
		wantSuccess             bool
		wantError               string
		failVersionCommand      bool
	}{
		{name: "success", version: "2.0.0", wantSuccess: true},
		{name: "checksum_rejected", checksum: strings.Repeat("0", 64), version: "2.0.0", wantError: "Error during installation: Checksum verification failed; installation aborted"},
		{name: "wrong_version_rejected", version: "3.0.0", wantError: "Error during installation: Downloaded binary reported an unexpected version:"},
		{name: "version_prefix_rejected", version: "2.0.00", wantError: "Error during installation: Downloaded binary reported an unexpected version:"},
		{name: "prerelease_rejected", version: "2.0.0-rc.1", wantError: "Error during installation: Downloaded binary reported an unexpected version:"},
		{name: "failed_self_test_rejected", version: "2.0.0", failVersionCommand: true, wantError: "Error during installation: Downloaded binary reported an unexpected version:"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			testWindowsInstaller(t, false, tc.checksum, tc.version, tc.wantSuccess, tc.wantError, tc.failVersionCommand)
		})
	}
}

func TestWindowsInstallerDecodedText(t *testing.T) {
	testWindowsInstaller(t, true, "", "2.0.0", true, "", false)
}

func testWindowsInstaller(t *testing.T, decoded bool, checksum, version string, wantSuccess bool, wantError string, failVersionCommand bool) {
	t.Helper()
	ps, err := exec.LookPath("powershell.exe")
	if err != nil {
		t.Fatal("Windows PowerShell is required for native installer verification")
	}
	source := os.Getenv("AGY_SWAP_INSTALLER_REVIEW_SOURCE")
	if source == "" {
		source = filepath.Join("..", "..", "install.ps1")
	}
	script, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	fixture, err := os.ReadFile(self)
	if err != nil {
		t.Fatal(err)
	}
	if checksum == "" {
		checksum = fmt.Sprintf("%x", sha256.Sum256(fixture))
	}
	root := t.TempDir()
	downloads := filepath.Join(root, "isolated downloads")
	targetDir := filepath.Join(root, "installed tool with spaces")
	for _, dir := range []string{downloads, targetDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	installer := filepath.Join(root, "saved installer.ps1")
	if err := os.WriteFile(installer, script, 0o644); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(targetDir, "agy-swap.exe")
	previous := []byte("previous installed binary\r\n")
	if err := os.WriteFile(target, previous, 0o644); err != nil {
		t.Fatal(err)
	}
	wrapper := filepath.Join(root, "run fixture.ps1")
	const wrapperScript = `
$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'
if ($PSVersionTable.PSVersion.Major -ne 5 -or $PSVersionTable.PSVersion.Minor -ne 1) {
  throw 'Native saved-file regression requires Windows PowerShell 5.1'
}
function Invoke-WebRequest {
  param([string]$Uri, [string]$OutFile, [switch]$UseBasicParsing)
  $base = 'https://github.com/aklkbqx/agy-swap/releases/download/v2.0.0/'
  if ($Uri -eq ($base + $env:AGY_SWAP_INSTALLER_ASSET)) {
    Copy-Item -LiteralPath $env:AGY_SWAP_INSTALLER_FIXTURE -Destination $OutFile
  } elseif ($Uri -eq ($base + 'checksums.txt')) {
    [IO.File]::WriteAllText($OutFile, ($env:AGY_SWAP_INSTALLER_CHECKSUM + '  ' + $env:AGY_SWAP_INSTALLER_ASSET), [Text.Encoding]::ASCII)
  } else {
    throw 'Unexpected release URL'
  }
}
if ($env:AGY_SWAP_INSTALLER_DECODED -eq '1') {
  $text = [IO.File]::ReadAllText($env:AGY_SWAP_INSTALLER_SCRIPT, [Text.Encoding]::UTF8)
  & ([ScriptBlock]::Create($text)) -Version '2.0.0' -TargetDir $env:AGY_SWAP_INSTALLER_TARGET
} else {
  & $env:AGY_SWAP_INSTALLER_SCRIPT -Version '2.0.0' -TargetDir $env:AGY_SWAP_INSTALLER_TARGET
}
exit $LASTEXITCODE
`
	if err := os.WriteFile(wrapper, []byte(wrapperScript), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, ps, "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-File", wrapper)
	cmd.Env = append(os.Environ(),
		"TEMP="+downloads, "TMP="+downloads,
		"AGY_SWAP_INSTALLER_FIXTURE="+self,
		"AGY_SWAP_INSTALLER_FIXTURE_VERSION="+version,
		fmt.Sprintf("AGY_SWAP_INSTALLER_FIXTURE_FAIL=%d", boolToInt(failVersionCommand)),
		"AGY_SWAP_INSTALLER_ASSET=agy-swap_v2.0.0_windows_"+runtime.GOARCH+".exe",
		"AGY_SWAP_INSTALLER_CHECKSUM="+checksum,
		"AGY_SWAP_INSTALLER_SCRIPT="+installer,
		"AGY_SWAP_INSTALLER_TARGET="+targetDir,
		fmt.Sprintf("AGY_SWAP_INSTALLER_DECODED=%d", boolToInt(decoded)))
	out, runErr := cmd.CombinedOutput()
	// Report process failures without publishing user-profile/host paths.
	safe := strings.ReplaceAll(string(out), root, "<review-temp>")
	safe = strings.ReplaceAll(safe, os.Getenv("USERPROFILE"), "<user-profile>")
	if wantSuccess && runErr != nil {
		t.Fatalf("native installer failed: %v\n%s", runErr, safe)
	}
	if !wantSuccess && (runErr == nil || !strings.Contains(string(out), wantError)) {
		t.Fatalf("expected rejection containing %q; exit=%v\n%s", wantError, runErr, safe)
	}
	installed, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	want := previous
	if wantSuccess {
		want = fixture
		backup, err := os.ReadFile(target + ".bak")
		if err != nil || !bytes.Equal(backup, previous) {
			t.Fatal("installer did not preserve the previous binary backup")
		}
	} else if _, err := os.Stat(target + ".bak"); !os.IsNotExist(err) {
		t.Fatal("rejected candidate reached the replacement/backup stage")
	}
	if !bytes.Equal(installed, want) {
		t.Fatal("installed target bytes differ from the accepted fixture or preserved previous binary")
	}
	remaining, err := os.ReadDir(downloads)
	if err != nil || len(remaining) != 0 {
		t.Fatal("installer did not clean its isolated download directory")
	}
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
