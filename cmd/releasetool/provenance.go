package main

import (
	"bufio"
	"debug/buildinfo"
	"errors"
	"fmt"
	"go/version"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"strings"
)

var releaseTargets = []string{
	"darwin_amd64", "darwin_arm64", "linux_amd64", "linux_arm64",
	"windows_amd64.exe", "windows_arm64.exe",
}

// readModuleFloor returns the module path and the minimum release toolchain
// declared by go.mod's toolchain line.
func readModuleFloor(root string) (string, string, error) {
	file, err := os.Open(filepath.Join(root, "go.mod"))
	if err != nil {
		return "", "", err
	}
	defer func() { _ = file.Close() }()
	var module, toolchain string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) == 2 && fields[0] == "module" {
			module = fields[1]
		}
		if len(fields) == 2 && fields[0] == "toolchain" {
			toolchain = fields[1]
		}
	}
	if err := scanner.Err(); err != nil {
		return "", "", err
	}
	if module == "" {
		return "", "", errors.New("go.mod declares no module path")
	}
	if !version.IsValid(toolchain) {
		return "", "", errors.New("go.mod must declare a release toolchain floor (toolchain goX.Y.Z)")
	}
	return module, toolchain, nil
}

// goRelease drops build suffixes such as " X:boringcrypto" from a Go version.
func goRelease(goVersion string) string {
	fields := strings.Fields(goVersion)
	if len(fields) == 0 {
		return ""
	}
	return fields[0]
}

func checkToolchain(goVersion, floor string) error {
	if version.Compare(goRelease(goVersion), floor) < 0 {
		return fmt.Errorf("go toolchain %s is older than the release floor %s", goVersion, floor)
	}
	return nil
}

func gitOutput(root string, args ...string) (string, error) {
	out, err := exec.Command("git", append([]string{"-C", root}, args...)...).Output()
	if err != nil {
		return "", fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return strings.TrimSpace(string(out)), nil
}

// verifySource refuses to build release assets unless HEAD is the clean,
// annotated release tag and the compiler meets the go.mod toolchain floor.
func verifySource(releaseVersion, root, goVersion string) error {
	tag := "v" + strings.TrimPrefix(releaseVersion, "v")
	_, floor, err := readModuleFloor(root)
	if err != nil {
		return err
	}
	if err = checkToolchain(goVersion, floor); err != nil {
		return err
	}
	status, err := gitOutput(root, "status", "--porcelain")
	if err != nil {
		return err
	}
	if status != "" {
		return fmt.Errorf("release source has uncommitted or untracked changes:\n%s", status)
	}
	kind, err := gitOutput(root, "cat-file", "-t", "refs/tags/"+tag)
	if err != nil {
		return fmt.Errorf("release tag %s not found", tag)
	}
	if kind != "tag" {
		return fmt.Errorf("release tag %s must be annotated", tag)
	}
	tagged, err := gitOutput(root, "rev-parse", "refs/tags/"+tag+"^{commit}")
	if err != nil {
		return err
	}
	head, err := gitOutput(root, "rev-parse", "HEAD")
	if err != nil {
		return err
	}
	if tagged != head {
		return fmt.Errorf("HEAD %s is not release tag %s (%s)", head, tag, tagged)
	}
	return nil
}

// checkBuildInfo accepts only a binary built from the clean release tag of the
// expected module with a toolchain at or above the floor.
func checkBuildInfo(info *debug.BuildInfo, module, releaseVersion, floor string) error {
	tag := "v" + strings.TrimPrefix(releaseVersion, "v")
	if info.Main.Path != module {
		return fmt.Errorf("main module %q, expected %q", info.Main.Path, module)
	}
	if info.Main.Version != tag {
		return fmt.Errorf("main module version %q, expected clean tag %s", info.Main.Version, tag)
	}
	return checkToolchain(info.GoVersion, floor)
}

func verifyBuilds(releaseVersion, dir, module, floor string) error {
	for _, target := range releaseTargets {
		name := "agy-swap_v" + strings.TrimPrefix(releaseVersion, "v") + "_" + target
		info, err := buildinfo.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		if err := checkBuildInfo(info, module, releaseVersion, floor); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
	}
	return nil
}
