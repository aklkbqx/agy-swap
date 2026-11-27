package app

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestPhase1bSettingsHardFloorsAndDefaults(t *testing.T) {
	def := defaultSettings()
	if def.Quota.PollMode != "safe" || def.Quota.MaxConcurrency != 1 || def.Quota.ProjectMemoTTLHours != 24 {
		t.Fatalf("unexpected default quota settings: %+v", def.Quota)
	}
	if def.AutoSwap.Enabled || def.AutoSwap.HandoffMode != "clean-summary" || !def.AutoSwap.HaltOnTosViolation {
		t.Fatalf("unexpected default auto_swap settings: %+v", def.AutoSwap)
	}
	if def.AutoSwap.MinDwellSeconds != 900 || def.AutoSwap.MaxSwapsPerHour != 2 || def.AutoSwap.MaxSwapsPerDay != 6 {
		t.Fatalf("unexpected default auto_swap caps: %+v", def.AutoSwap)
	}

	// Non-bypassable hard floors must clamp unsafe values.
	unsafe := defaultSettings()
	unsafe.AutoSwap.MinDwellSeconds = 10
	unsafe.AutoSwap.MaxSwapsPerHour = 99
	unsafe.AutoSwap.MaxSwapsPerDay = 999
	unsafe.AutoSwap.SwitchOutPct = 20
	unsafe.AutoSwap.SwitchInPct = 22
	unsafe.AutoSwap.HaltOnTosViolation = false
	norm, err := normalizeSettings(unsafe)
	if err != nil {
		t.Fatalf("normalizeSettings error: %v", err)
	}
	if norm.AutoSwap.MinDwellSeconds < 300 {
		t.Fatalf("MinDwellSeconds hard floor bypassed: %d", norm.AutoSwap.MinDwellSeconds)
	}
	if norm.AutoSwap.MaxSwapsPerHour > 4 {
		t.Fatalf("MaxSwapsPerHour hard cap bypassed: %d", norm.AutoSwap.MaxSwapsPerHour)
	}
	if norm.AutoSwap.MaxSwapsPerDay > 12 {
		t.Fatalf("MaxSwapsPerDay hard cap bypassed: %d", norm.AutoSwap.MaxSwapsPerDay)
	}
	if norm.AutoSwap.SwitchInPct < norm.AutoSwap.SwitchOutPct+10 {
		t.Fatalf("hysteresis hard floor bypassed: out=%d in=%d", norm.AutoSwap.SwitchOutPct, norm.AutoSwap.SwitchInPct)
	}
	if !norm.AutoSwap.HaltOnTosViolation {
		t.Fatal("HaltOnTosViolation must be non-bypassable")
	}
}

