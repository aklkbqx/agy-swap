package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestChecksumsAndFormula(t *testing.T) {
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
	template := filepath.Join(dir, "formula.tmpl")
	templateText := "v=__VERSION__ a=__DARWIN_AMD64_SHA256__ b=__DARWIN_ARM64_SHA256__ c=__LINUX_AMD64_SHA256__ d=__LINUX_ARM64_SHA256__\n"
	if err := os.WriteFile(template, []byte(templateText), 0o644); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(dir, "formula.rb")
	if err := renderFormula("2.0.0", checksums, template, output); err != nil {
		t.Fatal(err)
	}
	rendered, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(rendered), "__") || !strings.Contains(string(rendered), "v=2.0.0") {
		t.Fatalf("unexpected formula: %s", rendered)
	}
}

func TestVerifyVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "install.sh")
	if err := os.WriteFile(path, []byte("VERSION=\"2.0.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := verifyVersion("2.0.0", path); err != nil {
		t.Fatal(err)
	}
	if err := verifyVersion("2.1.0", path); err == nil {
		t.Fatal("expected version mismatch")
	}
}
