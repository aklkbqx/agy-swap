package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func seedQuotaAccount(email, tierID, tierName string, pct float64, observedAt time.Time) Account {
	return Account{
		"email":      email,
		"name":       email,
		"token_data": sampleTokenWithRefresh("tok-"+email, "rt-"+email, time.Now().Add(time.Hour)),
		"quota_snapshot": map[string]any{
			"observed_at": isoTime(observedAt.UTC()),
			"tier":        map[string]any{"id": tierID, "name": tierName},
			"groups": []any{map[string]any{
				"id":   poolThirdParty,
				"name": "Claude/GPT",
				"buckets": []any{map[string]any{
					"id": "claude-5h", "name": "5-Hour", "window": "5h",
					"remaining_fraction": pct / 100.0,
					"reset_at":           isoTime(time.Now().UTC().Add(4 * time.Hour)),
				}},
			}},
		},
	}
}

func TestPhase4AutoSwapDwellBudgetsHysteresisPaidPreferenceAndProtectedExclusion(t *testing.T) {
	origSleep := autoSwapPaceSleep
	autoSwapPaceSleep = func(context.Context, time.Duration) {}
	defer func() { autoSwapPaceSleep = origSleep }()

	a, out, errOut := newPhaseTestApp(t)
	now := time.Now().UTC()
	accounts := NewAccounts()
	accounts.Set("active@example.invalid", seedQuotaAccount("active@example.invalid", "g1-pro-tier", "Google AI Pro", 3, now))
	// Protected account with 99% must NEVER be picked by auto-swap.
	prot := seedQuotaAccount("protected@example.invalid", "g1-ultra-tier", "Google AI Ultra", 99, now)
	prot["protected"] = true
	accounts.Set("protected@example.invalid", prot)
	// Below hysteresis switch_in_pct (25%) must NOT be picked.
	accounts.Set("low@example.invalid", seedQuotaAccount("low@example.invalid", "g1-ultra-tier", "Google AI Ultra", 20, now))
	// Free tier has 90% vs Pro tier 60% -> Pro tier must win when PreferPaidTiers=true.
	accounts.Set("free@example.invalid", seedQuotaAccount("free@example.invalid", "free-tier", "Free", 90, now))
	accounts.Set("pro@example.invalid", seedQuotaAccount("pro@example.invalid", "g1-pro-tier", "Google AI Pro", 60, now))
	if err := a.store.Save(accounts); err != nil {
		t.Fatal(err)
	}
	settings, _ := a.loadSettings()
	settings.Quota.CandidateStaleSeconds = 3600
	if err := a.store.SaveSettings(settings); err != nil {
		t.Fatal(err)
	}

	// 1. First swap chooses paid pro@ over free@ (90%) and skips protected@ (99%) & low@ (20%).
	outcome := a.attemptGuardrailedAutoSwap(context.Background(), "active@example.invalid", "", "", now)
	if !outcome.Swapped || outcome.TargetEmail != "pro@example.invalid" {
		t.Fatalf("expected swap to pro@example.invalid, got %+v", outcome)
	}

	// 2. Immediate second swap within dwell (900s default, 300s hard floor) is blocked!
	blockedDwell := a.attemptGuardrailedAutoSwap(context.Background(), "pro@example.invalid", "", "", now.Add(2*time.Minute))
	if blockedDwell.Swapped || blockedDwell.Status != "dwell_wait" {
		t.Fatalf("expected dwell_wait 2m after swap, got %+v", blockedDwell)
	}

	// 3. Rapid exhaustion within 60s on newly swapped account trips global circuit breaker!
	rapid := a.attemptGuardrailedAutoSwap(context.Background(), "pro@example.invalid", "", "", now.Add(30*time.Second))
	if rapid.Swapped || rapid.Status != "circuit_open" {
		t.Fatalf("expected rapid 429 (<60s) to trip circuit_open, got %+v", rapid)
	}
	if !a.loadAutoSwapState().CircuitOpen {
		t.Fatal("expected circuit_open to persist in runtime-state.json")
	}

	// 4. `agy-swap auto-swap status --json` and `reset --json` work and preserve rolling swap counters!
	out.Reset()
	errOut.Reset()
	if code := a.Run(context.Background(), []string{"auto-swap", "status", "--json"}); code != 0 {
		t.Fatalf("auto-swap status failed: code=%d err=%s", code, errOut.String())
	}
	if !strings.Contains(out.String(), `"circuit_open":true`) || !strings.Contains(out.String(), `"swaps_1h":1`) {
		t.Fatalf("unexpected auto-swap status JSON: %s", out.String())
	}

	out.Reset()
	if code := a.Run(context.Background(), []string{"auto-swap", "reset", "--json"}); code != 0 {
		t.Fatalf("auto-swap reset failed: code=%d err=%s", code, errOut.String())
	}
	if a.loadAutoSwapState().CircuitOpen {
		t.Fatal("expected auto-swap reset to close circuit breaker")
	}
	if len(a.loadAutoSwapState().SwapTimestamps) != 1 {
		t.Fatalf("auto-swap reset must preserve rolling swap budget timestamps, got %v", a.loadAutoSwapState().SwapTimestamps)
	}

	// 4b. `agy-swap auto-swap enable` and `disable` toggle settings cleanly.
	out.Reset()
	errOut.Reset()
	if code := a.Run(context.Background(), []string{"auto-swap", "enable", "--json"}); code != 0 {
		t.Fatalf("auto-swap enable failed: code=%d err=%s", code, errOut.String())
	}
	if loadedSettings, _ := a.loadSettings(); !loadedSettings.AutoSwap.Enabled {
		t.Fatal("expected auto-swap enable to persist auto_swap.enabled=true")
	}
	out.Reset()
	if code := a.Run(context.Background(), []string{"auto-swap", "disable", "--json"}); code != 0 {
		t.Fatalf("auto-swap disable failed: code=%d err=%s", code, errOut.String())
	}
	if loadedSettings, _ := a.loadSettings(); loadedSettings.AutoSwap.Enabled {
		t.Fatal("expected auto-swap disable to persist auto_swap.enabled=false")
	}

	// 5. Rolling hourly budget (default 2/hr): after 2nd swap past dwell, 3rd swap is blocked by hourly_budget_exceeded!
	secondTime := now.Add(20 * time.Minute)
	second := a.attemptGuardrailedAutoSwap(context.Background(), "pro@example.invalid", "", "", secondTime)
	if !second.Swapped || second.TargetEmail != "free@example.invalid" {
		t.Fatalf("expected 2nd swap after dwell to pick free@, got %+v", second)
	}
	third := a.attemptGuardrailedAutoSwap(context.Background(), "free@example.invalid", "", "", secondTime.Add(20*time.Minute))
	if third.Swapped || third.Status != "hourly_budget_exceeded" {
		t.Fatalf("expected 3rd swap within 1h to hit hourly_budget_exceeded, got %+v", third)
	}
}

