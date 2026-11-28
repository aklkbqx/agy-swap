package app

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func seedFamilyQuotaAccount(email, tierID, tierName string, geminiPct, thirdPartyPct float64, observedAt time.Time) Account {
	return Account{
		"email":      email,
		"name":       email,
		"token_data": sampleTokenWithRefresh("tok-"+email, "rt-"+email, time.Now().Add(time.Hour)),
		"quota_snapshot": map[string]any{
			"observed_at": isoTime(observedAt.UTC()),
			"tier":        map[string]any{"id": tierID, "name": tierName},
			"groups": []any{
				map[string]any{
					"id":   poolGemini,
					"name": "Gemini Models",
					"buckets": []any{map[string]any{
						"id": "gemini-weekly", "name": "Weekly", "window": "weekly",
						"remaining_fraction": geminiPct / 100.0,
						"reset_at":           isoTime(time.Now().UTC().Add(5 * 24 * time.Hour)),
					}},
				},
				map[string]any{
					"id":   poolThirdParty,
					"name": "Claude and GPT models",
					"buckets": []any{map[string]any{
						"id": "3p-5h", "name": "5-Hour", "window": "5h",
						"remaining_fraction": thirdPartyPct / 100.0,
						"reset_at":           isoTime(time.Now().UTC().Add(4 * time.Hour)),
					}},
				},
			},
		},
	}
}

func TestAutoSwapContinuousStaleLogLimitClearedByHealthySnapshot(t *testing.T) {
	origSleep := autoSwapPaceSleep
	autoSwapPaceSleep = func(context.Context, time.Duration) {}
	defer func() { autoSwapPaceSleep = origSleep }()

	a, _, _ := newPhaseTestApp(t)
	now := time.Now().UTC()

	// Ultra backup has an old log limit from 48h ago, but a live snapshot 1m ago showing 76.6% Gemini & 98.8% 3P.
	ultraBackup := seedFamilyQuotaAccount("ultra-backup@example.invalid", "g1-ultra-tier", "Google AI Ultra", 76.6, 98.8, now.Add(-1*time.Minute))
	ultraBackup["quota_limits"] = map[string]any{
		"gemini-4-hydrogen-xhigh": map[string]any{
			"family":      "gemini",
			"model":       "Gemini 4 Hydrogen (XHigh)",
			"observed_at": isoTime(now.Add(-48 * time.Hour)),
			"reset_at":    isoTime(now.Add(72 * time.Hour)),
			"source":      "log",
			"source_file": "cli-old.log",
		},
	}
	if !reconcileLogLimits(ultraBackup, nil, nil) {
		t.Fatal("expected reconcileLogLimits to clear stale log limit superseded by healthy snapshot")
	}
	if _, exists := ultraBackup["quota_limits"]; exists {
		t.Fatalf("expected stale log quota_limits to be cleared, got %v", ultraBackup["quota_limits"])
	}
	if wait, _ := accountCooldown(ultraBackup, now, "gemini"); wait != 0 {
		t.Fatalf("expected 0 cooldown after clearing stale log limit, got %v", wait)
	}

	// Manual limits must NEVER be cleared by snapshot reconciliation.
	manualAcc := seedFamilyQuotaAccount("manual@example.invalid", "g1-pro-tier", "Google AI Pro", 90, 90, now.Add(-1*time.Minute))
	manualAcc["quota_limits"] = map[string]any{
		"manual:gemini": map[string]any{
			"family":      "gemini",
			"model":       "Gemini",
			"observed_at": isoTime(now.Add(-48 * time.Hour)),
			"reset_at":    isoTime(now.Add(24 * time.Hour)),
			"source":      "manual",
		},
	}
	if reconcileSnapshotLogLimits(manualAcc) {
		t.Fatal("reconcileSnapshotLogLimits must not clear manual cooldowns")
	}
	if wait, _ := accountCooldown(manualAcc, now, "gemini"); wait <= 0 {
		t.Fatalf("expected manual cooldown to remain active, got %v", wait)
	}

	// Fresh log limits observed AFTER the snapshot must still be honored.
	freshAcc := seedFamilyQuotaAccount("fresh@example.invalid", "g1-pro-tier", "Google AI Pro", 85, 90, now.Add(-10*time.Minute))
	freshAcc["quota_limits"] = map[string]any{
		"gemini-4-neon-xhigh": map[string]any{
			"family":      "gemini",
			"model":       "Gemini 4 Neon (XHigh)",
			"observed_at": isoTime(now.Add(-1 * time.Minute)),
			"reset_at":    isoTime(now.Add(48 * time.Hour)),
			"source":      "log",
		},
	}
	if reconcileSnapshotLogLimits(freshAcc) {
		t.Fatal("reconcileSnapshotLogLimits must not clear fresh log limits newer than snapshot")
	}
	if wait, _ := accountCooldown(freshAcc, now, "gemini"); wait <= 0 {
		t.Fatalf("expected fresh log cooldown to remain active, got %v", wait)
	}

	// Re-seed ultraBackup with the stale log limit still on disk; attemptGuardrailedAutoSwap + accountCooldown must still clear/ignore it and pick Ultra over 100% Pro!
	ultraWithStale := seedFamilyQuotaAccount("ultra-backup@example.invalid", "g1-ultra-tier", "Google AI Ultra", 76.6, 98.8, now.Add(-1*time.Minute))
	ultraWithStale["quota_limits"] = map[string]any{
		"gemini-4-hydrogen-xhigh": map[string]any{
			"family":      "gemini",
			"model":       "Gemini 4 Hydrogen (XHigh)",
			"observed_at": isoTime(now.Add(-48 * time.Hour)),
			"reset_at":    isoTime(now.Add(72 * time.Hour)),
			"source":      "log",
			"source_file": "cli-old.log",
		},
	}
	accounts := NewAccounts()
	accounts.Set("active@example.invalid", seedFamilyQuotaAccount("active@example.invalid", "g1-ultra-tier", "Google AI Ultra", 0.01, 0, now))
	accounts.Set("pro-full@example.invalid", seedFamilyQuotaAccount("pro-full@example.invalid", "g1-pro-tier", "Google AI Pro", 100, 100, now))
	accounts.Set("ultra-backup@example.invalid", ultraWithStale)
	if err := a.store.Save(accounts); err != nil {
		t.Fatal(err)
	}
	settings, _ := a.loadSettings()
	settings.Quota.CandidateStaleSeconds = 3600
	if err := a.store.SaveSettings(settings); err != nil {
		t.Fatal(err)
	}

	outcome := a.attemptGuardrailedAutoSwap(context.Background(), "active@example.invalid", "", "gemini", now)
	if !outcome.Swapped || outcome.TargetEmail != "ultra-backup@example.invalid" {
		t.Fatalf("expected Ultra backup with stale log limit to beat 100%% Pro, got %+v", outcome)
	}
}

