package app

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const (
	defaultAgyUpdaterBaseURL = "https://antigravity-cli-auto-updater-974169037036.us-central1.run.app"
	maxAgyDownloadBytes      = 512 * 1024 * 1024
)

type agyReleaseManifest struct {
	Version string `json:"version"`
	URL     string `json:"url"`
	SHA512  string `json:"sha512"`
}

func localAppDataDir(home string) string {
	if v := strings.TrimSpace(os.Getenv("LOCALAPPDATA")); v != "" {
		return v
	}
	if strings.TrimSpace(home) != "" {
		return filepath.Join(home, "AppData", "Local")
	}
	return ""
}

func defaultAgyInstallDir(home string) string {
	if runtime.GOOS == "windows" {
		if base := localAppDataDir(home); base != "" {
			return filepath.Join(base, "agy", "bin")
		}
	}
	if strings.TrimSpace(home) == "" {
		if h, err := os.UserHomeDir(); err == nil {
			home = h
		}
	}
	return filepath.Join(home, ".local", "bin")
}

func agyBinaryName() string {
	if runtime.GOOS == "windows" {
		return "agy.exe"
	}
	return "agy"
}

func isExecutableFile(path string) bool {
	path = strings.TrimSpace(path)
	if path == "" {
		return false
	}
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return false
	}
	if runtime.GOOS == "windows" {
		return info.Size() > 0
	}
	return info.Mode()&0o111 != 0
}

func (a *Application) candidateAgyPaths() []string {
	var home string
	if a != nil {
		home = strings.TrimSpace(a.paths.Home)
	}
	if home == "" {
		if h, err := os.UserHomeDir(); err == nil {
			home = h
		}
	}
	exe := agyBinaryName()
	var candidates []string
	if runtime.GOOS == "windows" {
		if base := localAppDataDir(home); base != "" {
			candidates = append(candidates,
				filepath.Join(base, "agy", "bin", exe),
				filepath.Join(base, "Programs", "agy", exe),
			)
		}
		if home != "" {
			candidates = append(candidates,
				filepath.Join(home, ".local", "bin", exe),
				filepath.Join(home, "bin", exe),
			)
		}
		return candidates
	}
	if home != "" {
		candidates = append(candidates,
			filepath.Join(home, ".local", "bin", exe),
			filepath.Join(home, ".local", "libexec", exe),
			filepath.Join(home, "bin", exe),
		)
	}
	candidates = append(candidates,
		"/opt/homebrew/bin/"+exe,
		"/usr/local/bin/"+exe,
	)
	return candidates
}

// resolveAgyExecutable locates the Antigravity CLI ('agy') across environment
// overrides, saved target settings, PATH, and standard per-OS install locations.
func (a *Application) resolveAgyExecutable() string {
	if envBin := strings.TrimSpace(os.Getenv("AGY_SWAP_AGY_BIN")); envBin != "" {
		if isExecutableFile(envBin) {
			return envBin
		}
		if found, err := exec.LookPath(envBin); err == nil && isExecutableFile(found) {
			return found
		}
	}
	if a != nil && a.store != nil {
		if settings, err := a.loadSettings(); err == nil {
			if target, ok := settings.Targets["agy"]; ok && target.Enabled {
				cmd := strings.TrimSpace(target.Command)
				if cmd != "" && cmd != "agy" && cmd != "agy.exe" {
					if isExecutableFile(cmd) {
						return cmd
					}
					if found, lookErr := exec.LookPath(cmd); lookErr == nil && isExecutableFile(found) {
						return found
					}
				}
			}
		}
	}
	if found, err := exec.LookPath(agyBinaryName()); err == nil && isExecutableFile(found) {
		return found
	}
	if runtime.GOOS == "windows" {
		if found, err := exec.LookPath("agy"); err == nil && isExecutableFile(found) {
			return found
		}
	}
	for _, candidate := range a.candidateAgyPaths() {
		if isExecutableFile(candidate) {
			return candidate
		}
	}
	return ""
}

func isMuslLinux() bool {
	if runtime.GOOS != "linux" {
		return false
	}
	for _, marker := range []string{"/lib/libc.musl-x86_64.so.1", "/lib/libc.musl-aarch64.so.1"} {
		if _, err := os.Stat(marker); err == nil {
			return true
		}
	}
	return false
}

