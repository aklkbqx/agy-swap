package app

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func sessionTestJWT(email, padding string) string {
	claims, _ := json.Marshal(map[string]any{"iss": "https://accounts.google.com", "email": email, "email_verified": true, "padding": padding})
	return "header." + base64.RawURLEncoding.EncodeToString(claims) + ".signature"
}

func sessionTestPayload() string {
	data, _ := json.Marshal(map[string]any{
		"auth_method": "oauth", "id_token": sessionTestJWT("old@example.invalid", strings.Repeat("o", 850)),
		"provider_extension": map[string]any{"count": json.Number("9007199254740993")},
		"token":              map[string]any{"access_token": "access", "refresh_token": "refresh", "scope": "openid", "token_type": "Bearer", "expiry_date": int64(1893456000000), "expiry": "2030-01-01T00:00:00Z", "id_token": sessionTestJWT("user@example.invalid", strings.Repeat("n", 850))},
	})
	return string(data)
}

func TestWindowsSessionNormalizesWrappedCredential(t *testing.T) {
	raw := sessionTestPayload()
	wrapped := "go-keyring-base64:" + base64.StdEncoding.EncodeToString([]byte(raw))
	if len(wrapped) <= 2560 {
		t.Fatal("fixture does not reproduce oversized credential")
	}
	got, err := windowsSessionCredential(wrapped)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) > 2560 || strings.HasPrefix(got, "go-keyring-base64:") {
		t.Fatal("credential was not converted to compact session JSON")
	}
	decoded := decodeToken(got)
	original := decodeToken(raw)
	inner := tokenObject(decoded)
	if getString(decoded, "id_token") != getString(tokenObject(original), "id_token") {
		t.Fatal("stale root identity overrode refreshed nested identity")
	}
	if _, exists := inner["id_token"]; exists {
		t.Fatal("duplicated ID token remains")
	}
	for _, key := range []string{"access_token", "refresh_token", "scope", "token_type", "expiry_date", "expiry"} {
		want, _ := json.Marshal(tokenObject(original)[key])
		actual, _ := json.Marshal(inner[key])
		if string(actual) != string(want) {
			t.Fatalf("authentication field %s changed", key)
		}
	}
	if getString(decoded, "auth_method") != "oauth" || getMap(decoded["provider_extension"])["count"].(json.Number).String() != "9007199254740993" {
		t.Fatal("provider fields or numeric precision lost")
	}
	if extractEmailHint(got) != "user@example.invalid" {
		t.Fatal("prepared credential changed account identity")
	}
	second, err := windowsSessionCredential(got)
	if err != nil || second != got {
		t.Fatal("session preparation is not idempotent")
	}
}

func TestWindowsSessionByteLimitsAndInvalidInput(t *testing.T) {
	// Literal JSON overhead is 14 bytes; Unicode payloads must be counted in bytes.
	for _, tc := range []struct {
		name, value string
		tooLarge    bool
	}{
		{"boundary", `{"padding":"` + strings.Repeat("x", 2546) + `"}`, false},
		{"over", `{"padding":"` + strings.Repeat("x", 2547) + `"}`, true},
		{"unicode", `{"padding":"` + strings.Repeat("界", 850) + `"}`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := windowsSessionCredential(tc.value)
			if tc.tooLarge {
				if !errors.Is(err, errWindowsCredentialTooLarge) || got != "" {
					t.Fatal("oversized payload accepted or returned")
				}
				return
			}
			if err != nil || len(got) != 2560 {
				t.Fatalf("exact boundary: %d bytes, %v", len(got), err)
			}
		})
	}
	for _, value := range []string{"", "null", "[]", "not-json", "go-keyring-base64:%%%", `{"token":{}} trailing`, `{"token":{}} {}`} {
		if got, err := windowsSessionCredential(value); err == nil || got != "" {
			t.Fatal("invalid or trailing credential data accepted")
		}
	}
	for _, raw := range []string{`{"id_token":"root","token":{"access_token":"access"}}`, `{"token":{"id_token":"nested","access_token":"access"}}`} {
		got, err := windowsSessionCredential(raw)
		if err != nil {
			t.Fatal(err)
		}
		if getString(decodeToken(got), "id_token") == "" {
			t.Fatal("sole identity token lost")
		}
	}
}

