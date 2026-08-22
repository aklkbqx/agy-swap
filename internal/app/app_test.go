package app

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func testPaths(t *testing.T) Paths {
	t.Helper()
	home := t.TempDir()
	config := filepath.Join(home, ".gemini", "agy-swap")
	return Paths{Home: home, ConfigDir: config, Accounts: filepath.Join(config, "accounts.json"), AccountsBackup: filepath.Join(config, "accounts.json.bak"), AccountsLock: filepath.Join(config, ".accounts.lock"), SessionLock: filepath.Join(config, ".session.lock"), LogCache: filepath.Join(config, "log-cache-v1.json"), OAuthToken: filepath.Join(home, ".gemini", "antigravity-cli", "antigravity-oauth-token"), OAuthCredentials: filepath.Join(home, ".gemini", "oauth_creds.json"), GoogleAccounts: filepath.Join(home, ".gemini", "google_accounts.json")}
}

func tokenBlob(t *testing.T, email string, verified bool, refresh string, expiry time.Time) string {
	t.Helper()
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none"}`))
	claims := map[string]any{"iss": "https://accounts.google.com", "email_verified": verified}
	if email != "" {
		claims["email"] = email
	}
	claimData, _ := json.Marshal(claims)
	claim := base64.RawURLEncoding.EncodeToString(claimData)
	inner := map[string]any{"access_token": "access", "refresh_token": refresh, "id_token": header + "." + claim + ".signature"}
	if !expiry.IsZero() {
		inner["expiry"] = isoTime(expiry)
	}
	payload, _ := json.Marshal(map[string]any{"token": inner})
	return "go-keyring-base64:" + base64.StdEncoding.EncodeToString(payload)
}

func TestDecodeTokenAcceptsWrappedAndRawJSON(t *testing.T) {
	wrapped := tokenBlob(t, "user@example.com", true, "refresh", time.Time{})
	if getString(tokenObject(decodeToken(wrapped)), "access_token") != "access" {
		t.Fatal("wrapped token was not decoded")
	}
	raw, _ := base64.StdEncoding.DecodeString(strings.TrimPrefix(wrapped, "go-keyring-base64:"))
	if getString(tokenObject(decodeToken(string(raw))), "access_token") != "access" {
		t.Fatal("raw Windows token was not decoded")
	}
	if decodeToken("go-keyring-base64:not base64") != nil {
		t.Fatal("invalid token accepted")
	}
}

func TestVerifiedGoogleClaimIsStrict(t *testing.T) {
	if got := extractVerifiedEmail(tokenBlob(t, "User@Example.com", true, "r", time.Time{})); got != "user@example.com" {
		t.Fatalf("unexpected claim: %q", got)
	}
	if got := extractVerifiedEmail(tokenBlob(t, "user@example.com", false, "r", time.Time{})); got != "" {
		t.Fatal("unverified email accepted")
	}
}

func TestOAuthClientIDAudienceForms(t *testing.T) {
	known := "884354919052-36trc1jjb3tguiac32ov6cod268c5blh.apps.googleusercontent.com"
	for _, audience := range []any{known, []any{"unknown", known}} {
		header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none"}`))
		claims, _ := json.Marshal(map[string]any{"aud": audience})
		jwt := header + "." + base64.RawURLEncoding.EncodeToString(claims) + ".sig"
		if got := oauthClientID(map[string]any{"token": map[string]any{"id_token": jwt}}); got != known {
			t.Fatalf("audience %v selected %s", audience, got)
		}
	}
	if got := oauthClientID(nil); got != defaultOAuthClientID {
		t.Fatal("default client id not selected")
	}
}

func TestDurationParserStrictAndBounded(t *testing.T) {
	cases := map[string]time.Duration{"4h30m": 4*time.Hour + 30*time.Minute, "6d": 6 * 24 * time.Hour, "90": 90 * time.Minute, "reset": 0, "1h 2m 3s": time.Hour + 2*time.Minute + 3*time.Second}
	for input, want := range cases {
		got, ok := parseDuration(input)
		if !ok || got != want {
			t.Fatalf("%q: got %s, %v", input, got, ok)
		}
	}
	for _, input := range []string{"", "4hours", "8d", "1h junk", "-1"} {
		if _, ok := parseDuration(input); ok {
			t.Fatalf("accepted %q", input)
		}
	}
}

