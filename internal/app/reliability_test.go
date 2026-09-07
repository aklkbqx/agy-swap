package app

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode"
)

func TestStoreConcurrentCreation(t *testing.T) {
	s := NewStore(testPaths(t))
	a, err := s.Load(false)
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.Load(false)
	if err != nil {
		t.Fatal(err)
	}
	a.Set("a@example.com", Account{"email": "a@example.com"})
	b.Set("b@example.com", Account{"email": "b@example.com"})
	if err := s.Save(a); err != nil {
		t.Fatal(err)
	}
	if err := s.Save(b); !errors.Is(err, errStoreConflict) {
		t.Fatalf("expected conflict: %v", err)
	}
}

func TestRecommendationEligibilityAndPolicies(t *testing.T) {
	accounts := NewAccounts()
	reset := time.Now().Add(time.Hour)
	accounts.Set("a@example.com", quotaAccount("a@example.com", 1, 0, reset))
	accounts.Set("b@example.com", quotaAccount("b@example.com", .2, .2, reset))
	a := &Application{}
	settings := defaultSettings()
	choices := a.buildRecommendations(context.Background(), accounts, settings, "", "", "")
	if choices[0].Email != "b@example.com" || !choices[0].Ready || choices[1].Ready {
		t.Fatalf("ineligible account ranked first: %#v", choices)
	}
	accounts.ByEmail["b@example.com"]["quota_limits"] = map[string]any{"manual:gemini": map[string]any{"family": "gemini", "source": "manual", "reset_at": isoTime(reset), "observed_at": isoTime(time.Now())}}
	if wait, _ := accountCooldown(accounts.ByEmail["b@example.com"], time.Now(), "gemini"); wait <= 0 {
		t.Fatal("manual cooldown ignored")
	}
	delete(accounts.ByEmail["b@example.com"], "quota_limits")
	settings.Profiles["work"] = Profile{Account: "a@example.com", Policy: "sticky", ReserveAccounts: []string{"b@example.com"}}
	choices = a.buildRecommendations(context.Background(), accounts, settings, "work", "", "")
	if choices[0].Email != "b@example.com" {
		t.Fatal("reserve not selected when primary depleted")
	}
	getMap(accounts.ByEmail["b@example.com"]["quota_snapshot"])["observed_at"] = isoTime(time.Now().Add(-time.Hour))
	if a.buildRecommendations(context.Background(), accounts, settings, "work", "", "")[0].Ready {
		t.Fatal("stale quota was eligible")
	}
}

func TestTerminalUnicodeAndFormPreservesCase(t *testing.T) {
	for input, want := range map[string]string{"\t": "tab", "\x1b[Z": "shift-tab", "ก": "ก", "界": "界", "A": "A"} {
		if got := readTerminalKey(strings.NewReader(input)); got != want {
			t.Fatalf("input %q became %q", input, got)
		}
	}
	s := &tuiState{form: &tuiFormState{Fields: []tuiFormField{{Key: "passphrase"}, {Key: "path"}}}}
	for _, r := range "Mixedก界" {
		s.formKey(string(r))
	}
	if s.form.Fields[0].Value != "Mixedก界" {
		t.Fatal("form changed input")
	}
	s.formKey("backspace")
	if s.form.Fields[0].Value != "Mixedก" {
		t.Fatal("backspace split a rune")
	}
	s.formKey("tab")
	if s.form.Index != 1 {
		t.Fatal("tab did not advance")
	}
	_, cancel := s.formKey("shift-tab")
	if cancel || s.form.Index != 0 {
		t.Fatal("shift-tab canceled form")
	}
}

