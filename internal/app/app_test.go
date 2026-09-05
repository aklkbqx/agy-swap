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
	return Paths{Home: home, ConfigDir: config, Accounts: filepath.Join(config, "accounts.json"), AccountsBackup: filepath.Join(config, "accounts.json.bak"), AccountsLock: filepath.Join(config, ".accounts.lock"), SessionLock: filepath.Join(config, ".session.lock"), LogCache: filepath.Join(config, "log-cache-v1.json"), Settings: filepath.Join(config, "config.json"), History: filepath.Join(config, "history-v1.jsonl"), RuntimeState: filepath.Join(config, "runtime-state.json"), JournalDir: filepath.Join(config, "journals"), OAuthToken: filepath.Join(home, ".gemini", "antigravity-cli", "antigravity-oauth-token"), OAuthCredentials: filepath.Join(home, ".gemini", "oauth_creds.json"), GoogleAccounts: filepath.Join(home, ".gemini", "google_accounts.json")}
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

func TestReadLinePreservesBufferedInput(t *testing.T) {
	var out bytes.Buffer
	a := &Application{In: strings.NewReader("first\nsecond\n"), Out: &out}
	if got := a.readLine(""); got != "first" {
		t.Fatalf("first line = %q", got)
	}
	if got := a.readLine(""); got != "second" {
		t.Fatalf("second line = %q", got)
	}
}

func TestTokenIdentityMatchesTargetAccount(t *testing.T) {
	matching := tokenBlob(t, "User@Example.com", true, "r", time.Time{})
	if !tokenMatchesEmail(matching, "user@example.com") {
		t.Fatal("matching verified email was rejected")
	}
	if tokenMatchesEmail(matching, "other@example.com") {
		t.Fatal("mismatched verified email was accepted")
	}
	unverified := tokenBlob(t, "other@example.com", false, "r", time.Time{})
	if !tokenMatchesEmail(unverified, "user@example.com") {
		t.Fatal("token without a verified email claim was rejected")
	}
}

func TestNormalizedReleaseTag(t *testing.T) {
	for input, want := range map[string]string{"2.1.3": "v2.1.3", "v2.1.3": "v2.1.3", " 2.1.3 ": "v2.1.3", "": ""} {
		if got := normalizedReleaseTag(input); got != want {
			t.Fatalf("%q normalized to %q, want %q", input, got, want)
		}
	}
}

