package main

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "releasetool:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return errors.New("usage: releasetool <checksums|verify-version|verify-assets|verify-metadata|bump> ...")
	}
	switch args[0] {
	case "bump":
		if len(args) < 2 || len(args) > 3 {
			return errors.New("usage: releasetool bump <patch|minor|major|VERSION> [ROOT]")
		}
		root := "."
		if len(args) == 3 {
			root = args[2]
		}
		return bumpVersion(args[1], root)
	case "checksums":
		if len(args) != 2 {
			return errors.New("usage: releasetool checksums DIST_DIR")
		}
		return writeChecksums(args[1], filepath.Join(args[1], "checksums.txt"))
	case "verify-version":
		if len(args) != 3 {
			return errors.New("usage: releasetool verify-version VERSION INSTALLER")
		}
		return verifyVersion(args[1], args[2])
	case "verify-metadata":
		if len(args) != 3 {
			return errors.New("usage: releasetool verify-metadata VERSION ROOT")
		}
		return verifyMetadata(args[1], args[2])
	case "verify-assets":
		if len(args) != 3 {
			return errors.New("usage: releasetool verify-assets VERSION DIST_DIR")
		}
		return verifyAssets(args[1], args[2])
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func verifyAssets(version, dir string) error {
	checksums, err := readChecksums(filepath.Join(dir, "checksums.txt"))
	if err != nil {
		return err
	}
	for _, target := range []string{
		"darwin_amd64", "darwin_arm64", "linux_amd64", "linux_arm64",
		"windows_amd64.exe", "windows_arm64.exe",
	} {
		name := "agy-swap_v" + version + "_" + target
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			return fmt.Errorf("release asset missing: %s", name)
		}
		if _, ok := checksums[name]; !ok {
			return fmt.Errorf("release checksum missing: %s", name)
		}
		file, err := os.Open(filepath.Join(dir, name))
		if err != nil {
			return err
		}
		digest := sha256.New()
		_, copyErr := io.Copy(digest, file)
		closeErr := file.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
		if !strings.EqualFold(hex.EncodeToString(digest.Sum(nil)), checksums[name]) {
			return fmt.Errorf("release checksum mismatch: %s", name)
		}
	}
	return nil
}

func writeChecksums(dir, output string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || entry.Name() == filepath.Base(output) || !strings.HasPrefix(entry.Name(), "agy-swap_v") {
			continue
		}
		names = append(names, entry.Name())
	}
	sort.Strings(names)
	if len(names) == 0 {
		return errors.New("no release assets found")
	}
	var result strings.Builder
	for _, name := range names {
		file, openErr := os.Open(filepath.Join(dir, name))
		if openErr != nil {
			return openErr
		}
		digest := sha256.New()
		_, copyErr := io.Copy(digest, file)
		closeErr := file.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
		fmt.Fprintf(&result, "%s  %s\n", hex.EncodeToString(digest.Sum(nil)), name)
	}
	return os.WriteFile(output, []byte(result.String()), 0o644)
}

func readChecksums(path string) (map[string]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	checksums := map[string]string{}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) != 2 || len(fields[0]) != sha256.Size*2 {
			return nil, fmt.Errorf("invalid checksum line %q", scanner.Text())
		}
		checksums[strings.TrimPrefix(fields[1], "*")] = strings.ToLower(fields[0])
	}
	return checksums, scanner.Err()
}

func verifyVersion(version, installerPath string) error {
	installer, err := os.ReadFile(installerPath)
	if err != nil {
		return err
	}
	version = strings.TrimPrefix(strings.TrimSpace(version), "v")
	text := string(installer)
	patterns := []string{
		`VERSION="` + version + `"`,
		`VERSION="${AGY_SWAP_VERSION:-` + version + `}"`,
		`$Version = '` + version + `'`,
		`$Version = "` + version + `"`,
	}
	for _, pattern := range patterns {
		if strings.Contains(text, pattern) {
			return nil
		}
	}
	return fmt.Errorf("%s does not contain the expected installer version %s", installerPath, version)
}

