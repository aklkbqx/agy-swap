package app

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"sort"
	"strings"
	"time"
)

// AutoSwapState persists rolling auto-swap budgets and the global circuit
// breaker inside runtime-state.json so safety limits survive process restarts.
type AutoSwapState struct {
	CircuitOpen         bool     `json:"circuit_open,omitempty"`
	CircuitReason       string   `json:"circuit_reason,omitempty"`
	CircuitTrippedAt    string   `json:"circuit_tripped_at,omitempty"`
	LastSwapAt          string   `json:"last_swap_at,omitempty"`
	LastSwapEmail       string   `json:"last_swap_email,omitempty"`
	SwapTimestamps      []string `json:"swap_timestamps,omitempty"`
	ConsecutiveFailures int      `json:"consecutive_failures,omitempty"`
}

const rapidPostSwap429Window = 60 * time.Second

// autoSwapPaceSleep is overridden in unit tests to keep test runs instant.
var autoSwapPaceSleep = func(ctx context.Context, d time.Duration) {
	if d <= 0 {
		return
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
	case <-timer.C:
	}
}

type autoSwapOutcome struct {
	Swapped     bool   `json:"swapped"`
	TargetEmail string `json:"target_email,omitempty"`
	Status      string `json:"status"` // swapped, circuit_open, dwell_wait, hourly_budget_exceeded, daily_budget_exceeded, no_candidate, disabled
	Reason      string `json:"reason,omitempty"`
	WaitSeconds int    `json:"wait_seconds,omitempty"`
}

func pruneRollingTimestamps(stamps []string, now time.Time) ([]string, int, int) {
	kept := make([]string, 0, len(stamps))
	swaps1h := 0
	swaps24h := 0
	for _, raw := range stamps {
		t, err := parseUTC(raw)
		if err != nil {
			continue
		}
		age := now.Sub(t)
		if age < 0 || age > 24*time.Hour {
			continue
		}
		kept = append(kept, isoTime(t))
		swaps24h++
		if age <= time.Hour {
			swaps1h++
		}
	}
	return kept, swaps1h, swaps24h
}

func accountTierRank(account Account) int {
	snapshot := getMap(account["quota_snapshot"])
	tier := getMap(snapshot["tier"])
	id := strings.ToLower(strings.TrimSpace(getString(tier, "id")))
	if id == "" {
		if cached := getMap(account["cloud_project_cache"]); cached != nil {
			id = strings.ToLower(strings.TrimSpace(getString(getMap(cached["tier"]), "id")))
		}
	}
	switch {
	case strings.Contains(id, "ultra"):
		return 3
	case strings.Contains(id, "pro"):
		return 2
	case id == "free-tier" || strings.Contains(id, "free"):
		return 0
	case id != "":
		return 1
	default:
		return 1
	}
}

func accountIsFreeTier(account Account) bool {
	return accountTierRank(account) == 0
}

func accountFamilyRemainingPct(account Account, family string) (float64, bool) {
	groups := quotaGroupHealths(account)
	var best *quotaGroupHealth
	for i := range groups {
		g := &groups[i]
		if !poolInFamily(account, g.id, family) {
			continue
		}
		if best == nil || g.fraction < best.fraction {
			best = g
		}
	}
	if best == nil {
		return 0, false
	}
	return best.fraction * 100, true
}

func (a *Application) loadAutoSwapState() AutoSwapState {
	rt := a.loadNotificationState()
	if rt.AutoSwap == nil {
		return AutoSwapState{}
	}
	return *rt.AutoSwap
}

func (a *Application) saveAutoSwapState(state AutoSwapState) {
	rt := a.loadNotificationState()
	copyState := state
	rt.AutoSwap = &copyState
	a.saveNotificationState(rt)
}

func (a *Application) tripAutoSwapCircuit(state *AutoSwapState, now time.Time, reason string) {
	state.CircuitOpen = true
	state.CircuitReason = strings.TrimSpace(reason)
	state.CircuitTrippedAt = isoTime(now.UTC())
	a.saveAutoSwapState(*state)
}

func (a *Application) isAutoSwapFailureTrigger(errMsg string) bool {
	lower := strings.ToLower(strings.TrimSpace(errMsg))
	if lower == "" {
		return false
	}
	return strings.HasPrefix(errMsg, eligibilityErrorPrefix) ||
		strings.Contains(lower, "401") ||
		strings.Contains(lower, "403") ||
		strings.Contains(lower, "oauth") ||
		strings.Contains(lower, "token") ||
		strings.Contains(lower, "invalid_grant") ||
		strings.Contains(lower, "verify") ||
		strings.Contains(lower, "suspend")
}

