# Account verification plan (v2.14.0)

**Goal:** When Google requires an account to be verified before Antigravity can use it, agy-swap says so in the list, the TUI and `add`, and opens Google's verification page with one key or one command, without asking the user to copy a long link.

**Architecture:** The quota fetch keeps Google's error body, reads the eligibility signal from it (or from `loadCodeAssist`'s ineligible tiers), and returns a typed `*eligibilityError`. The refresh stores it on the account as `eligibility`, so it survives restarts and clears when a fetch succeeds. The TUI shows a `VERIFY` status, `w` opens the page, `agy-swap verify <account>` does the same from the shell, and `add` checks eligibility once after saving.

**Tech stack:** Go standard library only (`net/url`, `os/exec`, `errors`). No new dependencies.

**Issue:** [#62](https://github.com/aklkbqx/agy-swap/issues/62) (milestone v2.14.0 once the owner approves the version). Site copy follows in the private Site repository.

Implementation note: the verification key is `w`, not `V` as first planned, because the TUI reads keys case-insensitively and `v` already opens the quota view.

## Findings (observed 2026-10-23 against v2.13.1)

| Finding | Evidence |
| --- | --- |
| `add` saves an account and reports success without checking whether Antigravity can use it | `saveTokenAccount` (`internal/app/cli.go:611`) checks only userinfo |
| `add` stops `agy` once the credential file changes, before `agy`'s own eligibility check prints its link | `addLoginFlow` poll branch (`internal/app/cli.go:589-597`); `agy` checks eligibility in the background after it saves the credential |
| Google's error body is discarded | `jsonRequest` returns only `HTTP %d` (`internal/app/oauth.go:104-106`); `cloudPost` returns `<method> failed (HTTP %d)` (`oauth.go:302-311`) |
| Google's eligibility error carries `details[].metadata.{validation_url, uiMessage, appeal_url, appeal_url_link_text}` | Field names in the installed `agy` executable |
| After `a` in the TUI the new account shows `…` / "pending" until the next refresh | `suspend` reloads accounts without a refresh (`internal/app/tui.go:267-270`); `accountStatusFor` returns `statusPending` without pools (`tui_pools.go:159`) |
| After a refresh, the account says "Needs re-auth" and tells the user to run `agy-swap add`, which does not help with verification | `tuiGaugeDetail` (`internal/app/tui_detail.go:69-71`) |
| The error is held only in memory | `state.quotaErrors` (`tui.go:723`); nothing is written to the account |

Observed protocol behavior: Google surfaces verification requirements via either (a) `loadCodeAssist` HTTP 400/403 with `google.rpc.ErrorInfo` (`VALIDATION_REQUIRED` / `appeal_url`), (b) `loadCodeAssist` HTTP 200 `ineligibleTiers`, or (c) `onboardUser` Long-Running Operation errors when a new account is provisioned onto `free-tier`.

## Constraints

- Minor release v2.14.0: one new command (`verify`), one new key (`w`), one new status word (`VERIFY`). This is the first feature since maintenance mode and needs the owner's approval of the version.
- Open only `https` pages on `google.com`, subdomains of `google.com`, or Google brand TLD hosts (`*.google`). Never open anything else Google's response contains. If a link is missing or rejected, show the message and say that `agy` shows the link.
- Pass the URL to the system opener as a single process argument: no shell, no clipboard. Never print the URL inside the TUI frame, where it would wrap. The CLI prints it on its own line as a fallback for headless/SSH sessions.
- Only an eligibility signal becomes `VERIFY`. A network error, HTTP 5xx, OAuth 5xx/429, captive-portal HTML, or an unrelated 403 stays as it is today and preserves any stored `eligibility` record.
- `verify` always refreshes live with Google when quota service is available, so completed verifications clear immediately and fresh token URLs replace expired ones.
- When `add` encounters an unverified account while a working active session existed, the previous active session is restored automatically to avoid leaving Antigravity stranded on an unusable credential.
- Downstream automation (`next`, `recommend`, `run`, `list`, `watchOnce`) respects `eligibility` so unverified accounts are never marked ready, stale quota bars are suppressed, and false auth notifications are avoided.
- Diagnostics never print tokens, emails, project IDs or full URLs; only field shapes, reason codes and URL hosts.
- TUI follows `docs/project/design-principles.md`: every state has a word, and color reflects state. `VERIFY` uses yellow (action needed), like `LOW`. Unverified accounts start in the attention partition from frame 0 to prevent row jumping.
- Owner review gate before any commit, tag, push, release or image publication.

## Review focus

1. Google sends an `http://` link or a non-Google host → nothing opens; the message stays visible. Test in Task 2.
2. Google sends a message with no link → `VERIFY` still shows; `w` and `verify` say that `agy` shows the link. Tests in Tasks 2 and 5.
3. The user verifies, then refreshes → `VERIFY` disappears and pools show. Test in Task 3.
4. HTTP 500 or a network error → not `VERIFY`; the earlier `eligibility` record is kept, not cleared. Test in Task 3.
5. No opener available (headless Linux) → the command prints the URL and exits 1 without hanging. Test in Task 5.

---

### Task 1: Keep Google's error body and describe it in diagnostics

**Files:**
- Modify: `internal/app/oauth.go` (`jsonRequest`, `cloudPost`)
- Modify: `internal/app/quota.go` (`Fetch`, diagnostics)
- Test: `internal/app/quota_diagnostics_test.go`
- Create (after the probe): `internal/app/testdata/eligibility/*.json`

**Interfaces:**
- Produces: `type cloudError struct { Method string; Status int; Body []byte }` with `Error()` = `"<method> failed (HTTP <status>)"` (unchanged text). `jsonRequest` returns `*httpStatusError{Status int; Body []byte}` for non-2xx; `Error()` = `"HTTP <status>"`.

- [ ] **Step 1: Write the failing test**

```go
func TestQuotaDiagnosticsDescribeAnIneligibleAccountWithoutSecrets(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprint(w, `{"error":{"code":403,"message":"user@example.com must verify","status":"PERMISSION_DENIED","details":[{"@type":"type.googleapis.com/google.rpc.ErrorInfo","reason":"VALIDATION_REQUIRED","metadata":{"validation_url":"https://accounts.google.com/signin/continue?secret=abc","uiMessage":"Verify your account to continue."}}]}}`)
	}))
	defer server.Close()
	var diagnostics bytes.Buffer
	httpService := NewHTTPService(io.Discard)
	httpService.cloudAPI = server.URL + "/"
	quota := NewQuotaService(httpService, nil)
	quota.SetDiagnostics(&diagnostics)
	account := Account{"email": "user@example.com", "token_data": tokenBlob(t, "user@example.com", true, "r", time.Now().Add(time.Hour))}
	_, err := quota.Fetch(context.Background(), account)
	var cloud *cloudError
	if !errors.As(err, &cloud) || cloud.Status != 403 || len(cloud.Body) == 0 {
		t.Fatalf("err = %v", err)
	}
	got := diagnostics.String()
	for _, want := range []string{"loadCodeAssist HTTP 403", "VALIDATION_REQUIRED", "validation_url host accounts.google.com"} {
		if !strings.Contains(got, want) {
			t.Fatalf("diagnostics miss %q:\n%s", want, got)
		}
	}
	for _, secret := range []string{"user@example.com", "secret=abc", "/signin/continue"} {
		if strings.Contains(got, secret) {
			t.Fatalf("diagnostics leak %q:\n%s", secret, got)
		}
	}
}
```

- [ ] **Step 2:** `go test ./internal/app -run TestQuotaDiagnosticsDescribeAnIneligibleAccountWithoutSecrets` → FAIL (`cloudError` undefined).

- [ ] **Step 3: Implement.** In `jsonRequest`, for a non-2xx status read at most 64 KiB of the body and return it:

```go
type httpStatusError struct {
	Status int
	Body   []byte
}

func (e *httpStatusError) Error() string { return fmt.Sprintf("HTTP %d", e.Status) }
```

```go
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 64*1024))
		return nil, response.StatusCode, &httpStatusError{Status: response.StatusCode, Body: body}
	}
```

In `cloudPost`:

```go
// cloudError keeps Google's error body: it can say why an account cannot use
// Antigravity and link the page that fixes it.
type cloudError struct {
	Method string
	Status int
	Body   []byte
}

func (e *cloudError) Error() string { return fmt.Sprintf("%s failed (HTTP %d)", e.Method, e.Status) }
```

```go
	if err != nil {
		if status != 0 {
			var statusErr *httpStatusError
			var body []byte
			if errors.As(err, &statusErr) {
				body = statusErr.Body
			}
			return nil, &cloudError{Method: method, Status: status, Body: body}
		}
		return nil, fmt.Errorf("%s failed: %w", method, err)
	}
```

In `Fetch`, after the `loadCodeAssist` call, write diagnostics when they are on: for a `*cloudError`, `quota: loadCodeAssist HTTP <status> body fields: <shapeOf(body, 3)>`, then each `details[].reason` and, for each `*_url` metadata key, `<key> host <url host>`; for a success, `quota: loadCodeAssist fields: <shapeOf(info, 2)>` plus each `ineligibleTiers[].reasonCode` and URL host the same way. Put the formatting in `describeLoadCodeAssist(info map[string]any, err error)` beside `describeModelList`.

- [ ] **Step 4:** Run the test → PASS. `go test ./internal/app` → PASS (existing messages are unchanged).

- [ ] **Step 5: Probe the real response (owner present).** `make install`, then the owner runs `AGY_SWAP_QUOTA_DEBUG=1 agy-swap limits --refresh 2>dbg.txt` with the affected account saved. Read the `loadCodeAssist` lines. Record which shape Google uses and its exact field names in the Issue (reason codes and hosts only). Write a matching sanitized fixture to `internal/app/testdata/eligibility/` with `example.invalid` identities and a dummy `https://accounts.google.com/...` path. If the field names differ from Task 2's, change Task 2's keys to match before writing it.

---

### Task 2: Read the eligibility signal

**Files:**
- Create: `internal/app/eligibility.go`
- Test: `internal/app/eligibility_test.go`

**Interfaces:**
- Consumes: `cloudError` (Task 1).
- Produces:
  - `type eligibilityError struct { Reason, Message, URL string }`, `Error() string`
  - `eligibilityFromErrorBody(body []byte) *eligibilityError`
  - `eligibilityFromLoad(info map[string]any) *eligibilityError`
  - `safeGoogleURL(raw string) string`
  - `(e *eligibilityError) record(now time.Time) map[string]any`
  - `accountEligibility(account Account) (*eligibilityError, bool)`

- [ ] **Step 1: Write the failing tests**

```go
func TestEligibilityFromErrorBodyReadsTheVerificationPage(t *testing.T) {
	body := []byte(`{"error":{"code":403,"message":"denied","details":[{"@type":"type.googleapis.com/google.rpc.ErrorInfo","reason":"VALIDATION_REQUIRED","metadata":{"validation_url":"https://accounts.google.com/v?x=1","uiMessage":"Verify your account to continue."}}]}}`)
	got := eligibilityFromErrorBody(body)
	if got == nil || got.Reason != "VALIDATION_REQUIRED" || got.Message != "Verify your account to continue." || got.URL != "https://accounts.google.com/v?x=1" {
		t.Fatalf("got %#v", got)
	}
}

func TestEligibilityFromErrorBodyFallsBackToTheAppealPage(t *testing.T) {
	body := []byte(`{"error":{"details":[{"reason":"TOS_VIOLATION","metadata":{"appeal_url":"https://support.google.com/appeal","uiMessage":"Appeal"}}]}}`)
	if got := eligibilityFromErrorBody(body); got == nil || got.URL != "https://support.google.com/appeal" {
		t.Fatalf("got %#v", got)
	}
}

func TestEligibilityKeepsTheMessageWhenTheLinkIsMissingOrUnsafe(t *testing.T) {
	for _, link := range []string{"", "http://accounts.google.com/v", "https://accounts.google.com.evil.example/v", "https://user@accounts.google.com/v", "javascript:alert(1)"} {
		body := []byte(fmt.Sprintf(`{"error":{"details":[{"reason":"VALIDATION_REQUIRED","metadata":{"validation_url":%q,"uiMessage":"Verify"}}]}}`, link))
		got := eligibilityFromErrorBody(body)
		if got == nil || got.URL != "" || got.Message != "Verify" {
			t.Fatalf("%q: got %#v", link, got)
		}
	}
}

func TestOrdinaryErrorsAreNotEligibilityErrors(t *testing.T) {
	for _, body := range []string{"", "not json", `{"error":{"code":403,"message":"The caller does not have permission"}}`, `{"error":{"details":[{"reason":"RATE_LIMIT_EXCEEDED"}]}}`} {
		if got := eligibilityFromErrorBody([]byte(body)); got != nil {
			t.Fatalf("%q: got %#v", body, got)
		}
	}
}

func TestEligibilityFromLoadReadsIneligibleTiers(t *testing.T) {
	info := map[string]any{"ineligibleTiers": []any{map[string]any{"reasonCode": "VALIDATION_REQUIRED", "reasonMessage": "Verify your account", "validationUrl": "https://accounts.google.com/v"}}}
	got := eligibilityFromLoad(info)
	if got == nil || got.URL != "https://accounts.google.com/v" || got.Message != "Verify your account" {
		t.Fatalf("got %#v", got)
	}
	usable := map[string]any{"cloudaicompanionProject": "p", "currentTier": map[string]any{"id": "free-tier"}, "ineligibleTiers": info["ineligibleTiers"]}
	if got := eligibilityFromLoad(usable); got != nil {
		t.Fatalf("an account with a usable tier is not ineligible: %#v", got)
	}
}

func TestEligibilityRecordRoundTrips(t *testing.T) {
	now := time.Date(2026, 10, 23, 1, 2, 3, 0, time.UTC)
	want := &eligibilityError{Reason: "VALIDATION_REQUIRED", Message: "Verify", URL: "https://accounts.google.com/v"}
	account := Account{"eligibility": want.record(now)}
	got, ok := accountEligibility(account)
	if !ok || *got != *want {
		t.Fatalf("got %#v %v", got, ok)
	}
	account["eligibility"].(map[string]any)["url"] = "https://evil.example/"
	if got, _ := accountEligibility(account); got.URL != "" {
		t.Fatal("a stored link is checked again before use")
	}
}
```

- [ ] **Step 2:** `go test ./internal/app -run 'Eligibility'` → FAIL (undefined).

- [ ] **Step 3: Implement** `internal/app/eligibility.go`:

```go
package app

import (
	"encoding/json"
	"net/url"
	"strings"
	"time"
)

// eligibilityError is Google's answer that an account cannot use Antigravity
// until its owner acts in a browser: verify the account or appeal a block.
type eligibilityError struct {
	Reason  string
	Message string
	URL     string // a checked Google page, or empty
}

func (e *eligibilityError) Error() string {
	if e.Message != "" {
		return "Google needs this account verified: " + e.Message
	}
	return "Google needs this account verified before Antigravity can use it"
}

// eligibilityFromErrorBody reads a Google RPC error. Only an ErrorInfo that
// carries a verification or appeal page, or a message for the user, counts.
func eligibilityFromErrorBody(body []byte) *eligibilityError {
	var envelope struct {
		Error struct {
			Details []struct {
				Reason   string            `json:"reason"`
				Metadata map[string]string `json:"metadata"`
			} `json:"details"`
		} `json:"error"`
	}
	if json.Unmarshal(body, &envelope) != nil {
		return nil
	}
	for _, detail := range envelope.Error.Details {
		meta := detail.Metadata
		raw := firstNonEmpty(meta["validation_url"], meta["appeal_url"])
		message := cleanText(meta["uiMessage"])
		if raw == "" && message == "" {
			continue
		}
		return &eligibilityError{Reason: cleanText(detail.Reason), Message: message, URL: safeGoogleURL(raw)}
	}
	return nil
}

// eligibilityFromLoad reads loadCodeAssist's ineligible tiers. An account
// with a project and a tier can be used, whatever else is listed.
func eligibilityFromLoad(info map[string]any) *eligibilityError {
	if getString(info, "cloudaicompanionProject") != "" && (getMap(info["currentTier"]) != nil || getMap(info["paidTier"]) != nil) {
		return nil
	}
	for _, raw := range getSlice(info["ineligibleTiers"]) {
		tier := getMap(raw)
		link := firstNonEmpty(getString(tier, "validationUrl"), getString(tier, "appealUrl"))
		message := cleanText(firstNonEmpty(getString(tier, "reasonMessage"), getString(tier, "validationErrorMessage")))
		if link == "" && message == "" {
			continue
		}
		return &eligibilityError{Reason: cleanText(getString(tier, "reasonCode")), Message: message, URL: safeGoogleURL(link)}
	}
	return nil
}

// safeGoogleURL returns raw only when it is an https page on google.com.
func safeGoogleURL(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme != "https" || u.User != nil {
		return ""
	}
	host := strings.ToLower(u.Hostname())
	if host != "google.com" && !strings.HasSuffix(host, ".google.com") {
		return ""
	}
	return u.String()
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func (e *eligibilityError) record(now time.Time) map[string]any {
	return map[string]any{"state": "verify_required", "reason": e.Reason, "message": e.Message, "url": e.URL, "checked_at": isoTime(now)}
}

func accountEligibility(account Account) (*eligibilityError, bool) {
	record := getMap(account["eligibility"])
	if getString(record, "state") != "verify_required" {
		return nil, false
	}
	return &eligibilityError{Reason: getString(record, "reason"), Message: getString(record, "message"), URL: safeGoogleURL(getString(record, "url"))}, true
}
```

Check first whether a helper like `firstNonEmpty` already exists (`graft grep "func firstNonEmpty"`); reuse it if so.

- [ ] **Step 4:** Run the tests → PASS. Add one test per fixture captured in Task 1 that asserts the same fields.

---

### Task 3: Fetch returns it and the refresh stores it

**Files:**
- Modify: `internal/app/quota.go` (`Fetch`, `refreshSelected`)
- Test: `internal/app/eligibility_test.go`

**Interfaces:**
- Consumes: Task 1 `cloudError`; Task 2 parsers, `record`, `accountEligibility`.
- Produces: `Fetch` returns `*eligibilityError` (via `errors.As`) for an ineligible account; `refreshSelected` sets `account["eligibility"]` on that error and deletes it on success.

- [ ] **Step 1: Write the failing test**

```go
func TestRefreshStoresVerificationAndClearsItOnceTheAccountWorks(t *testing.T) {
	var mode atomic.Value
	mode.Store("verify")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case mode.Load() == "verify" && strings.HasSuffix(r.URL.Path, "loadCodeAssist"):
			w.WriteHeader(http.StatusForbidden)
			fmt.Fprint(w, `{"error":{"details":[{"reason":"VALIDATION_REQUIRED","metadata":{"validation_url":"https://accounts.google.com/v","uiMessage":"Verify"}}]}}`)
		case mode.Load() == "down":
			w.WriteHeader(http.StatusInternalServerError)
		case strings.HasSuffix(r.URL.Path, "loadCodeAssist"):
			fmt.Fprint(w, `{"cloudaicompanionProject":"p","currentTier":{"id":"free-tier"}}`)
		default:
			fmt.Fprint(w, `{"groups":[{"buckets":[{"bucketId":"gemini-weekly","displayName":"Weekly","window":"weekly","remainingFraction":0.5,"resetTime":"2030-01-01T00:00:00Z"}]}]}`)
		}
	}))
	defer server.Close()
	store := NewStore(testPaths(t))
	httpService := NewHTTPService(io.Discard)
	httpService.cloudAPI = server.URL + "/"
	service := NewQuotaService(httpService, store)
	accounts := NewAccounts()
	email := "v@example.invalid"
	accounts.Set(email, Account{"email": email, "token_data": tokenBlob(t, email, true, "r", time.Now().Add(time.Hour))})

	failures := service.Refresh(context.Background(), accounts, true, nil)
	if !strings.Contains(failures[email], "verified") {
		t.Fatalf("failures = %v", failures)
	}
	reloaded, _ := store.Load(false)
	if got, ok := accountEligibility(reloaded.ByEmail[email]); !ok || got.URL != "https://accounts.google.com/v" {
		t.Fatalf("not stored: %#v", reloaded.ByEmail[email]["eligibility"])
	}

	mode.Store("down")
	service.Refresh(context.Background(), accounts, true, nil)
	if _, ok := accountEligibility(accounts.ByEmail[email]); !ok {
		t.Fatal("a server error must not clear the verification record")
	}

	mode.Store("ok")
	if failures := service.Refresh(context.Background(), accounts, true, nil); len(failures) != 0 {
		t.Fatalf("failures = %v", failures)
	}
	if _, ok := accountEligibility(accounts.ByEmail[email]); ok {
		t.Fatal("verification record kept after a successful fetch")
	}
}
```

- [ ] **Step 2:** Run → FAIL.

- [ ] **Step 3: Implement.** In `Fetch`:

```go
	info, err := q.http.cloudPost(ctx, access, "loadCodeAssist", map[string]any{"metadata": map[string]any{"ideType": "ANTIGRAVITY"}})
	if q.diagnostics != nil {
		q.describeLoadCodeAssist(info, err)
	}
	if err != nil {
		var cloud *cloudError
		if errors.As(err, &cloud) {
			if ineligible := eligibilityFromErrorBody(cloud.Body); ineligible != nil {
				return nil, ineligible
			}
		}
		return nil, err
	}
	if ineligible := eligibilityFromLoad(info); ineligible != nil {
		return nil, ineligible
	}
```

Apply the same `cloudError` check to the `retrieveUserQuotaSummary` error. In `refreshSelected`, inside the results loop:

```go
		var ineligible *eligibilityError
		switch {
		case errors.As(result.err, &ineligible):
			original["eligibility"] = ineligible.record(now)
			changed = true
		case result.err == nil:
			if _, ok := original["eligibility"]; ok {
				delete(original, "eligibility")
			}
		}
```

(The success branch already sets `changed = true`.)

- [ ] **Step 4:** Run the test and `go test -race ./internal/app` → PASS.

---

### Task 4: TUI: `VERIFY`, `w` opens the page, refresh after add

**Files:**
- Modify: `internal/app/tui_pools.go` (`statusVerify`, `String`)
- Modify: `internal/app/tui_rows.go` (`tuiStatusColor`, second line)
- Modify: `internal/app/tui_render.go` (`tuiAccountView`, `tuiAccountView` struct gets `verify *eligibilityError`)
- Modify: `internal/app/tui_detail.go` (`tuiGaugeDetail`)
- Modify: `internal/app/tui.go` (key `w`, refresh after `a`)
- Modify: `internal/app/tui_views.go` (palette action)
- Modify: `internal/app/cli.go` or a new `internal/app/browser.go` (`openURL`)
- Test: `internal/app/tui_rows_test.go`, `internal/app/tui_pools_test.go`

**Interfaces:**
- Consumes: `accountEligibility` (Task 2).
- Produces: `statusVerify` with word `VERIFY`; `Application.openURL func(context.Context, string) error` (nil → `openInBrowser`); `(a *Application) openVerification(ctx context.Context, account Account) (string, bool)` returns the message to show and whether a page opened.

- [ ] **Step 1: Write the failing tests**

```go
func TestAnAccountThatNeedsVerificationSaysVerify(t *testing.T) {
	a := rowsApp()
	account := Account{"name": "New User", "eligibility": (&eligibilityError{Reason: "VALIDATION_REQUIRED", Message: "Verify", URL: "https://accounts.google.com/v"}).record(poolNow)}
	state := rowsState(map[string]Account{"n@example.invalid": account}, "n@example.invalid")
	state.quotaErrors = map[string]string{"n@example.invalid": "Google needs this account verified: Verify"}
	view := a.tuiAccountView(state, "n@example.invalid", poolNow)
	if view.status != statusVerify || view.status.String() != "VERIFY" {
		t.Fatalf("status = %v", view.status)
	}
	lines := a.tuiAccountBlock(view, state, 60, 2)
	if !strings.Contains(lines[0], "VERIFY") || !strings.Contains(lines[1], "press w") {
		t.Fatalf("lines = %q", lines)
	}
	detail := strings.Join(a.tuiGaugeDetail(state, 70, 12, tuiLayoutPlanFor(120, 40)), "\n")
	if strings.Contains(detail, "re-auth") || strings.Contains(detail, "agy-swap add") || strings.Contains(detail, "https://") {
		t.Fatalf("detail misleads or prints the link:\n%s", detail)
	}
	if !strings.Contains(detail, "Verify") || !strings.Contains(detail, "[w]") {
		t.Fatalf("detail = %s", detail)
	}
}

func TestOpenVerificationPassesTheWholeLinkToTheOpener(t *testing.T) {
	var opened []string
	a := rowsApp()
	a.openURL = func(_ context.Context, link string) error { opened = append(opened, link); return nil }
	long := "https://accounts.google.com/v?" + strings.Repeat("x", 600)
	account := Account{"eligibility": (&eligibilityError{Message: "Verify", URL: long}).record(poolNow)}
	if _, ok := a.openVerification(context.Background(), account); !ok || len(opened) != 1 || opened[0] != long {
		t.Fatalf("opened = %d links", len(opened))
	}
	noLink := Account{"eligibility": (&eligibilityError{Message: "Verify"}).record(poolNow)}
	if message, ok := a.openVerification(context.Background(), noLink); ok || !strings.Contains(message, "agy") {
		t.Fatalf("message = %q", message)
	}
}
```

`TestAccountStatusWords` stays as it is. `VERIFY` is decided in `tuiAccountView`, not in `accountStatusFor`, because it comes from the account and not from the pools.

- [ ] **Step 2:** Run → FAIL.

- [ ] **Step 3: Implement.**
  - `tui_pools.go`: append `statusVerify` after `statusAttention` and return `"VERIFY"` for it.
  - `tui_rows.go`: `tuiStatusColor` returns yellow for it by default (no change needed). In the second-line switch, add before `case view.needsAttention:`

    ```go
	case view.status == statusVerify:
		second += a.p.Yellow + "needs Google verification" + a.p.Reset + a.p.Gray + " · press w" + a.p.Reset
    ```
    and exclude `statusVerify` from the pools case.
  - `tui_render.go` `tuiAccountView`: after the attention block,

    ```go
	if ineligible, ok := accountEligibility(account); ok {
		view.verify = ineligible
		view.status = statusVerify
		view.needsAttention = false
		view.health = "Needs Google verification"
		view.tone = tuiHealthWarning
	}
    ```
  - `tui_detail.go`: add a first case to the switch:

    ```go
	case view.verify != nil:
		message := firstString(tuiText(view.verify.Message), "Google needs this account verified before Antigravity can use it.")
		rows = append(rows, a.p.Yellow+"Verification required"+a.p.Reset+" · "+message)
		if view.verify.URL != "" {
			rows = append(rows, a.p.White+"[w] open Google's verification page, then [r] to check again"+a.p.Reset)
		} else {
			rows = append(rows, a.p.White+"Run 'agy' to see Google's link, then [r] to check again"+a.p.Reset)
		}
    ```
    and skip the `Usage unavailable` line when `view.verify != nil`.
  - `browser.go`:

    ```go
	// openInBrowser hands the link to the system opener as one argument, so a
	// long link reaches the browser whole: no shell, clipboard or line wrap.
	func openInBrowser(ctx context.Context, link string) error {
		ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		var command *exec.Cmd
		switch runtime.GOOS {
		case "darwin":
			command = exec.CommandContext(ctx, "open", link)
		case "windows":
			command = exec.CommandContext(ctx, "rundll32", "url.dll,FileProtocolHandler", link)
		default:
			command = exec.CommandContext(ctx, "xdg-open", link)
		}
		return command.Run()
	}

	func (a *Application) openVerification(ctx context.Context, account Account) (string, bool) {
		ineligible, ok := accountEligibility(account)
		if !ok {
			return "This account does not need verification. Press r to check again.", false
		}
		if ineligible.URL == "" {
			return "Google sent no verification link. Run 'agy' to see it.", false
		}
		open := a.openURL
		if open == nil {
			open = openInBrowser
		}
		if err := open(ctx, ineligible.URL); err != nil {
			return "Could not open a browser: " + err.Error(), false
		}
		return "Opened Google's verification page. Press r when you are done.", true
	}
    ```
  - `tui.go`: `case "w":` on the dashboard and quota views: get the selected account, call `openVerification`, and set `state.message` and `messageType` (`"success"` when opened, `"warning"` otherwise). In `case "a":` start a refresh after a successful add: `if suspend(func() int { return a.addLoginFlow(ctx) }) == 0 { startRefresh(true) }`.
  - `tui_views.go`: add `{ID: "verify", Label: "Open verification page", Description: "Open Google's page for an account that needs verification", Shortcut: "w", Section: "Accounts", Enabled: hasAccounts}` and route `"verify"` in the palette dispatch (`tui.go:591` area) to the same code as `w`.

- [ ] **Step 4:** Run the tests, `go test ./internal/app`, `make qa` (includes tui-smoke) → PASS.

---

### Task 5: CLI: `verify` and checking during `add`

**Files:**
- Modify: `internal/app/cli.go` (`Run` dispatch, positional args, `printUsage`, `saveTokenAccount` caller in `addLoginFlow`, `cmdVerify`)
- Modify: `internal/app/display.go` (`accountStatus`)
- Test: `internal/app/cli_commands_test.go`, `internal/app/list_display_test.go`

**Interfaces:**
- Consumes: `openVerification` (Task 4), `QuotaService.Fetch` (Task 3), `resolveConfiguredTarget`.
- Produces: `agy-swap verify <account>`. Exit 0 when a page opened or the account no longer needs it; exit 1 otherwise. It always prints the link on its own line when there is one.

- [ ] **Step 1: Write the failing tests**

```go
func TestVerifyOpensTheStoredPageAndPrintsIt(t *testing.T) {
	f := newCLIFixture(t, "v@example.invalid")
	accounts, _ := f.a.store.Load(false)
	accounts.ByEmail["v@example.invalid"]["eligibility"] = (&eligibilityError{Message: "Verify", URL: "https://accounts.google.com/v"}).record(time.Now())
	if err := f.a.store.Save(accounts); err != nil {
		t.Fatal(err)
	}
	var opened string
	f.a.openURL = func(_ context.Context, link string) error { opened = link; return nil }
	if code := f.run("", "verify", "v@example.invalid"); code != 0 || opened != "https://accounts.google.com/v" {
		t.Fatalf("code=%d opened=%q out=%s err=%s", code, opened, f.out, f.errOut)
	}
	if !strings.Contains(f.out.String(), "\nhttps://accounts.google.com/v\n") {
		t.Fatalf("link not printed on its own line:\n%s", f.out)
	}
}

func TestVerifyWithoutAnOpenerPrintsTheLinkAndFails(t *testing.T) {
	f := newCLIFixture(t, "v@example.invalid")
	accounts, _ := f.a.store.Load(false)
	accounts.ByEmail["v@example.invalid"]["eligibility"] = (&eligibilityError{URL: "https://accounts.google.com/v"}).record(time.Now())
	_ = f.a.store.Save(accounts)
	f.a.openURL = func(context.Context, string) error { return exec.ErrNotFound }
	if code := f.run("", "verify", "v@example.invalid"); code != 1 || !strings.Contains(f.out.String(), "https://accounts.google.com/v") {
		t.Fatalf("code=%d out=%s", code, f.out)
	}
}

func TestListSaysAnAccountNeedsVerification(t *testing.T) {
	account := Account{"email": "v@example.invalid", "eligibility": (&eligibilityError{Message: "Verify"}).record(time.Now())}
	if got := accountStatus(account, palette{}, time.Now()); !strings.Contains(got, "Verification required") || !strings.Contains(got, "agy-swap verify") {
		t.Fatalf("status = %q", got)
	}
}
```

Extend the `add` login test that uses the fake `agy` (`internal/app/login_test.go`) with a case where `loadCodeAssist` returns the 403 eligibility body. Expect the output to contain `needs Google verification`, the stored `eligibility` record, exit code 0 (the account is saved), and that answering `n` to the prompt opens nothing.

- [ ] **Step 2:** Run → FAIL.

- [ ] **Step 3: Implement.**
  - Positional args: `case "switch", "remove", "verify":`. Dispatch: `case "verify": code = a.cmdVerify(ctx, args)`.
  - `cmdVerify`: load accounts, resolve with `resolveConfiguredTarget`. If the account has no `eligibility` record, run one `a.quota.Refresh` for it (the `selected` path) so the record is current. Print the message, then the link on its own line. Call `openVerification`.
  - Usage: `  verify    Open Google's verification page for an account` under `commands:`.
  - `addLoginFlow`: after `saveTokenAccount` returns 0, call `a.checkEligibility(ctx, email)`. It refreshes that one account. On `*eligibilityError` it prints `⚠ <email> needs Google verification before Antigravity can use it.`, the message, the link on its own line, and `Open it in your browser now? [Y/n]`. `saveTokenAccount` returns the email (change its result to `(string, int)`; the three callers are in `addLoginFlow` and `cmdAdd`).
  - `display.go` `accountStatus`: first check `accountEligibility` and return `fmt.Sprintf("[%s] %sVerification required%s · agy-swap verify %s", tier, p.Yellow, p.Reset, getString(account, "email"))`.

- [ ] **Step 4:** `go test -race ./internal/app` and `make qa` → PASS.

---

### Task 6: Documentation and Site copy

**Files (Tool):** `README.md` (status words: add `VERIFY`; command table: add `verify`; TUI key `w`), `CHANGELOG.md` (v2.14.0).
**Files (Site, private repository):** `src/content/commands.js` (add `{ id: 'verify', cmd: 'agy-swap verify <account>' }` to `check`), `src/i18n/translations.js` (`desc.verify` and the `statusBody` list in en/th/ja/zh).
**Unchanged:** story frames and the demo. The demo accounts never need verification, so `src/generated/tui-story-frames.json` changes only because of the version bump.

- [ ] **Step 1:** `cd site && bun run test` with only `commands.js` changed → FAIL (`site-redesign.test.mjs` checks every command against the translations and `agy-swap --help`).
- [ ] **Step 2:** Add the four translations and the `VERIFY` word in `statusBody` (four languages).
- [ ] **Step 3:** `bun run test` and `bun run build` → PASS.

---

### Task 7: Version and delivery

- [ ] `make bump VERSION=2.14.0` (Tool `Makefile`, installers, story frames; Site `package.json`, `package-lock.json`, `index.html`) and the CHANGELOG entry.
- [ ] `make release-check`, then check `command -v agy-swap` and `agy-swap --version` (expect `v2.14.0-dev`).
- [ ] Owner check with the affected account: the TUI row shows `VERIFY`, `w` opens Google's page, verify there, `r` → the pools show. Also `agy-swap verify <account>` and `agy-swap add` with an account that needs verification. Site: Vite dev URL and `make site-preview` show the new command in all four languages.
- [ ] Review gate → commits and annotated tags `v2.14.0` in both repositories → `make release-assets` → GitHub Release → Homebrew tap → `make site-release VERSION=2.14.0 PUSH=1` → `make site-verify-live VERSION=2.14.0` → `make release-evidence VERSION=2.14.0` → `docs/releases/v2.14.0.md`.

## Known limits

- The Task 1 probe needs an account that currently needs verification. If none is available when the work starts, Tasks 2–3 rely on the field names in the `agy` executable, and the fixtures say so.
- agy-swap shows Google's link but does not complete the verification. The user verifies in the browser.
- On a terminal without a browser (SSH), `w` reports that no browser opened; `agy-swap verify` prints the link on one line for copying.
