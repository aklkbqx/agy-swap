package app

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func summaryGroups(raw ...map[string]any) map[string]any {
	groups := make([]any, 0, len(raw))
	for _, group := range raw {
		groups = append(groups, group)
	}
	return map[string]any{"groups": groups}
}

func TestQuotaGroupsFromSummaryExplainsEveryDroppedBucket(t *testing.T) {
	summary := summaryGroups(
		map[string]any{"displayName": "Gemini Models", "buckets": []any{
			map[string]any{"bucketId": "gemini-weekly", "displayName": "Weekly", "window": "weekly", "remainingFraction": 0.4, "resetTime": "2030-01-01T00:00:00Z"},
			map[string]any{"bucketId": "gemini-daily", "displayName": "Daily", "window": "daily", "remainingFraction": 0.9, "resetTime": "2030-01-01T00:00:00Z"},
			map[string]any{"bucketId": "gemini-5h", "displayName": "Five Hour", "window": "5h", "remainingFraction": 1.0},
		}},
		map[string]any{"displayName": "Mystery", "buckets": []any{
			map[string]any{"bucketId": "models-flash-weekly", "displayName": "Weekly", "window": "weekly", "remainingFraction": 0.5, "resetTime": "2030-01-01T00:00:00Z"},
		}},
	)
	groups, notes := quotaGroupsFromSummary(summary)
	if len(groups) != 1 || getString(getMap(groups[0]), "id") != "gemini" {
		t.Fatalf("groups = %#v", groups)
	}
	if got := len(getSlice(getMap(groups[0])["buckets"])); got != 1 {
		t.Fatalf("kept %d Gemini buckets, want only the weekly one", got)
	}
	report := strings.Join(notes, "\n")
	for _, want := range []string{
		`bucket "gemini-daily" window "daily": skipped, window is not weekly or 5h`,
		`bucket "gemini-5h" window "5h": skipped, no reset time`,
		`bucket "models-flash-weekly" window "weekly": group dropped, bucket id has no gemini- or 3p- prefix`,
		`bucket "gemini-weekly" window "weekly": kept as gemini`,
	} {
		if !strings.Contains(report, want) {
			t.Fatalf("report is missing %q:\n%s", want, report)
		}
	}
}