func TestOrderedAccountStorePreservesOrderAndUnknownFields(t *testing.T) {
	paths := testPaths(t)
	store := NewStore(paths)
	data := `{"second@example.com":{"email":"second@example.com","name":"Second","custom":{"x":1}},"first@example.com":{"email":"first@example.com","name":"First","flag":true}}`
	if err := atomicWrite(paths.Accounts, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	accounts, err := store.Load(false)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(accounts.Order, ",") != "second@example.com,first@example.com" {
		t.Fatalf("order changed: %v", accounts.Order)
	}
	if getMap(accounts.ByEmail["second@example.com"]["custom"])["x"] == nil {
		t.Fatal("unknown field lost")
	}
	accounts.ByEmail["first@example.com"]["name"] = "Changed"
	if err := store.Save(accounts); err != nil {
		t.Fatal(err)
	}
	reloaded, err := store.Load(false)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(reloaded.Order, ",") != "second@example.com,first@example.com" {
		t.Fatalf("saved order changed: %v", reloaded.Order)
	}
	if getMap(reloaded.ByEmail["second@example.com"]["custom"])["x"] == nil {
		t.Fatal("unknown field lost after save")
	}
	if runtime.GOOS != "windows" {
		stat, _ := os.Stat(paths.Accounts)
		if stat.Mode().Perm() != 0o600 {
			t.Fatalf("mode = %o", stat.Mode().Perm())
		}
	}
}

func TestStoreFailsClosedOnCorruptionDuplicateAndTokenMismatch(t *testing.T) {
	paths := testPaths(t)
	store := NewStore(paths)
	for _, data := range []string{`[]`, `{"a@example.com":`, `{"A@example.com":{"email":"a@example.com"},"a@example.com":{"email":"a@example.com"}}`} {
		if err := atomicWrite(paths.Accounts, []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := store.Load(false); err == nil {
			t.Fatalf("accepted invalid store %s", data)
		}
	}
	token := tokenBlob(t, "other@example.com", true, "r", time.Time{})
	data := fmt.Sprintf(`{"user@example.com":{"email":"user@example.com","token_data":%q}}`, token)
	if err := atomicWrite(paths.Accounts, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Load(false); err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("mismatched identity accepted: %v", err)
	}
}

func TestStoreRevisionConflictAndBackup(t *testing.T) {
	paths := testPaths(t)
	store := NewStore(paths)
	accounts := NewAccounts()
	accounts.Set("user@example.com", Account{"email": "user@example.com", "name": "User"})
	if err := store.Save(accounts); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.Load(false)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(time.Millisecond)
	if err := os.WriteFile(paths.Accounts, []byte(`{"external@example.com":{"email":"external@example.com"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(loaded); !errors.Is(err, errStoreConflict) {
		t.Fatalf("expected conflict, got %v", err)
	}
	fresh, _ := store.Load(false)
	fresh.ByEmail["external@example.com"]["name"] = "External"
	if err := store.Save(fresh); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(paths.AccountsBackup); err != nil {
		t.Fatal("backup not created")
	}
}

func TestLegacyQuotaMigrationKeepsLegacyData(t *testing.T) {
	accounts := NewAccounts()
	accounts.Set("user@example.com", Account{"email": "user@example.com", "quota_schema": 1, "limit_reset_claude": "old"})
	if !migrateAndExpire(accounts, time.Now()) {
		t.Fatal("migration not reported")
	}
	account := accounts.ByEmail["user@example.com"]
	schema, _ := numberInt(account["quota_schema"])
	if schema != 2 {
		t.Fatalf("schema = %d", schema)
	}
	if getMap(account["legacy_quota"])["limit_reset_claude"] != "old" {
		t.Fatal("legacy data lost")
	}
}

func quotaAccount(email string, gemini, thirdParty float64, reset time.Time) Account {
	return Account{"email": email, "name": email, "quota_snapshot": map[string]any{"observed_at": isoTime(time.Now()), "tier": map[string]any{"id": "free-tier", "name": "Free"}, "groups": []any{map[string]any{"id": "gemini", "name": "Gemini Models", "buckets": []any{map[string]any{"id": "gemini-weekly", "name": "Weekly", "window": "weekly", "remaining_fraction": gemini, "reset_at": isoTime(reset)}}}, map[string]any{"id": "third_party", "name": "Third Party", "buckets": []any{map[string]any{"id": "3p-weekly", "name": "Weekly", "window": "weekly", "remaining_fraction": thirdParty, "reset_at": isoTime(reset)}}}}}}
}

func TestTUILayoutRespondsToTerminalShape(t *testing.T) {
	if got := tuiLayoutFor(120, 30); got != tuiLayoutWide {
		t.Fatalf("wide layout = %v", got)
	}
	if got := tuiLayoutFor(80, 24); got != tuiLayoutStacked {
		t.Fatalf("stacked layout = %v", got)
	}
	if got := tuiLayoutFor(60, 24); got != tuiLayoutCompact {
		t.Fatalf("compact width layout = %v", got)
	}
	if got := tuiLayoutFor(100, 12); got != tuiLayoutCompact {
		t.Fatalf("compact height layout = %v", got)
	}
}

func TestTUIStateSearchSelectionAndReducedMotion(t *testing.T) {
	accounts := NewAccounts()
	accounts.Set("alpha@example.com", Account{"email": "alpha@example.com", "name": "Alpha"})
	accounts.Set("beta@example.com", Account{"email": "beta@example.com", "name": "Beta"})
	state := newTUIState(accounts, "")
	state.selectedEmail = "beta@example.com"
	state.beginSearch()
	state.search = "alp"
	state.clampSelection()
	if got := state.visibleEmails(); len(got) != 1 || got[0] != "alpha@example.com" {
		t.Fatalf("filtered accounts = %v", got)
	}
	state.cancelSearch()
	if state.search != "" || state.selectedEmail != "beta@example.com" {
		t.Fatalf("search cancel changed browse state: search=%q selected=%q", state.search, state.selectedEmail)
	}

	t.Setenv("AGY_SWAP_REDUCED_MOTION", "1")
	reduced := newTUIState(accounts, "")
	reduced.beginAnimation("success", time.Second)
	if reduced.animation.active {
		t.Fatal("reduced-motion state started an animation")
	}
}

func TestTUIRenderProducesOneTerminalLinePerEntry(t *testing.T) {
	accounts := NewAccounts()
	accounts.Set("user@example.com", quotaAccount("user@example.com", 0.85, 0.45, time.Now().Add(time.Hour)))
	a := &Application{Version: "2.1.0", p: makePalette(false), color: false}
	state := newTUIState(accounts, "user@example.com")
	for _, testCase := range []struct {
		inner, height int
	}{
		{inner: 118, height: 30},
		{inner: 78, height: 24},
		{inner: 58, height: 14},
		{inner: 26, height: 12},
	} {
		lines := a.tuiLines(state, testCase.inner, testCase.height)
		if len(lines) == 0 {
			t.Fatal("TUI produced no lines")
		}
		for i, line := range lines {
			if strings.ContainsRune(line, '\n') || strings.ContainsRune(line, '\r') {
				t.Fatalf("inner=%d line %d contains an embedded newline: %q", testCase.inner, i, line)
			}
			if visibleWidth(line) > testCase.inner+2 {
				t.Fatalf("inner=%d line %d exceeds frame width: %d", testCase.inner, i, visibleWidth(line))
			}
		}
	}
}

func TestNextAccountSelectionPreservesUnknownAndCooldownSemantics(t *testing.T) {
	accounts := NewAccounts()
	reset := time.Now().Add(time.Hour)
	accounts.Set("one@example.com", quotaAccount("one@example.com", 0, 0, reset))
	accounts.Set("two@example.com", quotaAccount("two@example.com", 1, 0, reset))
	accounts.Set("three@example.com", Account{"email": "three@example.com", "name": "Three"})
	selected, state := selectNext(accounts, "one@example.com", "gemini")
	if getString(selected, "email") != "two@example.com" || state != 0 {
		t.Fatalf("selected %s state %d", getString(selected, "email"), state)
	}
	selected, state = selectNext(accounts, "one@example.com", "claude")
	if getString(selected, "email") != "three@example.com" || state != -1 {
		t.Fatalf("unknown fallback selected %s state %d", getString(selected, "email"), state)
	}
}

func TestQuotaFetchRoutesGroupsAndReusesAccessToken(t *testing.T) {
	var oauthCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/oauth":
			oauthCalls.Add(1)
			fmt.Fprint(w, `{"access_token":"refreshed"}`)
		case "/loadCodeAssist":
			fmt.Fprint(w, `{"cloudaicompanionProject":"p","paidTier":{"id":"g1-pro-tier"}}`)
		case "/retrieveUserQuotaSummary":
			fmt.Fprint(w, `{"groups":[{"displayName":"Gemini","buckets":[{"bucketId":"gemini-weekly","displayName":"Weekly","window":"weekly","remainingFraction":0.4,"resetTime":"2030-01-01T00:00:00Z"}]},{"displayName":"Third Party","buckets":[{"bucketId":"3p-weekly","displayName":"Weekly","window":"weekly","remainingFraction":0.2,"resetTime":"2030-01-01T00:00:00Z"}]}]}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	httpService := NewHTTPService(io.Discard)
	httpService.oauthURL = server.URL + "/oauth"
	httpService.cloudAPI = server.URL + "/"
	quota := NewQuotaService(httpService, nil)
	account := Account{"email": "user@example.com", "token_data": tokenBlob(t, "user@example.com", true, "r", time.Now().Add(time.Hour))}
	snapshot, err := quota.Fetch(context.Background(), account)
	if err != nil {
		t.Fatal(err)
	}
	if oauthCalls.Load() != 0 {
		t.Fatal("valid access token was unnecessarily refreshed")
	}
	groups := getSlice(snapshot["groups"])
	if len(groups) != 2 || getString(getMap(groups[0]), "id") != "gemini" || getString(getMap(groups[1]), "id") != "third_party" {
		t.Fatalf("groups = %#v", groups)
	}
}

func TestQuotaRefreshIsConcurrentAndKeepsCachedFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(40 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "loadCodeAssist") {
			fmt.Fprint(w, `{"cloudaicompanionProject":"p","currentTier":{"id":"free-tier"}}`)
		} else {
			fmt.Fprint(w, `{"groups":[{"buckets":[{"bucketId":"gemini-weekly","displayName":"Weekly","window":"weekly","remainingFraction":0.5,"resetTime":"2030-01-01T00:00:00Z"}]}]}`)
		}
	}))
	defer server.Close()
	paths := testPaths(t)
	store := NewStore(paths)
	httpService := NewHTTPService(io.Discard)
	httpService.cloudAPI = server.URL + "/"
	service := NewQuotaService(httpService, store)
	accounts := NewAccounts()
	for i := 0; i < 8; i++ {
		email := fmt.Sprintf("user%d@example.com", i)
		accounts.Set(email, Account{"email": email, "name": email, "token_data": tokenBlob(t, email, true, "r", time.Now().Add(time.Hour))})
	}
	start := time.Now()
	failures := service.Refresh(context.Background(), accounts, true, nil)
	if len(failures) != 0 {
		t.Fatalf("failures: %v", failures)
	}
	if elapsed := time.Since(start); elapsed > 250*time.Millisecond {
		t.Fatalf("refresh not concurrent: %s", elapsed)
	}
	cached := accounts.ByEmail[accounts.Order[0]]["quota_snapshot"]
	httpService.cloudAPI = "http://127.0.0.1:1/"
	failures = service.Refresh(context.Background(), accounts, true, nil)
	if len(failures) != 8 {
		t.Fatalf("expected failures, got %d", len(failures))
	}
	if !reflect.DeepEqual(accounts.ByEmail[accounts.Order[0]]["quota_snapshot"], cached) {
		t.Fatal("cached snapshot was replaced on failure")
	}
}