func TestPhase1bSuspendedVsVerifyAndProtectedAccountExclusion(t *testing.T) {
	// 1. TOS_VIOLATION / appeal_url must be classified as Suspended, distinct from VERIFY.
	tosBody := []byte(`{"error":{"code":403,"message":"denied","details":[{"@type":"type.googleapis.com/google.rpc.ErrorInfo","reason":"TOS_VIOLATION","metadata":{"appeal_url":"https://support.google.com/appeal/1","uiMessage":"Account suspended for policy violation"}}]}}`)
	tosErr := eligibilityFromErrorBody(http.StatusForbidden, tosBody)
	if tosErr == nil || !tosErr.Suspended {
		t.Fatalf("expected Suspended=true for TOS_VIOLATION, got %#v", tosErr)
	}
	now := time.Date(2026, 11, 27, 0, 0, 0, 0, time.UTC)
	rec := tosErr.record(now)
	if getString(rec, "state") != "suspended" {
		t.Fatalf("expected state=suspended, got %q", getString(rec, "state"))
	}

	verifyBody := []byte(`{"error":{"code":403,"message":"verify","details":[{"@type":"type.googleapis.com/google.rpc.ErrorInfo","reason":"VALIDATION_REQUIRED","metadata":{"validation_url":"https://accounts.google.com/v","uiMessage":"Verify your account"}}]}}`)
	verifyErr := eligibilityFromErrorBody(http.StatusForbidden, verifyBody)
	if verifyErr == nil || verifyErr.Suspended {
		t.Fatalf("expected Suspended=false for VALIDATION_REQUIRED, got %#v", verifyErr)
	}
	if getString(verifyErr.record(now), "state") != "verify_required" {
		t.Fatalf("expected state=verify_required")
	}

	// 2. selectNext must NEVER return a suspended account even as a fallback, and skips protected accounts.
	accounts := NewAccounts()
	accounts.Set("active@example.invalid", Account{"email": "active@example.invalid", "name": "Active"})
	accounts.Set("banned@example.invalid", Account{"email": "banned@example.invalid", "name": "Banned", "eligibility": tosErr.record(now)})
	accounts.Set("main@example.invalid", Account{"email": "main@example.invalid", "name": "Primary", "protected": true})
	if picked, _ := selectNext(accounts, "active@example.invalid", ""); picked != nil && getString(picked, "email") == "banned@example.invalid" {
		t.Fatalf("selectNext must never fall back to a suspended account")
	}

	// 3. rankAccounts marks suspended and protected appropriately and respects 10m safe-mode stale window.
	settings := defaultSettings()
	accounts2 := NewAccounts()
	fiveMinAgo := isoTime(time.Now().UTC().Add(-5 * time.Minute))
	snap := map[string]any{
		"observed_at": fiveMinAgo,
		"tier":        map[string]any{"id": "g1-pro-tier", "name": "Google AI Pro"},
		"groups": []any{
			map[string]any{"id": "gemini", "name": "Gemini", "buckets": []any{map[string]any{"id": "gemini-weekly", "name": "Weekly", "window": "weekly", "remaining_fraction": 0.8, "reset_at": isoTime(time.Now().UTC().Add(24 * time.Hour))}}},
		},
	}
	accounts2.Set("good@example.invalid", Account{"email": "good@example.invalid", "name": "Good", "quota_snapshot": snap})
	accounts2.Set("banned@example.invalid", Account{"email": "banned@example.invalid", "name": "Banned", "quota_snapshot": snap, "eligibility": tosErr.record(now)})
	ranked := rankAccounts(accounts2, settings, "", "", "gemini", "")
	if len(ranked) != 2 || !ranked[0].Ready || ranked[0].Email != "good@example.invalid" {
		t.Fatalf("expected good@example.invalid ready with 5m snapshot under safe mode (10m stale), got %+v", ranked)
	}
	if ranked[1].Ready || !strings.Contains(strings.Join(ranked[1].Reasons, " "), "suspended") {
		t.Fatalf("expected suspended account not ready with suspended reason, got %+v", ranked[1])
	}
}