// attemptGuardrailedAutoSwap enforces every non-bypassable safety guardrail
// (global circuit breaker, TOS_VIOLATION halt, rapid-429 detection, dwell time,
// rolling 1h/24h budgets, protected/suspended exclusion, paid-tier preference,
// lazy single-candidate refresh, hysteresis >= switch_in_pct, and 3-8s human
// pacing jitter) before switching credentials.
func (a *Application) attemptGuardrailedAutoSwap(ctx context.Context, activeEmail, profileName, family string, now time.Time) autoSwapOutcome {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	settings, err := a.loadSettings()
	if err != nil {
		return autoSwapOutcome{Status: "error", Reason: err.Error()}
	}
	settings, err = normalizeSettings(settings)
	if err != nil {
		return autoSwapOutcome{Status: "error", Reason: err.Error()}
	}
	cfg := settings.AutoSwap

	state := a.loadAutoSwapState()
	state.SwapTimestamps, _, _ = pruneRollingTimestamps(state.SwapTimestamps, now)

	if state.CircuitOpen {
		return autoSwapOutcome{
			Status: "circuit_open",
			Reason: fmt.Sprintf("auto-swap circuit breaker is open (%s); run 'agy-swap auto-swap reset' after reviewing accounts", firstString(state.CircuitReason, "safety halt")),
		}
	}

	accounts, err := a.store.Load(true)
	if err != nil {
		return autoSwapOutcome{Status: "error", Reason: err.Error()}
	}

	// Gate 1a: Any SUSPENDED (TOS_VIOLATION) account trips the global circuit breaker immediately.
	if cfg.HaltOnTosViolation {
		for _, email := range accounts.Order {
			if accountIsSuspended(accounts.ByEmail[email]) {
				reason := "suspended account detected: " + email
				a.tripAutoSwapCircuit(&state, now, reason)
				return autoSwapOutcome{Status: "circuit_open", Reason: reason}
			}
		}
	}

	// Gate 1b: Rapid 429 / exhaustion within 60s of a fresh auto-swap indicates IP/device throttling.
	if state.LastSwapAt != "" && state.LastSwapEmail != "" && strings.EqualFold(state.LastSwapEmail, activeEmail) {
		if lastAt, parseErr := parseUTC(state.LastSwapAt); parseErr == nil {
			if since := now.Sub(lastAt); since >= 0 && since < rapidPostSwap429Window {
				reason := fmt.Sprintf("rapid exhaustion on %s (%ds after last swap); halting to protect accounts", activeEmail, int(since.Seconds()))
				a.tripAutoSwapCircuit(&state, now, reason)
				return autoSwapOutcome{Status: "circuit_open", Reason: reason}
			}
		}
	}

	// Gate 2a: Minimum dwell time (non-bypassable floor >= 300s, default 900s).
	if state.LastSwapAt != "" {
		if lastAt, parseErr := parseUTC(state.LastSwapAt); parseErr == nil {
			dwell := time.Duration(cfg.MinDwellSeconds) * time.Second
			if remaining := dwell - now.Sub(lastAt); remaining > 0 {
				return autoSwapOutcome{
					Status:      "dwell_wait",
					Reason:      fmt.Sprintf("minimum dwell time active (%s remaining)", formatDuration(remaining.Seconds())),
					WaitSeconds: int(remaining.Seconds()),
				}
			}
		}
	}

	// Gate 2b: Rolling hourly and daily swap budgets.
	kept, swaps1h, swaps24h := pruneRollingTimestamps(state.SwapTimestamps, now)
	state.SwapTimestamps = kept
	if swaps1h >= cfg.MaxSwapsPerHour {
		return autoSwapOutcome{
			Status: "hourly_budget_exceeded",
			Reason: fmt.Sprintf("hourly auto-swap budget reached (%d/%d)", swaps1h, cfg.MaxSwapsPerHour),
		}
	}
	if swaps24h >= cfg.MaxSwapsPerDay {
		return autoSwapOutcome{
			Status: "daily_budget_exceeded",
			Reason: fmt.Sprintf("daily auto-swap budget reached (%d/%d)", swaps24h, cfg.MaxSwapsPerDay),
		}
	}

	// Gate 3: Build eligible candidate list (respecting profile scope, protected, suspended, verify, free-tier exclusion).
	allowed := map[string]bool{}
	if profileName != "" {
		for _, e := range scopedEligibleEmails(accounts, settings, profileName, "") {
			allowed[e] = true
		}
	}

	type scoredCandidate struct {
		email    string
		tierRank int
		pct      float64
		known    bool
		age      time.Duration
		hasSnap  bool
		orderIdx int
	}
	candidates := make([]scoredCandidate, 0, accounts.Len())
	for idx, email := range accounts.Order {
		if strings.EqualFold(email, activeEmail) {
			continue
		}
		if len(allowed) > 0 && !allowed[email] {
			continue
		}
		acc := accounts.ByEmail[email]
		if accountIsProtected(acc) || accountIsSuspended(acc) {
			continue
		}
		if _, ineligible := accountEligibility(acc); ineligible {
			continue
		}
		if cfg.ExcludeFreeTier && accountIsFreeTier(acc) {
			continue
		}
		if wait, hasWait := accountCooldown(acc, now, family); hasWait && wait > 0 {
			continue
		}
		pct, known := accountFamilyRemainingPct(acc, family)
		age, hasSnap := quotaAge(acc, now)
		candidates = append(candidates, scoredCandidate{
			email:    email,
			tierRank: accountTierRank(acc),
			pct:      pct,
			known:    known,
			age:      age,
			hasSnap:  hasSnap,
			orderIdx: idx,
		})
	}
	if len(candidates) == 0 {
		return autoSwapOutcome{Status: "no_candidate", Reason: "no eligible backup account available"}
	}

	sort.Slice(candidates, func(i, j int) bool {
		if cfg.PreferPaidTiers && candidates[i].tierRank != candidates[j].tierRank {
			return candidates[i].tierRank > candidates[j].tierRank
		}
		if candidates[i].known != candidates[j].known {
			return candidates[i].known
		}
		if candidates[i].pct != candidates[j].pct {
			return candidates[i].pct > candidates[j].pct
		}
		return candidates[i].orderIdx < candidates[j].orderIdx
	})

	staleWindow := candidateStaleWindow(settings)
	attemptFailures := 0
	var chosenEmail string
	for _, cand := range candidates {
		acc := accounts.ByEmail[cand.email]
		// Lazy single-candidate refresh only if snapshot is missing or older than staleWindow.
		if !cand.hasSnap || cand.age > staleWindow {
			failures := a.quota.RefreshEmails(ctx, accounts, []string{cand.email})
			if accountIsSuspended(accounts.ByEmail[cand.email]) {
				reason := "suspended account discovered during candidate check: " + cand.email
				a.tripAutoSwapCircuit(&state, now, reason)
				return autoSwapOutcome{Status: "circuit_open", Reason: reason}
			}
			if failure := failures[cand.email]; failure != "" {
				if a.isAutoSwapFailureTrigger(failure) {
					attemptFailures++
					state.ConsecutiveFailures = attemptFailures
					if attemptFailures >= 2 {
						reason := fmt.Sprintf("%d consecutive candidate auth/eligibility failures (latest %s: %s)", attemptFailures, cand.email, failure)
						a.tripAutoSwapCircuit(&state, now, reason)
						return autoSwapOutcome{Status: "circuit_open", Reason: reason}
					}
				}
				continue
			}
			acc = accounts.ByEmail[cand.email]
		}
		if accountIsSuspended(acc) {
			reason := "suspended account detected: " + cand.email
			a.tripAutoSwapCircuit(&state, now, reason)
			return autoSwapOutcome{Status: "circuit_open", Reason: reason}
		}
		if _, ineligible := accountEligibility(acc); ineligible {
			attemptFailures++
			if attemptFailures >= 2 {
				reason := fmt.Sprintf("%d consecutive ineligible candidates", attemptFailures)
				a.tripAutoSwapCircuit(&state, now, reason)
				return autoSwapOutcome{Status: "circuit_open", Reason: reason}
			}
			continue
		}
		if cfg.ExcludeFreeTier && accountIsFreeTier(acc) {
			continue
		}
		if wait, hasWait := accountCooldown(acc, now, family); hasWait && wait > 0 {
			continue
		}
		pct, known := accountFamilyRemainingPct(acc, family)
		if !known || pct < float64(cfg.SwitchInPct) {
			continue
		}
		chosenEmail = cand.email
		break
	}

	if chosenEmail == "" {
		return autoSwapOutcome{
			Status: "no_candidate",
			Reason: fmt.Sprintf("no candidate meets switch_in_pct (>= %d%%)", cfg.SwitchInPct),
		}
	}

	chosenAcc := accounts.ByEmail[chosenEmail]
	token, tokenErr := a.accountToken(ctx, chosenAcc)
	if tokenErr != nil {
		return autoSwapOutcome{Status: "error", Reason: fmt.Sprintf("read credential for %s: %v", chosenEmail, tokenErr)}
	}

	// Gate 4: Human-paced jitter (3-8s default) before atomic swap.
	jitter := time.Duration(cfg.PaceJitterMinMs) * time.Millisecond
	if cfg.PaceJitterMaxMs > cfg.PaceJitterMinMs {
		jitter += time.Duration(rand.IntN(cfg.PaceJitterMaxMs-cfg.PaceJitterMinMs)) * time.Millisecond
	}
	autoSwapPaceSleep(ctx, jitter)
	if ctx.Err() != nil {
		return autoSwapOutcome{Status: "cancelled", Reason: ctx.Err().Error()}
	}

	// Unattended auto-swap NEVER force-restarts the IDE window (app-restart).
	reloadMode := settings.IDE.ReloadMode
	if reloadMode == "app-restart" {
		reloadMode = "hub-restart"
	}
	if !a.applyAccountWithMode(ctx, token, chosenEmail, reloadMode) {
		return autoSwapOutcome{Status: "error", Reason: "failed to apply credentials for " + chosenEmail}
	}

	state.LastSwapAt = isoTime(now.UTC())
	state.LastSwapEmail = chosenEmail
	state.SwapTimestamps = append(state.SwapTimestamps, isoTime(now.UTC()))
	state.ConsecutiveFailures = 0
	a.saveAutoSwapState(state)

	return autoSwapOutcome{
		Swapped:     true,
		TargetEmail: chosenEmail,
		Status:      "swapped",
	}
}

