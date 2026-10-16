package app

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func modelsWith(entries map[string]map[string]any) map[string]any {
	models := map[string]any{}
	for id, quotaInfo := range entries {
		entry := map[string]any{"model": id}
		if quotaInfo != nil {
			entry["quotaInfo"] = quotaInfo
		}
		models[id] = entry
	}
	return map[string]any{"models": models}
}

func TestGeminiGroupFromModelsUsesTheModelQuotaAndInfersTheWindowFromTheReset(t *testing.T) {
	now := time.Date(2026, 10, 16, 13, 45, 0, 0, time.UTC)
	cases := []struct {
		name       string
		reset      string
		wantWindow string
	}{
		{"a reset within five hours is the 5-hour limit", "2026-10-16T15:28:00Z", "5h"},
		{"a reset days away is the weekly limit", "2026-10-23T13:41:40Z", "weekly"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			group := geminiGroupFromModels(modelsWith(map[string]map[string]any{
				"gemini-3-flash": {"remainingFraction": 0.6437, "resetTime": tc.reset},
				"gemini-2.5-pro": {"remainingFraction": 0.6437, "resetTime": tc.reset},
				"chat_20706":     {"remainingFraction": 0.01},
				"claude-sonnet":  {"remainingFraction": 0.9, "resetTime": tc.reset},
			}), now)
			if group == nil {
				t.Fatal("no Gemini group")
			}
			if getString(group, "id") != "gemini" {
				t.Fatalf("group = %#v", group)
			}
			buckets := getSlice(group["buckets"])
			if len(buckets) != 1 {
				t.Fatalf("buckets = %#v", buckets)
			}
			bucket := getMap(buckets[0])
			fraction, _ := getFloat(bucket["remaining_fraction"])
			if getString(bucket, "window") != tc.wantWindow || fraction != 0.6437 || getString(bucket, "reset_at") != tc.reset {
				t.Fatalf("bucket = %#v", bucket)
			}
		})
	}
}

func TestGeminiGroupFromModelsTakesTheTightestModelAndReadsAMissingFractionAsEmpty(t *testing.T) {
	now := time.Date(2026, 10, 16, 13, 45, 0, 0, time.UTC)
	group := geminiGroupFromModels(modelsWith(map[string]map[string]any{
		"gemini-a": {"remainingFraction": 0.5, "resetTime": "2026-10-23T13:41:40Z"},
		"gemini-b": {"resetTime": "2026-10-23T13:41:40Z"}, // proto3 omits a zero fraction
	}), now)
	bucket := getMap(getSlice(group["buckets"])[0])
	if fraction, _ := getFloat(bucket["remaining_fraction"]); fraction != 0 {
		t.Fatalf("fraction = %v, want 0", fraction)
	}
}

func TestGeminiGroupFromModelsIsNilWithoutGeminiQuota(t *testing.T) {
	now := time.Now()
	if geminiGroupFromModels(modelsWith(map[string]map[string]any{"claude-x": {"remainingFraction": 1.0, "resetTime": "2030-01-01T00:00:00Z"}}), now) != nil {
		t.Fatal("expected no group")
	}
	if geminiGroupFromModels(modelsWith(map[string]map[string]any{"gemini-x": nil}), now) != nil {
		t.Fatal("a model without quotaInfo must not make a group")
	}
	if geminiGroupFromModels(modelsWith(map[string]map[string]any{"gemini-x": {"remainingFraction": 0.5}}), now) != nil {
		t.Fatal("quota without a reset time must not make a group")
	}
	if geminiGroupFromModels(map[string]any{}, now) != nil {
		t.Fatal("empty response must not make a group")
	}
}