func verifyMetadata(version, root string) error {
	version = strings.TrimPrefix(version, "v")
	for _, name := range []string{"install.sh", "install.ps1"} {
		if err := verifyVersion(version, filepath.Join(root, name)); err != nil {
			return err
		}
	}
	for name, expected := range map[string]string{
		"Makefile":             "VERSION ?= " + version + "\n",
		"cmd/agy-swap/main.go": "version = \"" + version + "\"",
		"site/index.html":      "\"softwareVersion\": \"v" + version + "\"",
		"CHANGELOG.md":         "## " + version + "\n",
	} {
		data, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			return err
		}
		if !strings.Contains(string(data), expected) {
			return fmt.Errorf("version drift in %s: expected %s", name, version)
		}
	}
	names := []string{"site/package.json", "site/package-lock.json", "site/src/generated/tui-initial-fixtures.json"}
	shards, err := filepath.Glob(filepath.Join(root, "site/src/generated/shards/*.json"))
	if err != nil {
		return err
	}
	for _, name := range names {
		shards = append(shards, filepath.Join(root, name))
	}
	for _, name := range shards {
		data, err := os.ReadFile(name)
		if err != nil {
			return err
		}
		var document struct {
			Version  string `json:"version"`
			Packages map[string]struct {
				Version string `json:"version"`
			} `json:"packages"`
		}
		if err := json.Unmarshal(data, &document); err != nil {
			return err
		}
		if document.Version != version {
			return fmt.Errorf("version drift in %s", name)
		}
		if pkg, ok := document.Packages[""]; ok && pkg.Version != version {
			return fmt.Errorf("root package version drift in %s", name)
		}
	}
	return nil
}

func readCurrentVersion(root string) (string, error) {
	data, err := os.ReadFile(filepath.Join(root, "Makefile"))
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "VERSION ?= ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "VERSION ?= ")), nil
		}
	}
	return "", errors.New("cannot find VERSION in Makefile")
}

func parseSemVer(v string) (int, int, int, error) {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	parts := strings.Split(v, ".")
	if len(parts) != 3 {
		return 0, 0, 0, fmt.Errorf("invalid semver %q", v)
	}
	var major, minor, patch int
	if _, err := fmt.Sscanf(parts[0], "%d", &major); err != nil {
		return 0, 0, 0, err
	}
	if _, err := fmt.Sscanf(parts[1], "%d", &minor); err != nil {
		return 0, 0, 0, err
	}
	if _, err := fmt.Sscanf(parts[2], "%d", &patch); err != nil {
		return 0, 0, 0, err
	}
	return major, minor, patch, nil
}

func calculateNextVersion(current, target string) (string, error) {
	target = strings.TrimSpace(target)
	major, minor, patch, err := parseSemVer(current)
	if err != nil {
		return "", fmt.Errorf("current version %q is invalid: %w", current, err)
	}
	switch strings.ToLower(target) {
	case "patch":
		return fmt.Sprintf("%d.%d.%d", major, minor, patch+1), nil
	case "minor":
		return fmt.Sprintf("%d.%d.%d", major, minor+1, 0), nil
	case "major":
		return fmt.Sprintf("%d.%d.%d", major+1, 0, 0), nil
	default:
		target = strings.TrimPrefix(target, "v")
		if _, _, _, err := parseSemVer(target); err != nil {
			return "", fmt.Errorf("invalid target version %q: must be patch, minor, major, or X.Y.Z", target)
		}
		return target, nil
	}
}

func replaceInFile(path, oldText, newText string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	content := string(data)
	if !strings.Contains(content, oldText) {
		if strings.Contains(content, newText) {
			return nil
		}
		return fmt.Errorf("text %q not found in %s", oldText, path)
	}
	replaced := strings.ReplaceAll(content, oldText, newText)
	return os.WriteFile(path, []byte(replaced), 0o644)
}

func replacePackageLockVersion(path, oldVer, newVer string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	lines := strings.Split(string(data), "\n")
	limit := 20
	if len(lines) < limit {
		limit = len(lines)
	}
	header := strings.Join(lines[:limit], "\n")
	rest := strings.Join(lines[limit:], "\n")
	header = strings.ReplaceAll(header, `"version": "`+oldVer+`"`, `"version": "`+newVer+`"`)
	return os.WriteFile(path, []byte(header+"\n"+rest), 0o644)
}