func TestPhase4CircuitBreakerTripsOnSuspendedAccountAndConsecutiveCandidateFailures(t *testing.T) {
	origSleep := autoSwapPaceSleep
	autoSwapPaceSleep = func(context.Context, time.Duration) {}
	defer func() { autoSwapPaceSleep = origSleep }()

	// Case A: Any SUSPENDED account halts auto-swap globally!
	a, _, _ := newPhaseTestApp(t)
	now := time.Now().UTC()
	accounts := NewAccounts()
	accounts.Set("active@example.invalid", seedQuotaAccount("active@example.invalid", "g1-pro-tier", "Google AI Pro", 2, now))
	accounts.Set("healthy@example.invalid", seedQuotaAccount("healthy@example.invalid", "g1-pro-tier", "Google AI Pro", 90, now))
	susp := seedQuotaAccount("banned@example.invalid", "g1-pro-tier", "Google AI Pro", 0, now)
	susp["eligibility"] = map[string]any{"state": "suspended", "reason": "TOS_VIOLATION", "message": "ToS violation"}
	accounts.Set("banned@example.invalid", susp)
	if err := a.store.Save(accounts); err != nil {
		t.Fatal(err)
	}

	res := a.attemptGuardrailedAutoSwap(context.Background(), "active@example.invalid", "", "", now)
	if res.Swapped || res.Status != "circuit_open" || !strings.Contains(res.Reason, "banned@example.invalid") {
		t.Fatalf("expected SUSPENDED account to trip global circuit breaker, got %+v", res)
	}

	// Case B: >= 2 consecutive candidate auth/eligibility failures in one swap attempt trips circuit breaker!
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, `{"error":{"code":401,"message":"invalid_grant"}}`)
	}))
	defer srv.Close()

	a2, _, _ := newPhaseTestApp(t)
	a2.http.cloudAPI = srv.URL + "/v1internal:"
	stale := now.Add(-30 * time.Minute)
	accs2 := NewAccounts()
	accs2.Set("active@example.invalid", seedQuotaAccount("active@example.invalid", "g1-pro-tier", "Google AI Pro", 2, now))
	accs2.Set("bad1@example.invalid", seedQuotaAccount("bad1@example.invalid", "g1-pro-tier", "Google AI Pro", 80, stale))
	accs2.Set("bad2@example.invalid", seedQuotaAccount("bad2@example.invalid", "g1-pro-tier", "Google AI Pro", 75, stale))
	accs2.Set("good3@example.invalid", seedQuotaAccount("good3@example.invalid", "g1-pro-tier", "Google AI Pro", 70, stale))
	if err := a2.store.Save(accs2); err != nil {
		t.Fatal(err)
	}

	res2 := a2.attemptGuardrailedAutoSwap(context.Background(), "active@example.invalid", "", "", now)
	if res2.Swapped || res2.Status != "circuit_open" {
		t.Fatalf("expected 2 consecutive candidate auth failures to trip circuit_open, got %+v", res2)
	}
	// Lazy check must stop after bad1 and bad2 without even hitting good3!
	if hits.Load() != 2 {
		t.Fatalf("expected lazy candidate check to stop after 2 failures (not check 3rd), got %d HTTP hits", hits.Load())
	}
}