func agyPlatformKey(goos, goarch string) (string, error) {
	var arch string
	switch goarch {
	case "amd64":
		arch = "amd64"
	case "arm64":
		arch = "arm64"
	default:
		return "", fmt.Errorf("unsupported architecture for Antigravity CLI: %s", goarch)
	}
	switch goos {
	case "darwin":
		return "darwin_" + arch, nil
	case "windows":
		return "windows_" + arch, nil
	case "android":
		return "android_" + arch, nil
	case "linux":
		if isMuslLinux() {
			return "linux_" + arch + "_musl", nil
		}
		return "linux_" + arch, nil
	default:
		return "", fmt.Errorf("unsupported operating system for Antigravity CLI: %s", goos)
	}
}

func isHexSHA512(value string) bool {
	if len(value) != 128 {
		return false
	}
	for i := 0; i < len(value); i++ {
		ch := value[i]
		if (ch < '0' || ch > '9') && (ch < 'a' || ch > 'f') && (ch < 'A' || ch > 'F') {
			return false
		}
	}
	return true
}

func extractAgyTarGz(archivePath, destPath string) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("invalid gzip archive: %w", err)
	}
	defer func() { _ = gz.Close() }()

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("invalid tar archive: %w", err)
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		base := filepath.Base(hdr.Name)
		if base != "antigravity" && base != "agy" && base != "agy.exe" {
			continue
		}
		out, err := os.OpenFile(destPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
		if err != nil {
			return err
		}
		written, copyErr := io.Copy(out, io.LimitReader(tr, maxAgyDownloadBytes+1))
		closeErr := out.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
		if written > maxAgyDownloadBytes {
			return errors.New("extracted binary exceeds size limit")
		}
		return nil
	}
	return errors.New("archive did not contain an antigravity binary")
}