func updateChangelog(path, oldVer, newVer string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	content := string(data)
	if strings.Contains(content, "## "+newVer) {
		return nil
	}
	target := "## " + oldVer
	if !strings.Contains(content, target) {
		target = "\n## "
		if !strings.Contains(content, target) {
			return fmt.Errorf("cannot find insertion point in %s", path)
		}
	}
	entry := fmt.Sprintf("## %s\n\n", newVer)
	idx := strings.Index(content, target)
	content = content[:idx] + entry + content[idx:]
	return os.WriteFile(path, []byte(content), 0o644)
}

func bumpVersion(target, root string) error {
	current, err := readCurrentVersion(root)
	if err != nil {
		return err
	}
	next, err := calculateNextVersion(current, target)
	if err != nil {
		return err
	}
	if current == next {
		return fmt.Errorf("target version %s matches current version", next)
	}

	fmt.Printf("Bumping version %s -> %s across repository...\n", current, next)

	replacements := []struct {
		file, old, new string
	}{
		{"Makefile", "VERSION ?= " + current, "VERSION ?= " + next},
		{"cmd/agy-swap/main.go", `version = "` + current + `"`, `version = "` + next + `"`},
		{"install.sh", `VERSION="${AGY_SWAP_VERSION:-` + current + `}"`, `VERSION="${AGY_SWAP_VERSION:-` + next + `}"`},
		{"install.ps1", `$Version = '` + current + `'`, `$Version = '` + next + `'`},
		{"README.md", `-X main.version=` + current, `-X main.version=` + next},
		{"site/index.html", `"softwareVersion": "v` + current + `"`, `"softwareVersion": "v` + next + `"`},
		{"site/package.json", `"version": "` + current + `"`, `"version": "` + next + `"`},
		{"site/src/components/tuiState.js", `release v` + current + `.`, `release v` + next + `.`},
		{"site/tests/app.test.mjs", `assert.equal(fullData.version, "` + current + `");`, `assert.equal(fullData.version, "` + next + `");`},
		{"internal/app/app_test.go", `"` + current + `": "v` + current + `"`, `"` + next + `": "v` + next + `"`},
		{"internal/app/app_test.go", `Version: "` + current + `"`, `Version: "` + next + `"`},
		{"internal/app/tui_web_fixtures_test.go", `Version:     "` + current + `"`, `Version:     "` + next + `"`},
		{"internal/app/tui_web_fixtures_test.go", `Version:           "` + current + `"`, `Version:           "` + next + `"`},
		{"internal/app/tui_web_fixtures_test.go", `outFirst.Version != "` + current + `"`, `outFirst.Version != "` + next + `"`},
		{"internal/app/tui_web_fixtures_test.go", `want ` + current + `"`, `want ` + next + `"`},
	}

	for _, r := range replacements {
		filePath := filepath.Join(root, r.file)
		if err := replaceInFile(filePath, r.old, r.new); err != nil {
			return fmt.Errorf("failed replacing in %s: %w", r.file, err)
		}
	}

	if err := replacePackageLockVersion(filepath.Join(root, "site", "package-lock.json"), current, next); err != nil {
		return fmt.Errorf("failed updating site/package-lock.json: %w", err)
	}

	if err := updateChangelog(filepath.Join(root, "CHANGELOG.md"), current, next); err != nil {
		return fmt.Errorf("failed updating CHANGELOG.md: %w", err)
	}

	// Regenerate web fixtures and shards if generator test is present
	fixturesTest := filepath.Join(root, "internal", "app", "tui_web_fixtures_test.go")
	if _, err := os.Stat(fixturesTest); err == nil {
		fmt.Println("Regenerating web fixtures and shards (UPDATE_TUI_WEB_FIXTURES=1)...")
		cmd := exec.Command("go", "test", "-run", "TestTUIWebFixtures", "./internal/app")
		cmd.Dir = root
		cmd.Env = append(os.Environ(), "UPDATE_TUI_WEB_FIXTURES=1")
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("failed to regenerate TUI fixtures: %w\n%s", err, string(out))
		}
	}

	if err := verifyMetadata(next, root); err != nil {
		return fmt.Errorf("metadata verification failed after bump: %w", err)
	}

	fmt.Printf("✓ Version successfully bumped from %s to %s across all surfaces.\n", current, next)
	return nil
}
