package app

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
)

type CredentialBackend interface {
	Get(context.Context) string
	Set(context.Context, string) bool
	Delete(context.Context) bool
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

func (c *Credentials) Current(ctx context.Context) string {
	if token := c.Secure(ctx); token != "" {
		return token
	}
	return c.readOAuthToken()
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

func (c *Credentials) readOAuthToken() string {
	data, err := os.ReadFile(c.paths.OAuthToken)
	if err != nil {
		return ""
	}
	var parsed any
	if json.Unmarshal(data, &parsed) != nil {
		return ""
	}
	return "go-keyring-base64:" + base64.StdEncoding.EncodeToString(data)
}

func (c *Credentials) writeOAuthFiles(tokenData, email string) bool {
	decoded := decodeToken(tokenData)
	inner := tokenObject(decoded)
	if inner == nil || getString(inner, "access_token") == "" {
		return false
	}
	paths := []string{c.paths.OAuthToken, c.paths.OAuthCredentials}
	if email != "" {
		paths = append(paths, c.paths.GoogleAccounts)
	}
	snapshot, err := snapshotFiles(paths...)
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
	previous := c.Secure(ctx)
	updated := c.Set(ctx, tokenData)
	if !updated {
		current := c.Secure(ctx)
		if current == tokenData {
			updated = true
		} else if previous != "" {
			if current != previous {
				_ = c.Set(ctx, previous)
			}
			return false
		} else if current != "" {
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

func (c *Credentials) Apply(ctx context.Context, tokenData, email string) bool {
	lock, err := acquireFileLock(c.paths.SessionLock)
	if err != nil {
		return false
	}
	defer lock.Close()
	return c.applyUnlocked(ctx, tokenData, email)
}

func (c *Credentials) deleteOAuthFiles() bool {
	ok := true
	for _, path := range []string{c.paths.OAuthToken, c.paths.OAuthCredentials, c.paths.GoogleAccounts} {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			ok = false
		}
	}
	return ok
}

func (c *Credentials) clearUnlocked(ctx context.Context) bool {
	_ = c.Delete(ctx)
	_ = c.deleteOAuthFiles()
	return c.Secure(ctx) == "" && c.readOAuthToken() == ""
}
func (c *Credentials) Clear(ctx context.Context) bool {
	lock, err := acquireFileLock(c.paths.SessionLock)
	if err != nil {
		return false
	}
	defer lock.Close()
	return c.clearUnlocked(ctx)
}