func (a *Application) fetchAgyManifest(ctx context.Context, baseURL, platform string) (agyReleaseManifest, error) {
	manifestURL := strings.TrimRight(baseURL, "/") + "/manifests/" + platform + ".json"
	reqCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, manifestURL, nil)
	if err != nil {
		return agyReleaseManifest{}, err
	}
	resp, err := a.http.do(req)
	if err != nil {
		return agyReleaseManifest{}, fmt.Errorf("could not fetch Antigravity CLI manifest: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return agyReleaseManifest{}, fmt.Errorf("manifest request returned HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if err != nil {
		return agyReleaseManifest{}, err
	}
	var manifest agyReleaseManifest
	if err := json.Unmarshal(body, &manifest); err != nil {
		return agyReleaseManifest{}, fmt.Errorf("invalid manifest JSON: %w", err)
	}
	manifest.Version = strings.TrimSpace(manifest.Version)
	manifest.URL = strings.TrimSpace(manifest.URL)
	manifest.SHA512 = strings.ToLower(strings.TrimSpace(manifest.SHA512))
	if manifest.URL == "" || !isHexSHA512(manifest.SHA512) {
		return agyReleaseManifest{}, errors.New("manifest is missing a valid download URL or SHA-512 checksum")
	}
	return manifest, nil
}

func (a *Application) installAgyCLI(ctx context.Context, customDir string, force bool) (map[string]any, error) {
	platform, err := agyPlatformKey(runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return nil, err
	}
	baseURL := strings.TrimSpace(os.Getenv("AGY_SWAP_AGY_INSTALL_URL"))
	if baseURL == "" {
		baseURL = defaultAgyUpdaterBaseURL
	}

	targetDir := strings.TrimSpace(customDir)
	if targetDir == "" {
		targetDir = strings.TrimSpace(os.Getenv("AGY_SWAP_AGY_DIR"))
	}
	isCustomDir := targetDir != ""
	if targetDir == "" {
		targetDir = defaultAgyInstallDir(a.paths.Home)
	}
	if absDir, absErr := filepath.Abs(targetDir); absErr == nil {
		targetDir = absDir
	}
	binaryPath := filepath.Join(targetDir, agyBinaryName())

	manifest, err := a.fetchAgyManifest(ctx, baseURL, platform)
	if err != nil {
		return nil, err
	}

	if mkdirErr := os.MkdirAll(targetDir, 0o755); mkdirErr != nil {
		return nil, fmt.Errorf("could not create target directory %s: %w", targetDir, mkdirErr)
	}
	stagingDir, err := os.MkdirTemp(targetDir, ".agy-install-*")
	if err != nil {
		return nil, fmt.Errorf("could not create staging directory in %s: %w", targetDir, err)
	}
	defer func() { _ = os.RemoveAll(stagingDir) }()

	payloadPath := filepath.Join(stagingDir, "payload")
	dlCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	req, err := http.NewRequestWithContext(dlCtx, http.MethodGet, manifest.URL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := a.http.do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to download Antigravity CLI: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download returned HTTP %d", resp.StatusCode)
	}

	payloadFile, err := os.OpenFile(payloadPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, err
	}
	hasher := sha512.New()
	written, copyErr := io.Copy(io.MultiWriter(payloadFile, hasher), io.LimitReader(resp.Body, maxAgyDownloadBytes+1))
	closeErr := payloadFile.Close()
	if copyErr != nil {
		return nil, fmt.Errorf("download interrupted: %w", copyErr)
	}
	if closeErr != nil {
		return nil, closeErr
	}
	if written > maxAgyDownloadBytes {
		return nil, errors.New("downloaded package exceeds maximum size limit")
	}

	actualSHA512 := hex.EncodeToString(hasher.Sum(nil))
	if !strings.EqualFold(actualSHA512, manifest.SHA512) {
		return nil, fmt.Errorf("SHA-512 checksum verification failed for Antigravity CLI package")
	}

	extractedPath := payloadPath
	headerBytes, _ := os.ReadFile(payloadPath)
	isTarGz := strings.HasSuffix(strings.ToLower(manifest.URL), ".tar.gz") || (len(headerBytes) >= 2 && headerBytes[0] == 0x1f && headerBytes[1] == 0x8b)
	if isTarGz {
		extractedPath = filepath.Join(stagingDir, agyBinaryName())
		if err := extractAgyTarGz(payloadPath, extractedPath); err != nil {
			return nil, fmt.Errorf("failed to extract Antigravity CLI archive: %w", err)
		}
	}
	_ = os.Chmod(extractedPath, 0o755)

	// If binaryPath is already a session-guard wrapper on Unix, remove it before placing the fresh binary
	// so ensureAgyLauncherGuardForPath wraps the new binary cleanly.
	if runtime.GOOS != "windows" {
		if existing, readErr := os.ReadFile(binaryPath); readErr == nil && bytes.HasPrefix(existing, []byte("#!")) {
			_ = os.Remove(binaryPath)
		}
	}
	if err := moveOrCopyExecutable(extractedPath, binaryPath); err != nil {
		return nil, fmt.Errorf("failed to install binary to %s: %w", binaryPath, err)
	}

	if runtime.GOOS == "darwin" {
		_ = exec.CommandContext(ctx, "xattr", "-d", "com.apple.quarantine", binaryPath).Run()
	}

	// Best-effort native environment setup ('agy install'), matching official installer
	setupCtx, setupCancel := context.WithTimeout(ctx, 10*time.Second)
	setupArgs := []string{"install"}
	if isCustomDir {
		setupArgs = append(setupArgs, "--dir", targetDir)
	}
	setupCmd := exec.CommandContext(setupCtx, binaryPath, setupArgs...)
	setupCmd.Env = append(os.Environ(), "AGY_SWAP_SKIP_SYNC=1")
	_ = setupCmd.Run()
	setupCancel()

	// If installed into a custom directory, persist target in settings so agy-swap always finds it
	if isCustomDir && a.store != nil {
		if settings, loadErr := a.loadSettings(); loadErr == nil {
			if settings.Targets == nil {
				settings.Targets = map[string]TargetConfig{}
			}
			settings.Targets["agy"] = TargetConfig{Command: binaryPath, Enabled: true}
			_ = a.store.SaveSettings(settings)
		}
	}

	if runtime.GOOS != "windows" {
		a.ensureAgyLauncherGuardForPath(binaryPath)
	}

	return map[string]any{
		"installed": true,
		"version":   manifest.Version,
		"platform":  platform,
		"path":      binaryPath,
		"sha512":    manifest.SHA512,
	}, nil
}

func (a *Application) cmdInstallAgy(ctx context.Context, opts extendedOptions, positional []string) int {
	customDir := strings.TrimSpace(opts.Dir)
	if customDir == "" && len(positional) > 0 {
		// Support 'agy-swap install agy' or 'agy-swap agy install'
		for _, tok := range positional {
			if tok != "agy" && tok != "install" {
				customDir = tok
				break
			}
		}
	}
	stop := func() {}
	if !opts.JSON {
		stop = a.spinner("Installing Antigravity CLI ('agy')...")
	}
	result, err := a.installAgyCLI(ctx, customDir, opts.Force)
	stop()
	if err != nil {
		return a.extendedError("install-agy", opts, err)
	}
	if opts.JSON {
		return a.extendedResult("install-agy", opts, result, nil)
	}
	fmt.Fprintf(a.Out, "%s✓ Installed Antigravity CLI v%v to %v%s\n", a.p.Green, result["version"], result["path"], a.p.Reset)
	return 0
}
