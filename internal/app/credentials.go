package app

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

type CredentialBackend interface {
	Get(context.Context) string
	Set(context.Context, string) bool
	Delete(context.Context) bool
}

// sessionPreparer adapts a validated token to a platform's active-session
// representation. Such backends require an authoritative store write; mirrored
// OAuth files alone cannot establish a successful switch.
type sessionPreparer interface {
	PrepareSession(string) (string, error)
}

type headlessSessionBackend interface {
	NoLogonSession() bool
}

type osCredentialBackend struct{}

func (osCredentialBackend) Get(ctx context.Context) string { return platformCredentialGet(ctx) }
func (osCredentialBackend) Set(ctx context.Context, token string) bool {
	return platformCredentialSet(ctx, token)
}
func (osCredentialBackend) Delete(ctx context.Context) bool { return platformCredentialDelete(ctx) }

type Credentials struct {
	paths   Paths
	backend CredentialBackend
}

func NewCredentials(paths Paths) *Credentials {
	return &Credentials{paths: paths, backend: osCredentialBackend{}}
}

func (c *Credentials) Secure(ctx context.Context) string { return c.backend.Get(ctx) }

func (c *Credentials) useFileSessionStore() bool {
	if c == nil {
		return false
	}
	mode := strings.ToLower(strings.TrimSpace(os.Getenv("AGY_SWAP_SESSION_STORE")))
	if mode == "keychain" || mode == "os" {
		return false
	}
	_, isOS := c.backend.(osCredentialBackend)
	if !isOS {
		return false
	}
	if mode == "file" {
		return true
	}
	return runtime.GOOS != "windows"
}

// shouldMirrorOSKeyring reports whether file-session operations should also
// mirror the active OAuth token into the OS keyring (macOS Keychain / Linux
// Secret Service) so Antigravity IDE processes spawned without WSL_INTEROP=1
// read the exact same account token. Unit tests running against temporary
// directories skip OS keyring calls unless AGY_SWAP_MIRROR_OS_KEYRING=1 is set.
func (c *Credentials) shouldMirrorOSKeyring() bool {
	if c == nil || runtime.GOOS == "windows" {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(os.Getenv("AGY_SWAP_MIRROR_OS_KEYRING"))) {
	case "1", "true":
		return true
	case "0", "false":
		return false
	}
	if _, isOS := c.backend.(osCredentialBackend); !isOS {
		return false
	}
	if currentSwapExecutable() == "" || strings.TrimSpace(c.paths.Home) == "" {
		return false
	}
	realHome, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(realHome) == "" {
		return false
	}
	return filepath.Clean(c.paths.Home) == filepath.Clean(realHome)
}

func (c *Credentials) shouldReloadIDEHubs() bool {
	return c.shouldReloadIDEHubsForBinary(currentSwapExecutable())
}

func (c *Credentials) shouldReloadIDEHubsForBinary(swapBin string) bool {
	if c == nil || runtime.GOOS == "windows" || strings.TrimSpace(swapBin) == "" || strings.TrimSpace(c.paths.Home) == "" {
		return false
	}
	if os.Getenv("AGY_SWAP_SKIP_SYNC") == "1" || os.Getenv("AGY_SWAP_SKIP_HUB_RELOAD") == "1" {
		return false
	}
	realHome, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(realHome) == "" {
		return false
	}
	return filepath.Clean(c.paths.Home) == filepath.Clean(realHome)
}

func signalRunningIDEHubs() {
	pkillBin, err := exec.LookPath("pkill")
	if err != nil || pkillBin == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	args := []string{"-TERM"}
	if uid := os.Getuid(); uid >= 0 {
		args = append(args, "-u", strconv.Itoa(uid))
	}
	args = append(args, "-f", `(\.agy\.real|libexec/agy).*--hub`)
	_ = exec.CommandContext(ctx, pkillBin, args...).Run()
}

// hubReloadMarkerPath is the file agy-swap rewrites just before it stops the
// IDE's background agy hub after a switch. The session-guard wrapper restarts
// the hub only when this file changed while the hub was running.
func (c *Credentials) hubReloadMarkerPath() string {
	if c == nil || c.paths.OAuthToken == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(c.paths.OAuthToken), "cache", "agy-swap-hub-reload")
}

