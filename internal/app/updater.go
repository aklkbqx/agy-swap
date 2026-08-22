package app

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

type releaseAsset struct {
	Name string `json:"name"`
	URL  string `json:"browser_download_url"`
}
type githubRelease struct {
	Tag     string         `json:"tag_name"`
	HTMLURL string         `json:"html_url"`
	Assets  []releaseAsset `json:"assets"`
}

func (h *HTTPService) getBytes(ctx context.Context, endpoint string, headers map[string]string, timeout time.Duration, limit int64) ([]byte, int, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, 0, err
	}
	for key, value := range headers {
		request.Header.Set(key, value)
	}
	if request.Header.Get("User-Agent") == "" {
		request.Header.Set("User-Agent", "agy-swap")
	}
	requestCtx, cancel := context.WithTimeout(request.Context(), timeout)
	defer cancel()
	response, err := h.do(request.WithContext(requestCtx))
	if err != nil {
		return nil, 0, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, response.StatusCode, fmt.Errorf("HTTP %d", response.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err != nil {
		return nil, response.StatusCode, err
	}
	if int64(len(data)) > limit {
		return nil, response.StatusCode, fmt.Errorf("response exceeds %d bytes", limit)
	}
	return data, response.StatusCode, nil
}

func expectedChecksum(manifest []byte, name string) (string, error) {
	scanner := bufio.NewScanner(strings.NewReader(string(manifest)))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) >= 2 && strings.TrimPrefix(fields[len(fields)-1], "*") == name {
			sum := strings.ToLower(fields[0])
			if len(sum) == 64 {
				if _, err := hex.DecodeString(sum); err == nil {
					return sum, nil
				}
			}
		}
	}
	return "", fmt.Errorf("checksum for %s not found", name)
}

