package app

import (
	"strings"
	"testing"
	"time"
)

// proArgonSummary is the shape Google sends a Pro account once the client is
// new enough: Gemini, Claude/GPT and a separate weekly Argon pool.
func proArgonSummary() map[string]any {
	return summaryGroups(
		map[string]any{"displayName": "Gemini Models", "buckets": []any{
			map[string]any{"bucketId": "gemini-weekly", "displayName": "Weekly Limit Remaining", "window": "weekly", "remainingFraction": 0.98, "resetTime": "2030-01-07T00:00:00Z"},
			map[string]any{"bucketId": "gemini-5h", "displayName": "Five Hour Limit Remaining", "window": "5h", "remainingFraction": 0.97, "resetTime": "2030-01-01T05:00:00Z"},
		}},
		map[string]any{"displayName": "Claude and GPT models", "buckets": []any{
			map[string]any{"bucketId": "3p-weekly", "displayName": "Weekly Limit Remaining", "window": "weekly", "remainingFraction": 1.0, "resetTime": "2030-01-07T00:00:00Z"},
		}},
		map[string]any{"displayName": "Gemini 4 Argon", "buckets": []any{
			map[string]any{"bucketId": "gemini-argon-wtu-weekly", "displayName": "Gemini 4 Argon Weekly Limit", "window": "weekly", "remainingFraction": 0.88, "resetTime": "2030-01-07T00:00:00Z"},
		}},
	)
}

func groupIDs(groups []any) []string {
	ids := make([]string, 0, len(groups))
	for _, raw := range groups {
		ids = append(ids, getString(getMap(raw), "id"))
	}
	return ids
}

func TestQuotaGroupsFromSummaryKeepsArgonApartFromGemini(t *testing.T) {
	groups, notes := quotaGroupsFromSummary(proArgonSummary())
	if got := strings.Join(groupIDs(groups), ","); got != "gemini,third_party,argon" {
		t.Fatalf("group ids = %s", got)
	}
	argon := getMap(groups[2])
	if getString(argon, "name") != "Gemini 4 Argon" || len(getSlice(argon["buckets"])) != 1 {
		t.Fatalf("argon group = %#v", argon)
	}
	if gemini := getSlice(getMap(groups[0])["buckets"]); len(gemini) != 2 {
		t.Fatalf("gemini kept %d buckets, want 2 (Argon must not join it)", len(gemini))
	}
	if !strings.Contains(strings.Join(notes, "\n"), `bucket "gemini-argon-wtu-weekly" window "weekly": kept as argon`) {
		t.Fatalf("notes = %s", strings.Join(notes, "\n"))
	}
}

func TestQuotaGroupsFromSummaryKeepsUnknownPoolsAndWindows(t *testing.T) {
	summary := summaryGroups(map[string]any{"displayName": "Veo Preview", "buckets": []any{
		map[string]any{"bucketId": "veo-preview-monthly", "displayName": "Monthly Limit", "window": "monthly", "remainingFraction": 0.5, "resetTime": "2030-02-01T00:00:00Z"},
	}})
	groups, _ := quotaGroupsFromSummary(summary)
	if len(groups) != 1 || getString(getMap(groups[0]), "id") != "veo_preview" {
		t.Fatalf("groups = %#v", groups)
	}
	bucket := getMap(getSlice(getMap(groups[0])["buckets"])[0])
	if getString(bucket, "window") != "monthly" {
		t.Fatalf("bucket = %#v", bucket)
	}
	if _, err := normalizeQuotaSnapshot(map[string]any{"observed_at": "2030-01-01T00:00:00Z", "tier": map[string]any{"id": "t", "name": "T"}, "groups": groups}, "user@example.com"); err != nil {
		t.Fatalf("store rejected an unknown pool: %v", err)
	}
}