func TestAutoSwapContinuousModernAgyCLILogParsing(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "cli-20261128_124437.log")
	now := time.Now()
	ts1 := now.Add(-10 * time.Second).Format("0102 15:04:05.000000")
	ts2 := now.Add(-8 * time.Second).Format("0102 15:04:05.000000")
	ts3 := now.Add(-6 * time.Second).Format("0102 15:04:05.000000")
	ts4 := now.Add(-4 * time.Second).Format("0102 15:04:05.000000")
	ts5 := now.Add(-2 * time.Second).Format("0102 15:04:05.000000")

	content := strings.Join([]string{
		fmt.Sprintf("I%s 577 server_oauth.go:221] applyAuthResult: email=user@example.invalid, authMethod=consumer, quotaProject=", ts1),
		fmt.Sprintf("I%s 578 server.go:100] label = \"Orchestrator (Neon)\"", ts1),
		fmt.Sprintf("I%s 579 server.go:1883] Sending user message to conversation 11111111-1111-1111-1111-111111111111 (items=1, media=0)", ts2),
		fmt.Sprintf("I%s 580 server.go:1883] Sending user message to conversation 22222222-2222-2222-2222-222222222222 (items=1, media=0)", ts2),
		fmt.Sprintf("I%s 581 server.go:1883] Sending user message to conversation 33333333-3333-3333-3333-333333333333 (items=1, media=0)", ts2),
		fmt.Sprintf("I%s 582 server.go:1351] Stream goroutine exited for 33333333-3333-3333-3333-333333333333, sending completion signal", ts3),
		fmt.Sprintf("E%s 583 errorreport.go:282] generating and executing: transport error (ResourceExhausted, HTTP 429): Third-party quota reached. Please switch to Gemini models or upgrade your subscription to increase your limits. Resets in 4h43m6s.: RESOURCE_EXHAUSTED (code 429)", ts4),
		fmt.Sprintf("E%s 584 errorreport.go:282] generating and executing: transport error (ResourceExhausted, HTTP 429): Individual quota reached. Please upgrade your subscription to increase your limits. Resets in 121h36m16s.: RESOURCE_EXHAUSTED (code 429)", ts5),
	}, "\n") + "\n"
	if err := os.WriteFile(logPath, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(logPath)
	if err != nil {
		t.Fatal(err)
	}
	ev, err := scanLogFile(logPath, info)
	if err != nil {
		t.Fatalf("scanLogFile failed: %v", err)
	}
	userEv := ev["user@example.invalid"]
	if len(userEv) == 0 {
		t.Fatalf("expected non-empty evidence for Orchestrator (Neon) + multi-pending log, got %+v", ev)
	}
	var sawThirdParty, sawGemini bool
	for _, rec := range userEv {
		if rec.State != "limited" || rec.Limit == nil {
			continue
		}
		if rec.Family == "claude" || rec.Family == "third_party" {
			sawThirdParty = true
		}
		if rec.Family == "gemini" {
			sawGemini = true
		}
	}
	if !sawThirdParty || !sawGemini {
		t.Fatalf("expected both third-party and gemini limits extracted, got %+v", userEv)
	}
}

