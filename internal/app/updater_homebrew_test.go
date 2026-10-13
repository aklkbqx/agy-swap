package app

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// The stub models Homebrew: upgrade installs the new version only when the tap
// is fresh (or after brew update), and exits 0 either way, like real brew.
const brewStub = `#!/bin/sh
printf '%s\n' "$*" >> "$STUB_LOG"
case "$1" in
--prefix) echo "$STUB_PREFIX";;
update) [ "$STUB_NEVER" = 1 ] || touch "$STUB_PREFIX/tap-fresh"; exit "${STUB_UPDATE_EXIT:-0}";;
upgrade)
  [ "$STUB_UPGRADE_EXIT" = 1 ] && exit 1
  if [ -f "$STUB_PREFIX/tap-fresh" ]; then
    printf '#!/bin/sh\necho "agy-swap v%s (release-v%s)"\n' "$STUB_LATEST" "$STUB_LATEST" > "$STUB_PREFIX/bin/agy-swap"
  else
    echo "Warning: agy-swap already installed" >&2
  fi;;
esac
`

func homebrewFixture(t *testing.T, fresh bool, env ...string) (string, func() []string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("Homebrew upgrades run on macOS and Linux")
	}
	root := t.TempDir()
	prefix := filepath.Join(root, "prefix")
	if err := os.MkdirAll(filepath.Join(prefix, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(prefix, "bin", "agy-swap"), []byte("#!/bin/sh\necho \"agy-swap v1.0.0 (release-v1.0.0)\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if fresh {
		if err := os.WriteFile(filepath.Join(prefix, "tap-fresh"), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	stubs := filepath.Join(root, "stubs")
	if err := os.Mkdir(stubs, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stubs, "brew"), []byte(brewStub), 0o755); err != nil {
		t.Fatal(err)
	}
	log := filepath.Join(root, "brew.log")
	t.Setenv("PATH", stubs+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("STUB_LOG", log)
	t.Setenv("STUB_PREFIX", prefix)
	t.Setenv("STUB_LATEST", "2.0.0")
	for _, pair := range env {
		key, value, _ := strings.Cut(pair, "=")
		t.Setenv(key, value)
	}
	return prefix, func() []string {
		data, _ := os.ReadFile(log)
		return strings.Split(strings.TrimSpace(string(data)), "\n")
	}
}

func TestHomebrewUpgradeConfirmsTheNewVersion(t *testing.T) {
	_, calls := homebrewFixture(t, true)
	var out bytes.Buffer
	if err := upgradeViaHomebrew(context.Background(), "2.0.0", &out, &out); err != nil {
		t.Fatalf("fresh tap upgrade failed: %v\n%s", err, out.String())
	}
	if strings.Contains(strings.Join(calls(), "\n"), "update") {
		t.Fatal("brew update ran although the first upgrade succeeded")
	}
}

func TestHomebrewUpgradeRefreshesAStaleTapOnce(t *testing.T) {
	_, calls := homebrewFixture(t, false)
	var out bytes.Buffer
	if err := upgradeViaHomebrew(context.Background(), "2.0.0", &out, &out); err != nil {
		t.Fatalf("stale tap was not refreshed: %v\n%s", err, out.String())
	}
	log := strings.Join(calls(), "\n")
	if strings.Count(log, "update") != 1 || strings.Count(log, "upgrade") < 2 {
		t.Fatalf("expected upgrade, one update, then upgrade again:\n%s", log)
	}
}

func TestHomebrewUpgradeReportsAnUnchangedVersion(t *testing.T) {
	homebrewFixture(t, false, "STUB_NEVER=1")
	var out bytes.Buffer
	err := upgradeViaHomebrew(context.Background(), "2.0.0", &out, &out)
	if err == nil || !strings.Contains(err.Error(), "v1.0.0") {
		t.Fatalf("unchanged install reported as %v", err)
	}
}

func TestHomebrewUpgradeFailureIsReported(t *testing.T) {
	homebrewFixture(t, true, "STUB_UPGRADE_EXIT=1")
	var out bytes.Buffer
	if err := upgradeViaHomebrew(context.Background(), "2.0.0", &out, &out); err == nil {
		t.Fatal("failed brew upgrade reported as success")
	}
}