func TestPhase1bSafePollingSkipsIdleSuspendedVerifyAndSerializedDiskProjectMemo(t *testing.T) {
	now := time.Date(2030, 1, 1, 12, 0, 0, 0, time.UTC)
	accounts := NewAccounts()
	accounts.Set("active@example.invalid", scheduledAccount(now.Add(-90*time.Second), 0.7, 0.7, now.Add(48*time.Hour)))
	accounts.ByEmail["active@example.invalid"]["email"] = "active@example.invalid"
	accounts.ByEmail["active@example.invalid"]["token_data"] = "tok"

	idle := scheduledAccount(now.Add(-30*time.Minute), 0.9, 0.9, now.Add(48*time.Hour))
	idle["email"] = "idle@example.invalid"
	idle["token_data"] = "tok"
	accounts.Set("idle@example.invalid", idle)

	banned := scheduledAccount(now.Add(-30*time.Minute), 0.9, 0.9, now.Add(48*time.Hour))
	banned["email"] = "banned@example.invalid"
	banned["token_data"] = "tok"
	banned["eligibility"] = map[string]any{"state": "suspended", "reason": "TOS_VIOLATION"}
	accounts.Set("banned@example.invalid", banned)

	verifyAcc := scheduledAccount(now.Add(-30*time.Minute), 0.9, 0.9, now.Add(48*time.Hour))
	verifyAcc["email"] = "verify@example.invalid"
	verifyAcc["token_data"] = "tok"
	verifyAcc["eligibility"] = map[string]any{"state": "verify_required", "reason": "VALIDATION_REQUIRED"}
	accounts.Set("verify@example.invalid", verifyAcc)

	st := &tuiState{accounts: accounts, active: "active@example.invalid"}
	noBackoff := func(string) time.Time { return time.Time{} }
	// Under safe mode, with no activity, idle/suspended/verify accounts are NOT polled.
	dueIdle := st.dueEmailsForMode(now, map[string]time.Time{}, noBackoff, false, "safe")
	if len(dueIdle) != 0 {
		t.Fatalf("safe mode with no activity should poll 0 accounts, got %v", dueIdle)
	}
	// With activity, ONLY the active account is polled (never suspended/verify/idle).
	dueBusy := st.dueEmailsForMode(now, map[string]time.Time{}, noBackoff, true, "safe")
	if len(dueBusy) != 1 || dueBusy[0] != "active@example.invalid" {
		t.Fatalf("safe mode with activity should poll only active account, got %v", dueBusy)
	}

	// Verify serialized refresh (maxConcurrency=1), disk-persisted project memo (1 loadCodeAssist across two QuotaService instances),
	// and zero silent onboardUser on passive Fetch/Refresh.
	var inflight, maxInflight, loadCalls, onboardCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cur := inflight.Add(1)
		for {
			prev := maxInflight.Load()
			if cur <= prev || maxInflight.CompareAndSwap(prev, cur) {
				break
			}
		}
		time.Sleep(15 * time.Millisecond)
		defer inflight.Add(-1)
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, ":loadCodeAssist"):
			loadCalls.Add(1)
			if r.Header.Get("Authorization") == "Bearer tok-new" {
				fmt.Fprint(w, `{"allowedTiers":[{"id":"free-tier","isDefault":true}]}`)
				return
			}
			fmt.Fprint(w, `{"cloudaicompanionProject":"proj-cached","currentTier":{"id":"g1-pro-tier"}}`)
		case strings.HasSuffix(r.URL.Path, ":onboardUser"):
			onboardCalls.Add(1)
			fmt.Fprint(w, `{"name":"operations/op-1","done":true,"response":{"cloudaicompanionProject":{"id":"proj-onboard"}}}`)
		case strings.HasSuffix(r.URL.Path, ":retrieveUserQuotaSummary"):
			fmt.Fprint(w, `{"groups":[{"buckets":[{"bucketId":"gemini-weekly","displayName":"Weekly","window":"weekly","remainingFraction":0.8,"resetTime":"2030-01-01T00:00:00Z"}]}]}`)
		}
	}))
	defer server.Close()

	paths := testPaths(t)
	store := NewStore(paths)
	httpSvc := NewHTTPService(io.Discard)
	httpSvc.cloudAPI = server.URL + "/v1internal:"

	accs := NewAccounts()
	accs.Set("a@example.invalid", Account{"email": "a@example.invalid", "token_data": sampleTokenWithRefresh("tok-a", "rt-a", time.Now().Add(time.Hour))})
	accs.Set("b@example.invalid", Account{"email": "b@example.invalid", "token_data": sampleTokenWithRefresh("tok-b", "rt-b", time.Now().Add(time.Hour))})
	accs.Set("new@example.invalid", Account{"email": "new@example.invalid", "token_data": sampleTokenWithRefresh("tok-new", "rt-new", time.Now().Add(time.Hour))})
	if err := store.Save(accs); err != nil {
		t.Fatal(err)
	}

	qs1 := NewQuotaService(httpSvc, store)
	_ = qs1.Refresh(context.Background(), accs, true, nil)
	if maxInflight.Load() != 1 {
		t.Fatalf("expected serialized refresh (maxInflight=1), got %d", maxInflight.Load())
	}
	if onboardCalls.Load() != 0 {
		t.Fatalf("passive Refresh must not silently invoke onboardUser, got %d calls", onboardCalls.Load())
	}
	if loadCalls.Load() != 3 {
		t.Fatalf("expected 3 initial loadCodeAssist calls, got %d", loadCalls.Load())
	}

	// Brand-new QuotaService instance loading from disk should reuse persisted 24h cloud_project_cache for a and b!
	reloaded, err := store.Load(false)
	if err != nil {
		t.Fatal(err)
	}
	qs2 := NewQuotaService(httpSvc, store)
	errs2 := qs2.RefreshEmails(context.Background(), reloaded, []string{"a@example.invalid", "b@example.invalid"})
	if len(errs2) != 0 {
		t.Fatalf("second refresh failed: %v", errs2)
	}
	if loadCalls.Load() != 3 {
		t.Fatalf("expected disk project memo to skip loadCodeAssist on second process, total loadCalls=%d want 3", loadCalls.Load())
	}
}

