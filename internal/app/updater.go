package app

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/aklkbqx/agy-swap/v2/internal/client"
)

type releaseAsset struct {
	Name   string `json:"name"`
	URL    string `json:"browser_download_url"`
	APIURL string `json:"url"`
}

type releaseCandidate struct {
	URL     string
	Headers map[string]string
}

type githubRelease struct {
	Tag     string         `json:"tag_name"`
	HTMLURL string         `json:"html_url"`
	Assets  []releaseAsset `json:"assets"`
}

func normalizedReleaseTag(tag string) string {
	return client.NormalizedReleaseTag(tag)
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
	defer func() { _ = response.Body.Close() }()
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
	return client.ExpectedChecksum(manifest, name)
}

const defaultSiteBaseURL = "https://agy-swap.aklkbqx.com"

var softwareVersionPattern = regexp.MustCompile(`"softwareVersion"\s*:\s*"v?([0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z.-]+)?)"`)

func releaseMetadataURL() string {
	if custom := strings.TrimSpace(os.Getenv("AGY_SWAP_UPDATE_URL")); custom != "" {
		return custom
	}
	return "https://api.github.com/repos/" + githubRepo + "/releases/latest"
}

func releaseSiteFallbackURL() string {
	if custom := strings.TrimSpace(os.Getenv("AGY_SWAP_SITE_URL")); custom != "" {
		return custom
	}
	if strings.TrimSpace(os.Getenv("AGY_SWAP_UPDATE_URL")) != "" {
		return ""
	}
	return defaultSiteBaseURL + "/"
}

func syntheticReleaseFromVersion(ver string) githubRelease {
	tag := normalizedReleaseTag(ver)
	assetName := "agy-swap_" + tag + "_" + runtime.GOOS + "_" + runtime.GOARCH
	if runtime.GOOS == "windows" {
		assetName += ".exe"
	}
	dlBase := "https://github.com/" + githubRepo + "/releases/download/" + tag
	return githubRelease{
		Tag:     tag,
		HTMLURL: "https://github.com/" + githubRepo + "/releases/tag/" + tag,
		Assets: []releaseAsset{
			{Name: "checksums.txt", URL: dlBase + "/checksums.txt"},
			{Name: assetName, URL: dlBase + "/" + assetName},
		},
	}
}

var errInvalidReleaseMetadata = errors.New("invalid release metadata")

func (h *HTTPService) fetchLatestRelease(ctx context.Context) (githubRelease, error) {
	data, _, err := h.getBytes(ctx, releaseMetadataURL(), map[string]string{"Accept": "application/vnd.github+json"}, 10*time.Second, 1024*1024)
	if err == nil {
		var release githubRelease
		if json.Unmarshal(data, &release) == nil && release.Tag != "" {
			return release, nil
		}
		err = errInvalidReleaseMetadata
	}
	if siteURL := releaseSiteFallbackURL(); siteURL != "" {
		if siteData, _, siteErr := h.getBytes(ctx, siteURL, nil, 10*time.Second, 1024*1024); siteErr == nil {
			if m := softwareVersionPattern.FindSubmatch(siteData); len(m) == 2 {
				return syntheticReleaseFromVersion(string(m[1])), nil
			}
		}
	}
	return githubRelease{}, err
}

func assetCandidateURLs(releaseTag, assetName string, asset releaseAsset) []releaseCandidate {
	tag := normalizedReleaseTag(releaseTag)
	seen := make(map[string]struct{}, 4)
	var out []releaseCandidate
	add := func(rawURL string, headers map[string]string) {
		u := strings.TrimSpace(rawURL)
		if u == "" {
			return
		}
		if _, ok := seen[u]; ok {
			return
		}
		seen[u] = struct{}{}
		out = append(out, releaseCandidate{URL: u, Headers: headers})
	}

	add(asset.URL, nil)
	if mirror := strings.TrimRight(strings.TrimSpace(os.Getenv("AGY_SWAP_RELEASE_MIRROR")), "/"); mirror != "" {
		if tag != "" {
			add(mirror+"/"+tag+"/"+assetName, nil)
		}
		add(mirror+"/"+assetName, nil)
	}
	if asset.APIURL != "" {
		add(asset.APIURL, map[string]string{"Accept": "application/octet-stream"})
	}
	return out
}