func TestAutoSwapContinuousInFlightTurnGateAndInstantNoHubReload(t *testing.T) {
	home := t.TempDir()
	logDir := filepath.Join(home, ".gemini", "antigravity-cli", "log")
	if err := os.MkdirAll(logDir, 0o700); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	tsNow := now.Format("0102 15:04:05.000000")
	tsOld := now.Add(-25 * time.Second).Format("0102 15:04:05.000000")
	logFile := filepath.Join(logDir, "cli-turn.log")

	// 1. Stream goroutine exited clears the open stream.
	exited := fmt.Sprintf("I%s 1 server.go:1883] Sending user message to conversation c1\nI%s 1 server.go:1351] Stream goroutine exited for c1, sending completion signal\n", tsNow, tsNow)
	if err := os.WriteFile(logFile, []byte(exited), 0o600); err != nil {
		t.Fatal(err)
	}
	if hasInFlightIDETurn(home, now) {
		t.Fatal("Stream goroutine exited must clear in-flight turn gate")
	}

	// 2. Agent executor error / RESOURCE_EXHAUSTED clears the open stream.
	errored := fmt.Sprintf("I%s 1 server.go:1883] Sending user message to conversation c2\nE%s 1 errorreport.go:282] agent executor error: generating and executing: transport error (ResourceExhausted, HTTP 429): RESOURCE_EXHAUSTED\n", tsNow, tsNow)
	if err := os.WriteFile(logFile, []byte(errored), 0o600); err != nil {
		t.Fatal(err)
	}
	if hasInFlightIDETurn(home, now) {
		t.Fatal("RESOURCE_EXHAUSTED error must clear in-flight turn gate")
	}

	// 3. Stale unpaired Sending user message (>15s ago with no activity) must not freeze auto-swap.
	stale := fmt.Sprintf("I%s 1 server.go:1883] Sending user message to conversation c3\n", tsOld)
	if err := os.WriteFile(logFile, []byte(stale), 0o600); err != nil {
		t.Fatal(err)
	}
	if hasInFlightIDETurn(home, now) {
		t.Fatal("stale unpaired Sending user message (>15s idle) must not block turn gate")
	}

	// 4. Fresh unpaired Sending user message at now IS still in-flight.
	fresh := fmt.Sprintf("I%s 1 server.go:1883] Sending user message to conversation c4\n", tsNow)
	if err := os.WriteFile(logFile, []byte(fresh), 0o600); err != nil {
		t.Fatal(err)
	}
	if !hasInFlightIDETurn(home, now) {
		t.Fatal("fresh unpaired Sending user message must still be detected as in-flight")
	}

	// 5. reloadIDEForSwitch in hub-restart mode returns instantly (<200ms) when no IDE hubs are running, even when CLI log has an active stream!
	origCheck := ideRunningCheck
	ideRunningCheck = func() bool { return false }
	defer func() { ideRunningCheck = origCheck }()
	origAllow := allowIDEHubReloadInTests
	allowIDEHubReloadInTests = true
	defer func() { allowIDEHubReloadInTests = origAllow }()

	t.Setenv("AGY_SWAP_SKIP_SYNC", "")
	t.Setenv("AGY_SWAP_SKIP_HUB_RELOAD", "")
	app, _, _ := newPhaseTestApp(t)
	app.paths.Home = home
	app.credentials = NewCredentials(app.paths)
	// Bypass shouldReloadIDEHubs() home check indirectly by ensuring reloadIDEForSwitch does not sleep 5s even if waitTimeout=5s.
	start := time.Now()
	app.reloadIDEForSwitch(context.Background(), "hub-restart", 5*time.Second, "a@example.invalid", false)
	if elapsed := time.Since(start); elapsed > 500*time.Millisecond {
		t.Fatalf("reloadIDEForSwitch waited %v when no IDE hubs were running", elapsed)
	}
}

