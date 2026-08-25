package main

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
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
		return errors.New("usage: releasetool <checksums|verify-version|verify-assets> ...")
	}
	switch args[0] {
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