// reloadRunningIDEHubsTargeted writes the reload marker and sends SIGTERM
// strictly to discovered `agy --hub` PIDs from daemon/hub_*.json, falling back
// to pkill only when no discovery descriptor is present.
func (c *Credentials) reloadRunningIDEHubsTargeted(home string) {
	if !c.shouldReloadIDEHubs() {
		return
	}
	marker := c.hubReloadMarkerPath()
	if marker == "" {
		return
	}
	if err := os.MkdirAll(filepath.Dir(marker), 0o700); err != nil {
		return
	}
	if err := os.WriteFile(marker, []byte(strconv.FormatInt(time.Now().UnixNano(), 10)), 0o600); err != nil {
		return
	}
	if hubs := discoverRunningIDEHubs(home); len(hubs) > 0 {
		for _, hub := range hubs {
			_ = signalProcessTerm(hub.PID)
		}
		return
	}
	signalRunningIDEHubs()
}

// stopRunningIDEHubs terminates running IDE hubs without touching the reload
// marker so the session-guard wrapper exits cleanly instead of restarting them.
func (c *Credentials) stopRunningIDEHubs() {
	if !c.shouldReloadIDEHubs() {
		return
	}
	signalRunningIDEHubs()
}

func (c *Credentials) keyringMarkerPath() string {
	if c == nil || c.paths.OAuthToken == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(c.paths.OAuthToken), "cache", "antigravity-keyring-unavailable")
}

func (c *Credentials) keyringMarkerPaths() []string {
	if c == nil {
		return nil
	}
	var out []string
	seen := make(map[string]struct{}, 6)
	add := func(marker string) {
		if strings.TrimSpace(marker) == "" {
			return
		}
		clean := filepath.Clean(marker)
		if _, ok := seen[clean]; ok {
			return
		}
		seen[clean] = struct{}{}
		out = append(out, clean)
	}
	// Explicitly cover all three Antigravity marker locations:
	// ~/.gemini/cache (agy --hub DefaultKeyringMarkerPath),
	// ~/.gemini/antigravity-cli/cache (CLI), and ~/.gemini/antigravity/cache (IDE/ACP).
	if strings.TrimSpace(c.paths.Home) != "" {
		add(filepath.Join(c.paths.Home, ".gemini", "cache", "antigravity-keyring-unavailable"))
		add(filepath.Join(c.paths.Home, ".gemini", "antigravity-cli", "cache", "antigravity-keyring-unavailable"))
		add(filepath.Join(c.paths.Home, ".gemini", "antigravity", "cache", "antigravity-keyring-unavailable"))
	}
	for _, tokenPath := range []string{c.paths.OAuthToken, c.paths.IDEOAuthToken, c.paths.JetskiToken} {
		if tokenPath == "" {
			continue
		}
		add(filepath.Join(filepath.Dir(tokenPath), "cache", "antigravity-keyring-unavailable"))
	}
	return out
}

func (c *Credentials) writeKeyringBypassMarker() {
	future := time.Now().Add(10 * 365 * 24 * time.Hour)
	for _, marker := range c.keyringMarkerPaths() {
		if info, err := os.Stat(marker); err == nil && time.Until(info.ModTime()) > 5*365*24*time.Hour {
			continue
		}
		if err := ensurePrivateDir(filepath.Dir(marker)); err != nil {
			continue
		}
		if _, err := os.Stat(marker); err != nil {
			_ = os.WriteFile(marker, []byte("file-session\n"), 0o600)
		}
		_ = os.Chtimes(marker, future, future)
	}
}

// ensureKeyringBypassMarker maintains the antigravity-keyring-unavailable
// marker with a far-future modification timestamp so Antigravity's
// CompositeTokenStorage uses antigravity-oauth-token directly without invoking
// /usr/bin/security or triggering macOS Keychain password dialogs.
func (c *Credentials) ensureKeyringBypassMarker() {
	if !c.useFileSessionStore() {
		return
	}
	c.writeKeyringBypassMarker()
}

func (c *Credentials) Current(ctx context.Context) string {
	if token := c.readOAuthToken(); token != "" {
		return token
	}
	if c.useFileSessionStore() {
		return ""
	}
	return c.Secure(ctx)
}

// StoredActiveEmail returns the last identity written by the Antigravity
// credential flow. It is a local hint used to paint the active badge before a
// remote userinfo lookup completes; callers still verify it against accounts.
func (c *Credentials) StoredActiveEmail() string {
	data, err := os.ReadFile(c.paths.GoogleAccounts)
	if err != nil {
		return ""
	}
	var parsed map[string]any
	if json.Unmarshal(data, &parsed) != nil {
		return ""
	}
	return normalizeEmail(getString(parsed, "active"))
}