func TestRefreshedOAuthCredentialPersistsOnQuotaFailure(t *testing.T) {
	paths := testPaths(t)
	s := NewStore(paths)
	accounts := NewAccounts()
	token := tokenBlob(t, "a@example.com", true, "old-refresh", time.Now().Add(-time.Hour))
	accounts.Set("a@example.com", newAccount("a@example.com", "A", token))
	if err := s.Save(accounts); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/oauth" {
			fmt.Fprint(w, `{"access_token":"new-access","refresh_token":"rotated-refresh","expires_in":3600}`)
			return
		}
		http.Error(w, "unavailable", 503)
	}))
	defer server.Close()
	h := NewHTTPService(&bytes.Buffer{})
	h.oauthURL = server.URL + "/oauth"
	h.cloudAPI = server.URL + "/"
	q := NewQuotaService(h, s)
	q.SetVault(fakeAccountVault{})
	failures := q.Refresh(context.Background(), accounts, true, nil)
	if failures["a@example.com"] == "" {
		t.Fatal("expected quota failure")
	}
	loaded, err := s.Load(false)
	if err != nil {
		t.Fatal(err)
	}
	got, err := accountToken(context.Background(), loaded.ByEmail["a@example.com"], q.vault)
	if err != nil {
		t.Fatal(err)
	}
	inner := tokenObject(decodeToken(got))
	if getString(inner, "refresh_token") != "rotated-refresh" || getString(inner, "access_token") != "new-access" {
		t.Fatal("refreshed credential lost")
	}
	if expiry, ok := tokenExpiry(inner); !ok || time.Until(expiry) < 59*time.Minute {
		t.Fatal("expiry not persisted")
	}
}

func TestHistoryConcurrentAppendAndOversizedTrim(t *testing.T) {
	paths := testPaths(t)
	a := &Application{paths: paths, store: NewStore(paths)}
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := a.appendHistory("switch", "a@example.com", nil); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	events, err := a.readHistory(100)
	if err != nil || len(events) != 12 {
		t.Fatalf("lost history: %d %v", len(events), err)
	}
	if err := a.trimHistory(1, 30); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(paths.History)
	if len(data) != 0 {
		t.Fatal("oversized final event escaped trim")
	}
}