func TestQuotaGroupsFromSummaryMergesGroupsThatShareAnID(t *testing.T) {
	summary := summaryGroups(
		map[string]any{"displayName": "Gemini Models", "buckets": []any{
			map[string]any{"bucketId": "gemini-weekly", "window": "weekly", "remainingFraction": 0.5, "resetTime": "2030-01-07T00:00:00Z"},
		}},
		map[string]any{"displayName": "Gemini Models", "buckets": []any{
			map[string]any{"bucketId": "gemini-5h", "window": "5h", "remainingFraction": 0.4, "resetTime": "2030-01-01T05:00:00Z"},
		}},
	)
	groups, _ := quotaGroupsFromSummary(summary)
	if len(groups) != 1 || len(getSlice(getMap(groups[0])["buckets"])) != 2 {
		t.Fatalf("groups = %#v", groups)
	}
}

func TestNormalizeQuotaSnapshotRejectsMalformedIDsAndWindows(t *testing.T) {
	snapshot := func(groupID, window string) map[string]any {
		return map[string]any{"observed_at": "2030-01-01T00:00:00Z", "tier": map[string]any{"id": "t", "name": "T"}, "groups": []any{
			map[string]any{"id": groupID, "name": "Pool", "buckets": []any{
				map[string]any{"id": "b", "name": "B", "window": window, "remaining_fraction": 0.5, "reset_at": "2030-01-02T00:00:00Z"},
			}},
		}}
	}
	for _, ok := range [][2]string{{"argon", "weekly"}, {"gemini", "daily"}, {"veo_preview", "monthly"}} {
		if _, err := normalizeQuotaSnapshot(snapshot(ok[0], ok[1]), "u"); err != nil {
			t.Fatalf("%v rejected: %v", ok, err)
		}
	}
	for _, bad := range [][2]string{{"Bad Id", "weekly"}, {"", "weekly"}, {"gemini", "1 week"}, {"gemini", ""}, {strings.Repeat("a", 40), "weekly"}} {
		if _, err := normalizeQuotaSnapshot(snapshot(bad[0], bad[1]), "u"); err == nil {
			t.Fatalf("%v accepted", bad)
		}
	}
}

func TestGeminiGroupFromModelsIgnoresArgonModels(t *testing.T) {
	now := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
	models := map[string]any{"models": map[string]any{
		"gemini-3.8-flash-high": map[string]any{"quotaInfo": map[string]any{"remainingFraction": 0.9, "resetTime": "2030-01-07T00:00:00Z"}},
		"gemini-4-argon-high":   map[string]any{"quotaInfo": map[string]any{"remainingFraction": 0.1, "resetTime": "2030-01-07T00:00:00Z"}},
	}}
	group := geminiGroupFromModels(models, now)
	bucket := getMap(getSlice(group["buckets"])[0])
	if fraction, _ := getFloat(bucket["remaining_fraction"]); fraction != 0.9 {
		t.Fatalf("gemini fraction = %v, want the Flash quota, not Argon's", fraction)
	}
}

func argonAccount(gemini, thirdParty, argon float64, withArgon bool) Account {
	reset := time.Now().Add(48 * time.Hour)
	groups := []any{
		map[string]any{"id": "gemini", "name": "Gemini Models", "buckets": []any{map[string]any{"id": "gemini-weekly", "name": "Weekly", "window": "weekly", "remaining_fraction": gemini, "reset_at": isoTime(reset)}}},
		map[string]any{"id": "third_party", "name": "Claude and GPT models", "buckets": []any{map[string]any{"id": "3p-weekly", "name": "Weekly", "window": "weekly", "remaining_fraction": thirdParty, "reset_at": isoTime(reset)}}},
	}
	if withArgon {
		groups = append(groups, map[string]any{"id": "argon", "name": "Gemini 4 Argon", "buckets": []any{map[string]any{"id": "gemini-argon-wtu-weekly", "name": "Gemini 4 Argon Weekly Limit", "window": "weekly", "remaining_fraction": argon, "reset_at": isoTime(reset)}}})
	}
	return Account{"email": "user@example.com", "name": "User", "quota_snapshot": map[string]any{"observed_at": isoTime(time.Now()), "tier": map[string]any{"id": "g1-pro-tier", "name": "Pro"}, "groups": groups}}
}

