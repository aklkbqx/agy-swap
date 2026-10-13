package app

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

// cliFixture drives Application.Run end to end with isolated paths, a fake
// credential store and a local Google stand-in; no network or real accounts.
type cliFixture struct {
	a        *Application
	out      *bytes.Buffer
	errOut   *bytes.Buffer
	backend  *fakeCredentialBackend
	vault    fakeAccountVault
	paths    Paths
	tokens   map[string]string
	userInfo string
}

func newCLIFixture(t *testing.T, emails ...string) *cliFixture {
	t.Helper()
	f := &cliFixture{paths: testPaths(t), tokens: map[string]string{}, vault: fakeAccountVault{}, backend: &fakeCredentialBackend{}, out: &bytes.Buffer{}, errOut: &bytes.Buffer{}}
	store := NewStore(f.paths)
	accounts := NewAccounts()
	for _, email := range emails {
		f.tokens[email] = tokenBlob(t, email, true, "refresh-"+email, time.Now().Add(time.Hour))
		accounts.Set(email, newAccount(email, strings.Split(email, "@")[0], f.tokens[email]))
	}
	if err := store.Save(accounts); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"email":%q,"name":"Google User","verified_email":true}`, f.userInfo)
	}))
	t.Cleanup(server.Close)
	httpService := NewHTTPService(io.Discard)
	httpService.userInfoURL = server.URL
	credentials := NewCredentials(f.paths)
	credentials.backend = f.backend
	f.a = &Application{Version: "2.11.6", Out: f.out, Err: f.errOut, paths: f.paths, store: store, credentials: credentials, vault: f.vault, http: httpService, p: makePalette(false)}
	return f
}

func (f *cliFixture) run(stdin string, args ...string) int {
	f.out.Reset()
	f.errOut.Reset()
	f.a.In = strings.NewReader(stdin)
	f.a.lineReader = nil
	return f.a.Run(context.Background(), args)
}

func (f *cliFixture) savedEmails(t *testing.T) []string {
	t.Helper()
	accounts, err := f.a.store.Load(false)
	if err != nil {
		t.Fatal(err)
	}
	return accounts.Order
}

func TestCLISwitchResolvesIndexEmailAndAlias(t *testing.T) {
	f := newCLIFixture(t, "alpha@example.com", "beta@example.com")
	for _, tc := range []struct{ target, email string }{
		{"2", "beta@example.com"},
		{"alpha@example.com", "alpha@example.com"},
	} {
		if code := f.run("", "switch", tc.target); code != 0 {
			t.Fatalf("switch %s: code %d, err %s", tc.target, code, f.errOut)
		}
		if f.backend.token != f.tokens[tc.email] || !strings.Contains(f.out.String(), "Successfully switched to "+tc.email) {
			t.Fatalf("switch %s did not activate %s: %s", tc.target, tc.email, f.out)
		}
	}
	if code := f.run("", "alias", "set", "work", "beta@example.com"); code != 0 {
		t.Fatalf("alias set: %s", f.errOut)
	}
	if code := f.run("", "switch", "work"); code != 0 || f.backend.token != f.tokens["beta@example.com"] {
		t.Fatalf("switch by alias: code %d, err %s", code, f.errOut)
	}
	history, err := os.ReadFile(f.paths.History)
	if err != nil || strings.Count(string(history), `"switch"`) != 3 {
		t.Fatalf("switches not recorded in history: %v\n%s", err, history)
	}
}

func TestCLISwitchRejectsBadTargetsWithoutTouchingTheSession(t *testing.T) {
	f := newCLIFixture(t, "alpha@example.com", "beta@example.com")
	for _, args := range [][]string{{"switch", "9"}, {"switch", "nobody@example.com"}, {"switch"}} {
		if code := f.run("", args...); code != 1 {
			t.Fatalf("%v: code %d", args, code)
		}
	}
	if f.backend.sets != 0 {
		t.Fatalf("rejected switch wrote the session %d times", f.backend.sets)
	}
	if code := f.run("", "switch", "9"); code != 1 || !strings.Contains(f.errOut.String(), "Account '9' not found") {
		t.Fatalf("unknown target message: %s", f.errOut)
	}
	if code := f.run("", "switch"); code != 1 || !strings.Contains(f.errOut.String(), "Interactive mode requires TTY") {
		t.Fatalf("missing target without TTY: %s", f.errOut)
	}
	empty := newCLIFixture(t)
	if code := empty.run("", "switch", "1"); code != 1 || !strings.Contains(empty.out.String(), "No accounts found") {
		t.Fatalf("empty store: code %d, out %s", code, empty.out)
	}
}

func TestCLISwitchFailureKeepsThePreviousSession(t *testing.T) {
	f := newCLIFixture(t, "alpha@example.com", "beta@example.com")
	if code := f.run("", "switch", "1"); code != 0 {
		t.Fatalf("initial switch: %s", f.errOut)
	}
	before, err := os.ReadFile(f.paths.OAuthToken)
	if err != nil {
		t.Fatal(err)
	}
	f.backend.failSet = true
	if code := f.run("", "switch", "2"); code != 1 || !strings.Contains(f.errOut.String(), "Failed to switch account") {
		t.Fatalf("failed write reported code %d: %s", code, f.errOut)
	}
	after, err := os.ReadFile(f.paths.OAuthToken)
	if err != nil || !bytes.Equal(before, after) || f.backend.token != f.tokens["alpha@example.com"] {
		t.Fatalf("previous session not preserved (err %v)", err)
	}
}

func TestCLIRemoveRequiresConfirmationAndDeletesTheVaultSecret(t *testing.T) {
	f := newCLIFixture(t, "alpha@example.com", "beta@example.com")
	accounts, err := f.a.store.Load(false)
	if err != nil {
		t.Fatal(err)
	}
	beta := accounts.ByEmail["beta@example.com"]
	ref := accountSecretRef("beta@example.com")
	f.vault[ref] = f.tokens["beta@example.com"]
	beta["secret_ref"] = ref
	delete(beta, "token_data")
	if err := f.a.store.Save(accounts); err != nil {
		t.Fatal(err)
	}
	if code := f.run("n\n", "remove", "2"); code != 0 || len(f.savedEmails(t)) != 2 {
		t.Fatalf("declined removal changed accounts: code %d", code)
	}
	if code := f.run("y\n", "remove", "2"); code != 0 || !strings.Contains(f.out.String(), "Removed account 'beta@example.com'") {
		t.Fatalf("confirmed removal: code %d, out %s", code, f.out)
	}
	if saved := f.savedEmails(t); len(saved) != 1 || saved[0] != "alpha@example.com" {
		t.Fatalf("remaining accounts %v", saved)
	}
	if _, ok := f.vault[ref]; ok {
		t.Fatal("vault secret left behind after removal")
	}
	if code := f.run("y\n", "remove", "nobody@example.com"); code != 1 {
		t.Fatalf("unknown removal target accepted: code %d", code)
	}
}

func TestCLIStatusAndListReportTheActiveSession(t *testing.T) {
	f := newCLIFixture(t, "alpha@example.com", "beta@example.com")
	if code := f.run("", "status"); code != 0 || !strings.Contains(f.out.String(), "Not logged in") {
		t.Fatalf("logged-out status: %s", f.out)
	}
	if code := f.run("", "switch", "2"); code != 0 {
		t.Fatal(f.errOut)
	}
	if code := f.run("", "status"); code != 0 || !strings.Contains(f.out.String(), "Active Account:") || !strings.Contains(f.out.String(), "<beta@example.com>") {
		t.Fatalf("saved active status: %s", f.out)
	}
	if code := f.run("", "list"); code != 0 {
		t.Fatalf("list: %s", f.errOut)
	}
	for _, line := range strings.Split(f.out.String(), "\n") {
		if strings.Contains(line, "<alpha@example.com>") && strings.Contains(line, "(Active)") {
			t.Fatalf("inactive account marked active: %q", line)
		}
		if strings.Contains(line, "[2]") && !strings.Contains(line, "(Active)") {
			t.Fatalf("active account not marked: %q", line)
		}
	}
	// An outside login writes the session files the agy CLI reads first.
	f.userInfo = "gamma@example.com"
	gamma := tokenBlob(t, "gamma@example.com", true, "r", time.Now().Add(time.Hour))
	if !f.a.credentials.writeOAuthFiles(gamma, "gamma@example.com") {
		t.Fatal("could not simulate an outside login")
	}
	f.backend.token = gamma
	if code := f.run("", "status"); code != 0 || !strings.Contains(f.out.String(), "gamma@example.com") || !strings.Contains(f.out.String(), "Unsaved in agy-swap") {
		t.Fatalf("unsaved active status: %s", f.out)
	}
	empty := newCLIFixture(t)
	if code := empty.run("", "list"); code != 0 || !strings.Contains(empty.out.String(), "No accounts added yet") {
		t.Fatalf("empty list: %s", empty.out)
	}
}

func TestCLILogoutClearsEveryActiveCredential(t *testing.T) {
	f := newCLIFixture(t, "alpha@example.com")
	if code := f.run("", "switch", "1"); code != 0 {
		t.Fatal(f.errOut)
	}
	if code := f.run("", "logout"); code != 0 || !strings.Contains(f.out.String(), "Logged out") {
		t.Fatalf("logout: code %d, out %s, err %s", code, f.out, f.errOut)
	}
	if f.backend.token != "" {
		t.Fatal("secure credential survived logout")
	}
	for _, path := range []string{f.paths.OAuthToken, f.paths.OAuthCredentials} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("%s survived logout: %v", path, err)
		}
	}
	if code := f.run("", "logout"); code != 0 || !strings.Contains(f.out.String(), "Already logged out") {
		t.Fatalf("second logout: %s", f.out)
	}
}

func TestCLIAddFromStdinVerifiesAndSavesTheAccount(t *testing.T) {
	f := newCLIFixture(t)
	f.userInfo = "delta@example.com"
	token := tokenBlob(t, "delta@example.com", true, "refresh", time.Now().Add(time.Hour))
	if code := f.run(token+"\n", "add", "--token", "-"); code != 0 || !strings.Contains(f.out.String(), "Successfully saved account") {
		t.Fatalf("add from stdin: code %d, out %s, err %s", code, f.out, f.errOut)
	}
	if saved := f.savedEmails(t); len(saved) != 1 || saved[0] != "delta@example.com" {
		t.Fatalf("saved accounts %v", saved)
	}
	f.userInfo = "someone-else@example.com"
	if code := f.run(token+"\n", "add", "--token", "-"); code != 1 || !strings.Contains(f.errOut.String(), "does not match") {
		t.Fatalf("mismatched identity accepted: code %d, err %s", code, f.errOut)
	}
	if code := f.run("\n", "add", "--token", "-"); code != 1 || !strings.Contains(f.errOut.String(), "Empty token") {
		t.Fatalf("empty token accepted: code %d", code)
	}
}

// next must move away from the active account whenever another account is
// eligible, whatever the policy; balanced then prefers the most quota.
func TestCLINextNeverRotatesToTheActiveAccount(t *testing.T) {
	quotas := map[string]float64{"alpha@example.com": 0.3, "beta@example.com": 0.9, "gamma@example.com": 0.5, "delta@example.com": 0.7}
	for policy, want := range map[string]string{"sticky": "gamma@example.com", "round-robin": "gamma@example.com", "balanced": "delta@example.com"} {
		f := newCLIFixture(t, "alpha@example.com", "beta@example.com", "gamma@example.com", "delta@example.com")
		accounts, err := f.a.store.Load(true)
		if err != nil {
			t.Fatal(err)
		}
		for email, remaining := range quotas {
			for key, value := range quotaAccount(email, remaining, remaining, time.Now().Add(time.Hour)) {
				accounts.ByEmail[email][key] = value
			}
		}
		if err := f.a.store.Save(accounts); err != nil {
			t.Fatal(err)
		}
		settings := defaultSettings()
		settings.Policy.Name = policy
		if err := f.a.store.SaveSettings(settings); err != nil {
			t.Fatal(err)
		}
		// Refresh fails offline, so the fresh snapshots above decide eligibility.
		f.a.http.cloudAPI = "http://127.0.0.1:1/"
		f.a.quota = NewQuotaService(f.a.http, f.a.store)
		f.a.quota.SetVault(f.a.vault)
		if code := f.run("", "switch", "beta@example.com"); code != 0 {
			t.Fatalf("%s: switch: %s", policy, f.errOut)
		}
		f.userInfo = "beta@example.com"
		if code := f.run("", "next"); code != 0 || !strings.Contains(f.out.String(), "auto-rotated to "+want) {
			t.Fatalf("%s: next from the fullest active account: code=%d\n%s%s", policy, code, f.out, f.errOut)
		}
	}
}

func TestCLINextKeepsTheOnlyEligibleAccount(t *testing.T) {
	f := newCLIFixture(t, "alpha@example.com", "beta@example.com")
	accounts, err := f.a.store.Load(true)
	if err != nil {
		t.Fatal(err)
	}
	for email, remaining := range map[string]float64{"alpha@example.com": 0.8, "beta@example.com": 0} {
		for key, value := range quotaAccount(email, remaining, remaining, time.Now().Add(time.Hour)) {
			accounts.ByEmail[email][key] = value
		}
	}
	if err := f.a.store.Save(accounts); err != nil {
		t.Fatal(err)
	}
	settings := defaultSettings()
	settings.Policy.Name = "balanced"
	if err := f.a.store.SaveSettings(settings); err != nil {
		t.Fatal(err)
	}
	f.a.http.cloudAPI = "http://127.0.0.1:1/"
	f.a.quota = NewQuotaService(f.a.http, f.a.store)
	f.a.quota.SetVault(f.a.vault)
	if code := f.run("", "switch", "alpha@example.com"); code != 0 {
		t.Fatal(f.errOut)
	}
	f.userInfo = "alpha@example.com"
	if code := f.run("", "next"); code != 0 || !strings.Contains(f.out.String(), "auto-rotated to alpha@example.com") {
		t.Fatalf("only eligible account: code=%d\n%s%s", code, f.out, f.errOut)
	}
}