func (c *Credentials) Set(ctx context.Context, token string) bool {
	return c.backend.Set(ctx, token)
}
func (c *Credentials) Delete(ctx context.Context) bool { return c.backend.Delete(ctx) }

func (c *Credentials) oauthTokenPaths() []string {
	if c == nil {
		return nil
	}
	paths := make([]string, 0, 3)
	for _, path := range []string{c.paths.OAuthToken, c.paths.IDEOAuthToken, c.paths.JetskiToken} {
		if path != "" {
			paths = append(paths, path)
		}
	}
	return paths
}

func (c *Credentials) sessionFilePaths(includeGoogleAccounts bool) []string {
	paths := make([]string, 0, 5)
	paths = append(paths, c.oauthTokenPaths()...)
	if c != nil && c.paths.OAuthCredentials != "" {
		paths = append(paths, c.paths.OAuthCredentials)
	}
	if includeGoogleAccounts && c != nil && c.paths.GoogleAccounts != "" {
		paths = append(paths, c.paths.GoogleAccounts)
	}
	return paths
}

func readOAuthTokenFromFile(path string) string {
	if path == "" {
		return ""
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	var parsed any
	if json.Unmarshal(data, &parsed) != nil {
		return ""
	}
	return "go-keyring-base64:" + base64.StdEncoding.EncodeToString(data)
}

func tokenIsStrictlyFresher(existingToken, candidateToken string) bool {
	if hashToken(existingToken) == hashToken(candidateToken) {
		return false
	}
	candInner := tokenObject(decodeToken(candidateToken))
	if candInner == nil || getString(candInner, "access_token") == "" {
		return false
	}
	existInner := tokenObject(decodeToken(existingToken))
	if existInner == nil {
		return true
	}
	existExp, existOK := tokenExpiry(existInner)
	candExp, candOK := tokenExpiry(candInner)
	return existOK && candOK && candExp.After(existExp)
}

func (c *Credentials) readOAuthToken() string {
	best := ""
	bestRefresh := ""
	for _, path := range c.oauthTokenPaths() {
		candidate := readOAuthTokenFromFile(path)
		if !hasOAuthSession(candidate) {
			continue
		}
		if best == "" {
			best = candidate
			bestRefresh = tokenRefreshToken(candidate)
			continue
		}
		if bestRefresh != "" && tokenRefreshToken(candidate) == bestRefresh && tokenIsStrictlyFresher(best, candidate) {
			best = candidate
		}
	}
	return best
}

func (c *Credentials) writeOAuthFiles(tokenData, email string) bool {
	decoded := decodeToken(tokenData)
	inner := tokenObject(decoded)
	if inner == nil || getString(inner, "access_token") == "" {
		return false
	}
	if strings.TrimSpace(getString(decoded, "auth_method")) == "" && strings.TrimSpace(getString(decoded, "wif_provider")) == "" {
		if oauthClientID(decoded) == gcpOAuthClientID {
			decoded["auth_method"] = "gcp"
		} else {
			decoded["auth_method"] = "consumer"
		}
	}
	snapshot, err := snapshotFiles(c.sessionFilePaths(email != "")...)
	if err != nil {
		return false
	}
	creds := map[string]any{"access_token": getString(inner, "access_token"), "refresh_token": getString(inner, "refresh_token"), "scope": firstString(inner["scope"], "https://www.googleapis.com/auth/cloud-platform openid https://www.googleapis.com/auth/userinfo.email"), "token_type": firstString(inner["token_type"], "Bearer"), "id_token": getString(inner, "id_token")}
	if expiry, ok := inner["expiry_date"]; ok {
		creds["expiry_date"] = expiry
	} else {
		creds["expiry_date"] = 0
	}
	for _, tokenPath := range c.oauthTokenPaths() {
		if err := atomicWriteJSON(tokenPath, decoded); err != nil {
			_ = restoreFiles(snapshot)
			return false
		}
	}
	if err := atomicWriteJSON(c.paths.OAuthCredentials, creds); err != nil {
		_ = restoreFiles(snapshot)
		return false
	}
	if email != "" {
		// Keep "old" empty so local session files never accumulate a cross-account roster.
		ga := map[string]any{"active": email, "old": []any{}}
		if err := atomicWriteJSON(c.paths.GoogleAccounts, ga); err != nil {
			_ = restoreFiles(snapshot)
			return false
		}
	}
	return true
}

func (c *Credentials) applyUnlockedWithReload(ctx context.Context, tokenData, email string, autoReload bool) bool {
	if decodeToken(tokenData) == nil {
		return false
	}
	email = normalizeEmail(email)
	if email == "" {
		return false
	}
	if !tokenMatchesEmail(tokenData, email) {
		return false
	}
	if c.useFileSessionStore() {
		prevEmail := c.StoredActiveEmail()
		prevRefresh := tokenRefreshToken(c.readOAuthToken())
		c.ensureKeyringBypassMarker()
		if !c.writeOAuthFiles(tokenData, email) {
			return false
		}
		if c.shouldMirrorOSKeyring() {
			_ = c.Set(ctx, tokenData)
		}
		if autoReload && (!strings.EqualFold(prevEmail, email) || (prevRefresh != "" && prevRefresh != tokenRefreshToken(tokenData))) {
			c.reloadRunningIDEHubsTargeted(c.paths.Home)
		}
		return true
	}
	prepared := tokenData
	requireSecure := false
	if preparer, ok := c.backend.(sessionPreparer); ok {
		requireSecure = true
		var err error
		prepared, err = preparer.PrepareSession(tokenData)
		if err != nil {
			// When a Windows token exceeds WinCred's 2560B blob limit on the real OS
			// backend, fall back cleanly to file-based session storage with keyring
			// bypass markers while keeping explicit preparedSessionBackend rejections intact.
			if _, isOS := c.backend.(osCredentialBackend); isOS && errors.Is(err, errWindowsCredentialTooLarge) {
				c.writeKeyringBypassMarker()
				return c.writeOAuthFiles(tokenData, email)
			}
			return false
		}
	}
	previous := c.Secure(ctx)
	if previous == prepared {
		return c.writeOAuthFiles(tokenData, email)
	}
	updated := c.Set(ctx, prepared)
	if !updated {
		current := c.Secure(ctx)
		switch {
		case current == prepared:
			updated = true
		case previous != "":
			if current != previous {
				_ = c.Set(ctx, previous)
			}
			return false
		case current != "":
			return false
		case requireSecure:
			if hb, ok := c.backend.(headlessSessionBackend); ok && hb.NoLogonSession() {
				c.writeKeyringBypassMarker()
				return c.writeOAuthFiles(tokenData, email)
			}
			return false
		}
	}
	if c.writeOAuthFiles(tokenData, email) {
		return true
	}
	if updated {
		if previous != "" {
			_ = c.Set(ctx, previous)
		} else {
			_ = c.Delete(ctx)
		}
	}
	return false
}

func tokenMatchesEmail(tokenData, email string) bool {
	claimed := extractEmailHint(tokenData)
	return claimed == "" || claimed == normalizeEmail(email)
}

func (c *Credentials) Apply(ctx context.Context, tokenData, email string) bool {
	lock, err := acquireFileLock(c.paths.SessionLock)
	if err != nil {
		return false
	}
	defer func() { _ = lock.Close() }()
	return c.applyUnlockedWithReload(ctx, tokenData, email, true)
}

func (c *Credentials) ApplyWithoutReload(ctx context.Context, tokenData, email string) bool {
	lock, err := acquireFileLock(c.paths.SessionLock)
	if err != nil {
		return false
	}
	defer func() { _ = lock.Close() }()
	return c.applyUnlockedWithReload(ctx, tokenData, email, false)
}

func (c *Credentials) deleteOAuthFiles() bool {
	ok := true
	for _, path := range c.sessionFilePaths(true) {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			ok = false
		}
	}
	return ok
}

func (c *Credentials) clearUnlocked(ctx context.Context) bool {
	if c.useFileSessionStore() {
		if c.shouldMirrorOSKeyring() {
			_ = c.Delete(ctx)
		}
		ok := c.deleteOAuthFiles()
		c.ensureKeyringBypassMarker()
		return ok && c.readOAuthToken() == ""
	}
	_ = c.Delete(ctx)
	_ = c.deleteOAuthFiles()
	return c.Secure(ctx) == "" && c.readOAuthToken() == ""
}
func (c *Credentials) Clear(ctx context.Context) bool {
	lock, err := acquireFileLock(c.paths.SessionLock)
	if err != nil {
		return false
	}
	defer func() { _ = lock.Close() }()
	ok := c.clearUnlocked(ctx)
	clearIDEOnboardingStateOffline(c.paths.Home)
	c.stopRunningIDEHubs()
	return ok
}