func TestPhase1bPerAccountIdentityIsolationAndAccountProtectCLI(t *testing.T) {
	paths := testPaths(t)
	store := NewStore(paths)
	// Seed existing pbtxt with other fields that MUST be preserved.
	antigravityDir := filepath.Join(paths.Home, ".gemini", "antigravity")
	cliDir := filepath.Join(paths.Home, ".gemini", "antigravity-cli")
	if err := os.MkdirAll(antigravityDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(cliDir, 0o700); err != nil {
		t.Fatal(err)
	}
	pbtxtPath := filepath.Join(antigravityDir, "antigravity_state.pbtxt")
	if err := os.WriteFile(pbtxtPath, []byte("onboarding_complete: true\ninstallation_uuid: \"old-shared-uuid\"\ncustom_flag: 42\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	accA := Account{"email": "a@example.invalid", "name": "A"}
	accB := Account{"email": "b@example.invalid", "name": "B"}
	idA, uuidA := ensureAccountIdentity(accA)
	idB, uuidB := ensureAccountIdentity(accB)
	if idA == "" || uuidA == "" || idA == idB || uuidA == uuidB {
		t.Fatalf("expected distinct non-empty per-account UUIDs: A=(%s,%s) B=(%s,%s)", idA, uuidA, idB, uuidB)
	}

	syncAccountIdentityFiles(paths.Home, idA, uuidA)
	gotID, _ := os.ReadFile(filepath.Join(antigravityDir, "installation_id"))
	if strings.TrimSpace(string(gotID)) != idA {
		t.Fatalf("installation_id = %q, want %q", string(gotID), idA)
	}
	gotPb, _ := os.ReadFile(pbtxtPath)
	pbStr := string(gotPb)
	if !strings.Contains(pbStr, `installation_uuid: "`+uuidA+`"`) || !strings.Contains(pbStr, "onboarding_complete: true") || !strings.Contains(pbStr, "custom_flag: 42") {
		t.Fatalf("pbtxt did not update installation_uuid cleanly while preserving other fields:\n%s", pbStr)
	}

	// Test account protect / unprotect CLI command.
	accounts := NewAccounts()
	accounts.Set("a@example.invalid", accA)
	if err := store.Save(accounts); err != nil {
		t.Fatal(err)
	}
	var out, errOut bytes.Buffer
	app := &Application{paths: paths, store: store, credentials: NewCredentials(paths), Out: &out, Err: &errOut}
	if code := app.runExtended(context.Background(), []string{"account", "protect", "a@example.invalid", "--json"}); code != 0 {
		t.Fatalf("account protect failed: code=%d out=%s err=%s", code, out.String(), errOut.String())
	}
	loaded, _ := store.Load(false)
	if !accountIsProtected(loaded.ByEmail["a@example.invalid"]) {
		t.Fatal("expected account to be marked protected=true")
	}
	out.Reset()
	if code := app.runExtended(context.Background(), []string{"account", "unprotect", "a@example.invalid", "--json"}); code != 0 {
		t.Fatalf("account unprotect failed: code=%d", code)
	}
	loaded, _ = store.Load(false)
	if accountIsProtected(loaded.ByEmail["a@example.invalid"]) {
		t.Fatal("expected account to be unprotected")
	}
}