type preparedSessionBackend struct {
	value        string
	sets         int
	fail, mutate bool
	prepareErr   error
	afterSet     func()
}

func (b *preparedSessionBackend) Get(context.Context) string  { return b.value }
func (b *preparedSessionBackend) Delete(context.Context) bool { b.value = ""; return true }
func (b *preparedSessionBackend) PrepareSession(value string) (string, error) {
	if b.prepareErr != nil {
		return "", b.prepareErr
	}
	return windowsSessionCredential(value)
}
func (b *preparedSessionBackend) Set(_ context.Context, value string) bool {
	b.sets++
	if b.fail && !b.mutate {
		return false
	}
	b.value = value
	if b.afterSet != nil {
		b.afterSet()
	}
	return !b.fail
}

func TestPreparedSessionApplyKeepsOriginalOAuthMirrors(t *testing.T) {
	paths := testPaths(t)
	backend := &preparedSessionBackend{}
	c := NewCredentials(paths)
	c.backend = backend
	raw := sessionTestPayload()
	if !c.Apply(context.Background(), raw, "user@example.invalid") {
		t.Fatal("prepared session apply failed")
	}
	if strings.Contains(backend.value, `"id_token"`) == false || len(backend.value) > 2560 {
		t.Fatal("store did not receive compact session")
	}
	if _, exists := tokenObject(decodeToken(backend.value))["id_token"]; exists {
		t.Fatal("store still received duplicated ID")
	}
	data, err := os.ReadFile(paths.OAuthToken)
	if err != nil {
		t.Fatal(err)
	}
	if getString(tokenObject(decodeToken(string(data))), "id_token") != getString(tokenObject(decodeToken(raw)), "id_token") {
		t.Fatal("OAuth mirror lost original nested ID")
	}
	data, err = os.ReadFile(paths.OAuthCredentials)
	if err != nil {
		t.Fatal(err)
	}
	if getString(decodeToken(string(data)), "id_token") != getString(tokenObject(decodeToken(raw)), "id_token") {
		t.Fatal("OAuth credentials lost refreshed identity")
	}
	if !c.Apply(context.Background(), raw, "user@example.invalid") || backend.sets != 1 {
		t.Fatal("repeat apply did not compare prepared session representation")
	}
}

func TestPreparedSessionRejectsBeforeWrites(t *testing.T) {
	for _, name := range []string{"preparation", "identity", "oversized"} {
		t.Run(name, func(t *testing.T) {
			paths := testPaths(t)
			backend := &preparedSessionBackend{value: "previous raw bytes"}
			c := NewCredentials(paths)
			c.backend = backend
			raw, email := sessionTestPayload(), "user@example.invalid"
			switch name {
			case "preparation":
				backend.prepareErr = errors.New("preflight failed")
			case "identity":
				email = "other@example.invalid"
			case "oversized":
				raw = `{"token":{"access_token":"` + strings.Repeat("x", 3000) + `"}}`
			}
			if c.Apply(context.Background(), raw, email) || backend.sets != 0 || backend.value != "previous raw bytes" {
				t.Fatal("rejected credential mutated active session")
			}
			for _, path := range []string{paths.OAuthToken, paths.OAuthCredentials, paths.GoogleAccounts} {
				if _, err := os.Stat(path); !os.IsNotExist(err) {
					t.Fatal("preflight rejection wrote OAuth files")
				}
			}
		})
	}
}

