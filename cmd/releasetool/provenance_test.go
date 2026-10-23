package main

import (
	"go/version"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"strings"
	"testing"
)

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestReadModuleFloor(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, filepath.Join(dir, "go.mod"), "module example.com/tool/v2\n\ngo 1.26.0\n\ntoolchain go1.27.2\n")
	module, toolchain, err := readModuleFloor(dir)
	if err != nil || module != "example.com/tool/v2" || toolchain != "go1.27.2" {
		t.Fatalf("readModuleFloor = %q, %q, %v", module, toolchain, err)
	}
	writeTestFile(t, filepath.Join(dir, "go.mod"), "module example.com/tool/v2\n")
	if _, _, err := readModuleFloor(dir); err == nil {
		t.Fatal("go.mod without any Go version floor was accepted")
	}
}

// go install applies only the go directive (not the toolchain line), so the
// directive itself must name the patched compiler.
func TestRepositoryGoDirectiveEnforcesTheSecurityFloor(t *testing.T) {
	data, err := os.ReadFile("../../go.mod")
	if err != nil {
		t.Fatal(err)
	}
	directive := ""
	for _, line := range strings.Split(string(data), "\n") {
		if fields := strings.Fields(line); len(fields) == 2 && fields[0] == "go" {
			directive = "go" + fields[1]
		}
	}
	if version.Compare(directive, "go1.27.3") < 0 {
		t.Fatalf("go directive %q lets go install build with a vulnerable toolchain", directive)
	}
}

func TestReadModuleFloorFallsBackToTheGoDirective(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, filepath.Join(dir, "go.mod"), "module example.com/tool/v2\n\ngo 1.27.2\n")
	module, floor, err := readModuleFloor(dir)
	if err != nil || module != "example.com/tool/v2" || floor != "go1.27.2" {
		t.Fatalf("readModuleFloor = %q, %q, %v", module, floor, err)
	}
	writeTestFile(t, filepath.Join(dir, "go.mod"), "module example.com/tool/v2\n\ngo 1.27.2\n\ntoolchain go1.27.5\n")
	if _, floor, _ := readModuleFloor(dir); floor != "go1.27.5" {
		t.Fatalf("newer toolchain line ignored: %q", floor)
	}
}

// The released module must be installable as v2 and require a patched compiler.
func TestRepositoryReleaseFloor(t *testing.T) {
	module, toolchain, err := readModuleFloor("../..")
	if err != nil {
		t.Fatal(err)
	}
	if module != "github.com/aklkbqx/agy-swap/v2" {
		t.Fatalf("module path %q cannot resolve v2 tags with go install", module)
	}
	if version.Compare(toolchain, "go1.27.3") < 0 {
		t.Fatalf("toolchain floor %s predates the Go 1.27.3 security fixes", toolchain)
	}
}

func newSourceRepo(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	scriptGit(t, repo, "init", "--initial-branch=main")
	scriptGit(t, repo, "config", "user.name", "Release test")
	scriptGit(t, repo, "config", "user.email", "release@example.invalid")
	writeTestFile(t, filepath.Join(repo, ".gitignore"), "/dist/\n")
	writeTestFile(t, filepath.Join(repo, "go.mod"), "module example.com/tool/v2\n\ngo 1.26.0\n\ntoolchain go1.27.2\n")
	writeTestFile(t, filepath.Join(repo, "main.go"), "package main\n\nfunc main() {}\n")
	scriptGit(t, repo, "add", ".")
	scriptGit(t, repo, "commit", "-m", "initial")
	scriptGit(t, repo, "tag", "-a", "v2.0.0", "-m", "v2.0.0")
	return repo
}