func (a *Application) cmdUpdate(ctx context.Context, args cliArgs) int {
	stop := a.spinner("Checking for updates...")
	data, _, err := a.http.getBytes(ctx, "https://api.github.com/repos/"+githubRepo+"/releases/latest", map[string]string{"Accept": "application/vnd.github+json"}, 10*time.Second, 1024*1024)
	stop()
	if err != nil {
		fmt.Fprintf(a.Err, "%s✕ Could not fetch update metadata: %v%s\n", a.p.Red, err, a.p.Reset)
		return 1
	}
	var release githubRelease
	if json.Unmarshal(data, &release) != nil || release.Tag == "" {
		fmt.Fprintf(a.Err, "%s✕ Could not determine latest version.%s\n", a.p.Red, a.p.Reset)
		return 1
	}
	latest := strings.TrimPrefix(release.Tag, "v")
	if latest == a.Version && !args.force {
		fmt.Fprintf(a.Out, "%s✓ Already up to date (v%s).%s\n", a.p.Green, a.Version, a.p.Reset)
		return 0
	}
	assetName := "agy-swap_" + release.Tag + "_" + runtime.GOOS + "_" + runtime.GOARCH
	if runtime.GOOS == "windows" {
		assetName += ".exe"
	}
	var binaryURL, checksumsURL string
	for _, asset := range release.Assets {
		if asset.Name == assetName {
			binaryURL = asset.URL
		}
		if asset.Name == "checksums.txt" {
			checksumsURL = asset.URL
		}
	}
	if binaryURL == "" || checksumsURL == "" {
		fmt.Fprintf(a.Err, "%s✕ Release does not contain an asset for %s/%s.%s\n", a.p.Red, runtime.GOOS, runtime.GOARCH, a.p.Reset)
		return 1
	}
	fmt.Fprintf(a.Out, "  Current: %sv%s%s\n  Latest:  %sv%s%s\n", a.p.Gray, a.Version, a.p.Reset, a.p.Green, latest, a.p.Reset)
	stop = a.spinner("Downloading v" + latest + "...")
	manifest, _, manifestErr := a.http.getBytes(ctx, checksumsURL, nil, 15*time.Second, 1024*1024)
	binary, _, binaryErr := a.http.getBytes(ctx, binaryURL, nil, 30*time.Second, 64*1024*1024)
	stop()
	if manifestErr != nil || binaryErr != nil {
		if manifestErr != nil {
			err = manifestErr
		} else {
			err = binaryErr
		}
		fmt.Fprintf(a.Err, "%s✕ Download failed: %v%s\n", a.p.Red, err, a.p.Reset)
		return 1
	}
	expected, err := expectedChecksum(manifest, assetName)
	if err != nil {
		fmt.Fprintf(a.Err, "%s✕ Integrity verification failed: %v%s\n", a.p.Red, err, a.p.Reset)
		return 1
	}
	actual := sha256.Sum256(binary)
	if hex.EncodeToString(actual[:]) != expected {
		fmt.Fprintf(a.Err, "%s✕ Integrity verification failed: checksum mismatch%s\n", a.p.Red, a.p.Reset)
		return 1
	}
	current, err := os.Executable()
	if err != nil {
		fmt.Fprintf(a.Err, "%s✕ Failed to locate current executable: %v%s\n", a.p.Red, err, a.p.Reset)
		return 1
	}
	current, _ = filepath.EvalSymlinks(current)
	stat, statErr := os.Stat(current)
	mode := os.FileMode(0o755)
	if statErr == nil {
		mode = stat.Mode()
	}
	tmpPattern := ".agy-swap.*"
	if runtime.GOOS == "windows" {
		tmpPattern = ".agy-swap.*.exe"
	}
	tmp, err := os.CreateTemp(filepath.Dir(current), tmpPattern)
	if err != nil {
		fmt.Fprintf(a.Err, "%s✕ Failed to write update: %v%s\n", a.p.Red, err, a.p.Reset)
		return 1
	}
	tmpName := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpName)
		}
	}()
	if _, err = tmp.Write(binary); err == nil {
		err = tmp.Sync()
	}
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err == nil && runtime.GOOS != "windows" {
		err = os.Chmod(tmpName, mode)
	}
	if err != nil {
		fmt.Fprintf(a.Err, "%s✕ Failed to write update: %v%s\n", a.p.Red, err, a.p.Reset)
		return 1
	}
	backup := current + ".bak"
	if runtime.GOOS == "windows" {
		command := exec.Command(tmpName, "__update-finalize", strconv.Itoa(os.Getpid()), current, backup, latest)
		command.Stdout, command.Stderr = a.Out, a.Err
		if err := command.Start(); err != nil {
			fmt.Fprintf(a.Err, "%s✕ Failed to launch update finalizer: %v%s\n", a.p.Red, err, a.p.Reset)
			return 1
		}
		cleanup = false
		fmt.Fprintf(a.Out, "%s✓ Downloaded agy-swap v%s; finalizing update after exit.%s\n", a.p.Green, latest, a.p.Reset)
		return 0
	}
	_ = os.Remove(backup)
	if err = os.Rename(current, backup); err != nil {
		fmt.Fprintf(a.Err, "%s✕ Failed to replace running executable: %v%s\n", a.p.Red, err, a.p.Reset)
		return 1
	}
	if err = os.Rename(tmpName, current); err != nil {
		_ = os.Rename(backup, current)
		fmt.Fprintf(a.Err, "%s✕ Failed to write update: %v%s\n", a.p.Red, err, a.p.Reset)
		return 1
	}
	cleanup = false
	fmt.Fprintf(a.Out, "%s✓ Updated agy-swap v%s → v%s%s\n", a.p.Green, a.Version, latest, a.p.Reset)
	if release.HTMLURL != "" {
		fmt.Fprintf(a.Out, "  %sRelease notes: %s%s\n", a.p.Gray, release.HTMLURL, a.p.Reset)
	}
	return 0
}

func (a *Application) runUpdateFinalizer(ctx context.Context, args []string) int {
	if runtime.GOOS != "windows" || len(args) != 4 {
		fmt.Fprintln(a.Err, "invalid update finalizer invocation")
		return 2
	}
	_, _ = strconv.Atoi(args[0]) // retained for auditability; retries below wait for the parent lock to release.
	target, backup, latest := args[1], args[2], args[3]
	self, err := os.Executable()
	if err != nil {
		return 1
	}
	deadline := time.Now().Add(15 * time.Second)
	for {
		select {
		case <-ctx.Done():
			return 1
		default:
		}
		_ = os.Remove(backup)
		err = os.Rename(target, backup)
		if err == nil {
			break
		}
		if time.Now().After(deadline) {
			fmt.Fprintf(a.Err, "update finalizer could not replace %s: %v\n", target, err)
			return 1
		}
		time.Sleep(100 * time.Millisecond)
	}
	for {
		err = os.Rename(self, target)
		if err == nil {
			break
		}
		if time.Now().After(deadline) {
			_ = os.Rename(backup, target)
			fmt.Fprintf(a.Err, "update finalizer could not install %s: %v\n", target, err)
			return 1
		}
		time.Sleep(100 * time.Millisecond)
	}
	fmt.Fprintf(a.Out, "✓ Updated agy-swap to v%s\n", latest)
	return 0
}