func (h *HTTPService) downloadReleaseAsset(ctx context.Context, candidates []releaseCandidate, timeout time.Duration, limit int64) ([]byte, error) {
	if len(candidates) == 0 {
		return nil, errors.New("no download URL candidates available")
	}
	var firstErr error
	for _, candidate := range candidates {
		data, _, err := h.getBytes(ctx, candidate.URL, candidate.Headers, timeout, limit)
		if err == nil {
			return data, nil
		}
		if firstErr == nil {
			firstErr = err
		}
	}
	return nil, firstErr
}

func (a *Application) cmdUpdate(ctx context.Context, args cliArgs) int {
	stop := a.spinner("Checking for updates...")
	release, err := a.http.fetchLatestRelease(ctx)
	stop()
	if errors.Is(err, errInvalidReleaseMetadata) {
		fmt.Fprintf(a.Err, "%s✕ Could not determine latest version.%s\n", a.p.Red, a.p.Reset)
		return 1
	}
	if err != nil {
		fmt.Fprintf(a.Err, "%s✕ Could not fetch update metadata: %s%s\n", a.p.Red, formatUpdateDownloadError(err), a.p.Reset)
		if hint := diagnoseUpdateDownloadError(err); hint != "" {
			fmt.Fprintf(a.Err, "  %sHint: %s%s\n", a.p.Yellow, hint, a.p.Reset)
		}
		return 1
	}
	releaseTag := normalizedReleaseTag(release.Tag)
	latest := strings.TrimPrefix(releaseTag, "v")
	currentVersion := strings.TrimPrefix(strings.TrimSpace(a.Version), "v")
	order, versionErr := compareVersions(latest, currentVersion)
	if versionErr != nil {
		fmt.Fprintln(a.Err, "Cannot compare release versions:", versionErr)
		return 1
	}
	if order < 0 && !args.force {
		fmt.Fprintf(a.Out, "Installed v%s is newer than release v%s; use --force for an explicit downgrade.\n", currentVersion, latest)
		return 0
	}
	if order == 0 && !args.force {
		fmt.Fprintf(a.Out, "%s✓ Already up to date (v%s).%s\n", a.p.Green, currentVersion, a.p.Reset)
		return 0
	}
	assetName := "agy-swap_" + releaseTag + "_" + runtime.GOOS + "_" + runtime.GOARCH
	if runtime.GOOS == "windows" {
		assetName += ".exe"
	}
	var binaryAsset, checksumsAsset releaseAsset
	for _, asset := range release.Assets {
		if asset.Name == assetName {
			binaryAsset = asset
		}
		if asset.Name == "checksums.txt" {
			checksumsAsset = asset
		}
	}
	binaryCandidates := assetCandidateURLs(releaseTag, assetName, binaryAsset)
	checksumCandidates := assetCandidateURLs(releaseTag, "checksums.txt", checksumsAsset)
	if len(binaryCandidates) == 0 || len(checksumCandidates) == 0 {
		fmt.Fprintf(a.Err, "%s✕ Release v%s does not contain %s (or checksums.txt) for %s/%s. Install from %s or run the matching platform installer.%s\n", a.p.Red, latest, assetName, runtime.GOOS, runtime.GOARCH, release.HTMLURL, a.p.Reset)
		return 1
	}
	if args.updateCheck {
		fmt.Fprintf(a.Out, "%s✓ Update asset is available: %s%s\n", a.p.Green, assetName, a.p.Reset)
		return 0
	}
	fmt.Fprintf(a.Out, "  Current: %sv%s%s\n  Latest:  %sv%s%s\n", a.p.Gray, a.Version, a.p.Reset, a.p.Green, latest, a.p.Reset)

	current, err := os.Executable()
	if err == nil {
		if resolved, resolveErr := filepath.EvalSymlinks(current); resolveErr == nil {
			current = resolved
		}
	}
	if isHomebrewManaged(current) {
		fmt.Fprintf(a.Out, "%s● Detected Homebrew installation. Upgrading via brew...%s\n", a.p.Cyan, a.p.Reset)
		cleanShadowedLocalBinary(a.Out, a.p)
		if upgradeErr := upgradeViaHomebrew(ctx, latest, a.Out, a.Err); upgradeErr != nil {
			fmt.Fprintf(a.Err, "%s✕ Homebrew update not completed: %v%s\n", a.p.Red, upgradeErr, a.p.Reset)
			return 1
		}
		fmt.Fprintf(a.Out, "%s✓ Updated agy-swap v%s → v%s via Homebrew%s\n", a.p.Green, a.Version, latest, a.p.Reset)
		if release.HTMLURL != "" {
			fmt.Fprintf(a.Out, "  %sRelease notes: %s%s\n", a.p.Gray, release.HTMLURL, a.p.Reset)
		}
		return 0
	}

	stop = a.spinner("Downloading v" + latest + "...")
	manifest, manifestErr := a.http.downloadReleaseAsset(ctx, checksumCandidates, 15*time.Second, 1024*1024)
	if manifestErr != nil {
		stop()
		if a.tryGoToolchainFallback(ctx, latest, current, release.HTMLURL, manifestErr) {
			return 0
		}
		fmt.Fprintf(a.Err, "%s✕ Download failed for checksums.txt: %s%s\n", a.p.Red, formatUpdateDownloadError(manifestErr), a.p.Reset)
		if hint := diagnoseUpdateDownloadError(manifestErr); hint != "" {
			fmt.Fprintf(a.Err, "  %sHint: %s%s\n", a.p.Yellow, hint, a.p.Reset)
		}
		return 1
	}
	expected, err := expectedChecksum(manifest, assetName)
	if err != nil {
		stop()
		fmt.Fprintf(a.Err, "%s✕ Integrity verification failed: %v%s\n", a.p.Red, err, a.p.Reset)
		return 1
	}
	binary, binaryErr := a.http.downloadReleaseAsset(ctx, binaryCandidates, 30*time.Second, 64*1024*1024)
	stop()
	if binaryErr != nil {
		if a.tryGoToolchainFallback(ctx, latest, current, release.HTMLURL, binaryErr) {
			return 0
		}
		fmt.Fprintf(a.Err, "%s✕ Download failed for %s: %s%s\n", a.p.Red, assetName, formatUpdateDownloadError(binaryErr), a.p.Reset)
		if hint := diagnoseUpdateDownloadError(binaryErr); hint != "" {
			fmt.Fprintf(a.Err, "  %sHint: %s%s\n", a.p.Yellow, hint, a.p.Reset)
		}
		return 1
	}
	actual := sha256.Sum256(binary)
	if hex.EncodeToString(actual[:]) != expected {
		fmt.Fprintf(a.Err, "%s✕ Integrity verification failed: checksum mismatch%s\n", a.p.Red, a.p.Reset)
		return 1
	}
	if current == "" {
		current, err = os.Executable()
		if err != nil {
			fmt.Fprintf(a.Err, "%s✕ Failed to locate current executable: %v%s\n", a.p.Red, err, a.p.Reset)
			return 1
		}
		current, err = filepath.EvalSymlinks(current)
		if err != nil {
			fmt.Fprintln(a.Err, "Cannot resolve installed executable:", err)
			return 1
		}
	}
	if err != nil {
		fmt.Fprintln(a.Err, "Cannot resolve installed executable:", err)
		return 1
	}
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
	if verifyErr := verifyUpdateBinary(ctx, tmpName, latest); verifyErr != nil {
		fmt.Fprintln(a.Err, "Update candidate failed self-test:", verifyErr)
		return 1
	}
	backup := current + ".bak"
	if runtime.GOOS == "windows" {
		command := exec.Command(tmpName, "__update-finalize", strconv.Itoa(os.Getpid()), current, backup, latest)
		command.Stdout, command.Stderr = a.Out, a.Err
		if startErr := command.Start(); startErr != nil {
			fmt.Fprintf(a.Err, "%s✕ Failed to launch update finalizer: %v%s\n", a.p.Red, startErr, a.p.Reset)
			return 1
		}
		cleanup = false
		fmt.Fprintf(a.Out, "%s✓ Downloaded agy-swap v%s; finalizing update after exit.%s\n", a.p.Green, latest, a.p.Reset)
		return 0
	}
	if err = replaceUpdateBinary(current, tmpName, backup); err != nil {
		fmt.Fprintf(a.Err, "%s✕ Failed to install update: %v%s\n", a.p.Red, err, a.p.Reset)
		return 1
	}
	cleanup = false
	fmt.Fprintf(a.Out, "%s✓ Updated agy-swap v%s → v%s%s\n", a.p.Green, a.Version, latest, a.p.Reset)
	if release.HTMLURL != "" {
		fmt.Fprintf(a.Out, "  %sRelease notes: %s%s\n", a.p.Gray, release.HTMLURL, a.p.Reset)
	}
	return 0
}