func TestParseYesNoRequiresAnExplicitAnswerWhenDefaultIsNo(t *testing.T) {
	for _, input := range []string{"", "n", "no"} {
		if answer, valid := parseYesNo(input, false); !valid || answer {
			t.Fatalf("%q parsed as answer=%v valid=%v", input, answer, valid)
		}
	}
	for _, input := range []string{"y", "yes", " Y "} {
		if answer, valid := parseYesNo(input, false); !valid || !answer {
			t.Fatalf("%q parsed as answer=%v valid=%v", input, answer, valid)
		}
	}
	if answer, valid := parseYesNo("maybe", false); valid || answer {
		t.Fatalf("invalid confirmation accepted: answer=%v valid=%v", answer, valid)
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

func TestAccountAvailabilityKeepsFamiliesIndependent(t *testing.T) {
	reset := time.Now().Add(3 * time.Hour)
	account := quotaAccount("user@example.com", 0.9584, 0, reset)
	p := makePalette(false)
	if got := accountHealthCompact(account, time.Now()); got != "Gemini 96% ready" {
		t.Fatalf("compact health = %q, want Gemini ready state", got)
	}
	status := accountStatus(account, p, time.Now())
	if !strings.Contains(status, "Ready") || !strings.Contains(status, "Gemini") || !strings.Contains(status, "Claude/GPT limited") {
		t.Fatalf("family-aware status = %q", status)
	}

	allLimited := quotaAccount("limited@example.com", 0, 0, reset)
	if got := accountHealthCompact(allLimited, time.Now()); got != "limited" {
		t.Fatalf("all-limited compact health = %q", got)
	}
	if status := accountStatus(allLimited, p, time.Now()); !strings.Contains(status, "Limited") {
		t.Fatalf("all-limited status = %q", status)
	}
}

func TestTUILayoutRespondsToTerminalShape(t *testing.T) {
	if got := tuiLayoutFor(120, 30); got != tuiLayoutWide {
		t.Fatalf("wide layout = %v", got)
	}
	if got := tuiLayoutFor(92, 18); got != tuiLayoutWide {
		t.Fatalf("wide boundary layout = %v", got)
	}
	if got := tuiLayoutFor(91, 18); got != tuiLayoutStacked {
		t.Fatalf("stacked boundary layout = %v", got)
	}
	if got := tuiLayoutFor(80, 24); got != tuiLayoutStacked {
		t.Fatalf("stacked layout = %v", got)
	}
	if got := tuiLayoutFor(64, 16); got != tuiLayoutStacked {
		t.Fatalf("stacked minimum layout = %v", got)
	}
	if got := tuiLayoutFor(60, 24); got != tuiLayoutCompact {
		t.Fatalf("compact width layout = %v", got)
	}
	if got := tuiLayoutFor(63, 16); got != tuiLayoutCompact {
		t.Fatalf("compact boundary layout = %v", got)
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
		{inner: 90, height: 18},
		{inner: 62, height: 24},
		{inner: 58, height: 14},
		{inner: 38, height: 20},
		{inner: 26, height: 12},
	} {
		lines := a.tuiLines(state, testCase.inner, testCase.height)
		if len(lines) != testCase.height {
			t.Fatalf("inner=%d height=%d produced %d lines", testCase.inner, testCase.height, len(lines))
		}
		for i, line := range lines {
			if strings.ContainsRune(line, '\n') || strings.ContainsRune(line, '\r') {
				t.Fatalf("inner=%d line %d contains an embedded newline: %q", testCase.inner, i, line)
			}
			if visibleWidth(line) != testCase.inner+2 {
				t.Fatalf("inner=%d line %d width=%d want=%d: %q", testCase.inner, i, visibleWidth(line), testCase.inner+2, line)
			}
		}
	}
}

func TestTUITopLineShowsCreditDuringSync(t *testing.T) {
	accounts := NewAccounts()
	accounts.Set("user@example.com", quotaAccount("user@example.com", 0.85, 0.45, time.Now().Add(time.Hour)))
	a := &Application{Version: "2.1.1", p: makePalette(false), color: false}
	state := newTUIState(accounts, "user@example.com")
	for _, refreshing := range []bool{false, true} {
		state.refreshing = refreshing
		lines := a.tuiTopLines(state, 118)
		if !strings.Contains(strings.Join(lines, "\n"), tuiCredit) {
			t.Fatalf("refreshing=%v top line missing %q: %q", refreshing, tuiCredit, lines)
		}
	}
}

func TestTUIResponsiveRenderersShareVisualContract(t *testing.T) {
	accounts := NewAccounts()
	accounts.Set("user@example.com", quotaAccount("user@example.com", 0.85, 0.45, time.Now().Add(time.Hour)))
	a := &Application{Version: "2.1.1", p: makePalette(false), color: false}
	state := newTUIState(accounts, "user@example.com")
	stacked := a.tuiAccountRows(state, 78, 2)
	wide := a.tuiAccountTableRows(state, 56, 4)
	for name, rows := range map[string][]string{"stacked": stacked, "wide": wide} {
		joined := strings.Join(rows, "\n")
		for _, want := range []string{">", "[US]", "Gemini 85% ready"} {
			if !strings.Contains(joined, want) {
				t.Fatalf("%s renderer missing %q: %q", name, want, rows)
			}
		}
	}
	stackedDetail := a.tuiDetailLines(state, 62, 12)
	wideDetail := a.tuiDetailTableLines(state, 62, 12)
	for _, want := range []string{"user@example.com", "STATUS", "Gemini Models", "WEEKLY"} {
		if !strings.Contains(strings.Join(stackedDetail, "\n"), want) || !strings.Contains(strings.Join(wideDetail, "\n"), want) {
			t.Fatalf("detail renderers lost shared field %q:\nstacked=%q\nwide=%q", want, stackedDetail, wideDetail)
		}
	}
	if len(stackedDetail) > 12 || len(wideDetail) > 12 {
		t.Fatalf("detail renderers exceeded row budget: stacked=%d wide=%d", len(stackedDetail), len(wideDetail))
	}
}

func TestTUIResponsiveRenderersShareColorTokens(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	accounts := NewAccounts()
	accounts.Set("user@example.com", quotaAccount("user@example.com", 0.85, 0.45, time.Now().Add(time.Hour)))
	p := makePalette(true)
	a := &Application{Version: "2.1.1", p: p, color: true}
	state := newTUIState(accounts, "user@example.com")
	state.active = "user@example.com"
	stacked := strings.Join(a.tuiAccountRows(state, 78, 2), "\n") + "\n" + strings.Join(a.tuiDetailLines(state, 62, 12), "\n")
	wide := strings.Join(a.tuiAccountTableRows(state, 56, 4), "\n") + "\n" + strings.Join(a.tuiDetailTableLines(state, 62, 12), "\n")
	for name, rendered := range map[string]string{"stacked": stacked, "wide": wide} {
		for token, want := range map[string]string{"orange": p.Orange, "green": p.Green, "blue": p.Blue, "cyan": p.Cyan} {
			if want == "" || !strings.Contains(rendered, want) {
				t.Fatalf("%s renderer missing %s semantic token", name, token)
			}
		}
	}
}

func TestTUIResponsiveDetailPreservesResetWindow(t *testing.T) {
	reset := time.Now().Add(6*24*time.Hour + 23*time.Hour + 59*time.Minute)
	bucket := map[string]any{"remaining_fraction": 1.0, "reset_at": isoTime(reset)}
	compact := formatQuotaBarResponsive(bucket, makePalette(false), time.Now(), 18, 40)
	if !strings.Contains(compact, "· 6d 23h") {
		t.Fatalf("compact quota value lost reset window: %q", compact)
	}
	minimum := formatQuotaBarResponsive(bucket, makePalette(false), time.Now(), 18, 24)
	if !strings.Contains(minimum, "100%") {
		t.Fatalf("minimum quota value lost percentage: %q", minimum)
	}
}

func TestTUICompactTallViewportKeepsSelectedDetail(t *testing.T) {
	accounts := NewAccounts()
	accounts.Set("user@example.com", quotaAccount("user@example.com", 0.85, 0.45, time.Now().Add(time.Hour)))
	a := &Application{Version: "2.1.1", p: makePalette(false), color: false}
	state := newTUIState(accounts, "user@example.com")
	lines := a.tuiLines(state, 58, 24)
	joined := strings.Join(lines, "\n")
	for _, want := range []string{"ACCOUNT HEALTH", "STATUS", "Gemini Models"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("compact tall viewport missing %q: %q", want, lines)
		}
	}
}

func TestTUIWideAccountTableUsesStableColumns(t *testing.T) {
	accounts := NewAccounts()
	alpha := quotaAccount("alpha@example.com", 1, 0.4, time.Now().Add(time.Hour))
	alpha["name"] = "Alpha"
	accounts.Set("alpha@example.com", alpha)
	accounts.Set("beta@example.com", Account{"email": "beta@example.com", "name": "Beta"})
	a := &Application{Version: "2.1.1", p: makePalette(false), color: false}
	state := newTUIState(accounts, "")
	rows := a.tuiAccountTableRows(state, 56, 8)
	if len(rows) != 4 { // header, rule, and two account rows
		t.Fatalf("rows = %d, want 4: %q", len(rows), rows)
	}
	if !strings.Contains(rows[0], "ACCOUNT") || !strings.Contains(rows[0], "HEALTH") {
		t.Fatalf("missing table headers: %q", rows[0])
	}
	if !strings.Contains(rows[2], "> · [AL]") || !strings.Contains(rows[2], "Alpha") {
		t.Fatalf("selected row lost its identity: %q", rows[2])
	}
	for i, row := range rows {
		if visibleWidth(row) != 56 {
			t.Fatalf("row %d width=%d want=56: %q", i, visibleWidth(row), row)
		}
		if strings.ContainsRune(row, '\uFFFD') {
			t.Fatalf("row %d contains replacement glyph: %q", i, row)
		}
	}
}

func TestTUIAccountTableSurvivesInvalidUTF8(t *testing.T) {
	accounts := NewAccounts()
	accounts.Set("bad@example.com", Account{"email": "bad@example.com", "name": string([]byte{'B', 0xff, 'd'})})
	a := &Application{Version: "2.1.1", p: makePalette(false), color: false}
	state := newTUIState(accounts, "")
	rows := a.tuiAccountTableRows(state, 42, 5)
	for i, row := range rows {
		if strings.ContainsRune(row, '\uFFFD') {
			t.Fatalf("row %d contains replacement glyph: %q", i, row)
		}
	}
	if got := visibleWidth(avatar("Akalak Kruaboon", "user@example.com", false)); got != 4 {
		t.Fatalf("avatar width=%d want=4", got)
	}
}

func TestTUIDetailTableUsesKeyValueColumns(t *testing.T) {
	accounts := NewAccounts()
	accounts.Set("user@example.com", quotaAccount("user@example.com", 0.85, 0.45, time.Now().Add(time.Hour)))
	a := &Application{Version: "2.1.1", p: makePalette(false), color: false}
	state := newTUIState(accounts, "user@example.com")
	rows := a.tuiDetailTableLines(state, 52, 12)
	statusRow := ""
	for _, row := range rows {
		if strings.Contains(row, "STATUS") {
			statusRow = row
			break
		}
	}
	if statusRow == "" || !strings.Contains(statusRow, "│") {
		t.Fatalf("detail table missing status column: %q", rows)
	}
	for i, row := range rows {
		if visibleWidth(row) != 52 {
			t.Fatalf("row %d width=%d want=52: %q", i, visibleWidth(row), row)
		}
	}
}

func TestTUIDetailTableOmitsVaultStorageRow(t *testing.T) {
	accounts := NewAccounts()
	account := quotaAccount("user@example.com", 0.85, 0.45, time.Now().Add(time.Hour))
	account["secret_ref"] = "account:user@example.com"
	groups := getSlice(getMap(account["quota_snapshot"])["groups"])
	buckets := getSlice(getMap(groups[0])["buckets"])
	getMap(buckets[0])["name"] = "Five Hour Limit Remaining"
	accounts.Set("user@example.com", account)
	a := &Application{Version: "2.1.1", p: makePalette(false), color: false}
	state := newTUIState(accounts, "user@example.com")
	rows := a.tuiDetailTableLines(state, 52, 12)
	detail := strings.Join(rows, "\n")

	if strings.Contains(detail, "SESSION TOKEN") || strings.Contains(detail, "Stored in OS vault") {
		t.Fatalf("detail table exposed vault storage row: %q", rows)
	}
	if !strings.Contains(detail, "FIVE HOUR LIMIT") {
		t.Fatalf("detail table lost quota rows while removing vault storage row: %q", rows)
	}
}

func TestTUIOverlayKeepsFrameGeometry(t *testing.T) {
	accounts := NewAccounts()
	accounts.Set("user@example.com", quotaAccount("user@example.com", 0.85, 0.45, time.Now().Add(time.Hour)))
	a := &Application{Version: "2.1.1", p: makePalette(false), color: false}
	for _, mode := range []tuiMode{tuiHelp, tuiConfirmDelete} {
		state := newTUIState(accounts, "user@example.com")
		state.mode = mode
		state.confirmEmail = "user@example.com"
		lines := a.tuiLines(state, 118, 30)
		if len(lines) != 30 {
			t.Fatalf("mode=%d produced %d lines", mode, len(lines))
		}
		for i, line := range lines {
			if visibleWidth(line) != 120 {
				t.Fatalf("mode=%d line=%d width=%d", mode, i, visibleWidth(line))
			}
		}
	}
}

func TestTUISuccessToastKeepsFrameGeometryAndExpires(t *testing.T) {
	accounts := NewAccounts()
	accounts.Set("user@example.com", quotaAccount("user@example.com", 0.85, 0.45, time.Now().Add(time.Hour)))
	a := &Application{Version: "2.1.3", p: makePalette(false), color: false}
	state := newTUIState(accounts, "user@example.com")
	state.showToast("Switched to user@example.com", "success")

	lines := a.tuiLines(state, 78, 24)
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "✓ Switched to user@example.com") {
		t.Fatalf("success toast missing: %q", joined)
	}
	for i, line := range lines {
		if visibleWidth(line) != 80 {
			t.Fatalf("line %d width=%d want=80: %q", i, visibleWidth(line), line)
		}
	}

	if !state.expireToast(time.Now().Add(tuiToastDuration + time.Second)) {
		t.Fatal("expired toast was not cleared")
	}
	if state.toastActive(time.Now()) {
		t.Fatal("expired toast is still active")
	}
}