func (a *Application) cmdAutoSwap(_ context.Context, opts extendedOptions, positional []string) int {
	sub := "status"
	if len(positional) > 0 {
		sub = strings.ToLower(strings.TrimSpace(positional[0]))
	}
	settings, err := a.loadSettings()
	if err != nil {
		return a.extendedError("auto-swap "+sub, opts, err)
	}
	accounts, err := a.store.Load(false)
	if err != nil {
		return a.extendedError("auto-swap "+sub, opts, err)
	}
	now := time.Now().UTC()
	state := a.loadAutoSwapState()
	kept, swaps1h, swaps24h := pruneRollingTimestamps(state.SwapTimestamps, now)
	state.SwapTimestamps = kept

	var protectedEmails, suspendedEmails []string
	for _, email := range accounts.Order {
		acc := accounts.ByEmail[email]
		if accountIsProtected(acc) {
			protectedEmails = append(protectedEmails, email)
		}
		if accountIsSuspended(acc) {
			suspendedEmails = append(suspendedEmails, email)
		}
	}

	switch sub {
	case "enable", "disable":
		enable := sub == "enable"
		updated, updateErr := a.store.UpdateSettings(func(s *AppSettings) error {
			s.AutoSwap.Enabled = enable
			if enable && opts.Handoff != "" {
				s.AutoSwap.HandoffMode = opts.Handoff
			}
			return nil
		})
		if updateErr != nil {
			return a.extendedError("auto-swap "+sub, opts, updateErr)
		}
		var warnings []string
		if enable && len(protectedEmails) == 0 && accounts.Len() > 0 {
			warnings = append(warnings, "no primary account is protected; run 'agy-swap account protect <email>' to keep a primary account out of automatic rotation")
		}
		if enable && updated.AutoSwap.HandoffMode == "resume" {
			warnings = append(warnings, "handoff_mode=resume reuses conversation trajectory across accounts; prefer clean-summary")
		}
		if enable && len(suspendedEmails) > 0 {
			warnings = append(warnings, fmt.Sprintf("%d account(s) marked SUSPENDED (%s); auto-swap will halt until resolved", len(suspendedEmails), strings.Join(suspendedEmails, ", ")))
		}
		data := map[string]any{
			"enabled":      updated.AutoSwap.Enabled,
			"handoff_mode": updated.AutoSwap.HandoffMode,
		}
		if opts.JSON {
			return a.extendedResult("auto-swap "+sub, opts, data, warnings)
		}
		if enable {
			fmt.Fprintf(a.Out, "✓ Auto-swap enabled (handoff=%s). Launch supervised sessions with 'agy-swap run' or 'agy-swap run -c'.\n", updated.AutoSwap.HandoffMode)
		} else {
			fmt.Fprintln(a.Out, "✓ Auto-swap disabled.")
		}
		for _, w := range warnings {
			fmt.Fprintf(a.Err, "! %s\n", w)
		}
		return 0

	case "status":
		dwellRemaining := 0
		if state.LastSwapAt != "" {
			if lastAt, parseErr := parseUTC(state.LastSwapAt); parseErr == nil {
				if rem := time.Duration(settings.AutoSwap.MinDwellSeconds)*time.Second - now.Sub(lastAt); rem > 0 {
					dwellRemaining = int(rem.Seconds())
				}
			}
		}
		data := map[string]any{
			"enabled":                 settings.AutoSwap.Enabled,
			"mode":                    settings.AutoSwap.Mode,
			"handoff_mode":            settings.AutoSwap.HandoffMode,
			"circuit_open":            state.CircuitOpen,
			"circuit_reason":          state.CircuitReason,
			"circuit_tripped_at":      state.CircuitTrippedAt,
			"last_swap_at":            state.LastSwapAt,
			"last_swap_email":         state.LastSwapEmail,
			"swaps_1h":                swaps1h,
			"max_swaps_per_hour":      settings.AutoSwap.MaxSwapsPerHour,
			"swaps_24h":               swaps24h,
			"max_swaps_per_day":       settings.AutoSwap.MaxSwapsPerDay,
			"min_dwell_seconds":       settings.AutoSwap.MinDwellSeconds,
			"dwell_remaining_seconds": dwellRemaining,
			"switch_out_pct":          settings.AutoSwap.SwitchOutPct,
			"switch_in_pct":           settings.AutoSwap.SwitchInPct,
			"protected_accounts":      protectedEmails,
			"suspended_accounts":      suspendedEmails,
		}
		if opts.JSON {
			return a.extendedResult("auto-swap status", opts, data, nil)
		}
		circuitLabel := "closed (armed)"
		if state.CircuitOpen {
			circuitLabel = "OPEN (" + firstString(state.CircuitReason, "halted") + ")"
		}
		fmt.Fprintf(a.Out, "Auto-swap:        enabled=%v · mode=%s · handoff=%s\n", settings.AutoSwap.Enabled, settings.AutoSwap.Mode, settings.AutoSwap.HandoffMode)
		fmt.Fprintf(a.Out, "Circuit breaker:  %s\n", circuitLabel)
		fmt.Fprintf(a.Out, "Budgets:          %d/%d per hour · %d/%d per 24h · min dwell %ds\n", swaps1h, settings.AutoSwap.MaxSwapsPerHour, swaps24h, settings.AutoSwap.MaxSwapsPerDay, settings.AutoSwap.MinDwellSeconds)
		fmt.Fprintf(a.Out, "Thresholds:       switch_out <= %d%% · switch_in >= %d%%\n", settings.AutoSwap.SwitchOutPct, settings.AutoSwap.SwitchInPct)
		if state.LastSwapEmail != "" {
			fmt.Fprintf(a.Out, "Last swap:        %s at %s\n", state.LastSwapEmail, state.LastSwapAt)
		}
		if len(protectedEmails) > 0 {
			fmt.Fprintf(a.Out, "Protected:        %s\n", strings.Join(protectedEmails, ", "))
		}
		if len(suspendedEmails) > 0 {
			fmt.Fprintf(a.Out, "Suspended:        %s (circuit will trip again until resolved)\n", strings.Join(suspendedEmails, ", "))
		}
		return 0

	case "reset":
		wasOpen := state.CircuitOpen
		state.CircuitOpen = false
		state.CircuitReason = ""
		state.CircuitTrippedAt = ""
		state.ConsecutiveFailures = 0
		a.saveAutoSwapState(state)
		var warnings []string
		if len(suspendedEmails) > 0 {
			warnings = append(warnings, fmt.Sprintf("%d account(s) still marked SUSPENDED (%s); remove or resolve them before auto-swapping", len(suspendedEmails), strings.Join(suspendedEmails, ", ")))
		}
		data := map[string]any{
			"reset":              true,
			"previously_open":    wasOpen,
			"swaps_1h":           swaps1h,
			"swaps_24h":          swaps24h,
			"suspended_accounts": suspendedEmails,
		}
		if opts.JSON {
			return a.extendedResult("auto-swap reset", opts, data, warnings)
		}
		fmt.Fprintln(a.Out, "✓ Auto-swap circuit breaker reset (rolling hourly/daily budgets preserved).")
		for _, w := range warnings {
			fmt.Fprintf(a.Err, "! %s\n", w)
		}
		return 0

	default:
		return a.extendedError("auto-swap", opts, errors.New("usage: auto-swap <enable|disable|status|reset>"))
	}
}