func TestQuotaGroupsFromSummarySkipsDisabledBuckets(t *testing.T) {
	summary := summaryGroups(map[string]any{"displayName": "Claude and GPT models", "buckets": []any{
		map[string]any{"bucketId": "3p-weekly", "window": "weekly", "remainingFraction": 0.0, "resetTime": "2030-01-01T00:00:00Z"},
		map[string]any{"bucketId": "3p-5h", "window": "5h", "remainingFraction": 1.0, "resetTime": "2030-01-01T00:00:00Z", "disabled": true},
	}})
	groups, notes := quotaGroupsFromSummary(summary)
	if got := len(getSlice(getMap(groups[0])["buckets"])); got != 1 {
		t.Fatalf("kept %d buckets, want only the weekly one", got)
	}
	if !strings.Contains(strings.Join(notes, "\n"), `bucket "3p-5h" window "5h": skipped, disabled`) {
		t.Fatalf("notes = %v", notes)
	}
}

func geminiQuotaServer(t *testing.T, summary, models string, modelCalls *atomic.Int32) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/loadCodeAssist":
			fmt.Fprint(w, `{"cloudaicompanionProject":"p","paidTier":{"id":"g1-pro-tier"}}`)
		case "/retrieveUserQuotaSummary":
			fmt.Fprint(w, summary)
		case "/fetchAvailableModels":
			modelCalls.Add(1)
			if models == "" {
				http.NotFound(w, r)
				return
			}
			fmt.Fprint(w, models)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server
}

func fetchQuota(t *testing.T, server *httptest.Server) map[string]any {
	t.Helper()
	httpService := NewHTTPService(io.Discard)
	httpService.oauthURL = server.URL + "/oauth"
	httpService.cloudAPI = server.URL + "/"
	quota := NewQuotaService(httpService, nil)
	account := Account{"email": "user@example.com", "token_data": tokenBlob(t, "user@example.com", true, "r", time.Now().Add(time.Hour))}
	snapshot, err := quota.Fetch(context.Background(), account)
	if err != nil {
		t.Fatal(err)
	}
	return snapshot
}

const thirdPartyOnlySummary = `{"groups":[{"displayName":"Claude and GPT models","buckets":[{"bucketId":"3p-weekly","displayName":"Weekly","window":"weekly","remainingFraction":0.2,"resetTime":"2030-01-01T00:00:00Z"}]}]}`

func TestQuotaFetchAddsGeminiFromTheModelListWhenTheSummaryHasNone(t *testing.T) {
	var modelCalls atomic.Int32
	reset := time.Now().Add(90 * time.Minute).UTC().Format(time.RFC3339)
	server := geminiQuotaServer(t, thirdPartyOnlySummary, fmt.Sprintf(`{"models":{"gemini-3-flash":{"quotaInfo":{"remainingFraction":0.6,"resetTime":%q}}}}`, reset), &modelCalls)
	groups := getSlice(fetchQuota(t, server)["groups"])
	if len(groups) != 2 || getString(getMap(groups[0]), "id") != "gemini" || getString(getMap(groups[1]), "id") != "third_party" {
		t.Fatalf("groups = %#v", groups)
	}
	if modelCalls.Load() != 1 {
		t.Fatalf("model list requested %d times, want 1", modelCalls.Load())
	}
}

func TestQuotaFetchDoesNotAskForModelsWhenTheSummaryAlreadyHasGemini(t *testing.T) {
	var modelCalls atomic.Int32
	summary := `{"groups":[{"displayName":"Gemini","buckets":[{"bucketId":"gemini-weekly","window":"weekly","remainingFraction":0.4,"resetTime":"2030-01-01T00:00:00Z"}]}]}`
	server := geminiQuotaServer(t, summary, "", &modelCalls)
	fetchQuota(t, server)
	if modelCalls.Load() != 0 {
		t.Fatalf("model list requested %d times, want 0", modelCalls.Load())
	}
}

func TestQuotaFetchKeepsTheSummaryWhenTheModelListFails(t *testing.T) {
	var modelCalls atomic.Int32
	server := geminiQuotaServer(t, thirdPartyOnlySummary, "", &modelCalls)
	groups := getSlice(fetchQuota(t, server)["groups"])
	if len(groups) != 1 || getString(getMap(groups[0]), "id") != "third_party" {
		t.Fatalf("groups = %#v", groups)
	}
}