func TestStrictTLSAndExplicitInsecureFallback(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, `{"ok":true}`) }))
	defer server.Close()
	var stderr bytes.Buffer
	service := NewHTTPService(&stderr)
	if _, _, err := service.jsonRequest(context.Background(), http.MethodGet, server.URL, nil, nil, time.Second, 1024); err == nil {
		t.Fatal("strict TLS accepted self-signed server")
	}
	t.Setenv("AGY_SWAP_INSECURE_TLS", "1")
	result, _, err := service.jsonRequest(context.Background(), http.MethodGet, server.URL, nil, nil, time.Second, 1024)
	if err != nil || result["ok"] != true {
		t.Fatalf("insecure opt-in failed: %v %#v", err, result)
	}
	if !strings.Contains(stderr.String(), "verification is disabled") {
		t.Fatal("missing insecure warning")
	}
}

func TestLogScannerDetectsLimitsAndUsesPersistentCache(t *testing.T) {
	paths := testPaths(t)
	logDir := filepath.Dir(filepath.Join(paths.Home, ".gemini", "antigravity-cli", "cli.log"))
	if err := os.MkdirAll(logDir, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(logDir, "cli.log")
	now := time.Now()
	prefix := fmt.Sprintf("I%02d%02d %02d:%02d:%02d", int(now.Month()), now.Day(), now.Hour(), now.Minute(), now.Second())
	content := strings.Join([]string{prefix + " applyAuthResult: email=user@example.com", prefix + ` resolving model claude-sonnet-4`, prefix + ` label = "Claude Sonnet 4"`, prefix + " RESOURCE_EXHAUSTED Resets in 1h"}, "\n") + "\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	scanner := NewLogScanner(paths)
	limits, evidence, err := scanner.Scan()
	if err != nil {
		t.Fatal(err)
	}
	if limits["user@example.com"] == nil || len(evidence["user@example.com"]) != 1 {
		t.Fatalf("not detected: %#v %#v", limits, evidence)
	}
	cacheStat, err := os.Stat(paths.LogCache)
	if err != nil {
		t.Fatal("persistent cache not created")
	}
	if _, _, err := scanner.Scan(); err != nil {
		t.Fatal(err)
	}
	cacheStat2, _ := os.Stat(paths.LogCache)
	if cacheStat2.ModTime() != cacheStat.ModTime() {
		t.Fatal("unchanged cache was rewritten")
	}
}

func TestNewerSuccessfulLogEventClearsOlderCooldown(t *testing.T) {
	paths := testPaths(t)
	path := filepath.Join(paths.Home, ".gemini", "antigravity-cli", "cli.log")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	stamp := func(offset time.Duration) string {
		v := now.Add(offset)
		return fmt.Sprintf("I%02d%02d %02d:%02d:%02d", int(v.Month()), v.Day(), v.Hour(), v.Minute(), v.Second())
	}
	id := "123e4567-e89b-12d3-a456-426614174000"
	content := strings.Join([]string{stamp(-2*time.Minute) + " applyAuthResult: email=user@example.com", stamp(-2*time.Minute) + ` label = "Claude Sonnet 4"`, stamp(-2*time.Minute) + " RESOURCE_EXHAUSTED Resets in 1h", stamp(-time.Minute) + " Sending user message to conversation " + id, stamp(-time.Minute) + " streamGenerateContent ResponseID: ok", stamp(0) + " Stream completed for " + id}, "\n") + "\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	limits, evidence, err := NewLogScanner(paths).Scan()
	if err != nil {
		t.Fatal(err)
	}
	if len(limits["user@example.com"]) != 0 {
		t.Fatalf("old limit not cleared: %#v", limits)
	}
	for _, event := range evidence["user@example.com"] {
		if event.State != "available" {
			t.Fatalf("state = %s", event.State)
		}
	}
}

type fakeCredentialBackend struct {
	token   string
	failSet bool
}

func (f *fakeCredentialBackend) Get(context.Context) string { return f.token }
func (f *fakeCredentialBackend) Set(_ context.Context, value string) bool {
	if f.failSet {
		return false
	}
	f.token = value
	return true
}
func (f *fakeCredentialBackend) Delete(context.Context) bool { f.token = ""; return true }

func TestCredentialFilesAndTransactionalRollback(t *testing.T) {
	paths := testPaths(t)
	backend := &fakeCredentialBackend{token: "previous"}
	credentials := NewCredentials(paths)
	credentials.backend = backend
	token := tokenBlob(t, "user@example.com", true, "r", time.Now().Add(time.Hour))
	if !credentials.Apply(context.Background(), token, "user@example.com") {
		t.Fatal("apply failed")
	}
	if backend.token != token {
		t.Fatal("secure credential not updated")
	}
	for _, path := range []string{paths.OAuthToken, paths.OAuthCredentials, paths.GoogleAccounts} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("missing %s", path)
		}
	}
	previous := backend.token
	if err := os.Remove(paths.OAuthCredentials); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(paths.OAuthCredentials, 0o700); err != nil {
		t.Fatal(err)
	}
	other := tokenBlob(t, "other@example.com", true, "r", time.Now().Add(time.Hour))
	if credentials.Apply(context.Background(), other, "other@example.com") {
		t.Fatal("partial write reported success")
	}
	if backend.token != previous {
		t.Fatal("secure credential was not rolled back")
	}
}