func verifyUpdateBinary(ctx context.Context, path, version string) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, path, "--version").Output()
	if err != nil {
		return err
	}
	fields := strings.Fields(string(output))
	if len(fields) < 2 || fields[0] != "agy-swap" || fields[1] != "v"+version {
		return fmt.Errorf("candidate did not report agy-swap v%s", version)
	}
	return nil
}

func replaceUpdateBinary(current, candidate, backup string) error {
	if err := os.Remove(backup); err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := os.Rename(current, backup); err != nil {
		return err
	}
	if err := os.Rename(candidate, current); err != nil {
		if rollbackErr := os.Rename(backup, current); rollbackErr != nil {
			return fmt.Errorf("install failed: %v; rollback failed: %w", err, rollbackErr)
		}
		return err
	}
	return nil
}

func (a *Application) runUpdateFinalizer(ctx context.Context, args []string) int {
	if runtime.GOOS != "windows" || len(args) != 4 {
		fmt.Fprintln(a.Err, "invalid update finalizer invocation")
		return 2
	}
	pid, parseErr := strconv.Atoi(args[0])
	if parseErr != nil || pid <= 0 {
		return 2
	}
	target, backup, latest := args[1], args[2], args[3]
	if !filepath.IsAbs(target) || backup != target+".bak" || latest != strings.TrimPrefix(a.Version, "v") {
		return 2
	}
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

// upgradeViaHomebrew upgrades the formula and confirms that the binary
// Homebrew links reports latest. brew upgrade exits 0 when its tap has not
// refreshed yet, so the version check is the success signal; a stale result
// gets one brew update and a second upgrade.
func upgradeViaHomebrew(ctx context.Context, latest string, out, errOut io.Writer) error {
	run := func(args ...string) error {
		cmd := exec.CommandContext(ctx, "brew", args...)
		cmd.Stdout, cmd.Stderr = out, errOut
		return cmd.Run()
	}
	upgrade := func() error {
		if err := run("upgrade", "aklkbqx/agy-swap/agy-swap"); err != nil {
			return run("upgrade", "agy-swap")
		}
		return nil
	}
	prefix, err := exec.CommandContext(ctx, "brew", "--prefix").Output()
	if err != nil {
		return fmt.Errorf("brew --prefix: %w", err)
	}
	linked := filepath.Join(strings.TrimSpace(string(prefix)), "bin", "agy-swap")
	if err := upgrade(); err != nil {
		return fmt.Errorf("brew upgrade: %w", err)
	}
	if verifyUpdateBinary(ctx, linked, latest) == nil {
		return nil
	}
	if err := run("update", "--quiet"); err != nil {
		return fmt.Errorf("brew update: %w", err)
	}
	if err := upgrade(); err != nil {
		return fmt.Errorf("brew upgrade: %w", err)
	}
	if err := verifyUpdateBinary(ctx, linked, latest); err != nil {
		return fmt.Errorf("Homebrew still installs %s, not v%s; run brew update and retry", installedVersion(ctx, linked), latest) //nolint:staticcheck,revive // "Homebrew" is a proper noun
	}
	return nil
}

// installedVersion returns the version token a binary reports, or "an unknown version".
func installedVersion(ctx context.Context, path string) string {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, path, "--version").Output()
	if fields := strings.Fields(string(output)); err == nil && len(fields) >= 2 {
		return fields[1]
	}
	return "an unknown version"
}

