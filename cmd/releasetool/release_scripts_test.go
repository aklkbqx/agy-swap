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

// Records every go invocation so the test can check release gate ordering.
const recordingGoStub = `#!/bin/sh
printf '%s\n' "$*" >> "$GO_STUB_LOG"
case "$1:$2" in
env:GOHOSTOS) echo darwin;;
env:GOHOSTARCH) echo arm64;;
build:*)
  while [ "$#" -gt 0 ]; do
    if [ "$1" = -o ]; then shift; printf '#!/bin/sh\necho "agy-swap v2.0.0 (test)"\n' > "$1"; chmod +x "$1"; break; fi
    shift
  done;;
run:./cmd/releasetool)
  if [ "$3" = "$GO_STUB_FAIL" ]; then exit 1; fi;;
run:*) exit 0;;
*) exit 1;;
esac
`

func runRecordedReleaseBuild(t *testing.T, fail string) (string, error) {
	t.Helper()
	f := newTapFixture(t)
	bin := filepath.Join(f.root, "bin")
	if err := os.Mkdir(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bin, "go"), []byte(recordingGoStub), 0o755); err != nil {
		t.Fatal(err)
	}
	log := filepath.Join(f.root, "go.log")
	cmd := exec.Command("sh", "./scripts/build-release.sh", "2.0.0", "test", f.dist)
	cmd.Dir = "../.."
	cmd.Env = append(os.Environ(), "PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"), "GO_STUB_LOG="+log, "GO_STUB_FAIL="+fail, "HOMEBREW_TAP_DIR="+f.tap, "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+os.DevNull)
	out, err := cmd.CombinedOutput()
	data, readErr := os.ReadFile(log)
	if readErr != nil {
		t.Fatalf("%v\n%s", readErr, out)
	}
	return string(data), err
}

func TestReleaseBuildGatesSourceBeforeCompilingAndBuildsAfter(t *testing.T) {
	log, err := runRecordedReleaseBuild(t, "")
	if err != nil {
		t.Fatalf("release build failed: %v\n%s", err, log)
	}
	source := strings.Index(log, "run ./cmd/releasetool verify-source 2.0.0 .")
	vuln := strings.Index(log, "run golang.org/x/vuln/cmd/govulncheck@")
	firstBuild := strings.Index(log, "\nbuild ")
	lastBuild := strings.LastIndex(log, "\nbuild ")
	builds := strings.Index(log, "run ./cmd/releasetool verify-builds 2.0.0 ")
	if source < 0 || vuln < 0 || firstBuild < 0 || builds < 0 {
		t.Fatalf("release gates missing:\n%s", log)
	}
	if source > firstBuild || vuln > firstBuild || builds < lastBuild {
		t.Fatalf("release gates out of order:\n%s", log)
	}
}

func TestReleaseBuildStopsWhenSourceIsNotReleasable(t *testing.T) {
	log, err := runRecordedReleaseBuild(t, "verify-source")
	if err == nil {
		t.Fatalf("release build continued after source verification failed:\n%s", log)
	}
	if strings.Contains(log, "\nbuild ") {
		t.Fatalf("assets compiled from unverified source:\n%s", log)
	}
}

const liveVerifyStubs = `#!/bin/sh
printf '%s %s\n' "$(basename "$0")" "$*" >> "$STUB_LOG"
case "$(basename "$0"):$1" in
git:-C)
  if [ "$3:$4" = "worktree:add" ]; then mkdir -p "$6"; fi;;
bun:run) mkdir -p dist/client;;
curl:*) printf '{"softwareVersion": "v%s"}' "$LIVE_VERSION";;
esac
exit 0
`

func runSiteVerifyLive(t *testing.T, liveVersion string) (string, error) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("POSIX release scripts require the macOS release builder")
	}
	root := t.TempDir()
	bin := filepath.Join(root, "bin")
	if err := os.Mkdir(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"git", "bun", "curl", "go"} {
		if err := os.WriteFile(filepath.Join(bin, name), []byte(liveVerifyStubs), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	log := filepath.Join(root, "calls.log")
	cmd := exec.Command("sh", "./scripts/site-verify-live.sh", "2.0.0")
	cmd.Dir = "../.."
	cmd.Env = append(os.Environ(), "PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"), "STUB_LOG="+log, "LIVE_VERSION="+liveVersion, "AGY_SITE_WAIT=0", "AGY_SITE_URL=https://site.example.invalid")
	out, err := cmd.CombinedOutput()
	calls, _ := os.ReadFile(log)
	return string(out) + "\n--- calls\n" + string(calls), err
}