func TestVerifySource(t *testing.T) {
	repo := newSourceRepo(t)
	if err := os.MkdirAll(filepath.Join(repo, "dist", "release"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(repo, "dist", "release", "old-asset"), "ignored output")
	if err := verifySource("2.0.0", repo, "go1.27.2"); err != nil {
		t.Fatalf("clean annotated release source rejected: %v", err)
	}
	if err := verifySource("v2.0.0", repo, "go1.28.0 X:boringcrypto"); err != nil {
		t.Fatalf("newer toolchain rejected: %v", err)
	}
	if err := verifySource("2.0.0", repo, "go1.27.0"); err == nil {
		t.Fatal("toolchain below the go.mod floor was accepted")
	}
	if err := verifySource("2.0.1", repo, "go1.27.2"); err == nil {
		t.Fatal("missing release tag was accepted")
	}

	untracked := newSourceRepo(t)
	writeTestFile(t, filepath.Join(untracked, "notes.txt"), "draft")
	if err := verifySource("2.0.0", untracked, "go1.27.2"); err == nil {
		t.Fatal("untracked source file was accepted")
	}

	modified := newSourceRepo(t)
	writeTestFile(t, filepath.Join(modified, "main.go"), "package main\n\nfunc main() { println() }\n")
	if err := verifySource("2.0.0", modified, "go1.27.2"); err == nil {
		t.Fatal("modified tracked file was accepted")
	}

	ahead := newSourceRepo(t)
	writeTestFile(t, filepath.Join(ahead, "main.go"), "package main\n\nfunc main() { println() }\n")
	scriptGit(t, ahead, "commit", "-am", "after tag")
	if err := verifySource("2.0.0", ahead, "go1.27.2"); err == nil {
		t.Fatal("HEAD after the release tag was accepted")
	}

	lightweight := newSourceRepo(t)
	scriptGit(t, lightweight, "tag", "-d", "v2.0.0")
	scriptGit(t, lightweight, "tag", "v2.0.0")
	if err := verifySource("2.0.0", lightweight, "go1.27.2"); err == nil {
		t.Fatal("lightweight release tag was accepted")
	}
}

func TestCheckBuildInfo(t *testing.T) {
	stamped := func(goVersion, path, mainVersion string) *debug.BuildInfo {
		return &debug.BuildInfo{GoVersion: goVersion, Main: debug.Module{Path: path, Version: mainVersion}}
	}
	module := "github.com/aklkbqx/agy-swap/v2"
	if err := checkBuildInfo(stamped("go1.27.2", module, "v2.11.5"), module, "2.11.5", "go1.27.2"); err != nil {
		t.Fatalf("release build rejected: %v", err)
	}
	for name, info := range map[string]*debug.BuildInfo{
		"dirty source":      stamped("go1.27.2", module, "v2.11.5+dirty"),
		"pseudo-version":    stamped("go1.27.2", module, "v2.11.6-0.20261012000000-0123456789ab"),
		"unstamped":         stamped("go1.27.2", module, "(devel)"),
		"other tag":         stamped("go1.27.2", module, "v2.11.4"),
		"unsuffixed module": stamped("go1.27.2", "github.com/aklkbqx/agy-swap", "v2.11.5"),
		"vulnerable Go":     stamped("go1.27.0", module, "v2.11.5"),
	} {
		if err := checkBuildInfo(info, module, "2.11.5", "go1.27.2"); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
}

// Builds a real binary at a tag so the check exercises Go's own VCS stamping.
func TestVerifyBuildsReadsGoStamping(t *testing.T) {
	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Skip("go command unavailable")
	}
	repo := t.TempDir()
	scriptGit(t, repo, "init", "--initial-branch=main")
	scriptGit(t, repo, "config", "user.name", "Release test")
	scriptGit(t, repo, "config", "user.email", "release@example.invalid")
	writeTestFile(t, filepath.Join(repo, "go.mod"), "module example.com/stamp/v2\n\ngo 1.26.0\n")
	writeTestFile(t, filepath.Join(repo, "main.go"), "package main\n\nfunc main() {}\n")
	scriptGit(t, repo, "add", ".")
	scriptGit(t, repo, "commit", "-m", "initial")
	scriptGit(t, repo, "tag", "-a", "v2.0.0", "-m", "v2.0.0")

	env := append(os.Environ(), "GOFLAGS=", "GOWORK=off", "GOTOOLCHAIN=local", "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+os.DevNull)
	goCommand := func(args ...string) string {
		t.Helper()
		cmd := exec.Command(goBin, args...)
		cmd.Dir = repo
		cmd.Env = env
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("go %v: %v\n%s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	goVersion := goCommand("env", "GOVERSION")
	buildDist := func() string {
		t.Helper()
		dist := t.TempDir()
		binary := filepath.Join(dist, "built")
		goCommand("build", "-trimpath", "-o", binary, ".")
		data, err := os.ReadFile(binary)
		if err != nil {
			t.Fatal(err)
		}
		for _, target := range releaseTargets {
			writeTestFile(t, filepath.Join(dist, "agy-swap_v2.0.0_"+target), string(data))
		}
		return dist
	}

	if err := verifyBuilds("2.0.0", buildDist(), "example.com/stamp/v2", goVersion); err != nil {
		t.Fatalf("binary built at the annotated tag rejected: %v", err)
	}
	writeTestFile(t, filepath.Join(repo, "notes.txt"), "draft")
	if err := verifyBuilds("2.0.0", buildDist(), "example.com/stamp/v2", goVersion); err == nil {
		t.Fatal("binary built from a dirty tree accepted")
	}
}
