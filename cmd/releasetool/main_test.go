package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestChecksums(t *testing.T) {
	dir := t.TempDir()
	for _, target := range []string{"darwin_amd64", "darwin_arm64", "linux_amd64", "linux_arm64"} {
		name := "agy-swap_v2.0.0_" + target
		if err := os.WriteFile(filepath.Join(dir, name), []byte(target), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	checksums := filepath.Join(dir, "checksums.txt")
	if err := writeChecksums(dir, checksums); err != nil {
		t.Fatal(err)
	}
	parsed, err := readChecksums(checksums)
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed) != 4 {
		t.Fatalf("expected 4 checksum entries, got %d", len(parsed))
	}
}

func TestVerifyVersion(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "install.sh")
	if err := os.WriteFile(path, []byte("VERSION=\"${AGY_SWAP_VERSION:-2.0.0}\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := verifyVersion("2.0.0", path); err != nil {
		t.Fatal(err)
	}
	if err := verifyVersion("2.1.0", path); err == nil {
		t.Fatal("expected version mismatch")
	}
	powershell := filepath.Join(dir, "install.ps1")
	if err := os.WriteFile(powershell, []byte("if ($true) { $Version = '2.0.0' }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := verifyVersion("v2.0.0", powershell); err != nil {
		t.Fatalf("PowerShell installer version was not recognized: %v", err)
	}
}

func TestVerifyAssetsRequiresAllReleasePlatforms(t *testing.T) {
	dir := t.TempDir()
	for _, target := range []string{"darwin_amd64", "darwin_arm64", "linux_amd64", "linux_arm64", "windows_amd64.exe", "windows_arm64.exe"} {
		if err := os.WriteFile(filepath.Join(dir, "agy-swap_v2.0.0_"+target), []byte(target), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := writeChecksums(dir, filepath.Join(dir, "checksums.txt")); err != nil {
		t.Fatal(err)
	}
	if err := verifyAssets("2.0.0", dir); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(dir, "agy-swap_v2.0.0_windows_arm64.exe")); err != nil {
		t.Fatal(err)
	}
	if err := verifyAssets("2.0.0", dir); err == nil {
		t.Fatal("missing Windows asset was accepted")
	}
}