func TestPhase4DoctorSafetyPostureAuditAndDisplayStatusBadges(t *testing.T) {
	a, out, _ := newPhaseTestApp(t)
	now := time.Now().UTC()

	// Seed an account marked SUSPENDED and one marked protected=false with auto_swap enabled.
	accounts := NewAccounts()
	susp := seedQuotaAccount("banned@example.invalid", "g1-pro-tier", "Google AI Pro", 0, now)
	susp["eligibility"] = map[string]any{"state": "suspended", "reason": "TOS_VIOLATION"}
	accounts.Set("banned@example.invalid", susp)
	prot := seedQuotaAccount("main@example.invalid", "g1-ultra-tier", "Google AI Ultra", 88, now)
	prot["protected"] = true
	accounts.Set("main@example.invalid", prot)
	if err := a.store.Save(accounts); err != nil {
		t.Fatal(err)
	}

	// Verify display.go accountStatus renders distinct Suspended (ToS) and Protected badges.
	if s := accountStatus(susp, palette{}, now); !strings.Contains(s, "Suspended (ToS)") {
		t.Fatalf("expected Suspended (ToS) badge, got %q", s)
	}
	if s := accountStatus(prot, palette{}, now); !strings.Contains(s, "Protected") {
		t.Fatalf("expected Protected badge, got %q", s)
	}

	// Configure risky settings + IDE settings.json with telemetry enabled + open circuit breaker.
	settings, _ := a.loadSettings()
	settings.AutoSwap.Enabled = true
	settings.AutoSwap.HandoffMode = "resume"
	settings.IDE.ReloadMode = "rpc-fast"
	if err := a.store.SaveSettings(settings); err != nil {
		t.Fatal(err)
	}
	a.saveAutoSwapState(AutoSwapState{CircuitOpen: true, CircuitReason: "test_halt"})

	// Create IDE User/settings.json with telemetry enabled.
	ideUserDir := filepath.Join(a.paths.Home, "Library", "Application Support", "Antigravity IDE", "User")
	if runtime.GOOS == "linux" {
		ideUserDir = filepath.Join(a.paths.Home, ".config", "Antigravity IDE", "User")
	}
	if runtime.GOOS != "windows" {
		if err := os.MkdirAll(ideUserDir, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(ideUserDir, "settings.json"), []byte(`{"antigravity.enableTelemetry":true,"telemetry.telemetryLevel":"all"}`), 0o600); err != nil {
			t.Fatal(err)
		}
		// Legacy wrapper with WSL_INTEROP=1 and --dangerously-skip-permissions.
		binDir := filepath.Join(a.paths.Home, ".local", "bin")
		libexecDir := filepath.Join(a.paths.Home, ".local", "libexec")
		_ = os.MkdirAll(binDir, 0o755)
		_ = os.MkdirAll(libexecDir, 0o755)
		_ = os.WriteFile(filepath.Join(libexecDir, "agy"), []byte("#!/bin/sh\n"), 0o755)
		_ = os.WriteFile(filepath.Join(binDir, "agy"), []byte("#!/bin/sh\n# Managed by agy-swap session guard\nexport WSL_INTEROP=1\n\"$real_command\" --dangerously-skip-permissions \"$@\"\n"), 0o755)
	}

	t.Setenv("DO_NOT_TRACK", "")
	// Skip auto-reconcile during this audit test so we verify doctor detects an un-upgraded legacy WSL_INTEROP wrapper.
	t.Setenv("AGY_SWAP_SKIP_SYNC", "1")
	if code := a.Run(context.Background(), []string{"doctor", "--json"}); code != 0 {
		t.Fatalf("doctor --json exited %d: %s", code, out.String())
	}
	var env struct {
		OK   bool `json:"ok"`
		Data struct {
			Checks []doctorCheck `json:"checks"`
		} `json:"data"`
	}
	if err := json.Unmarshal(out.Bytes(), &env); err != nil {
		t.Fatalf("failed to decode doctor JSON: %v\n%s", err, out.String())
	}
	byName := map[string]doctorCheck{}
	for _, c := range env.Data.Checks {
		byName[c.Name] = c
	}
	if byName["circuit_breaker"].Status != "warning" {
		t.Fatalf("expected circuit_breaker warning, got %+v", byName["circuit_breaker"])
	}
	if byName["suspended_accounts"].Status != "warning" {
		t.Fatalf("expected suspended_accounts warning, got %+v", byName["suspended_accounts"])
	}
	if byName["safety_posture"].Status != "warning" || !strings.Contains(byName["safety_posture"].Message, "resume") || !strings.Contains(byName["safety_posture"].Message, "rpc-fast") {
		t.Fatalf("expected safety_posture warning mentioning resume and rpc-fast, got %+v", byName["safety_posture"])
	}
	if runtime.GOOS != "windows" {
		if byName["ide_telemetry"].Status != "warning" || !strings.Contains(byName["ide_telemetry"].Message, "DO_NOT_TRACK") {
			t.Fatalf("expected ide_telemetry warning, got %+v", byName["ide_telemetry"])
		}
		if byName["launcher_guard"].Status != "warning" || !strings.Contains(byName["launcher_guard"].Message, "WSL_INTEROP") {
			t.Fatalf("expected launcher_guard warning about WSL_INTEROP, got %+v", byName["launcher_guard"])
		}

		// Also verify doctor catches missing settings.json when IDE User/ dir exists with default telemetry.
		_ = os.Remove(filepath.Join(ideUserDir, "settings.json"))
		checksMissing, _ := a.collectDoctorChecks(context.Background(), false)
		foundMissingWarn := false
		for _, c := range checksMissing {
			if c.Name == "ide_telemetry" && c.Status == "warning" && strings.Contains(c.Message, "antigravity.enableTelemetry") {
				foundMissingWarn = true
			}
		}
		if !foundMissingWarn {
			t.Fatalf("expected ide_telemetry warning when User/ dir exists without settings.json, got %+v", checksMissing)
		}
	}
}
