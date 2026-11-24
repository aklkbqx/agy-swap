package app

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"time"
)

const (
	updateCheckTTL      = 24 * time.Hour
	updateCheckTimeout  = 1500 * time.Millisecond
	updateCheckFileName = "update-check.json"
	updateCheckMaxBytes = 4096
)

var (
	readBuildInfo  = debug.ReadBuildInfo
	executablePath = os.Executable
	evalSymlinks   = filepath.EvalSymlinks
	updateCheckNow = func() time.Time { return time.Now().UTC() }
)

type updateCheckCache struct {
	CheckedAt     time.Time `json:"checked_at"`
	LatestVersion string    `json:"latest_version"`
}

func updateCheckURL() string {
	if custom := strings.TrimSpace(os.Getenv("AGY_SWAP_UPDATE_CHECK_URL")); custom != "" {
		return custom
	}
	if site := strings.TrimSpace(os.Getenv("AGY_SWAP_SITE_URL")); site != "" {
		return strings.TrimRight(site, "/") + "/version.json"
	}
	return defaultSiteBaseURL + "/version.json"
}

func isLocalDevBuild(buildID string, info *debug.BuildInfo) bool {
	if buildID == "release" || strings.HasPrefix(buildID, "release-") {
		return false
	}
	if buildID == "demo" {
		return true
	}
	if info == nil {
		return true
	}
	if info.Main.Version == "" || info.Main.Version == "(devel)" {
		return true
	}
	for _, setting := range info.Settings {
		if setting.Key == "vcs.modified" && setting.Value == "true" {
			return true
		}
	}
	return false
}

func detectInstallChannel(buildID string, info *debug.BuildInfo) string {
	if exe, err := executablePath(); err == nil && exe != "" {
		if resolved, symErr := evalSymlinks(exe); symErr == nil && resolved != "" {
			exe = resolved
		}
		if strings.Contains(filepath.ToSlash(exe), "/Cellar/agy-swap/") {
			return "brew"
		}
	}
	if buildID != "release" && !strings.HasPrefix(buildID, "release-") && info != nil && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return "go-install"
	}
	return "binary"
}

func bareUpdateVersion(version string) string {
	clean := strings.TrimPrefix(strings.TrimSpace(version), "v")
	if idx := strings.IndexByte(clean, '-'); idx >= 0 {
		clean = clean[:idx]
	}
	if !stableVersionPattern.MatchString(clean) {
		return ""
	}
	return clean
}

func updateCheckUserAgent(version, buildID string) string {
	info, _ := readBuildInfo()
	ver := bareUpdateVersion(version)
	if ver == "" {
		ver = "0.0.0"
	}
	return fmt.Sprintf("agy-swap/%s (%s; %s; %s)", ver, runtime.GOOS, runtime.GOARCH, detectInstallChannel(buildID, info))
}

func (a *Application) shouldSkipUpdateCheck() bool {
	if a == nil || a.demo || a.http == nil || a.paths.ConfigDir == "" {
		return true
	}
	if noCheck := strings.TrimSpace(os.Getenv("AGY_SWAP_NO_UPDATE_CHECK")); noCheck == "1" || strings.EqualFold(noCheck, "true") {
		return true
	}
	if strings.TrimSpace(os.Getenv("DO_NOT_TRACK")) == "1" {
		return true
	}
	if strings.TrimSpace(os.Getenv("CI")) != "" || strings.TrimSpace(os.Getenv("GITHUB_ACTIONS")) != "" {
		return true
	}
	info, _ := readBuildInfo()
	return isLocalDevBuild(a.BuildID, info)
}

func (a *Application) updateAvailableVersion(latest string) string {
	latest = bareUpdateVersion(latest)
	if latest == "" {
		return ""
	}
	current := strings.TrimPrefix(strings.TrimSpace(a.Version), "v")
	if current == "" {
		current = "0.0.0"
	}
	cmp, err := compareVersions(latest, current)
	if err != nil || cmp <= 0 {
		return ""
	}
	return latest
}

func (a *Application) fetchRemoteVersion(ctx context.Context) (string, bool) {
	reqCtx, cancel := context.WithTimeout(ctx, updateCheckTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, updateCheckURL(), nil)
	if err != nil {
		return "", false
	}
	req.Header.Set("User-Agent", updateCheckUserAgent(a.Version, a.BuildID))
	req.Header.Set("Accept", "application/json")

	client := &http.Client{Timeout: updateCheckTimeout}
	if a.http != nil && a.http.client != nil {
		client.Transport = a.http.client.Transport
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", false
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", false
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, updateCheckMaxBytes+1))
	if err != nil || len(body) > updateCheckMaxBytes {
		return "", false
	}
	var payload struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", false
	}
	ver := strings.TrimPrefix(strings.TrimSpace(payload.Version), "v")
	if !stableVersionPattern.MatchString(ver) {
		return "", false
	}
	return ver, true
}

func (a *Application) checkForUpdate(ctx context.Context) string {
	if a.shouldSkipUpdateCheck() {
		return ""
	}
	cachePath := filepath.Join(a.paths.ConfigDir, updateCheckFileName)
	now := updateCheckNow()
	if raw, err := readLimited(cachePath, updateCheckMaxBytes); err == nil {
		var cached updateCheckCache
		if json.Unmarshal(raw, &cached) == nil && !cached.CheckedAt.IsZero() && !now.Before(cached.CheckedAt) && now.Sub(cached.CheckedAt) < updateCheckTTL {
			return a.updateAvailableVersion(cached.LatestVersion)
		}
	}
	latest, ok := a.fetchRemoteVersion(ctx)
	if !ok {
		return ""
	}
	_ = atomicWriteJSON(cachePath, updateCheckCache{
		CheckedAt:     updateCheckNow(),
		LatestVersion: latest,
	})
	return a.updateAvailableVersion(latest)
}

func (a *Application) startBackgroundUpdateCheck(ctx context.Context) func() {
	if a.shouldSkipUpdateCheck() {
		return func() {}
	}
	checkCtx, cancel := context.WithTimeout(ctx, updateCheckTimeout)
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer cancel()
		_ = a.checkForUpdate(checkCtx)
	}()
	return func() {
		cancel()
		<-done
	}
}