func isHomebrewManaged(path string) bool {
	if strings.Contains(path, "/Cellar/agy-swap/") {
		return true
	}
	if strings.Contains(path, "/homebrew/") || strings.Contains(path, "/Homebrew/") {
		if _, err := exec.LookPath("brew"); err == nil {
			cmd := exec.Command("brew", "list", "--formula")
			if out, err := cmd.Output(); err == nil {
				for _, line := range strings.Split(string(out), "\n") {
					if strings.TrimSpace(line) == "agy-swap" {
						return true
					}
				}
			}
		}
	}
	return false
}

func cleanShadowedLocalBinary(out io.Writer, p palette) {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return
	}
	localBin := filepath.Join(home, ".local", "bin", "agy-swap")
	if _, err := os.Stat(localBin); err == nil {
		if rmErr := os.Remove(localBin); rmErr == nil {
			fmt.Fprintf(out, "%s✓ Removed shadowing binary at %s%s\n", p.Yellow, localBin, p.Reset)
		}
	}
}

var (
	urlQueryStripPattern = regexp.MustCompile(`(https?://[^\s"?]+)\?[^\s"]+`)
	ipv4ExtractPattern   = regexp.MustCompile(`\b(?:\d{1,3}\.){3}\d{1,3}\b`)
)

func formatUpdateDownloadError(err error) string {
	if err == nil {
		return ""
	}
	var uErr *url.Error
	if errors.As(err, &uErr) && uErr.URL != "" {
		sanitized := *uErr
		if parsed, parseErr := url.Parse(uErr.URL); parseErr == nil {
			parsed.RawQuery = ""
			parsed.Fragment = ""
			sanitized.URL = parsed.String()
		}
		return urlQueryStripPattern.ReplaceAllString(sanitized.Error(), "$1")
	}
	return urlQueryStripPattern.ReplaceAllString(err.Error(), "$1")
}