func TestCLIParsingLegacyAndSubcommands(t *testing.T) {
	cases := []struct {
		argv                     []string
		command, account, family string
		refresh                  bool
	}{{[]string{"switch", "2"}, "switch", "2", "", false}, {[]string{"next", "--family", "claude"}, "next", "", "claude", false}, {[]string{"limits", "--refresh", "--verbose"}, "limits", "", "", true}, {[]string{"limit", "set", "1", "6d", "--group", "gpt"}, "limit", "1", "", false}}
	for _, tc := range cases {
		parsed, err := parseCLI(tc.argv)
		if err != nil {
			t.Fatal(err)
		}
		if parsed.command != tc.command || parsed.account != tc.account || parsed.family != tc.family || parsed.refresh != tc.refresh {
			t.Fatalf("%v -> %#v", tc.argv, parsed)
		}
	}
	if _, err := parseCLI([]string{"add", "--token", "secret"}); err == nil {
		t.Fatal("inline token accepted")
	}
}

func TestTerminalKeysAndDisplayWidth(t *testing.T) {
	cases := map[string]string{"\x1b[A": "up", "\x1b[B": "down", "\x1b[3~": "delete", "\r": "enter", "\x7f": "backspace"}
	for input, want := range cases {
		if got := readTerminalKey(bytes.NewBufferString(input)); got != want {
			t.Fatalf("%q -> %q", input, got)
		}
	}
	if visibleWidth("A界e\u0301") != 4 {
		t.Fatalf("width = %d", visibleWidth("A界e\u0301"))
	}
}