func TestVersionOrderingAndUpdateRollback(t *testing.T) {
	for _, tc := range []struct {
		a, b string
		want int
	}{{"2.2.0", "2.1.3", 1}, {"2.2.0-rc.2", "2.2.0-rc.10", -1}, {"2.2.0", "2.2.0-rc.1", 1}, {"2.2.0+build", "2.2.0", 0}, {"2.2.0", "3.0.0", -1}} {
		got, err := compareVersions(tc.a, tc.b)
		if err != nil || got != tc.want {
			t.Fatalf("%s vs %s: %d %v", tc.a, tc.b, got, err)
		}
	}
	if _, err := compareVersions("2.2.0-01", "2.2.0"); err == nil {
		t.Fatal("invalid prerelease accepted")
	}
	dir := t.TempDir()
	current := filepath.Join(dir, "agy-swap")
	if err := os.WriteFile(current, []byte("original"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := replaceUpdateBinary(current, filepath.Join(dir, "missing"), current+".bak"); err == nil {
		t.Fatal("missing candidate installed")
	}
	data, _ := os.ReadFile(current)
	if string(data) != "original" {
		t.Fatal("failed replacement lost original")
	}
}

func TestBindingLongestMatchAndUnknownStatusline(t *testing.T) {
	root := t.TempDir()
	settings := defaultSettings()
	settings.Bindings = []Binding{{Path: root, Profile: "root", Mode: "auto"}, {Path: filepath.Join(root, "child"), Profile: "child", Mode: "disabled"}}
	if got := resolveBinding(settings, filepath.Join(root, "child", "src")); got.Profile != "child" || got.Mode != "disabled" {
		t.Fatalf("wrong binding: %#v", got)
	}
	a := &Application{}
	line := a.statuslineFromInput(map[string]any{"email": "a@example.com", "known": false, "remaining_percent": 0})
	if strings.Contains(line, "quota 0%") || !strings.Contains(line, "quota unknown") {
		t.Fatalf("unknown quota rendered as depleted: %s", line)
	}
}

func TestSettingsRejectStaleSnapshot(t *testing.T) {
	s := NewStore(testPaths(t))
	a, _ := s.LoadSettings()
	b, _ := s.LoadSettings()
	a.Aliases["work"] = "a@example.com"
	if err := s.SaveSettings(a); err != nil {
		t.Fatal(err)
	}
	b.Aliases["work"] = "b@example.com"
	if err := s.SaveSettings(b); err == nil {
		t.Fatal("stale settings overwrote concurrent update")
	}
}

func TestBackupValidationAndMetadataMerge(t *testing.T) {
	paths := testPaths(t)
	s := NewStore(paths)
	accounts := NewAccounts()
	accounts.Set("a@example.com", Account{"email": "a@example.com", "secret_ref": "existing"})
	if err := s.Save(accounts); err != nil {
		t.Fatal(err)
	}
	a := &Application{paths: paths, store: s, vault: fakeAccountVault{"existing": "credential"}}
	path := filepath.Join(t.TempDir(), "backup.json")
	for _, raw := range []string{`{}`, `{"schema":1,"accounts":{},"settings":{"schema":999}}`} {
		if err := os.WriteFile(path, []byte(raw), 0600); err != nil {
			t.Fatal(err)
		}
		before, _ := os.ReadFile(paths.Accounts)
		if _, err := readBackup(path, ""); err == nil {
			t.Fatal("invalid backup verified")
		}
		if _, _, err := a.importBackup(context.Background(), path, "", true); err == nil {
			t.Fatal("invalid backup imported")
		}
		after, _ := os.ReadFile(paths.Accounts)
		if !bytes.Equal(before, after) {
			t.Fatal("failed import mutated accounts")
		}
	}
	if err := os.WriteFile(path, []byte(`{"schema":1,"accounts":{"a@example.com":{"email":"a@example.com","name":"Restored","secret_ref":"untrusted"}}}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := a.importBackup(context.Background(), path, "", true); err != nil {
		t.Fatal(err)
	}
	got, err := s.Load(false)
	if err != nil {
		t.Fatal(err)
	}
	if getString(got.ByEmail["a@example.com"], "secret_ref") != "existing" {
		t.Fatal("metadata import replaced credential")
	}
}

func TestBackupRejectsInvalidNonceAndReadsPassphraseStdin(t *testing.T) {
	passphrase := "  MixedCASE passphrase  "
	envelope, err := encryptBackup(passphrase, []byte(`{"schema":1,"accounts":{}}`))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "encrypted.json")
	raw, _ := json.Marshal(envelope)
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	var out, errOut bytes.Buffer
	a := &Application{In: strings.NewReader(passphrase + "\n"), Out: &out, Err: &errOut}
	if code := a.cmdBackup(context.Background(), extendedOptions{PassphraseStdin: true}, []string{"verify", path}); code != 0 {
		t.Fatalf("verify failed: %s", errOut.String())
	}
	envelope.Nonce = base64.StdEncoding.EncodeToString([]byte("short"))
	if _, err := decryptBackup(passphrase, envelope); err == nil {
		t.Fatal("invalid nonce accepted")
	}
}

func TestAtomicExportPreservesParentPermissions(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := atomicWrite(filepath.Join(dir, "export.json"), []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	info, _ := os.Stat(dir)
	if info.Mode().Perm() != 0755 {
		t.Fatalf("parent permissions changed: %v", info.Mode())
	}
}

func TestInterruptedRestoreRecoversBeforeRead(t *testing.T) {
	paths := testPaths(t)
	s := NewStore(paths)
	accounts := NewAccounts()
	accounts.Set("a@example.com", Account{"email": "a@example.com"})
	if err := s.Save(accounts); err != nil {
		t.Fatal(err)
	}
	original, _ := os.ReadFile(paths.Accounts)
	if err := atomicWriteJSON(s.restoreJournalPath(), restoreJournal{Schema: 1, Accounts: original}); err != nil {
		t.Fatal(err)
	}
	if err := atomicWrite(paths.Accounts, []byte("broken"), 0600); err != nil {
		t.Fatal(err)
	}
	recovered, err := s.Load(false)
	if err != nil || recovered.Len() != 1 {
		t.Fatalf("recovery failed: %v", err)
	}
	if _, err := os.Stat(s.restoreJournalPath()); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("journal not cleaned")
	}
}

func TestTerminalWidthMatchesEscapeGrammar(t *testing.T) {
	random := rand.New(rand.NewSource(42))
	parts := []string{"plain", "界", "ภาษาไทย", "e\u0301", "\x1b[31m", "\x1b[0m", "\x1b[?25l", "\x1bM", "\x1b[", "\xff", "\x1b[1;", " ", "🚀"}
	for i := 0; i < 3000; i++ {
		var b strings.Builder
		for j := 0; j < 20; j++ {
			b.WriteString(parts[random.Intn(len(parts))])
		}
		value := b.String()
		want := 0
		for _, r := range ansiPattern.ReplaceAllString(value, "") {
			if unicode.Is(unicode.Mn, r) {
				continue
			}
			if isWide(r) {
				want += 2
			} else {
				want++
			}
		}
		if got := visibleWidth(value); got != want {
			t.Fatalf("width %d want %d for %q", got, want, value)
		}
	}
}

func TestExtendedErrorsExitNonzeroWithJSON(t *testing.T) {
	a := &Application{Out: &bytes.Buffer{}, Err: &bytes.Buffer{}}
	if got := a.extendedError("test", extendedOptions{JSON: true}, errors.New("failed")); got != 1 {
		t.Fatalf("exit = %d", got)
	}
}

func TestSwitchRecordsHistoryAndRepairsSessionFiles(t *testing.T) {
	paths := testPaths(t)
	token := tokenBlob(t, "saved@example.com", true, "refresh", time.Now().Add(time.Hour))
	credentials := NewCredentials(paths)
	credentials.backend = &fakeCredentialBackend{token: token}
	var output bytes.Buffer
	a := &Application{paths: paths, store: NewStore(paths), credentials: credentials, Out: &output, Err: &output}
	if !a.applyAccount(context.Background(), token, "saved@example.com") {
		t.Fatal("switch failed")
	}
	if _, err := os.Stat(paths.OAuthCredentials); err != nil {
		t.Fatalf("session not repaired: %v", err)
	}
	data, err := os.ReadFile(paths.History)
	if err != nil {
		t.Fatal(err)
	}
	var event historyEvent
	if err := json.Unmarshal(bytes.TrimSpace(data), &event); err != nil {
		t.Fatal(err)
	}
	if event.Kind != "switch" || event.Email != "saved@example.com" {
		t.Fatalf("event = %#v", event)
	}
}

func TestMetricsKeepLimitingBucket(t *testing.T) {
	paths := testPaths(t)
	account := quotaAccount("saved@example.com", .9, .8, time.Now().Add(time.Hour))
	group := getMap(quotaGroups(account)[0])
	group["buckets"] = append([]any{map[string]any{"id": "gemini-5h", "name": "5 hours", "window": "5h", "remaining_fraction": .1, "reset_at": isoTime(time.Now().Add(time.Hour))}}, getSlice(group["buckets"])...)
	accounts := NewAccounts()
	accounts.Set("saved@example.com", account)
	store := NewStore(paths)
	if err := store.Save(accounts); err != nil {
		t.Fatal(err)
	}
	a := &Application{store: store}
	snapshot, err := a.metricsSnapshot(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	samples := snapshot["accounts"].([]map[string]any)
	if samples[0]["family_gemini"] != .1 {
		t.Fatalf("limiting quota lost: %#v", samples[0])
	}
}

func TestLoginCorruptStorePreservesSession(t *testing.T) {
	paths := testPaths(t)
	token := tokenBlob(t, "saved@example.com", true, "refresh", time.Now().Add(time.Hour))
	backend := &fakeCredentialBackend{token: token}
	credentials := NewCredentials(paths)
	credentials.backend = backend
	if err := atomicWrite(paths.Accounts, []byte("invalid json"), 0600); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	a := &Application{paths: paths, store: NewStore(paths), credentials: credentials, Out: &output, Err: &output}
	if code := a.addLoginFlow(context.Background()); code != 1 {
		t.Fatalf("exit = %d", code)
	}
	if backend.token != token {
		t.Fatal("corrupt store changed active credential")
	}
}
