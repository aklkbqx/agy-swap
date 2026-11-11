package app

import (
	"context"
	"encoding/base64"
	"encoding/hex"
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

func ideStateDBPaths(home string) []string {
	home = strings.TrimSpace(home)
	if home == "" {
		return nil
	}
	return []string{
		filepath.Join(home, "Library", "Application Support", "Antigravity IDE", "User", "globalStorage", "state.vscdb"),
		filepath.Join(home, ".config", "Antigravity IDE", "User", "globalStorage", "state.vscdb"),
	}
}

func syncIDEOnboardingEmail(home, email string) {
	email = normalizeEmail(email)
	if email == "" || strings.TrimSpace(home) == "" {
		return
	}
	sqliteBin, err := exec.LookPath("sqlite3")
	if err != nil || sqliteBin == "" {
		return
	}
	for _, dbPath := range ideStateDBPaths(home) {
		if _, err := os.Stat(dbPath); err != nil {
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		out, err := exec.CommandContext(ctx, sqliteBin, "-cmd", ".timeout 1500", dbPath,
			"SELECT value FROM ItemTable WHERE key = 'google.google-antigravity';").Output()
		cancel()
		if err != nil {
			continue
		}
		raw := strings.TrimSpace(string(out))
		if idx := strings.IndexByte(raw, '{'); idx > 0 {
			raw = raw[idx:]
		}
		if raw == "" {
			continue
		}
		var outer map[string]any
		if json.Unmarshal([]byte(raw), &outer) != nil || outer == nil {
			continue
		}
		storage, ok := outer["antigravity-storage"].(map[string]any)
		if !ok || storage == nil {
			continue
		}
		if strings.EqualFold(getString(storage, "agy.onboarding.lastLoginUsername"), email) {
			continue
		}
		storage["agy.onboarding.lastLoginUsername"] = email
		outer["antigravity-storage"] = storage
		updatedJSON, err := json.Marshal(outer)
		if err != nil {
			continue
		}
		hexVal := hex.EncodeToString(updatedJSON)
		updateSQL := "UPDATE ItemTable SET value = CAST(X'" + hexVal + "' AS TEXT) WHERE key = 'google.google-antigravity';"
		uCtx, uCancel := context.WithTimeout(context.Background(), 2*time.Second)
		_ = exec.CommandContext(uCtx, sqliteBin, "-cmd", ".timeout 1500", dbPath, updateSQL).Run()
		uCancel()
	}
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

// reloadRunningIDEHubs records the switch in hubReloadMarkerPath and sends
// SIGTERM to any running backing agy --hub process. The session-guard wrapper
// sees the changed marker and starts the hub again on the same port with the
// newly switched account, which triggers Antigravity IDE's instance_changed
// webview refresh without exhausting its crash recovery.
func (c *Credentials) reloadRunningIDEHubs() {
	if c == nil || runtime.GOOS == "windows" || currentSwapExecutable() == "" || !c.shouldMirrorOSKeyring() {
		return
	}
	pkillBin, err := exec.LookPath("pkill")
	if err != nil || pkillBin == "" {
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
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = exec.CommandContext(ctx, pkillBin, "-TERM", "-f", `(\.agy\.real|libexec/agy).*--hub`).Run()
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
	seen := make(map[string]struct{}, 3)
	for _, tokenPath := range []string{c.paths.OAuthToken, c.paths.IDEOAuthToken, c.paths.JetskiToken} {
		if tokenPath == "" {
			continue
		}
		marker := filepath.Join(filepath.Dir(tokenPath), "cache", "antigravity-keyring-unavailable")
		if _, ok := seen[marker]; ok {
			continue
		}
		seen[marker] = struct{}{}
		out = append(out, marker)
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
		prevActive := ""
		ga := map[string]any{"active": email, "old": []any{}}
		if data, readErr := os.ReadFile(c.paths.GoogleAccounts); readErr == nil {
			var existing map[string]any
			if json.Unmarshal(data, &existing) == nil {
				oldActive := getString(existing, "active")
				prevActive = oldActive
				var oldList []any
				if raw, ok := existing["old"].([]any); ok {
					oldList = raw
				}
				if oldActive != "" && oldActive != email && !containsString(oldList, oldActive) {
					oldList = append([]any{oldActive}, oldList...)
				}
				filtered := make([]any, 0, len(oldList))
				for _, item := range oldList {
					if value, ok := item.(string); ok && value != email {
						filtered = append(filtered, value)
					}
				}
				ga = map[string]any{"active": email, "old": filtered}
			}
		}
		if err := atomicWriteJSON(c.paths.GoogleAccounts, ga); err != nil {
			_ = restoreFiles(snapshot)
			return false
		}
		if !strings.EqualFold(prevActive, email) {
			syncIDEOnboardingEmail(c.paths.Home, email)
		}
	}
	return true
}

func containsString(values []any, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func (c *Credentials) applyUnlocked(ctx context.Context, tokenData, email string) bool {
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
		if !strings.EqualFold(prevEmail, email) || (prevRefresh != "" && prevRefresh != tokenRefreshToken(tokenData)) {
			c.reloadRunningIDEHubs()
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
	return c.applyUnlocked(ctx, tokenData, email)
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
	return c.clearUnlocked(ctx)
}
