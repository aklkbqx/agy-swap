package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestEligibilityFromErrorBodyReadsTheVerificationPage(t *testing.T) {
	body := []byte(`{"error":{"code":403,"message":"denied","details":[{"@type":"type.googleapis.com/google.rpc.ErrorInfo","reason":"VALIDATION_REQUIRED","metadata":{"validation_url":"https://accounts.google.com/v?x=1","uiMessage":"Verify your account to continue."}}]}}`)
	got := eligibilityFromErrorBody(403, body)
	if got == nil || got.Reason != "VALIDATION_REQUIRED" || got.Message != "Verify your account to continue." || got.URL != "https://accounts.google.com/v?x=1" {
		t.Fatalf("got %#v", got)
	}
}

func TestEligibilityFromErrorBodyFallsBackToTheAppealPage(t *testing.T) {
	body := []byte(`{"error":{"details":[{"reason":"TOS_VIOLATION","metadata":{"appeal_url":"https://support.google.com/appeal","uiMessage":"Appeal"}}]}}`)
	if got := eligibilityFromErrorBody(403, body); got == nil || got.URL != "https://support.google.com/appeal" {
		t.Fatalf("got %#v", got)
	}
}

func TestEligibilityKeepsTheMessageWhenTheLinkIsMissingOrUnsafe(t *testing.T) {
	for _, link := range []string{"", "http://accounts.google.com/v", "https://accounts.google.com.evil.example/v", "https://user@accounts.google.com/v", "javascript:alert(1)"} {
		body := []byte(fmt.Sprintf(`{"error":{"details":[{"reason":"VALIDATION_REQUIRED","metadata":{"validation_url":%q,"uiMessage":"Verify"}}]}}`, link))
		got := eligibilityFromErrorBody(403, body)
		if got == nil || got.URL != "" || got.Message != "Verify" {
			t.Fatalf("%q: got %#v", link, got)
		}
	}
}

func TestOrdinaryErrorsAreNotEligibilityErrors(t *testing.T) {
	for _, body := range []string{"", "not json", `{"error":{"code":403,"message":"The caller does not have permission"}}`, `{"error":{"details":[{"reason":"RATE_LIMIT_EXCEEDED"}]}}`} {
		if got := eligibilityFromErrorBody(403, []byte(body)); got != nil {
			t.Fatalf("%q: got %#v", body, got)
		}
	}
}

