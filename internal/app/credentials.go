package app

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
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

func (c *Credentials) keyringMarkerPath() string {
	if c == nil || c.paths.OAuthToken == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(c.paths.OAuthToken), "cache", "antigravity-keyring-unavailable")
}

func (c *Credentials) writeKeyringBypassMarker() {
	marker := c.keyringMarkerPath()
	if marker == "" {
		return
	}
	if err := ensurePrivateDir(filepath.Dir(marker)); err != nil {
		return
	}
	if _, err := os.Stat(marker); err != nil {
		_ = os.WriteFile(marker, []byte("file-session\n"), 0o600)
	}
	future := time.Now().Add(10 * 365 * 24 * time.Hour)
	_ = os.Chtimes(marker, future, future)
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

func (c *Credentials) sessionFilePaths(includeGoogleAccounts bool) []string {
	paths := make([]string, 0, 4)
	for _, path := range []string{c.paths.OAuthToken, c.paths.JetskiToken, c.paths.OAuthCredentials} {
		if path != "" {
			paths = append(paths, path)
		}
	}
	if includeGoogleAccounts && c.paths.GoogleAccounts != "" {
		paths = append(paths, c.paths.GoogleAccounts)
	}
	return paths
}

func (c *Credentials) readOAuthToken() string {
	for _, path := range []string{c.paths.OAuthToken, c.paths.JetskiToken} {
		if path == "" {
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var parsed any
		if json.Unmarshal(data, &parsed) != nil {
			continue
		}
		return "go-keyring-base64:" + base64.StdEncoding.EncodeToString(data)
	}
	return ""
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
	if err := atomicWriteJSON(c.paths.OAuthToken, decoded); err != nil {
		_ = restoreFiles(snapshot)
		return false
	}
	if c.paths.JetskiToken != "" {
		if err := atomicWriteJSON(c.paths.JetskiToken, decoded); err != nil {
			_ = restoreFiles(snapshot)
			return false
		}
	}
	if err := atomicWriteJSON(c.paths.OAuthCredentials, creds); err != nil {
		_ = restoreFiles(snapshot)
		return false
	}
	if email != "" {
		ga := map[string]any{"active": email, "old": []any{}}
		if data, readErr := os.ReadFile(c.paths.GoogleAccounts); readErr == nil {
			var existing map[string]any
			if json.Unmarshal(data, &existing) == nil {
				oldActive := getString(existing, "active")
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
		c.ensureKeyringBypassMarker()
		return c.writeOAuthFiles(tokenData, email)
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