func TestSiteVerifyLiveChecksTheTagBuildOncePublished(t *testing.T) {
	out, err := runSiteVerifyLive(t, "2.0.0")
	if err != nil {
		t.Fatalf("verification failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "worktree add --detach") || !strings.Contains(out, " v2.0.0") {
		t.Fatalf("site tag was not checked out:\n%s", out)
	}
	if !strings.Contains(out, "go run ./cmd/releasetool verify-live 2.0.0 ") || !strings.Contains(out, "/dist/client https://site.example.invalid") {
		t.Fatalf("live comparison not run against the tag build:\n%s", out)
	}
}

func TestSiteVerifyLiveStopsWhenProductionNeverUpdates(t *testing.T) {
	out, err := runSiteVerifyLive(t, "1.9.0")
	if err == nil {
		t.Fatalf("stale production accepted:\n%s", out)
	}
	if strings.Contains(out, "go run ./cmd/releasetool verify-live") {
		t.Fatalf("compared before production reported the version:\n%s", out)
	}
}

const statusStubs = `#!/bin/sh
case "$(basename "$0")" in
gh)
  case "$1 $2" in
  "release view") echo "v$STUB_RELEASE 2026-10-12T00:00:00Z";;
  "issue list") case "$*" in *agy-swap-site*) printf '%s' "$STUB_SITE_ISSUES";; *) printf '%s' "$STUB_TOOL_ISSUES";; esac;;
  "pr list") echo "$STUB_PRS";;
  esac;;
curl)
  case "$*" in *-w*) printf '%s' "$STUB_HEALTH";; *) printf '{"softwareVersion": "v%s"}' "$STUB_LIVE";; esac;;
agy-swap) echo "agy-swap v$STUB_INSTALLED (release-v$STUB_INSTALLED)";;
esac
`

type statusFixture struct {
	root string
	env  []string
}

// newStatusFixture builds paired Tool/Site repositories, in sync with local
// bare remotes, at version 2.0.0; gh, curl and agy-swap are stubs.
func newStatusFixture(t *testing.T) statusFixture {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("POSIX project scripts require the macOS release builder")
	}
	base := t.TempDir()
	root := filepath.Join(base, "tool")
	for _, dir := range []string{root, filepath.Join(root, "site"), filepath.Join(base, "bin")} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	writeTestFile(t, filepath.Join(root, "Makefile"), "VERSION ?= 2.0.0\n")
	writeTestFile(t, filepath.Join(root, ".gitignore"), "/site/\n")
	writeTestFile(t, filepath.Join(root, "site", "package.json"), "{\n  \"version\": \"2.0.0\"\n}\n")
	for _, repo := range []string{root, filepath.Join(root, "site")} {
		remote := filepath.Join(base, filepath.Base(repo)+"-remote.git")
		scriptGit(t, base, "init", "--bare", "--initial-branch=main", remote)
		scriptGit(t, repo, "init", "--initial-branch=main")
		scriptGit(t, repo, "config", "user.name", "Status test")
		scriptGit(t, repo, "config", "user.email", "status@example.invalid")
		scriptGit(t, repo, "add", ".")
		scriptGit(t, repo, "commit", "-m", "initial")
		scriptGit(t, repo, "tag", "-a", "v2.0.0", "-m", "v2.0.0")
		scriptGit(t, repo, "remote", "add", "origin", remote)
		scriptGit(t, repo, "push", "-q", "-u", "origin", "main", "--tags")
	}
	for _, name := range []string{"gh", "curl", "agy-swap"} {
		if err := os.WriteFile(filepath.Join(base, "bin", name), []byte(statusStubs), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	env := append(os.Environ(), "PATH="+filepath.Join(base, "bin")+string(os.PathListSeparator)+os.Getenv("PATH"),
		"AGY_STATUS_ROOT="+root, "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+os.DevNull,
		"STUB_RELEASE=2.0.0", "STUB_LIVE=2.0.0", "STUB_INSTALLED=2.0.0", "STUB_HEALTH=200", "STUB_PRS=0",
		"STUB_TOOL_ISSUES=#7 p2 ready [distribution] verify windows\n", "STUB_SITE_ISSUES=")
	return statusFixture{root: root, env: env}
}

func runStatus(t *testing.T, f statusFixture, extra ...string) string {
	t.Helper()
	cmd := exec.Command("sh", "./scripts/project-status.sh")
	cmd.Dir = "../.."
	cmd.Env = append(append([]string{}, f.env...), extra...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("status failed: %v\n%s", err, out)
	}
	return string(out)
}

func TestProjectStatusReportsInSyncProject(t *testing.T) {
	out := runStatus(t, newStatusFixture(t))
	for _, want := range []string{"Drift: none", "v2.0.0", "#7 p2 ready [distribution] verify windows"} {
		if !strings.Contains(out, want) {
			t.Fatalf("status missing %q:\n%s", want, out)
		}
	}
}

func TestProjectStatusFlagsDrift(t *testing.T) {
	f := newStatusFixture(t)
	writeTestFile(t, filepath.Join(f.root, "notes.txt"), "draft")
	scriptGit(t, filepath.Join(f.root, "site"), "commit", "--allow-empty", "-m", "unpushed")
	out := runStatus(t, f, "STUB_LIVE=1.9.0", "STUB_INSTALLED=1.9.0", "STUB_PRS=2", "STUB_HEALTH=503")
	for _, want := range []string{"tool: 1 uncommitted", "site: 1 unpushed", "live site v1.9.0", "installed v1.9.0", "2 open PRs", "health 503"} {
		if !strings.Contains(out, want) {
			t.Fatalf("drift %q not reported:\n%s", want, out)
		}
	}
	if strings.Contains(out, "Drift: none") {
		t.Fatalf("drift hidden:\n%s", out)
	}
}