func TestEligibilityFromLoadReadsIneligibleTiers(t *testing.T) {
	info := map[string]any{"ineligibleTiers": []any{map[string]any{"reasonCode": "VALIDATION_REQUIRED", "reasonMessage": "Verify your account", "validationUrl": "https://accounts.google.com/v"}}}
	got := eligibilityFromLoad(info)
	if got == nil || got.URL != "https://accounts.google.com/v" || got.Message != "Verify your account" || got.Reason != "VALIDATION_REQUIRED" {
		t.Fatalf("got %#v", got)
	}
	usable := map[string]any{"cloudaicompanionProject": "p", "currentTier": map[string]any{"id": "free-tier"}, "ineligibleTiers": info["ineligibleTiers"]}
	if got := eligibilityFromLoad(usable); got != nil {
		t.Fatalf("an account with a usable tier is not ineligible: %#v", got)
	}
	if got := eligibilityFromLoad(map[string]any{"cloudaicompanionProject": "p", "currentTier": map[string]any{"id": "free-tier"}}); got != nil {
		t.Fatalf("an ordinary answer is not ineligible: %#v", got)
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
	if _, ok := accountEligibility(Account{}); ok {
		t.Fatal("an account without a record needs no verification")
	}
}

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
	reloaded, err := store.Load(false)
	if err != nil {
		t.Fatal(err)
	}
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

func TestFetchReportsIneligibleTiersFromASuccessfulAnswer(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"ineligibleTiers":[{"reasonCode":"VALIDATION_REQUIRED","reasonMessage":"Verify","validationUrl":"https://accounts.google.com/v"}]}`)
	}))
	defer server.Close()
	httpService := NewHTTPService(io.Discard)
	httpService.cloudAPI = server.URL + "/"
	email := "v@example.invalid"
	_, err := NewQuotaService(httpService, nil).Fetch(context.Background(), Account{"email": email, "token_data": tokenBlob(t, email, true, "r", time.Now().Add(time.Hour))})
	if ineligible, ok := err.(*eligibilityError); !ok || ineligible.URL != "https://accounts.google.com/v" {
		t.Fatalf("err = %#v", err)
	}
}

func TestOnlyAVerificationSignalBecomesVerify(t *testing.T) {
	withMessage := []byte(`{"error":{"details":[{"reason":"RATE_LIMIT_EXCEEDED","metadata":{"uiMessage":"Try again later"}}]}}`)
	for _, status := range []int{429, 500, 503} {
		if got := eligibilityFromErrorBody(status, withMessage); got != nil {
			t.Fatalf("HTTP %d: got %#v", status, got)
		}
	}
	if got := eligibilityFromErrorBody(403, withMessage); got != nil {
		t.Fatalf("a 403 message without a link or a verification reason: %#v", got)
	}
	verifyLink := []byte(`{"error":{"details":[{"reason":"VALIDATION_REQUIRED","metadata":{"validation_url":"https://accounts.google.com/v"}}]}}`)
	if got := eligibilityFromErrorBody(500, verifyLink); got != nil {
		t.Fatalf("a server error is not a verification request: %#v", got)
	}
	noLink := []byte(`{"error":{"details":[{"reason":"VALIDATION_REQUIRED","metadata":{"uiMessage":"Verify your account to continue."}}]}}`)
	if got := eligibilityFromErrorBody(403, noLink); got == nil || got.URL != "" {
		t.Fatalf("a verification reason without a link: %#v", got)
	}
	notOffered := map[string]any{"allowedTiers": []any{map[string]any{"id": "free-tier"}}, "ineligibleTiers": []any{map[string]any{"reasonCode": "UNSUPPORTED_TIER", "reasonMessage": "standard-tier not available"}}}
	if got := eligibilityFromLoad(notOffered); got != nil {
		t.Fatalf("a tier that is not offered is not a verification request: %#v", got)
	}
}

func TestAuthFailuresClearAStoredVerification(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, `{"error":"invalid_grant"}`)
	}))
	defer server.Close()
	httpService := NewHTTPService(io.Discard)
	httpService.oauthURL = server.URL + "/oauth"
	httpService.cloudAPI = server.URL + "/"
	service := NewQuotaService(httpService, NewStore(testPaths(t)))
	email := "v@example.invalid"
	accounts := NewAccounts()
	accounts.Set(email, Account{"email": email, "token_data": tokenBlob(t, email, true, "r", time.Now().Add(-time.Hour)), "eligibility": (&eligibilityError{Message: "Verify", URL: "https://accounts.google.com/v"}).record(time.Now())})
	failures := service.Refresh(context.Background(), accounts, true, nil)
	if !strings.Contains(failures[email], "OAuth") {
		t.Fatalf("failures = %v", failures)
	}
	if _, ok := accountEligibility(accounts.ByEmail[email]); ok {
		t.Fatal("a sign-in failure must not keep showing VERIFY")
	}
}

func TestEligibilityMatchesAntigravityProtocolEdgeCases(t *testing.T) {
	// 1. HTTP 400 FAILED_PRECONDITION with google.rpc.ErrorInfo
	body400 := []byte(`{"error":{"code":400,"status":"FAILED_PRECONDITION","details":[{"@type":"type.googleapis.com/google.rpc.ErrorInfo","reason":"VALIDATION_REQUIRED","metadata":{"validation_url":"https://antigravity.google/verify?t=1","uiMessage":"Verify your account."}}]}}`)
	if got := eligibilityFromErrorBody(400, body400); got == nil || got.URL != "https://antigravity.google/verify?t=1" {
		t.Fatalf("400 ErrorInfo: got %#v", got)
	}
	// 2. Non-ErrorInfo detail with incidental metadata is ignored when @type is a different protobuf
	otherDetail := []byte(`{"error":{"code":403,"details":[{"@type":"type.googleapis.com/google.rpc.Help","reason":"OTHER","metadata":{"validation_url":"https://accounts.google.com/v"}}]}}`)
	if got := eligibilityFromErrorBody(403, otherDetail); got != nil {
		t.Fatalf("non-ErrorInfo detail must be ignored: %#v", got)
	}
	// 3. loadCodeAssist with allowed free-tier + unrelated ineligible tier with URL must NOT false-positive
	usableFreeTier := map[string]any{
		"allowedTiers":    []any{map[string]any{"id": "free-tier", "isDefault": true}},
		"ineligibleTiers": []any{map[string]any{"tierId": "enterprise-tier", "reasonCode": "UNSUPPORTED_TIER", "reasonMessage": "Upgrade required", "validationUrl": "https://cloud.google.com/upgrade"}},
	}
	if got := eligibilityFromLoad(usableFreeTier); got != nil {
		t.Fatalf("allowed free-tier with unrelated ineligible tier must not require verification: %#v", got)
	}
	// 4. INELIGIBLE_ACCOUNT with verification message and missing validationUrl must become VERIFY
	missingLink := map[string]any{
		"ineligibleTiers": []any{map[string]any{"tierId": "free-tier", "reasonCode": "INELIGIBLE_ACCOUNT", "reasonMessage": "Verify your account to continue."}},
	}
	if got := eligibilityFromLoad(missingLink); got == nil || got.URL != "" || got.Reason != "INELIGIBLE_ACCOUNT" {
		t.Fatalf("missing validationUrl under INELIGIBLE_ACCOUNT: got %#v", got)
	}
	// 5. Bare "https://google/path" is rejected while "*.google" is accepted
	if got := safeGoogleURL("https://google/verify"); got != "" {
		t.Fatalf("bare google host must be rejected: %q", got)
	}
}

func TestFetchOnboardsNewAccountAndExtractsOperationEligibility(t *testing.T) {
	var mode atomic.Value
	mode.Store("op-verify")
	var polls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, ":loadCodeAssist"):
			fmt.Fprint(w, `{"allowedTiers":[{"id":"free-tier","isDefault":true}]}`)
		case strings.HasSuffix(r.URL.Path, ":onboardUser"):
			if mode.Load() == "op-verify" {
				fmt.Fprint(w, `{"name":"operations/op-1","done":true,"error":{"code":7,"message":"denied","details":[{"@type":"type.googleapis.com/google.rpc.ErrorInfo","reason":"VALIDATION_REQUIRED","metadata":{"validation_url":"https://accounts.google.com/v?op=1","uiMessage":"Verify during onboarding"}}]}}`)
				return
			}
			fmt.Fprint(w, `{"name":"operations/op-2","done":false}`)
		case strings.HasSuffix(r.URL.Path, "/operations/op-2") && r.Method == http.MethodGet:
			polls.Add(1)
			fmt.Fprint(w, `{"name":"operations/op-2","done":true,"response":{"cloudaicompanionProject":{"id":"proj-onboarded"}}}`)
		case strings.HasSuffix(r.URL.Path, ":retrieveUserQuotaSummary"):
			fmt.Fprint(w, `{"groups":[{"buckets":[{"bucketId":"gemini-weekly","displayName":"Weekly","window":"weekly","remainingFraction":0.75,"resetTime":"2030-01-01T00:00:00Z"}]}]}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	httpService := NewHTTPService(io.Discard)
	httpService.cloudAPI = server.URL + "/v1internal:"
	service := NewQuotaService(httpService, nil)
	service.pollInterval = time.Millisecond
	email := "new@example.invalid"
	account := Account{"email": email, "token_data": tokenBlob(t, email, true, "r", time.Now().Add(time.Hour))}

	_, err := service.Fetch(context.Background(), account)
	var ineligible *eligibilityError
	if !errors.As(err, &ineligible) || ineligible.URL != "https://accounts.google.com/v?op=1" {
		t.Fatalf("expected onboarding eligibility error, got %#v", err)
	}

	mode.Store("op-ok")
	snapshot, err := service.Fetch(context.Background(), account)
	if err != nil || polls.Load() == 0 || getString(getMap(snapshot["tier"]), "name") != "Free" {
		t.Fatalf("expected onboarded snapshot, polls=%d err=%v snapshot=%#v", polls.Load(), err, snapshot)
	}
}

