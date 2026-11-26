package main

import (
	"os"
	"path/filepath"
	"strings"
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
	asset := filepath.Join(dir, "agy-swap_v2.0.0_linux_amd64")
	if err := os.WriteFile(asset, []byte("tampered"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := verifyAssets("2.0.0", dir); err == nil {
		t.Fatal("tampered release asset accepted")
	}
	if err := writeChecksums(dir, filepath.Join(dir, "checksums.txt")); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(dir, "agy-swap_v2.0.0_windows_arm64.exe")); err != nil {
		t.Fatal(err)
	}
	if err := verifyAssets("2.0.0", dir); err == nil {
		t.Fatal("missing Windows asset was accepted")
	}
}

func TestCalculateNextVersion(t *testing.T) {
	for _, tc := range []struct {
		current, target, want string
	}{
		{"2.2.0", "patch", "2.2.1"},
		{"2.2.0", "minor", "2.3.0"},
		{"2.2.0", "major", "3.0.0"},
		{"2.2.0", "2.2.5", "2.2.5"},
		{"2.2.0", "v2.2.5", "2.2.5"},
	} {
		got, err := calculateNextVersion(tc.current, tc.target)
		if err != nil {
			t.Fatalf("calculateNextVersion(%q, %q) returned error: %v", tc.current, tc.target, err)
		}
		if got != tc.want {
			t.Fatalf("calculateNextVersion(%q, %q) = %q, want %q", tc.current, tc.target, got, tc.want)
		}
	}

	for _, invalid := range []string{"invalid", "2.2", "2.2.0.1", "foo.bar.baz", "2.25.0foo", "-1.0.0", "2.-1.0", "2.25.+1"} {
		if _, err := calculateNextVersion("2.2.0", invalid); err == nil {
			t.Fatalf("expected error for invalid target %q", invalid)
		}
	}
}

func TestBumpVersionRollsBackOnFailure(t *testing.T) {
	root := t.TempDir()
	makefilePath := filepath.Join(root, "Makefile")
	origMakefile := "VERSION ?= 2.25.0\n"
	if err := os.WriteFile(makefilePath, []byte(origMakefile), 0o644); err != nil {
		t.Fatal(err)
	}
	// Create cmd/agy-swap/main.go WITHOUT the expected version line so bump fails mid-way.
	mainDir := filepath.Join(root, "cmd", "agy-swap")
	if err := os.MkdirAll(mainDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(mainDir, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := bumpVersion("patch", root); err == nil {
		t.Fatal("expected bumpVersion to fail when cmd/agy-swap/main.go lacks version line")
	}
	got, err := os.ReadFile(makefilePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != origMakefile {
		t.Fatalf("Makefile was not rolled back after mid-bump failure: got %q want %q", string(got), origMakefile)
	}
}

func TestVerifyAssetsRejectsContaminatedRelease(t *testing.T) {
	for _, extra := range []string{"agy-swap_v1.0.0_linux_arm64", "agy-swap.rb", "notes.md"} {
		t.Run(extra, func(t *testing.T) {
			dir := t.TempDir()
			for _, target := range releaseTargets {
				if err := os.WriteFile(filepath.Join(dir, "agy-swap_v2.0.0_"+target), []byte(target), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			if err := writeChecksums(dir, filepath.Join(dir, "checksums.txt")); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, extra), []byte("unexpected"), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := verifyAssets("2.0.0", dir); err == nil {
				t.Fatal("contaminated release accepted")
			}
		})
	}
}

func TestReadChecksumsRejectsInvalidManifest(t *testing.T) {
	for name, data := range map[string]string{
		"duplicate": strings.Repeat("a", 64) + "  asset\n" + strings.Repeat("b", 64) + "  asset\n",
		"nonhex":    strings.Repeat("z", 64) + "  asset\n",
		"empty":     "",
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "checksums.txt")
			if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
				t.Fatal(err)
			}
			if _, err := readChecksums(path); err == nil {
				t.Fatal("invalid manifest accepted")
			}
		})
	}
}
