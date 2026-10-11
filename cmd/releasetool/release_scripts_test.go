package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

type tapFixture struct {
	root, tap, remote, dist, head string
}

func scriptGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+os.DevNull)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func newTapFixture(t *testing.T) tapFixture {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("POSIX release scripts require the macOS release builder")
	}
	root := t.TempDir()
	f := tapFixture{root: root, tap: filepath.Join(root, "tap"), remote: filepath.Join(root, "remote.git"), dist: filepath.Join(root, "dist")}
	for _, path := range []string{f.tap, f.remote, f.dist} {
		if err := os.Mkdir(path, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	scriptGit(t, f.remote, "init", "--bare", "--initial-branch=main")
	scriptGit(t, f.tap, "init", "--initial-branch=main")
	scriptGit(t, f.tap, "config", "user.name", "Release test")
	scriptGit(t, f.tap, "config", "user.email", "release@example.invalid")
	if err := os.WriteFile(filepath.Join(f.tap, "README"), []byte("fixture\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	scriptGit(t, f.tap, "add", "README")
	scriptGit(t, f.tap, "commit", "-m", "initial")
	scriptGit(t, f.tap, "remote", "add", "origin", f.remote)
	scriptGit(t, f.tap, "push", "origin", "main")
	f.head = scriptGit(t, f.tap, "rev-parse", "HEAD")
	var checksums strings.Builder
	for _, target := range []string{"darwin_arm64", "darwin_amd64", "linux_arm64", "linux_amd64"} {
		checksums.WriteString(strings.Repeat("a", 64) + "  agy-swap_v2.0.0_" + target + "\n")
	}
	if err := os.WriteFile(filepath.Join(f.dist, "checksums.txt"), []byte(checksums.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	return f
}

func runTapScript(t *testing.T, f tapFixture, args ...string) ([]byte, error) {
	t.Helper()
	script, err := filepath.Abs("../../scripts/update-homebrew-tap.sh")
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("sh", append([]string{script}, args...)...)
	cmd.Env = append(os.Environ(), "HOMEBREW_TAP_DIR="+f.tap, "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+os.DevNull)
	return cmd.CombinedOutput()
}

func TestHomebrewFormulaGenerationDoesNotPublish(t *testing.T) {
	f := newTapFixture(t)
	if out, err := runTapScript(t, f, "2.0.0", f.dist); err != nil {
		t.Fatalf("generate: %v\n%s", err, out)
	}
	formula, err := os.ReadFile(filepath.Join(f.dist, "agy-swap.rb"))
	if err != nil || !strings.Contains(string(formula), "version \"2.0.0\"") {
		t.Fatalf("missing generated formula: %v", err)
	}
	if scriptGit(t, f.tap, "rev-parse", "HEAD") != f.head || scriptGit(t, f.remote, "rev-parse", "refs/heads/main") != f.head {
		t.Fatal("formula generation committed or pushed the tap")
	}
	if status := scriptGit(t, f.tap, "status", "--porcelain"); status != "" {
		t.Fatalf("formula generation changed tap: %s", status)
	}
}

func TestHomebrewExplicitPublication(t *testing.T) {
	f := newTapFixture(t)
	if out, err := runTapScript(t, f, "2.0.0", f.dist, "--publish"); err != nil {
		t.Fatalf("publish: %v\n%s", err, out)
	}
	head := scriptGit(t, f.tap, "rev-parse", "HEAD")
	if head == f.head || scriptGit(t, f.remote, "rev-parse", "refs/heads/main") != head {
		t.Fatal("explicit publication did not update remote")
	}
	if files := scriptGit(t, f.tap, "diff-tree", "--no-commit-id", "--name-only", "-r", "HEAD"); files != "Formula/agy-swap.rb" {
		t.Fatalf("publication included unexpected files: %s", files)
	}
	if out, err := runTapScript(t, f, "2.0.0", f.dist, "--publish"); err != nil {
		t.Fatalf("repeat publish: %v\n%s", err, out)
	}
	if scriptGit(t, f.tap, "rev-parse", "HEAD") != head {
		t.Fatal("repeat publication created an empty commit")
	}
}

func TestHomebrewPublicationRejectsDirtyTap(t *testing.T) {
	for _, staged := range []bool{false, true} {
		t.Run(map[bool]string{false: "unstaged", true: "staged"}[staged], func(t *testing.T) {
			f := newTapFixture(t)
			if err := os.WriteFile(filepath.Join(f.tap, "README"), []byte("owner work\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			if staged {
				scriptGit(t, f.tap, "add", "README")
			}
			before := scriptGit(t, f.tap, "status", "--porcelain")
			if out, err := runTapScript(t, f, "2.0.0", f.dist, "--publish"); err == nil {
				t.Fatalf("dirty tap accepted: %s", out)
			}
			if scriptGit(t, f.tap, "rev-parse", "HEAD") != f.head || scriptGit(t, f.remote, "rev-parse", "refs/heads/main") != f.head {
				t.Fatal("dirty tap was committed or pushed")
			}
			if scriptGit(t, f.tap, "status", "--porcelain") != before {
				t.Fatal("publication changed owner work")
			}
		})
	}
}

func TestHomebrewRejectsInvalidInputs(t *testing.T) {
	for _, name := range []string{"version", "option", "checksum", "duplicate"} {
		t.Run(name, func(t *testing.T) {
			f := newTapFixture(t)
			args := []string{"2.0.0", f.dist, "--publish"}
			switch name {
			case "version":
				args[0] = "2.0.0/invalid"
			case "option":
				args[2] = "--typo"
			case "checksum", "duplicate":
				path := filepath.Join(f.dist, "checksums.txt")
				data, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				if name == "checksum" {
					data = []byte(strings.ReplaceAll(string(data), strings.Repeat("a", 64), "bad"))
				} else {
					data = append(data, data...)
				}
				if err := os.WriteFile(path, data, 0o644); err != nil {
					t.Fatal(err)
				}
			}
			if out, err := runTapScript(t, f, args...); err == nil {
				t.Fatalf("invalid %s accepted: %s", name, out)
			}
			if scriptGit(t, f.tap, "rev-parse", "HEAD") != f.head || scriptGit(t, f.tap, "status", "--porcelain") != "" {
				t.Fatal("invalid input changed tap")
			}
		})
	}
}

func TestHomebrewPublicationRejectsUnpublishedUnrelatedHistory(t *testing.T) {
	f := newTapFixture(t)
	if err := os.WriteFile(filepath.Join(f.tap, "README"), []byte("unpublished owner commit\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	scriptGit(t, f.tap, "add", "README")
	scriptGit(t, f.tap, "commit", "-m", "owner work")
	head := scriptGit(t, f.tap, "rev-parse", "HEAD")
	if out, err := runTapScript(t, f, "2.0.0", f.dist, "--publish"); err == nil {
		t.Fatalf("unrelated unpublished history accepted: %s", out)
	}
	if scriptGit(t, f.tap, "rev-parse", "HEAD") != head || scriptGit(t, f.remote, "rev-parse", "refs/heads/main") != f.head {
		t.Fatal("publication modified or pushed owner history")
	}
	if scriptGit(t, f.tap, "status", "--porcelain") != "" {
		t.Fatal("publication wrote formula before rejecting unrelated history")
	}
}

func TestHomebrewPublicationRetriesFormulaOnlyPendingCommit(t *testing.T) {
	f := newTapFixture(t)
	if out, err := runTapScript(t, f, "2.0.0", f.dist); err != nil {
		t.Fatalf("generate: %v\n%s", err, out)
	}
	data, err := os.ReadFile(filepath.Join(f.dist, "agy-swap.rb"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(f.tap, "Formula"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(f.tap, "Formula", "agy-swap.rb"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	scriptGit(t, f.tap, "add", "Formula/agy-swap.rb")
	scriptGit(t, f.tap, "commit", "-m", "formula awaiting push")
	head := scriptGit(t, f.tap, "rev-parse", "HEAD")
	if out, err := runTapScript(t, f, "2.0.0", f.dist, "--publish"); err != nil {
		t.Fatalf("retry: %v\n%s", err, out)
	}
	if scriptGit(t, f.tap, "rev-parse", "HEAD") != head || scriptGit(t, f.remote, "rev-parse", "refs/heads/main") != head {
		t.Fatal("formula-only retry failed or created an unnecessary commit")
	}
}

func TestHomebrewPublicationChecksActualRemoteTip(t *testing.T) {
	f := newTapFixture(t)
	other := filepath.Join(f.root, "other")
	scriptGit(t, f.root, "clone", f.remote, other)
	scriptGit(t, other, "config", "user.name", "Remote owner")
	scriptGit(t, other, "config", "user.email", "remote@example.invalid")
	if err := os.WriteFile(filepath.Join(other, "README"), []byte("remote advanced\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	scriptGit(t, other, "add", "README")
	scriptGit(t, other, "commit", "-m", "remote work")
	scriptGit(t, other, "push", "origin", "main")
	remoteHead := scriptGit(t, f.remote, "rev-parse", "refs/heads/main")
	if scriptGit(t, f.tap, "rev-parse", "origin/main") != f.head {
		t.Fatal("fixture should have stale tracking metadata")
	}
	if out, err := runTapScript(t, f, "2.0.0", f.dist, "--publish"); err == nil {
		t.Fatalf("behind tap accepted: %s", out)
	}
	if scriptGit(t, f.tap, "rev-parse", "HEAD") != f.head || scriptGit(t, f.remote, "rev-parse", "refs/heads/main") != remoteHead || scriptGit(t, f.tap, "status", "--porcelain") != "" {
		t.Fatal("stale tracking data caused a local formula commit or remote mutation")
	}
}

func TestReleaseBuildDoesNotTouchTap(t *testing.T) {
	f := newTapFixture(t)
	bin := filepath.Join(f.root, "bin")
	if err := os.Mkdir(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	// Substitute only compilation/metadata checks; all release orchestration and
	// the tap helper are real scripts. No binary or external service is started.
	stub := `#!/bin/sh
case "$1:$2" in
env:GOHOSTOS) echo darwin;;
env:GOHOSTARCH) echo arm64;;
build:*)
  while [ "$#" -gt 0 ]; do
    if [ "$1" = -o ]; then shift; printf '#!/bin/sh\necho "agy-swap v2.0.0 (test)"\n' > "$1"; chmod +x "$1"; break; fi
    shift
  done;;
run:*) exit 0;;
*) exit 1;;
esac
`
	if err := os.WriteFile(filepath.Join(bin, "go"), []byte(stub), 0o755); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("sh", "./scripts/build-release.sh", "2.0.0", "test", f.dist)
	cmd.Dir = "../.."
	cmd.Env = append(os.Environ(), "PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"), "HOMEBREW_TAP_DIR="+f.tap, "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+os.DevNull)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	if scriptGit(t, f.tap, "rev-parse", "HEAD") != f.head || scriptGit(t, f.remote, "rev-parse", "refs/heads/main") != f.head || scriptGit(t, f.tap, "status", "--porcelain") != "" {
		t.Fatal("local release build changed or published tap")
	}
}
