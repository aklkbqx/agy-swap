package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

type historyEvent struct {
	Schema int            `json:"schema"`
	At     string         `json:"at"`
	Kind   string         `json:"kind"`
	Email  string         `json:"email,omitempty"`
	Data   map[string]any `json:"data,omitempty"`
}

func (a *Application) appendHistory(kind, email string, data map[string]any) error {
	settings, err := a.loadSettings()
	if err != nil || !settings.History.Enabled {
		return err
	}
	event := historyEvent{Schema: historySchema, At: isoTime(time.Now().UTC()), Kind: cleanText(kind), Email: normalizeEmail(email), Data: data}
	encoded, err := json.Marshal(event)
	if err != nil {
		return err
	}
	encoded = append(encoded, '\n')
	if err := ensurePrivateDir(a.paths.ConfigDir); err != nil {
		return err
	}
	file, err := os.OpenFile(a.paths.History, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	_, writeErr := file.Write(encoded)
	closeErr := file.Close()
	if writeErr != nil {
		return writeErr
	}
	if closeErr != nil {
		return closeErr
	}
	return a.trimHistory(settings.History.MaxBytes, settings.History.RetentionDays)
}

func (a *Application) trimHistory(maxBytes, retentionDays int) error {
	if maxBytes <= 0 {
		maxBytes = maxHistoryBytes
	}
	data, err := os.ReadFile(a.paths.History)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	lines := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
	cutoff := time.Now().UTC().Add(-time.Duration(retentionDays) * 24 * time.Hour)
	kept := make([]string, 0, len(lines))
	for _, line := range lines {
		if line == "" {
			continue
		}
		var event historyEvent
		if json.Unmarshal([]byte(line), &event) != nil {
			continue
		}
		at, parseErr := parseUTC(event.At)
		if parseErr == nil && at.Before(cutoff) {
			continue
		}
		kept = append(kept, line)
	}
	start := 0
	size := 0
	for i := len(kept) - 1; i >= 0; i-- {
		lineSize := len(kept[i]) + 1
		if size+lineSize > maxBytes {
			break
		}
		size += lineSize
		start = i
	}
	if len(kept) == 0 {
		return atomicWrite(a.paths.History, nil, 0o600)
	}
	return atomicWrite(a.paths.History, []byte(strings.Join(kept[start:], "\n")+"\n"), 0o600)
}

func (a *Application) readHistory(limit int) ([]historyEvent, error) {
	if limit <= 0 {
		limit = maxHistoryBytes
	}
	data, err := readLimited(a.paths.History, int64(maxHistoryBytes)+1)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	lines := strings.Split(string(data), "\n")
	events := make([]historyEvent, 0, len(lines))
	for i := len(lines) - 1; i >= 0 && len(events) < limit; i-- {
		if lines[i] == "" {
			continue
		}
		var event historyEvent
		if json.Unmarshal([]byte(lines[i]), &event) == nil {
			events = append(events, event)
		}
	}
	return events, nil
}

func (a *Application) cmdHistory(opts extendedOptions, positional []string) int {
	sub := "list"
	if len(positional) > 0 {
		sub = positional[0]
	}
	switch sub {
	case "list":
		events, err := a.readHistory(opts.Limit)
		if err != nil {
			return a.extendedError("history list", opts, err)
		}
		if opts.JSON {
			return a.extendedResult("history list", opts, events, nil)
		}
		for _, event := range events {
			fmt.Fprintf(a.Out, "%s  %-12s  %-30s\n", event.At, event.Kind, event.Email)
		}
		return 0
	case "clear":
		if !opts.Force {
			return a.extendedError("history clear", opts, errors.New("rerun with --force to remove history"))
		}
		if err := os.Remove(a.paths.History); err != nil && !errors.Is(err, os.ErrNotExist) {
			return a.extendedError("history clear", opts, err)
		}
		return 0
	case "export":
		events, err := a.readHistory(maxHistoryBytes)
		if err != nil {
			return a.extendedError("history export", opts, err)
		}
		target := opts.Output
		if target == "" && len(positional) > 1 {
			target = positional[1]
		}
		if target == "" {
			target = "-"
		}
		if target == "-" {
			for _, event := range events {
				data, _ := json.Marshal(event)
				fmt.Fprintln(a.Out, string(data))
			}
			return 0
		}
		data, _ := json.MarshalIndent(events, "", "  ")
		if err := atomicWrite(target, append(data, '\n'), 0o600); err != nil {
			return a.extendedError("history export", opts, err)
		}
		return 0
	default:
		return a.extendedError("history", opts, fmt.Errorf("unknown subcommand %q", sub))
	}
}

func (a *Application) cmdHistoryStats(opts extendedOptions) int {
	events, err := a.readHistory(maxHistoryBytes)
	if err != nil {
		return a.extendedError("stats", opts, err)
	}
	byKind := map[string]int{}
	byEmail := map[string]int{}
	var newest, oldest string
	for _, event := range events {
		byKind[event.Kind]++
		if event.Email != "" {
			byEmail[event.Email]++
		}
		if newest == "" || event.At > newest {
			newest = event.At
		}
		if oldest == "" || event.At < oldest {
			oldest = event.At
		}
	}
	data := map[string]any{"events": len(events), "by_kind": byKind, "by_email": byEmail, "newest": newest, "oldest": oldest}
	if opts.JSON {
		return a.extendedResult("stats", opts, data, nil)
	}
	fmt.Fprintf(a.Out, "Events: %d\n", len(events))
	for key, count := range byKind {
		fmt.Fprintf(a.Out, "  %-18s %d\n", key, count)
	}
	return 0
}

func snapshotMinimum(account Account) (float64, string, time.Time, bool) {
	groups := quotaGroupHealths(account)
	var best *quotaGroupHealth
	for i := range groups {
		group := &groups[i]
		if best == nil || group.fraction < best.fraction {
			best = group
		}
	}
	if best == nil {
		return 0, "", time.Time{}, false
	}
	return best.fraction * 100, best.label, best.resetAt, true
}

func (a *Application) cmdForecast(ctx context.Context, opts extendedOptions, positional []string) int {
	accounts, err := a.store.Load(true)
	if err != nil {
		return a.extendedError("forecast", opts, err)
	}
	settings, _ := a.loadSettings()
	target := opts.Account
	if target == "" && len(positional) > 0 {
		target = positional[0]
	}
	if target == "" {
		target = "all"
	}
	if opts.Refresh {
		a.quota.Refresh(ctx, accounts, true, nil)
	}
	resolvedTarget := ""
	if target != "all" {
		resolvedTarget, err = resolveConfiguredTarget(target, accounts, settings)
		if err != nil || resolvedTarget == "" {
			if err == nil {
				err = errors.New("account not found")
			}
			return a.extendedError("forecast", opts, err)
		}
	}
	result := make([]map[string]any, 0)
	for _, email := range accounts.Order {
		if resolvedTarget != "" {
			if resolvedTarget != email {
				continue
			}
		}
		account := accounts.ByEmail[email]
		remaining, group, reset, known := snapshotMinimum(account)
		item := map[string]any{"email": email, "known": known, "remaining_pct": remaining, "group": group, "confidence": "low"}
		if known {
			item["confidence"] = "observed"
			item["reset_at"] = isoTime(reset)
			if reset.Before(time.Now()) {
				item["confidence"] = "stale"
			}
		}
		result = append(result, item)
	}
	if opts.JSON {
		return a.extendedResult("forecast", opts, result, nil)
	}
	for _, item := range result {
		fmt.Fprintf(a.Out, "%s: %.1f%% remaining", item["email"], item["remaining_pct"])
		if value, ok := item["reset_at"].(string); ok {
			fmt.Fprintf(a.Out, " · reset %s", value)
		}
		fmt.Fprintf(a.Out, " · confidence %s\n", item["confidence"])
	}
	return 0
}

type runtimeNotificationState struct {
	Sent map[string]string  `json:"sent,omitempty"`
	Last map[string]float64 `json:"last,omitempty"`
}

func (a *Application) loadNotificationState() runtimeNotificationState {
	data, err := os.ReadFile(a.paths.RuntimeState)
	if err != nil {
		return runtimeNotificationState{Sent: map[string]string{}, Last: map[string]float64{}}
	}
	var state runtimeNotificationState
	if json.Unmarshal(data, &state) != nil || state.Sent == nil {
		state.Sent = map[string]string{}
	}
	if state.Last == nil {
		state.Last = map[string]float64{}
	}
	return state
}
func (a *Application) saveNotificationState(state runtimeNotificationState) {
	_ = atomicWriteJSON(a.paths.RuntimeState, state)
}

func (a *Application) notifyIfDue(state *runtimeNotificationState, key, title, body string, cooldown time.Duration) {
	now := time.Now().UTC()
	previous := state.Sent[key]
	if previous != "" {
		if sentAt, parseErr := parseUTC(previous); parseErr == nil && now.Sub(sentAt) < cooldown {
			return
		}
	}
	a.notify(title, body)
	state.Sent[key] = isoTime(now)
}

func (a *Application) notify(title, body string) {
	if runtime.GOOS == "darwin" {
		_ = exec.Command("osascript", "-e", fmt.Sprintf("display notification %q with title %q", body, title)).Run()
		return
	}
	if runtime.GOOS == "linux" {
		if _, err := exec.LookPath("notify-send"); err == nil {
			_ = exec.Command("notify-send", title, body).Run()
			return
		}
	}
	if runtime.GOOS == "windows" {
		_ = exec.Command("powershell", "-NoProfile", "-Command", "[console]::beep(900,150)").Run()
	}
}

func (a *Application) watchOnce(ctx context.Context, opts extendedOptions) (map[string]any, error) {
	accounts, err := a.store.Load(true)
	if err != nil {
		return nil, err
	}
	settings, err := a.loadSettings()
	if err != nil {
		return nil, err
	}
	failures := a.quota.Refresh(ctx, accounts, opts.Refresh, nil)
	state := a.loadNotificationState()
	now := time.Now().UTC()
	summaries := make([]map[string]any, 0, accounts.Len())
	for _, email := range accounts.Order {
		account := accounts.ByEmail[email]
		remaining, group, reset, known := snapshotMinimum(account)
		summary := map[string]any{"email": email, "remaining_pct": remaining, "group": group, "known": known}
		if known {
			summary["reset_at"] = isoTime(reset)
		}
		key := email + ":" + group
		threshold := settings.Notifications.Threshold
		if opts.Threshold >= 0 {
			threshold = opts.Threshold
		}
		if known {
			if settings.Notifications.Enabled && settings.Notifications.Reset && state.Last[key] <= float64(threshold) && remaining > float64(threshold) && state.Last[key] > 0 {
				a.notifyIfDue(&state, key+":reset", "agy-swap quota reset", fmt.Sprintf("%s: %s is %.1f%% available again", email, group, remaining), time.Duration(settings.Notifications.CooldownSeconds)*time.Second)
			}
			state.Last[key] = remaining
			if age, ageOK := quotaAge(account, now); settings.Notifications.Enabled && settings.Notifications.Stale && ageOK && age > 2*opts.Interval {
				summary["stale"] = true
				a.notifyIfDue(&state, key+":stale", "agy-swap quota stale", fmt.Sprintf("%s: %s data is %s old", email, group, formatDuration(age.Seconds())), time.Duration(settings.Notifications.CooldownSeconds)*time.Second)
			}
		}
		if failure := failures[email]; failure != "" {
			summary["error"] = failure
			if settings.Notifications.Enabled && settings.Notifications.AuthFailure && (strings.Contains(failure, "401") || strings.Contains(strings.ToLower(failure), "auth")) {
				a.notify("agy-swap authentication", email+": "+failure)
			}
		}
		if known {
			_ = a.appendHistory("quota", email, map[string]any{"remaining_pct": remaining, "group": group, "reset_at": isoTime(reset)})
		}
		if settings.Notifications.Enabled && known && remaining <= float64(threshold) {
			a.notifyIfDue(&state, key+":low", "agy-swap quota warning", fmt.Sprintf("%s: %.1f%% remaining", email, remaining), time.Duration(settings.Notifications.CooldownSeconds)*time.Second)
		}
		summaries = append(summaries, summary)
	}
	a.saveNotificationState(state)
	return map[string]any{"observed_at": isoTime(now), "accounts": summaries, "errors": failures}, nil
}

func (a *Application) cmdWatch(ctx context.Context, opts extendedOptions, positional []string) int {
	if opts.Account == "" && len(positional) > 0 {
		opts.Account = positional[0]
	}
	for {
		data, err := a.watchOnce(ctx, opts)
		if err != nil {
			return a.extendedError("watch", opts, err)
		}
		if opts.JSON {
			a.extendedResult("watch", opts, data, nil)
		} else {
			fmt.Fprintf(a.Out, "[%s] quota watch\n", time.Now().Format(time.RFC3339))
			for _, item := range data["accounts"].([]map[string]any) {
				fmt.Fprintf(a.Out, "  %-30s %6.1f%%\n", item["email"], item["remaining_pct"])
			}
		}
		if opts.Once {
			return 0
		}
		timer := time.NewTimer(opts.Interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return 0
		case <-timer.C:
		}
	}
}