func TestTUIViewsPaletteFormsAndJobsKeepFrameGeometry(t *testing.T) {
	accounts := NewAccounts()
	accounts.Set("user@example.com", quotaAccount("user@example.com", 0.85, 0.45, time.Now().Add(time.Hour)))
	a := &Application{Version: "2.1.1", p: makePalette(false), color: false}
	state := newTUIState(accounts, "user@example.com")
	state.settings = defaultSettings()
	state.settingsLoaded = true
	state.settings.Profiles["work"] = Profile{Account: "user@example.com", Family: "gemini", Policy: "sticky"}
	state.profileNames = []string{"work"}
	state.history = []historyEvent{{At: isoTime(time.Now()), Kind: "switch", Email: "user@example.com"}}
	state.doctorChecks = []doctorCheck{{Name: "config", Status: "ok", Message: "ready"}}
	state.doctorHealthy = true
	state.backupPath = "backup.json"

	views := []tuiView{tuiViewDashboard, tuiViewQuota, tuiViewProfiles, tuiViewHistory, tuiViewSettings, tuiViewDoctor, tuiViewBackup}
	for _, view := range views {
		state.view = view
		state.mode = tuiBrowse
		lines := a.tuiLines(state, 78, 24)
		if len(lines) != 24 {
			t.Fatalf("view=%d produced %d lines", view, len(lines))
		}
		for index, line := range lines {
			if visibleWidth(line) != 80 {
				t.Fatalf("view=%d line=%d width=%d want=80", view, index, visibleWidth(line))
			}
		}
	}

	state.beginPalette()
	state.paletteQuery = "backup"
	state.view = tuiViewBackup
	items := state.paletteActions()
	if len(items) == 0 || items[0].ID != "backup" {
		t.Fatalf("backup palette filter = %#v", items)
	}
	if _, ok := state.selectedPaletteAction(); !ok {
		t.Fatal("selected backup action should be runnable")
	}
	state.form = &tuiFormState{Kind: "settings", Index: 1, Fields: []tuiFormField{{Key: "policy", Value: "sticky"}, {Key: "family", Options: []string{"", "gemini"}}}}
	state.mode = tuiForm
	if submit, cancel := state.formKey("right"); submit || cancel || state.form.Fields[1].Value != "gemini" {
		t.Fatalf("choice form key = submit=%v cancel=%v fields=%#v", submit, cancel, state.form.Fields)
	}
	if submit, cancel := state.formKey("esc"); submit || !cancel {
		t.Fatalf("form escape = submit=%v cancel=%v", submit, cancel)
	}
	foundUpdate := false
	for _, action := range tuiActions(state) {
		if action.ID == "update" {
			foundUpdate = action.Enabled && action.Shortcut == "u"
		}
	}
	if !foundUpdate {
		t.Fatal("TUI action palette does not expose the update action")
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
	sets    int
}

func (f *fakeCredentialBackend) Get(context.Context) string { return f.token }
func (f *fakeCredentialBackend) Set(_ context.Context, value string) bool {
	f.sets++
	if f.failSet {
		return false
	}
	f.token = value
	return true
}
func (f *fakeCredentialBackend) Delete(context.Context) bool { f.token = ""; return true }

func TestStoredActiveEmailHint(t *testing.T) {
	paths := testPaths(t)
	if err := os.MkdirAll(filepath.Dir(paths.GoogleAccounts), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := atomicWrite(paths.GoogleAccounts, []byte(`{"active":"User@Example.com","old":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := NewCredentials(paths).StoredActiveEmail(); got != "user@example.com" {
		t.Fatalf("stored active email = %q", got)
	}
}

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
	setCount := backend.sets
	if !credentials.Apply(context.Background(), token, "user@example.com") {
		t.Fatal("reapplying the active credential failed")
	}
	if backend.sets != setCount {
		t.Fatal("reapplying the active credential rewrote the keychain")
	}
	for _, path := range []string{paths.OAuthToken, paths.OAuthCredentials, paths.GoogleAccounts} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("missing %s", path)
		}
	}
	previous := backend.token
	other := tokenBlob(t, "other@example.com", true, "r", time.Now().Add(time.Hour))
	if credentials.Apply(context.Background(), other, "user@example.com") {
		t.Fatal("token for another verified account was accepted")
	}
	if backend.token != previous {
		t.Fatal("identity-mismatched token changed the secure credential")
	}
	if err := os.Remove(paths.OAuthCredentials); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(paths.OAuthCredentials, 0o700); err != nil {
		t.Fatal(err)
	}
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
	parsed, err := parseCLI([]string{"version"})
	if err != nil || parsed.command != "version" {
		t.Fatalf("version command parsed as %#v, err=%v", parsed, err)
	}
	if _, err := parseCLI([]string{"version", "extra"}); err == nil {
		t.Fatal("unexpected positional argument accepted by version command")
	}
	if _, err := parseCLI([]string{"add", "--token", "secret"}); err == nil {
		t.Fatal("inline token accepted")
	}
}

type fakeAccountVault map[string]string

func (f fakeAccountVault) Get(_ context.Context, ref string) (string, bool) {
	value, ok := f[ref]
	return value, ok
}
func (f fakeAccountVault) Set(_ context.Context, ref, token string) bool { f[ref] = token; return true }
func (f fakeAccountVault) Delete(_ context.Context, ref string) bool     { delete(f, ref); return true }

func TestExtendedSettingsAliasesAndEncryptedBackup(t *testing.T) {
	paths := testPaths(t)
	store := NewStore(paths)
	token := tokenBlob(t, "user@example.com", true, "refresh", time.Now().Add(time.Hour))
	accounts := NewAccounts()
	accounts.Set("user@example.com", newAccount("user@example.com", "User", token))
	if err := store.Save(accounts); err != nil {
		t.Fatal(err)
	}
	var out, errOut bytes.Buffer
	a := &Application{Version: "2.1.3", In: strings.NewReader(""), Out: &out, Err: &errOut, paths: paths, store: store, vault: fakeAccountVault{}, p: makePalette(false)}
	if code := a.Run(context.Background(), []string{"config", "set", "policy.min_remaining_pct", "25"}); code != 0 {
		t.Fatalf("config set code=%d err=%s", code, errOut.String())
	}
	if code := a.Run(context.Background(), []string{"alias", "set", "work", "user@example.com"}); code != 0 {
		t.Fatalf("alias set code=%d err=%s", code, errOut.String())
	}
	settings, err := store.LoadSettings()
	if err != nil || settings.Policy.MinRemainingPct != 25 || settings.Aliases["work"] != "user@example.com" {
		t.Fatalf("settings not persisted: %#v err=%v", settings, err)
	}
	resolved, err := resolveConfiguredTarget("work", accounts, settings)
	if err != nil || resolved != "user@example.com" {
		t.Fatalf("alias resolution = %q, %v", resolved, err)
	}
	metadata, err := a.backupDocument(context.Background(), false)
	if err != nil || strings.Contains(string(metadata), "token_data") || strings.Contains(string(metadata), token) {
		t.Fatalf("metadata backup leaked token: %v %s", err, metadata)
	}
	secret, err := a.backupDocument(context.Background(), true)
	if err != nil || !strings.Contains(string(secret), "token_data") {
		t.Fatalf("secret backup did not include token: %v", err)
	}
	envelope, err := encryptBackup("long-test-passphrase", secret)
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(envelope)
	var decoded encryptedBackup
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	plaintext, err := decryptBackup("long-test-passphrase", decoded)
	if err != nil || !bytes.Equal(plaintext, secret) {
		t.Fatalf("encrypted backup round trip failed: %v", err)
	}
	if _, err := decryptBackup("wrong-passphrase", decoded); err == nil {
		t.Fatal("wrong backup passphrase accepted")
	}
}

func TestExtendedParserAcceptsGlobalOptionsBeforeCommand(t *testing.T) {
	command, opts, positional, err := parseExtended([]string{"--json", "--family", "gemini", "recommend", "--tag", "work"})
	if err != nil || command != "recommend" || !opts.JSON || opts.Family != "gemini" || opts.Tag != "work" || len(positional) != 0 {
		t.Fatalf("parsed command=%q opts=%#v positional=%v err=%v", command, opts, positional, err)
	}
}

func TestRunNowParserAndTargetResolution(t *testing.T) {
	command, opts, positional, err := parseExtended([]string{"run", "now", "--account", "work", "--target", "agy", "--", "-p", "hello"})
	if err != nil || command != "run" || opts.Account != "work" || opts.Target != "agy" || strings.Join(positional, " ") != "now" || strings.Join(opts.RunArgs, " ") != "-p hello" {
		t.Fatalf("run now parsed as command=%q opts=%#v positional=%v err=%v", command, opts, positional, err)
	}

	settings := defaultSettings()
	if command, err := resolveRunTarget(settings, ""); err != nil || command != "agy" {
		t.Fatalf("default target = %q, %v", command, err)
	}
	settings.Targets["local"] = TargetConfig{Command: "/usr/local/bin/agy", Enabled: true}
	if command, err := resolveRunTarget(settings, "local"); err != nil || command != "/usr/local/bin/agy" {
		t.Fatalf("configured target = %q, %v", command, err)
	}
	settings.Targets["disabled"] = TargetConfig{Command: "agy", Enabled: false}
	if _, err := resolveRunTarget(settings, "disabled"); err == nil {
		t.Fatal("disabled target was accepted")
	}
	if _, err := resolveRunTarget(settings, "missing"); err == nil {
		t.Fatal("unknown target was accepted")
	}
}

func TestHistoryRetentionAndSecretVaultFallback(t *testing.T) {
	paths := testPaths(t)
	store := NewStore(paths)
	settings := defaultSettings()
	settings.History.MaxBytes = 64 * 1024
	if err := store.SaveSettings(settings); err != nil {
		t.Fatal(err)
	}
	vault := fakeAccountVault{"account:user@example.com": "vault-token"}
	a := &Application{paths: paths, store: store, vault: vault}
	account := Account{"email": "user@example.com", "secret_ref": "account:user@example.com"}
	if got, err := a.accountToken(context.Background(), account); err != nil || got != "vault-token" {
		t.Fatalf("vault token = %q, %v", got, err)
	}
	for i := 0; i < 5; i++ {
		if err := a.appendHistory("quota", "user@example.com", map[string]any{"remaining_pct": i}); err != nil {
			t.Fatal(err)
		}
	}
	events, err := a.readHistory(10)
	if err != nil || len(events) != 5 {
		t.Fatalf("history = %d, %v", len(events), err)
	}
	if events[0].Kind != "quota" || events[0].Email != "user@example.com" {
		t.Fatalf("unexpected history event: %#v", events[0])
	}
}

func TestVersionReportsBuildProvenance(t *testing.T) {
	var out bytes.Buffer
	a := &Application{Version: "2.1.1", BuildID: "local-20260823", Out: &out}
	for _, argv := range [][]string{{"--version"}, {"version"}} {
		out.Reset()
		if code := a.Run(context.Background(), argv); code != 0 {
			t.Fatalf("%v exit code = %d", argv, code)
		}
		if got := out.String(); got != "agy-swap v2.1.1 (local-20260823)\n" {
			t.Fatalf("%v output = %q", argv, got)
		}
	}
	out.Reset()
	a.BuildID = "unknown"
	if code := a.Run(context.Background(), []string{"--version"}); code != 0 || out.String() != "agy-swap v2.1.1\n" {
		t.Fatalf("unknown build output = %q (code %d)", out.String(), code)
	}
}

func TestCompletionIncludesVersionCommand(t *testing.T) {
	for _, shell := range []string{"bash", "zsh", "fish", "powershell"} {
		var out bytes.Buffer
		a := &Application{Out: &out}
		if code := a.cmdCompletion(extendedOptions{}, []string{shell}); code != 0 {
			t.Fatalf("%s completion exit code = %d", shell, code)
		}
		if !strings.Contains(out.String(), "version") {
			t.Fatalf("%s completion omitted version: %q", shell, out.String())
		}
	}
}

func TestTerminalKeysAndDisplayWidth(t *testing.T) {
	cases := map[string]string{"\x1b[A": "up", "\x1b[B": "down", "\x1b[C": "right", "\x1b[D": "left", "\x1b[H": "home", "\x1b[F": "end", "\x1b[3~": "delete", "\x1b[5~": "page-up", "\x1b[6~": "page-down", "\x1bOA": "up", "\r": "enter", "\x7f": "backspace", "\x15": "ctrl-u", "\x17": "ctrl-w", "\x0b": "ctrl-k", "\x1b": "esc"}
	for input, want := range cases {
		if got := readTerminalKey(bytes.NewBufferString(input)); got != want {
			t.Fatalf("%q -> %q", input, got)
		}
	}
	if visibleWidth("A界e\u0301") != 4 {
		t.Fatalf("width = %d", visibleWidth("A界e\u0301"))
	}
	colored := "\x1b[31mlong colored account\x1b[0m"
	if got := truncateVisible(colored, 8, makePalette(true)); visibleWidth(got) != 8 {
		t.Fatalf("colored truncation width = %d: %q", visibleWidth(got), got)
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

func BenchmarkTUIFrame100Accounts(b *testing.B) {
	accounts := NewAccounts()
	for i := 0; i < 100; i++ {
		email := fmt.Sprintf("user-%03d@example.com", i)
		accounts.Set(email, Account{"email": email, "name": fmt.Sprintf("Operator %03d", i)})
	}
	a := &Application{Version: "2.1.1", p: makePalette(false), color: false}
	state := newTUIState(accounts, "user-050@example.com")
	state.selectedEmail = "user-050@example.com"
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		state.selectedEmail = fmt.Sprintf("user-%03d@example.com", i%100)
		_ = a.tuiLines(state, 118, 30)
	}
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