func TestQuotaFetchPrintsDiagnosticsWhenAskedAndNeverTheToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/loadCodeAssist":
			fmt.Fprint(w, `{"cloudaicompanionProject":"p","paidTier":{"id":"g1-pro-tier"}}`)
		case "/retrieveUserQuotaSummary":
			fmt.Fprint(w, `{"groups":[{"displayName":"Third Party","buckets":[{"bucketId":"3p-weekly","displayName":"Weekly","window":"weekly","remainingFraction":0.2,"resetTime":"2030-01-01T00:00:00Z"}]},{"displayName":"Gemini","buckets":[{"bucketId":"gemini-monthly","displayName":"Monthly","window":"monthly","remainingFraction":0.3,"resetTime":"2030-01-01T00:00:00Z"}]}]}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	run := func(debug io.Writer) {
		httpService := NewHTTPService(io.Discard)
		httpService.oauthURL = server.URL + "/oauth"
		httpService.cloudAPI = server.URL + "/"
		quota := NewQuotaService(httpService, nil)
		quota.SetDiagnostics(debug)
		account := Account{"email": "user@example.com", "token_data": tokenBlob(t, "user@example.com", true, "r", time.Now().Add(time.Hour))}
		if _, err := quota.Fetch(context.Background(), account); err != nil {
			t.Fatal(err)
		}
	}
	run(nil) // diagnostics are off unless a writer is set
	var loud bytes.Buffer
	run(&loud)
	out := loud.String()
	if !strings.Contains(out, `bucket "gemini-monthly" window "monthly": skipped, window is not weekly or 5h`) || !strings.Contains(out, "quota: summary\n") {
		t.Fatalf("diagnostics = %q", out)
	}
	for _, secret := range []string{"user@example.com", "Bearer", "token_data", "access"} {
		if strings.Contains(out, secret) {
			t.Fatalf("diagnostics leak %q: %q", secret, out)
		}
	}
}

func TestQuotaGroupsFromSummaryDescribesTheResponseShape(t *testing.T) {
	summary := map[string]any{
		"groups": []any{
			map[string]any{"displayName": "Gemini Models", "groupId": "g", "buckets": []any{}},
			map[string]any{"displayName": "Claude and GPT Models", "buckets": []any{
				map[string]any{"bucketId": "3p-weekly", "window": "weekly", "remainingFraction": 0.5, "resetTime": "2030-01-01T00:00:00Z", "extra": true},
			}},
		},
		"models": []any{map[string]any{"name": "x"}},
	}
	_, notes := quotaGroupsFromSummary(summary)
	report := strings.Join(notes, "\n")
	for _, want := range []string{
		`response fields: groups, models`,
		`group "Gemini Models": 0 buckets, fields: buckets, displayName, groupId`,
		`group "Claude and GPT Models": 1 buckets, fields: buckets, displayName; bucket fields: bucketId, extra, remainingFraction, resetTime, window`,
	} {
		if !strings.Contains(report, want) {
			t.Fatalf("report is missing %q:\n%s", want, report)
		}
	}
	for _, secret := range []string{"2030-01-01", "0.5"} {
		if strings.Contains(report, secret) {
			t.Fatalf("report leaks a value %q:\n%s", secret, report)
		}
	}
}

func TestShapeOfNamesFieldsAndNeverValues(t *testing.T) {
	value := map[string]any{
		"models": map[string]any{
			"model-a": map[string]any{"displayName": "Secret Name", "quotaInfo": map[string]any{"remainingFraction": 0.123, "resetTime": "2031-02-03T04:05:06Z"}},
		},
		"list": []any{map[string]any{"id": "abc"}, map[string]any{"id": "def"}},
	}
	got := shapeOf(value, 5)
	for _, want := range []string{"models:{model-a:{displayName, quotaInfo:{remainingFraction, resetTime}}}", "list:[2]{id}"} {
		if !strings.Contains(got, want) {
			t.Fatalf("shape is missing %q: %s", want, got)
		}
	}
	for _, secret := range []string{"Secret Name", "0.123", "2031", "abc"} {
		if strings.Contains(got, secret) {
			t.Fatalf("shape leaks %q: %s", secret, got)
		}
	}
}

func TestQuotaFetchDiagnosticsAlsoDescribeTheModelList(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/loadCodeAssist":
			fmt.Fprint(w, `{"cloudaicompanionProject":"p","paidTier":{"id":"g1-pro-tier"}}`)
		case "/retrieveUserQuotaSummary":
			fmt.Fprint(w, `{"groups":[{"displayName":"Third Party","buckets":[{"bucketId":"3p-weekly","window":"weekly","remainingFraction":0.2,"resetTime":"2030-01-01T00:00:00Z"}]}]}`)
		case "/fetchAvailableModels":
			fmt.Fprint(w, `{"models":{"gemini-x":{"quotaInfo":{"remainingFraction":0.9,"resetTime":"2030-01-01T00:00:00Z"}}}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	httpService := NewHTTPService(io.Discard)
	httpService.oauthURL = server.URL + "/oauth"
	httpService.cloudAPI = server.URL + "/"
	quota := NewQuotaService(httpService, nil)
	var out bytes.Buffer
	quota.SetDiagnostics(&out)
	account := Account{"email": "user@example.com", "token_data": tokenBlob(t, "user@example.com", true, "r", time.Now().Add(time.Hour))}
	if _, err := quota.Fetch(context.Background(), account); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "fetchAvailableModels fields: {models:{gemini-x}}") {
		t.Fatalf("diagnostics = %q", out.String())
	}
}

func TestModelQuotaLinesListQuotaBearingModelsAndTheirGroups(t *testing.T) {
	models := map[string]any{
		"models": map[string]any{
			"gemini-x": map[string]any{"quotaInfo": map[string]any{"remainingFraction": 0.9, "resetTime": "2030-01-01T00:00:00Z"}},
			"gemini-y": map[string]any{"quotaInfo": map[string]any{"remainingFraction": 0.1}},
			"no-quota": map[string]any{"displayName": "N"},
			"claude-z": map[string]any{"quotaInfo": map[string]any{"resetTime": "2030-01-01T00:00:00Z"}},
		},
		"agentModelSorts": []any{
			map[string]any{"displayName": "Recommended", "groups": []any{map[string]any{"modelIds": []any{"gemini-x", "gemini-y"}}, map[string]any{"modelIds": []any{"claude-z"}}}},
		},
	}
	got := strings.Join(modelQuotaLines(models), "\n")
	for _, want := range []string{
		"model gemini-x: quotaInfo remainingFraction=0.9, resetTime=2030-01-01T00:00:00Z",
		"model gemini-y: quotaInfo remainingFraction=0.1",
		"model claude-z: quotaInfo resetTime=2030-01-01T00:00:00Z",
		`model group "Recommended" 1: gemini-x, gemini-y`,
		`model group "Recommended" 2: claude-z`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "no-quota") || strings.Contains(got, "displayName") {
		t.Fatalf("unexpected content:\n%s", got)
	}
}

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
	var ineligible *eligibilityError
	if !errors.As(err, &ineligible) {
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