func TestCaptivePortalAndOAuth5xxKeepStoredVerification(t *testing.T) {
	var step atomic.Value
	step.Store("oauth-503")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch step.Load() {
		case "oauth-503":
			w.WriteHeader(http.StatusServiceUnavailable)
		case "captive-html":
			w.Header().Set("Content-Type", "text/html")
			fmt.Fprint(w, `<html>captive portal</html>`)
		}
	}))
	defer server.Close()

	httpService := NewHTTPService(io.Discard)
	httpService.oauthURL = server.URL + "/oauth"
	httpService.cloudAPI = server.URL + "/"
	service := NewQuotaService(httpService, NewStore(testPaths(t)))
	email := "v@example.invalid"

	accounts := NewAccounts()
	accounts.Set(email, Account{
		"email":       email,
		"token_data":  tokenBlob(t, email, true, "r", time.Now().Add(-time.Hour)),
		"eligibility": (&eligibilityError{Reason: "VALIDATION_REQUIRED", Message: "Verify", URL: "https://accounts.google.com/v"}).record(time.Now()),
	})
	service.Refresh(context.Background(), accounts, true, nil)
	if _, ok := accountEligibility(accounts.ByEmail[email]); !ok {
		t.Fatal("OAuth 503 outage must not erase stored verification")
	}

	step.Store("captive-html")
	accounts.ByEmail[email]["token_data"] = tokenBlob(t, email, true, "r", time.Now().Add(time.Hour))
	service.Refresh(context.Background(), accounts, true, nil)
	if _, ok := accountEligibility(accounts.ByEmail[email]); !ok {
		t.Fatal("HTML captive-portal parse failure must not erase stored verification")
	}
}