func isTLSVerifyError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "tls:") ||
		strings.Contains(msg, "x509:") ||
		strings.Contains(msg, "certificate")
}

func diagnoseUpdateDownloadError(err error) string {
	if !isTLSVerifyError(err) {
		return ""
	}
	for _, match := range ipv4ExtractPattern.FindAllString(err.Error(), -1) {
		ip := net.ParseIP(match)
		if ip != nil && (ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast()) {
			return fmt.Sprintf("Network firewall or captive portal (%s) intercepted release-assets.githubusercontent.com. Complete Wi-Fi login, whitelist *.githubusercontent.com, set AGY_SWAP_RELEASE_MIRROR, or switch networks.", match)
		}
	}
	return "TLS certificate verification failed. If behind a corporate SSL-inspecting proxy, set AGY_SWAP_CA_BUNDLE=/path/to/ca-bundle.pem or AGY_SWAP_RELEASE_MIRROR."
}

func isPublicLookupHost(host string) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	if host == "" || host == "localhost" || strings.HasSuffix(host, ".localhost") || strings.HasSuffix(host, ".local") {
		return false
	}
	return net.ParseIP(host) == nil
}

func shouldRetryViaDoH(request *http.Request, err error) bool {
	if !isTLSVerifyError(err) {
		return false
	}
	if request != nil && request.URL != nil && isPublicLookupHost(request.URL.Hostname()) {
		return true
	}
	var uErr *url.Error
	if errors.As(err, &uErr) {
		if parsed, parseErr := url.Parse(uErr.URL); parseErr == nil && isPublicLookupHost(parsed.Hostname()) {
			return true
		}
	}
	return false
}

func filterPublicDoHIPs(candidates []string) []string {
	seen := make(map[string]struct{}, len(candidates))
	var out []string
	for _, raw := range candidates {
		ip := net.ParseIP(strings.TrimSpace(raw))
		if ip == nil {
			continue
		}
		if !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() {
			continue
		}
		canonical := ip.String()
		if _, ok := seen[canonical]; ok {
			continue
		}
		seen[canonical] = struct{}{}
		out = append(out, canonical)
	}
	return out
}

type dohJSONAnswer struct {
	Type int    `json:"type"`
	Data string `json:"data"`
}

type dohJSONResponse struct {
	Status int             `json:"Status"`
	Answer []dohJSONAnswer `json:"Answer"`
}

func defaultDoHEndpoints() []string {
	if custom := strings.TrimSpace(os.Getenv("AGY_SWAP_DOH_URL")); custom != "" {
		return []string{custom}
	}
	return []string{
		"https://1.1.1.1/dns-query",
		"https://8.8.8.8/resolve",
	}
}

func newDirectDoHHTTPClient(baseTLS *tls.Config) *http.Client {
	dialer := &net.Dialer{Timeout: 4 * time.Second, KeepAlive: 15 * time.Second}
	tr := &http.Transport{
		Proxy:             http.ProxyFromEnvironment,
		DialContext:       dialer.DialContext,
		ForceAttemptHTTP2: true,
		IdleConnTimeout:   30 * time.Second,
	}
	if baseTLS != nil {
		tr.TLSClientConfig = baseTLS.Clone()
	}
	return &http.Client{Transport: tr, Timeout: 5 * time.Second}
}

