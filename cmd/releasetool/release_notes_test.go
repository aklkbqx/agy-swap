package main

import (
	"path/filepath"
	"strings"
	"testing"
)

const changelogFixture = `# Changelog

## 2.0.1

### Fixes

- Second fix.

## 2.0.0

### Features

- First feature.
- Another.

## 1.9.0

- Old.
`

func TestReleaseNotesExtractsOneVersionSection(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "CHANGELOG.md")
	writeTestFile(t, path, changelogFixture)
	notes, err := releaseNotes("v2.0.0", path)
	if err != nil {
		t.Fatal(err)
	}
	want := "### Features\n\n- First feature.\n- Another.\n"
	if notes != want {
		t.Fatalf("notes = %q, want %q", notes, want)
	}
	if notes, err := releaseNotes("2.0.1", path); err != nil || !strings.Contains(notes, "Second fix.") || strings.Contains(notes, "First feature") {
		t.Fatalf("top section = %q, %v", notes, err)
	}
	if notes, err := releaseNotes("1.9.0", path); err != nil || notes != "- Old.\n" {
		t.Fatalf("last section = %q, %v", notes, err)
	}
}

func TestReleaseNotesRejectsMissingOrEmptySections(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "CHANGELOG.md")
	writeTestFile(t, path, "# Changelog\n\n## 3.0.0\n\n## 2.0.0\n\n- Shipped.\n")
	if _, err := releaseNotes("3.0.0", path); err == nil {
		t.Fatal("empty section accepted")
	}
	if _, err := releaseNotes("4.0.0", path); err == nil {
		t.Fatal("missing section accepted")
	}
	// A version that is a prefix of another must not match it.
	writeTestFile(t, path, "# Changelog\n\n## 2.0.10\n\n- Ten.\n")
	if _, err := releaseNotes("2.0.1", path); err == nil {
		t.Fatal("2.0.1 matched the 2.0.10 section")
	}
}