func TestAutoSwapContinuousTierPolicyAndFamilySelection(t *testing.T) {
	origSleep := autoSwapPaceSleep
	autoSwapPaceSleep = func(context.Context, time.Duration) {}
	defer func() { autoSwapPaceSleep = origSleep }()

	now := time.Now().UTC()

	// 1. same-tier-first: Active Ultra prefers Ultra (76%) over Pro (100%).
	a1, _, _ := newPhaseTestApp(t)
	accs1 := NewAccounts()
	accs1.Set("active-ultra@example.invalid", seedFamilyQuotaAccount("active-ultra@example.invalid", "g1-ultra-tier", "Google AI Ultra", 1, 1, now))
	accs1.Set("backup-pro@example.invalid", seedFamilyQuotaAccount("backup-pro@example.invalid", "g1-pro-tier", "Google AI Pro", 100, 100, now))
	accs1.Set("backup-ultra@example.invalid", seedFamilyQuotaAccount("backup-ultra@example.invalid", "g1-ultra-tier", "Google AI Ultra", 76, 90, now))
	_ = a1.store.Save(accs1)
	s1, _ := a1.loadSettings()
	s1.Quota.CandidateStaleSeconds = 3600
	s1.AutoSwap.TierPolicy = "same-tier-first"
	s1.AutoSwap.ExcludeFreeTier = true
	_ = a1.store.SaveSettings(s1)

	res1 := a1.attemptGuardrailedAutoSwap(context.Background(), "active-ultra@example.invalid", "", "gemini", now)
	if !res1.Swapped || res1.TargetEmail != "backup-ultra@example.invalid" {
		t.Fatalf("expected same-tier-first Ultra->Ultra, got %+v", res1)
	}

	// 2. same-tier-first falls back to Pro (never Free) when no Ultra is available.
	a2, _, _ := newPhaseTestApp(t)
	accs2 := NewAccounts()
	accs2.Set("active-ultra@example.invalid", seedFamilyQuotaAccount("active-ultra@example.invalid", "g1-ultra-tier", "Google AI Ultra", 1, 1, now))
	accs2.Set("backup-free@example.invalid", seedFamilyQuotaAccount("backup-free@example.invalid", "free-tier", "Free", 95, 95, now))
	accs2.Set("backup-pro@example.invalid", seedFamilyQuotaAccount("backup-pro@example.invalid", "g1-pro-tier", "Google AI Pro", 80, 80, now))
	_ = a2.store.Save(accs2)
	s2, _ := a2.loadSettings()
	s2.Quota.CandidateStaleSeconds = 3600
	s2.AutoSwap.TierPolicy = "same-tier-first"
	s2.AutoSwap.ExcludeFreeTier = true
	_ = a2.store.SaveSettings(s2)

	res2 := a2.attemptGuardrailedAutoSwap(context.Background(), "active-ultra@example.invalid", "", "gemini", now)
	if !res2.Swapped || res2.TargetEmail != "backup-pro@example.invalid" {
		t.Fatalf("expected same-tier-first fallback Ultra->Pro (skipping Free), got %+v", res2)
	}

	// 3. same-tier-only refuses to drop from Ultra to Pro.
	a2.saveAutoSwapState(AutoSwapState{})
	if _, err := a2.store.UpdateSettings(func(s *AppSettings) error {
		s.AutoSwap.TierPolicy = "same-tier-only"
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	res3 := a2.attemptGuardrailedAutoSwap(context.Background(), "active-ultra@example.invalid", "", "gemini", now)
	if res3.Swapped || res3.Status != "no_candidate" {
		t.Fatalf("expected same-tier-only to return no_candidate when only Pro/Free exist, got %+v", res3)
	}

	// 4. same-tier-first: Active Pro prefers Pro (60%) over Ultra (80%).
	a4, _, _ := newPhaseTestApp(t)
	accs4 := NewAccounts()
	accs4.Set("active-pro@example.invalid", seedFamilyQuotaAccount("active-pro@example.invalid", "g1-pro-tier", "Google AI Pro", 1, 1, now))
	accs4.Set("backup-ultra@example.invalid", seedFamilyQuotaAccount("backup-ultra@example.invalid", "g1-ultra-tier", "Google AI Ultra", 80, 80, now))
	accs4.Set("backup-pro@example.invalid", seedFamilyQuotaAccount("backup-pro@example.invalid", "g1-pro-tier", "Google AI Pro", 60, 60, now))
	_ = a4.store.Save(accs4)
	s4, _ := a4.loadSettings()
	s4.Quota.CandidateStaleSeconds = 3600
	s4.AutoSwap.TierPolicy = "same-tier-first"
	_ = a4.store.SaveSettings(s4)

	res4 := a4.attemptGuardrailedAutoSwap(context.Background(), "active-pro@example.invalid", "", "gemini", now)
	if !res4.Swapped || res4.TargetEmail != "backup-pro@example.invalid" {
		t.Fatalf("expected same-tier-first Pro->Pro, got %+v", res4)
	}

	// 5. Family-aware: when family="gemini", backup Ultra with 95% Gemini and 0% Claude 5-hour IS selected.
	a5, _, _ := newPhaseTestApp(t)
	accs5 := NewAccounts()
	accs5.Set("active@example.invalid", seedFamilyQuotaAccount("active@example.invalid", "g1-ultra-tier", "Google AI Ultra", 0, 50, now))
	accs5.Set("ultra-gemini-ok@example.invalid", seedFamilyQuotaAccount("ultra-gemini-ok@example.invalid", "g1-ultra-tier", "Google AI Ultra", 95, 0, now))
	_ = a5.store.Save(accs5)
	s5, _ := a5.loadSettings()
	s5.Quota.CandidateStaleSeconds = 3600
	_ = a5.store.SaveSettings(s5)

	res5 := a5.attemptGuardrailedAutoSwap(context.Background(), "active@example.invalid", "", "gemini", now)
	if !res5.Swapped || res5.TargetEmail != "ultra-gemini-ok@example.invalid" {
		t.Fatalf("expected gemini family swap to succeed even when 3P 5h is 0%%, got %+v", res5)
	}
}

func TestAutoSwapContinuousMultiSessionAdoptAlreadySwapped(t *testing.T) {
	origSleep := autoSwapPaceSleep
	autoSwapPaceSleep = func(context.Context, time.Duration) {}
	defer func() { autoSwapPaceSleep = origSleep }()

	a, _, _ := newPhaseTestApp(t)
	now := time.Now().UTC()
	accs := NewAccounts()
	accs.Set("acct-a@example.invalid", seedFamilyQuotaAccount("acct-a@example.invalid", "g1-ultra-tier", "Google AI Ultra", 0.01, 0, now))
	accs.Set("acct-b@example.invalid", seedFamilyQuotaAccount("acct-b@example.invalid", "g1-ultra-tier", "Google AI Ultra", 90, 90, now))
	if err := a.store.Save(accs); err != nil {
		t.Fatal(err)
	}
	settings, _ := a.loadSettings()
	settings.Quota.CandidateStaleSeconds = 3600
	if err := a.store.SaveSettings(settings); err != nil {
		t.Fatal(err)
	}
	// Session 1 already switched disk to acct-b 10s ago.
	if !a.credentials.ApplyWithoutReload(context.Background(), getString(accs.ByEmail["acct-b@example.invalid"], "token_data"), "acct-b@example.invalid") {
		t.Fatal("failed to activate acct-b")
	}
	a.saveAutoSwapState(AutoSwapState{
		LastSwapAt:     isoTime(now),
		LastSwapEmail:  "acct-b@example.invalid",
		SwapTimestamps: []string{isoTime(now)},
	})

	// Session 2 (started on acct-a) hits 429 10s later -> must adopt acct-b cleanly!
	res := a.resolveOrAttemptAutoSwap(context.Background(), "acct-a@example.invalid", "", "gemini", now.Add(10*time.Second))
	if !res.Swapped || !res.Adopted || res.Status != "adopted" || res.TargetEmail != "acct-b@example.invalid" {
		t.Fatalf("expected adopted swap to acct-b@example.invalid, got %+v", res)
	}
	state := a.loadAutoSwapState()
	if state.CircuitOpen {
		t.Fatalf("expected circuit breaker to stay closed when adopting, got %+v", state)
	}
	if len(state.SwapTimestamps) != 1 {
		t.Fatalf("expected SwapTimestamps not to increment on adopt, got %v", state.SwapTimestamps)
	}
}

func TestAutoSwapContinuousErrorDetectionAndRecentTurnsHandoff(t *testing.T) {
	infoLine := `I1128 13:10:43.773348  429678 runner.go:961] Monitor sir_flash_lite FINISHED: triggered=false, reason="grepping RESOURCE_EXHAUSTED"`
	if isRealAgyLogErrorLine(infoLine) {
		t.Fatal("Monitor info line must NOT be treated as a real agy log error line")
	}
	geminiErr := `E1128 13:13:33.581824  430985 errorreport.go:282] generating and executing: transport error (ResourceExhausted, HTTP 429): Individual quota reached. Please upgrade your subscription to increase your limits. Resets in 121h37m24s.: RESOURCE_EXHAUSTED`
	if !isRealAgyLogErrorLine(geminiErr) || !isQuotaExhaustedOutputLine(geminiErr) {
		t.Fatal("real Gemini 429 error line must be detected")
	}
	if got := inferExhaustedFamilyFromLine(geminiErr); got != "gemini" {
		t.Fatalf("inferExhaustedFamilyFromLine(geminiErr) = %q, want gemini", got)
	}
	thirdPartyErr := `E1128 13:05:46.674707  104838 errorreport.go:282] generating and executing: transport error (ResourceExhausted, HTTP 429): Third-party quota reached. Resets in 4h43m6s.: RESOURCE_EXHAUSTED`
	if got := inferExhaustedFamilyFromLine(thirdPartyErr); got != "claude" {
		t.Fatalf("inferExhaustedFamilyFromLine(thirdPartyErr) = %q, want claude", got)
	}

	// Verify buildCleanHandoffMarkdown prioritizes the MOST RECENT turns when space is tight.
	home := t.TempDir()
	workspace := t.TempDir()
	convID := "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"
	logsDir := filepath.Join(home, ".gemini", "antigravity-cli", "brain", convID, ".system_generated", "logs")
	if err := os.MkdirAll(logsDir, 0o700); err != nil {
		t.Fatal(err)
	}
	checkpoint := strings.Repeat("CHECKPOINT_PAD ", 140)
	var lines []string
	lines = append(lines, fmt.Sprintf(`{"type":"CHECKPOINT","content":%q}`, checkpoint))
	for i := 1; i <= 6; i++ {
		lines = append(lines, fmt.Sprintf(`{"type":"USER_INPUT","source":"USER_EXPLICIT","content":"TURN_%d_MARKER %s"}`, i, strings.Repeat("x", 600)))
	}
	if err := os.WriteFile(filepath.Join(logsDir, "transcript.jsonl"), []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	brief, err := buildCleanHandoffMarkdown(home, convID, workspace, 3600)
	if err != nil {
		t.Fatalf("buildCleanHandoffMarkdown failed: %v", err)
	}
	if !strings.Contains(brief, "TURN_6_MARKER") {
		t.Fatalf("expected latest TURN_6_MARKER to be preserved in bounded handoff:\n%s", brief)
	}
	if strings.Contains(brief, "TURN_1_MARKER") {
		t.Fatalf("expected oldest TURN_1_MARKER to be dropped before newest TURN_6_MARKER:\n%s", brief)
	}
}

func TestAutoSwapContinuousWaitAndContinueLoopAndCancel(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script fake agy harness runs on Unix")
	}
	origPace := autoSwapPaceSleep
	origWait := autoSwapWaitSleep
	origNow := autoSwapNow
	defer func() {
		autoSwapPaceSleep = origPace
		autoSwapWaitSleep = origWait
		autoSwapNow = origNow
	}()
	autoSwapPaceSleep = func(context.Context, time.Duration) {}

	var fakeNow atomic.Int64
	baseNow := time.Date(2026, 11, 28, 6, 0, 0, 0, time.UTC)
	fakeNow.Store(baseNow.UnixNano())
	autoSwapNow = func() time.Time { return time.Unix(0, fakeNow.Load()).UTC() }
	autoSwapWaitSleep = func(ctx context.Context, d time.Duration) {
		if ctx.Err() != nil {
			return
		}
		fakeNow.Add(int64(d))
	}

	// Case 1: Dwell wait active -> waits cleanly -> swaps to b@ -> attempt 2 exits 0!
	a, out, errOut := newPhaseTestApp(t)
	accs := NewAccounts()
	accs.Set("a@example.invalid", seedFamilyQuotaAccount("a@example.invalid", "g1-ultra-tier", "Google AI Ultra", 2, 2, baseNow))
	accs.Set("b@example.invalid", seedFamilyQuotaAccount("b@example.invalid", "g1-ultra-tier", "Google AI Ultra", 90, 90, baseNow))
	if err := a.store.Save(accs); err != nil {
		t.Fatal(err)
	}
	_ = a.credentials.ApplyWithoutReload(context.Background(), getString(accs.ByEmail["a@example.invalid"], "token_data"), "a@example.invalid")
	// Last swap was 290s ago; MinDwellSeconds=300 -> 10s dwell remaining.
	a.saveAutoSwapState(AutoSwapState{
		LastSwapAt:    isoTime(baseNow.Add(-290 * time.Second)),
		LastSwapEmail: "other@example.invalid",
	})

	fakeAgy := filepath.Join(t.TempDir(), "fake-agy")
	script := `#!/bin/sh
acct="$(cat ` + shellQuoteDouble(a.paths.GoogleAccounts) + ` 2>/dev/null || true)"
if echo "$acct" | grep -q "a@example.invalid"; then
  echo 'E1128 13:13:33.581824 1 errorreport.go:282] transport error (ResourceExhausted, HTTP 429): Individual quota reached.: RESOURCE_EXHAUSTED' >&2
  exit 1
fi
echo "CONTINUED_AFTER_WAIT"
exit 0
`
	if err := os.WriteFile(fakeAgy, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	settings, _ := a.loadSettings()
	settings.AutoSwap.Enabled = true
	settings.AutoSwap.MinDwellSeconds = 300
	settings.Quota.CandidateStaleSeconds = 3600
	settings.Targets["agy"] = TargetConfig{Command: fakeAgy, Enabled: true}
	if err := a.store.SaveSettings(settings); err != nil {
		t.Fatal(err)
	}

	code := a.Run(context.Background(), []string{"run", "now", "--auto-swap", "--", "-p", "continue work"})
	if code != 0 || !strings.Contains(out.String(), "CONTINUED_AFTER_WAIT") {
		t.Fatalf("expected wait-and-continue loop to succeed, code=%d out=%q err=%q", code, out.String(), errOut.String())
	}
	if !strings.Contains(errOut.String(), "waiting") {
		t.Fatalf("expected waiting message during dwell wait, got err=%q", errOut.String())
	}

	// Case 2: Cancelled context exits prompt without hanging during wait.
	ctx, cancel := context.WithCancel(context.Background())
	autoSwapWaitSleep = func(c context.Context, _ time.Duration) {
		cancel()
		<-c.Done()
	}
	fakeNow.Store(baseNow.UnixNano())
	_ = a.credentials.ApplyWithoutReload(context.Background(), getString(accs.ByEmail["a@example.invalid"], "token_data"), "a@example.invalid")
	a.saveAutoSwapState(AutoSwapState{
		LastSwapAt:    isoTime(baseNow.Add(-100 * time.Second)),
		LastSwapEmail: "other@example.invalid",
	})
	code2 := a.Run(ctx, []string{"run", "now", "--auto-swap", "--", "-p", "cancel test"})
	if code2 == 0 {
		t.Fatalf("expected cancelled wait loop to return non-zero, got %d", code2)
	}
}

func TestAutoSwapContinuousLauncherWrapperAndMarker(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX launcher wrapper test")
	}
	dir := t.TempDir()
	realLog := filepath.Join(dir, "real.log")
	swapLog := filepath.Join(dir, "swap.log")
	realBin := filepath.Join(dir, "agy.real")
	swapBin := filepath.Join(dir, "agy-swap")
	reloadMarker := filepath.Join(dir, "reload-marker")
	autoMarker := filepath.Join(dir, ".auto-swap-enabled")

	realScript := fmt.Sprintf("#!/bin/sh\necho \"REAL:$*\" >> %q\nexit 0\n", realLog)
	swapScript := fmt.Sprintf("#!/bin/sh\necho \"SWAP:$*\" >> %q\nexit 0\n", swapLog)
	if err := os.WriteFile(realBin, []byte(realScript), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(swapBin, []byte(swapScript), 0o755); err != nil {
		t.Fatal(err)
	}

	// Wrapper with skipPermsByDefault=true (like user's ~/.local/bin/agy).
	wrapperPath := filepath.Join(dir, "agy")
	script := buildAgyLauncherScriptForPathsWithAutoSwap(realBin, swapBin, reloadMarker, autoMarker, true)
	if err := os.WriteFile(wrapperPath, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	runWrap := func(extraEnv []string, args ...string) {
		cmd := exec.Command(wrapperPath, args...)
		env := append([]string{"HOME=" + dir, "PATH=/usr/bin:/bin", "AGY_SWAP_SKIP_SYNC=1"}, extraEnv...)
		cmd.Env = env
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("wrapper(%v) failed: %v\n%s", args, err, out)
		}
	}

	// 1. Marker absent and AGY_SWAP_AUTO unset -> runs real binary directly.
	runWrap(nil, "-c")
	// 2. Marker present -> interactive/resume flags delegate to agy-swap run --auto-swap -- --dangerously-skip-permissions ...
	if err := os.WriteFile(autoMarker, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	runWrap(nil, "-c")
	runWrap(nil, "--conversation", "uuid-123")
	// Do not duplicate --dangerously-skip-permissions if user already passed it.
	runWrap(nil, "--dangerously-skip-permissions", "-c")
	// 3. Bypass cases even with marker present: --version, --help, --hub, subcommands (models, mcp list, plugin list), AGY_SWAP_SUPERVISED=1, AGY_SWAP_NO_AUTO=1.
	runWrap(nil, "--version")
	runWrap(nil, "--help")
	runWrap(nil, "--hub")
	runWrap(nil, "models")
	runWrap(nil, "mcp", "list")
	runWrap(nil, "plugin", "list")
	runWrap([]string{"AGY_SWAP_SUPERVISED=1"}, "-c")
	runWrap([]string{"AGY_SWAP_NO_AUTO=1"}, "-c")
	// 4. AGY_SWAP_AUTO=1 delegates even when marker is absent.
	_ = os.Remove(autoMarker)
	runWrap([]string{"AGY_SWAP_AUTO=1"}, "--resume", "uuid-999")

	swapBytes, _ := os.ReadFile(swapLog)
	realBytes, _ := os.ReadFile(realLog)
	swapLines := strings.Split(strings.TrimSpace(string(swapBytes)), "\n")
	realLines := strings.Split(strings.TrimSpace(string(realBytes)), "\n")
	if len(swapLines) != 4 {
		t.Fatalf("expected 4 swap delegations, got %d:\n%s", len(swapLines), string(swapBytes))
	}
	if swapLines[0] != "SWAP:run --auto-swap -- --dangerously-skip-permissions -c" {
		t.Fatalf("unexpected swap line 0: %q", swapLines[0])
	}
	if swapLines[1] != "SWAP:run --auto-swap -- --dangerously-skip-permissions --conversation uuid-123" {
		t.Fatalf("unexpected swap line 1: %q", swapLines[1])
	}
	if swapLines[2] != "SWAP:run --auto-swap -- --dangerously-skip-permissions -c" {
		t.Fatalf("expected no duplicate --dangerously-skip-permissions, got: %q", swapLines[2])
	}
	if swapLines[3] != "SWAP:run --auto-swap -- --dangerously-skip-permissions --resume uuid-999" {
		t.Fatalf("unexpected swap line 3: %q", swapLines[3])
	}
	if len(realLines) != 9 {
		t.Fatalf("expected 9 direct real binary runs, got %d:\n%s", len(realLines), string(realBytes))
	}

	// Default skipPerms=false wrapper must NEVER contain --dangerously-skip-permissions or AGY_NORMAL_MODE.
	safeScript := buildAgyLauncherScriptForPathsWithAutoSwap(realBin, swapBin, reloadMarker, autoMarker, false)
	if strings.Contains(safeScript, "--dangerously-skip-permissions") || strings.Contains(safeScript, "AGY_NORMAL_MODE") {
		t.Fatalf("skipPermsByDefault=false wrapper leaked unsafe permission flag:\n%s", safeScript)
	}

	// 5. Verify Store.UpdateSettings creates and removes .auto-swap-enabled marker.
	a, _, _ := newPhaseTestApp(t)
	markerPath := filepath.Join(a.paths.ConfigDir, ".auto-swap-enabled")
	if _, err := a.store.UpdateSettings(func(s *AppSettings) error {
		s.AutoSwap.Enabled = true
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(markerPath); err != nil {
		t.Fatalf("expected .auto-swap-enabled marker after enabling, err=%v", err)
	}
	if _, err := a.store.UpdateSettings(func(s *AppSettings) error {
		s.AutoSwap.Enabled = false
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(markerPath); !os.IsNotExist(err) {
		t.Fatalf("expected .auto-swap-enabled marker removed after disabling, err=%v", err)
	}
}