func resolveHostViaDoH(ctx context.Context, dohHTTP *http.Client, host string) ([]string, error) {
	if !isPublicLookupHost(host) {
		return nil, fmt.Errorf("non-public host %q", host)
	}
	if dohHTTP == nil {
		dohHTTP = newDirectDoHHTTPClient(nil)
	}
	var lastErr error
	for _, endpoint := range defaultDoHEndpoints() {
		u, err := url.Parse(endpoint)
		if err != nil {
			lastErr = err
			continue
		}
		q := u.Query()
		q.Set("name", host)
		q.Set("type", "A")
		u.RawQuery = q.Encode()

		reqCtx, cancel := context.WithTimeout(ctx, 4*time.Second)
		req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, u.String(), nil)
		if err != nil {
			cancel()
			lastErr = err
			continue
		}
		req.Header.Set("Accept", "application/dns-json")
		resp, err := dohHTTP.Do(req)
		if err != nil {
			cancel()
			lastErr = err
			continue
		}
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
		_ = resp.Body.Close()
		cancel()
		if readErr != nil || resp.StatusCode != http.StatusOK {
			lastErr = fmt.Errorf("DoH HTTP %d", resp.StatusCode)
			continue
		}
		var parsed dohJSONResponse
		if err := json.Unmarshal(body, &parsed); err != nil {
			lastErr = err
			continue
		}
		var rawIPs []string
		for _, ans := range parsed.Answer {
			if ans.Type == 1 || ans.Type == 28 {
				rawIPs = append(rawIPs, ans.Data)
			}
		}
		if ips := filterPublicDoHIPs(rawIPs); len(ips) > 0 {
			return ips, nil
		}
		lastErr = fmt.Errorf("no public IPs in DoH response for %s", host)
	}
	if lastErr == nil {
		lastErr = errors.New("no DoH endpoints configured")
	}
	return nil, lastErr
}

func newDoHBackedTransport(base *http.Transport) *http.Transport {
	cloned := base.Clone()
	dohHTTP := newDirectDoHHTTPClient(base.TLSClientConfig)
	dialer := &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}
	cloned.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(addr)
		if err == nil && isPublicLookupHost(host) {
			if ips, dohErr := resolveHostViaDoH(ctx, dohHTTP, host); dohErr == nil && len(ips) > 0 {
				var dialErr error
				for _, ip := range ips {
					conn, cErr := dialer.DialContext(ctx, network, net.JoinHostPort(ip, port))
					if cErr == nil {
						return conn, nil
					}
					dialErr = cErr
				}
				if dialErr != nil {
					return nil, dialErr
				}
			}
		}
		return dialer.DialContext(ctx, network, addr)
	}
	return cloned
}

func (a *Application) tryGoToolchainFallback(ctx context.Context, latest, current, htmlURL string, dlErr error) bool {
	if strings.TrimSpace(os.Getenv("AGY_SWAP_NO_GO_FALLBACK")) == "1" {
		return false
	}
	if os.Getenv("AGY_SWAP_GO_FALLBACK") != "1" && !isTLSVerifyError(dlErr) {
		return false
	}
	goBin, err := exec.LookPath("go")
	if err != nil || goBin == "" {
		return false
	}
	if current == "" {
		current, err = os.Executable()
		if err != nil {
			return false
		}
		if resolved, resolveErr := filepath.EvalSymlinks(current); resolveErr == nil {
			current = resolved
		}
	}
	fmt.Fprintf(a.Out, "%s● Release CDN blocked; building verified v%s via Go module proxy...%s\n", a.p.Cyan, latest, a.p.Reset)
	tmpDir, err := os.MkdirTemp(filepath.Dir(current), ".agy-swap-gobuild.*")
	if err != nil {
		return false
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	binName := "agy-swap"
	if runtime.GOOS == "windows" {
		binName += ".exe"
	}
	candidate := filepath.Join(tmpDir, binName)
	buildCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	pkg := "github.com/" + githubRepo + "/v2/cmd/agy-swap@v" + latest
	cmd := exec.CommandContext(buildCtx, goBin, "install", "-trimpath", "-ldflags", "-s -w -X main.version="+latest+" -X main.buildID=release", pkg)
	cmd.Env = append(os.Environ(), "GOBIN="+tmpDir)
	if err := cmd.Run(); err != nil {
		return false
	}
	if verifyUpdateBinary(ctx, candidate, latest) != nil {
		return false
	}
	backup := current + ".bak"
	if runtime.GOOS == "windows" {
		return false
	}
	if replaceUpdateBinary(current, candidate, backup) != nil {
		return false
	}
	fmt.Fprintf(a.Out, "%s✓ Updated agy-swap v%s → v%s (via Go module proxy)%s\n", a.p.Green, a.Version, latest, a.p.Reset)
	if htmlURL != "" {
		fmt.Fprintf(a.Out, "  %sRelease notes: %s%s\n", a.p.Gray, htmlURL, a.p.Reset)
	}
	return true
}