func TestExpectedChecksum(t *testing.T) {
	name := "agy-swap_v2.0.0_linux_amd64"
	sum := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	got, err := expectedChecksum([]byte(sum+"  "+name+"\n"), name)
	if err != nil || got != sum {
		t.Fatalf("got %q, %v", got, err)
	}
	if _, err := expectedChecksum([]byte("bad"), name); err == nil {
		t.Fatal("invalid checksum accepted")
	}
}

func BenchmarkDecodeAndValidate100Accounts(b *testing.B) {
	var buffer strings.Builder
	buffer.WriteByte('{')
	for i := 0; i < 100; i++ {
		if i > 0 {
			buffer.WriteByte(',')
		}
		fmt.Fprintf(&buffer, "%q:{\"email\":%q,\"name\":%q}", fmt.Sprintf("user%03d@example.com", i), fmt.Sprintf("user%03d@example.com", i), fmt.Sprintf("User %d", i))
	}
	buffer.WriteByte('}')
	data := []byte(buffer.String())
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := decodeOrderedAccounts(data); err != nil {
			b.Fatal(err)
		}
	}
}

func benchmarkLogScanner(b *testing.B) *LogScanner {
	b.Helper()
	home := b.TempDir()
	logDir := filepath.Join(home, ".gemini", "antigravity-cli", "logs")
	if err := os.MkdirAll(logDir, 0o700); err != nil {
		b.Fatal(err)
	}
	chunk := bytes.Repeat([]byte("2026-08-23T10:00:00Z ordinary diagnostic message without quota evidence\n"), 1024)
	for fileIndex := 0; fileIndex < 8; fileIndex++ {
		file, err := os.Create(filepath.Join(logDir, fmt.Sprintf("bench-%d.log", fileIndex)))
		if err != nil {
			b.Fatal(err)
		}
		for written := 0; written < 8*1024*1024; written += len(chunk) {
			remaining := min(len(chunk), 8*1024*1024-written)
			if _, err := file.Write(chunk[:remaining]); err != nil {
				b.Fatal(err)
			}
		}
		if err := file.Close(); err != nil {
			b.Fatal(err)
		}
	}
	config := filepath.Join(home, ".gemini", "agy-swap")
	paths := Paths{Home: home, ConfigDir: config, LogCache: filepath.Join(config, "log-cache-v1.json")}
	return NewLogScanner(paths)
}

func BenchmarkLogScan64MBCold(b *testing.B) {
	scanner := benchmarkLogScanner(b)
	b.SetBytes(logTotalBytes)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := os.Remove(scanner.paths.LogCache); err != nil && !errors.Is(err, os.ErrNotExist) {
			b.Fatal(err)
		}
		if _, _, err := scanner.Scan(); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkLogScan64MBCached(b *testing.B) {
	scanner := benchmarkLogScanner(b)
	if _, _, err := scanner.Scan(); err != nil {
		b.Fatal(err)
	}
	b.SetBytes(logTotalBytes)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, _, err := scanner.Scan(); err != nil {
			b.Fatal(err)
		}
	}
}