func TestPreparedSessionFailedStoreCannotSucceedFromFiles(t *testing.T) {
	paths := testPaths(t)
	backend := &preparedSessionBackend{fail: true}
	c := NewCredentials(paths)
	c.backend = backend
	if c.Apply(context.Background(), sessionTestPayload(), "user@example.invalid") {
		t.Fatal("failed authoritative store was reported as a successful switch")
	}
	if _, err := os.Stat(paths.OAuthToken); !os.IsNotExist(err) {
		t.Fatal("failed store left a file-only session")
	}
}

func TestPreparedSessionRecoversWriteThenFalse(t *testing.T) {
	paths := testPaths(t)
	backend := &preparedSessionBackend{fail: true, mutate: true}
	c := NewCredentials(paths)
	c.backend = backend
	if !c.Apply(context.Background(), sessionTestPayload(), "user@example.invalid") {
		t.Fatal("successful write with false result was not detected")
	}
	if _, exists := tokenObject(decodeToken(backend.value))["id_token"]; exists {
		t.Fatal("write-then-false compared unprepared representation")
	}
}

func TestPreparedSessionRestoresExactPreviousBytes(t *testing.T) {
	paths := testPaths(t)
	previous := "go-keyring-base64:" + base64.StdEncoding.EncodeToString([]byte(`{ "previous": "unchanged" }`))
	backend := &preparedSessionBackend{value: previous}
	c := NewCredentials(paths)
	c.backend = backend
	if err := os.MkdirAll(filepath.Dir(paths.OAuthCredentials), 0o700); err != nil {
		t.Fatal(err)
	}
	backend.afterSet = func() {
		if backend.sets == 1 {
			if err := os.Mkdir(paths.OAuthCredentials, 0o700); err != nil {
				t.Fatal(err)
			}
		}
	}
	if c.Apply(context.Background(), sessionTestPayload(), "user@example.invalid") {
		t.Fatal("OAuth snapshot failure accepted")
	}
	if backend.value != previous || backend.sets != 2 {
		t.Fatal("rollback did not restore exact previous credential bytes")
	}
	if _, err := os.Stat(paths.OAuthToken); !os.IsNotExist(err) {
		t.Fatal("failed OAuth snapshot left token mirror")
	}
}

func TestPreparedSessionRollsBackAfterFirstOAuthWrite(t *testing.T) {
	paths := testPaths(t)
	// Initially both paths are absent, so snapshot succeeds. The first write
	// creates OAuthToken as a file, making the second path's parent invalid.
	// This forces failure after a real OAuth write, without test-only hooks.
	paths.OAuthCredentials = filepath.Join(paths.OAuthToken, "credentials.json")
	previous := "go-keyring-base64:" + base64.StdEncoding.EncodeToString([]byte(`{ "previous": "exact bytes" }`))
	backend := &preparedSessionBackend{value: previous}
	c := NewCredentials(paths)
	c.backend = backend
	if err := os.MkdirAll(filepath.Dir(paths.GoogleAccounts), 0o700); err != nil {
		t.Fatal(err)
	}
	oldAccounts := []byte("{\"active\":\"previous@example.invalid\",\"old\":[]}\n")
	if err := os.WriteFile(paths.GoogleAccounts, oldAccounts, 0o600); err != nil {
		t.Fatal(err)
	}
	if c.Apply(context.Background(), sessionTestPayload(), "user@example.invalid") {
		t.Fatal("later OAuth write failure accepted")
	}
	if backend.sets != 2 || backend.value != previous {
		t.Fatal("later-write failure did not restore exact secure credential")
	}
	if _, err := os.Stat(paths.OAuthToken); !os.IsNotExist(err) {
		t.Fatal("first OAuth write was not rolled back")
	}
	if _, err := os.Stat(paths.OAuthCredentials); !os.IsNotExist(err) {
		t.Fatal("failed OAuth credentials path remains")
	}
	accounts, err := os.ReadFile(paths.GoogleAccounts)
	if err != nil || string(accounts) != string(oldAccounts) {
		t.Fatal("previous account mirror changed after rollback")
	}
}