func TestFamilyPoolsSendArgonToItsOwnPoolOrToGemini(t *testing.T) {
	pro := argonAccount(0.9, 0.9, 0.2, true)
	ultra := argonAccount(0.7, 0.9, 0, false)
	for _, tc := range []struct {
		name    string
		account Account
		family  string
		want    string
	}{
		{"pro argon", pro, "argon", "argon"},
		{"ultra argon falls back to gemini", ultra, "argon", "gemini"},
		{"gemini", pro, "gemini", "gemini"},
		{"claude", pro, "claude", "third_party"},
		{"gpt", pro, "gpt", "third_party"},
		{"any family uses the shared pools", pro, "", "gemini,third_party"},
	} {
		if got := strings.Join(familyPools(tc.account, tc.family), ","); got != tc.want {
			t.Errorf("%s: pools = %s, want %s", tc.name, got, tc.want)
		}
	}
}

func TestUsedUpArgonDoesNotBenchTheWholeAccount(t *testing.T) {
	account := argonAccount(0.9, 0.8, 0, true)
	now := time.Now()
	if wait, _ := accountCooldown(account, now, ""); wait != 0 {
		t.Fatalf("plain rotation waits %s for Argon", wait)
	}
	if wait, _ := accountCooldown(account, now, "argon"); wait <= 0 {
		t.Fatal("an Argon request must wait for the Argon reset")
	}
	if remaining, ok, pool := accountRemaining(account, ""); !ok || remaining != 80 || pool != "third_party" {
		t.Fatalf("remaining = %v %v %s", remaining, ok, pool)
	}
	if remaining, ok, pool := accountRemaining(account, "argon"); !ok || remaining != 0 || pool != "argon" {
		t.Fatalf("argon remaining = %v %v %s", remaining, ok, pool)
	}
	if remaining, _, pool := accountRemaining(argonAccount(0.7, 0.9, 0, false), "argon"); remaining != 70 || pool != "gemini" {
		t.Fatalf("ultra argon remaining = %v from %s", remaining, pool)
	}
	if status := accountStatusFor(poolViews(account, now), false); status != statusReady {
		t.Fatalf("status = %s, want READY while Gemini and Claude/GPT have quota", status)
	}
}

func TestArgonIsAFamilyEverywhereFamiliesAreChosen(t *testing.T) {
	if got := modelFamily("Gemini 4 Argon (High)"); got != "argon" {
		t.Fatalf("modelFamily = %q", got)
	}
	if got := modelFamily("Gemini 3.8 Flash (High)"); got != "gemini" {
		t.Fatalf("modelFamily = %q", got)
	}
	account := Account{"quota_limits": map[string]any{"log:argon": map[string]any{"model": "Gemini 4 Argon (High)", "family": "argon", "source": "log", "reset_at": "2030-01-02T00:00:00Z", "observed_at": "2030-01-01T00:00:00Z"}}}
	if err := normalizeQuotaLimits(account, "user@example.com"); err != nil {
		t.Fatalf("store rejected an Argon limit: %v", err)
	}
	if _, err := parseCLI([]string{"next", "--family", "argon"}); err != nil {
		t.Fatalf("next --family argon: %v", err)
	}
	if _, _, _, err := parseExtended([]string{"recommend", "--family", "argon"}); err != nil {
		t.Fatalf("recommend --family argon: %v", err)
	}
}

func TestDefaultUserAgentNamesAHubClientNewEnoughForArgon(t *testing.T) {
	t.Setenv("AGY_SWAP_USER_AGENT", "")
	got := cloudUserAgent()
	if !strings.HasPrefix(got, "antigravity/hub/") || !strings.Contains(got, " ") {
		t.Fatalf("User-Agent = %q", got)
	}
}

func TestSnapshotMinimumJudgesTheSharedPools(t *testing.T) {
	remaining, label, _, ok := snapshotMinimum(argonAccount(0.9, 0.8, 0, true))
	if !ok || remaining != 80 || label != "Claude/GPT" {
		t.Fatalf("minimum = %v %q %v, want the Claude/GPT pool, not the used-up Argon pool", remaining, label, ok)
	}
}
