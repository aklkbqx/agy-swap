package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
)

func numericField(value any, keys ...string) (float64, bool) {
	if object, ok := value.(map[string]any); ok {
		for _, key := range keys {
			if number, ok := getFloat(object[key]); ok {
				return number, true
			}
		}
		for _, child := range object {
			if number, ok := numericField(child, keys...); ok {
				return number, true
			}
		}
	}
	if list, ok := value.([]any); ok {
		for _, child := range list {
			if number, ok := numericField(child, keys...); ok {
				return number, true
			}
		}
	}
	return 0, false
}

func stringField(value any, keys ...string) string {
	if object, ok := value.(map[string]any); ok {
		for _, key := range keys {
			if text := cleanText(getString(object, key)); text != "" {
				return text
			}
		}
		for _, child := range object {
			if text := stringField(child, keys...); text != "" {
				return text
			}
		}
	}
	if list, ok := value.([]any); ok {
		for _, child := range list {
			if text := stringField(child, keys...); text != "" {
				return text
			}
		}
	}
	return ""
}

func (a *Application) statuslineFromInput(value any) string {
	email := stringField(value, "email", "account_email", "account")
	remaining, hasRemaining := numericField(value, "remaining_percent", "quota_percent", "remaining_pct", "remainingPercent")
	if !hasRemaining {
		if fraction, ok := numericField(value, "remaining_fraction", "remainingFraction"); ok {
			remaining, hasRemaining = fraction*100, true
		}
	}
	if !hasRemaining {
		if used, ok := numericField(value, "used_percent"); ok {
			remaining, hasRemaining = 100-used, true
		}
	}
	plan := stringField(value, "plan", "tier", "model")
	reset := stringField(value, "reset_at", "resetAt", "reset_time", "resetTime")
	parts := []string{"agy-swap"}
	if email != "" {
		parts = append(parts, email)
	}
	if plan != "" {
		parts = append(parts, plan)
	}
	if hasRemaining {
		parts = append(parts, fmt.Sprintf("quota %.0f%%", max(0, min(100, remaining))))
	}
	if reset != "" {
		if parsed, err := parseUTC(reset); err == nil {
			reset = parsed.Local().Format("15:04")
		}
		parts = append(parts, "reset "+reset)
	}
	if len(parts) == 1 {
		parts = append(parts, "ready")
	}
	return strings.Join(parts, " · ")
}

func (a *Application) cmdStatusline(ctx context.Context, opts extendedOptions, positional []string) int {
	sub := "render"
	if len(positional) > 0 {
		sub = positional[0]
	}
	switch sub {
	case "render", "run":
		data, readErr := io.ReadAll(io.LimitReader(a.In, maxTokenBytes+1))
		if readErr != nil {
			return a.extendedError("statusline render", opts, readErr)
		}
		if len(data) > maxTokenBytes {
			return a.extendedError("statusline render", opts, errors.New("statusline input exceeds limit"))
		}
		var value any
		if strings.TrimSpace(string(data)) != "" {
			if err := json.Unmarshal(data, &value); err != nil {
				return a.extendedError("statusline render", opts, fmt.Errorf("invalid statusline JSON: %w", err))
			}
		} else {
			accounts, err := a.store.Load(false)
			if err != nil {
				return a.extendedError("statusline render", opts, err)
			}
			active := a.activeEmail(ctx, accounts, a.credentials.Current(ctx))
			if account, ok := accounts.Get(active); ok {
				remaining, group, reset, known := snapshotMinimum(account)
				value = map[string]any{"email": active, "remaining_percent": remaining, "group": group, "known": known}
				if known {
					value.(map[string]any)["reset_at"] = isoTime(reset)
				}
			} else {
				value = map[string]any{"email": active}
			}
		}
		line := a.statuslineFromInput(value)
		if opts.JSON {
			return a.extendedResult("statusline render", opts, map[string]string{"text": line}, nil)
		}
		fmt.Fprintln(a.Out, line)
		return 0
	case "install":
		settings, err := a.loadSettings()
		if err != nil {
			return a.extendedError("statusline install", opts, err)
		}
		settings.Statusline = StatuslineConfig{Installed: true, Command: "agy-swap statusline render"}
		if err := a.store.SaveSettings(settings); err != nil {
			return a.extendedError("statusline install", opts, err)
		}
		data := map[string]any{"installed": true, "command": settings.Statusline.Command, "hint": "pipe your CLI statusline JSON into: agy-swap statusline render"}
		if opts.JSON {
			return a.extendedResult("statusline install", opts, data, nil)
		}
		fmt.Fprintln(a.Out, data["hint"])
		return 0
	case "uninstall":
		settings, err := a.loadSettings()
		if err != nil {
			return a.extendedError("statusline uninstall", opts, err)
		}
		settings.Statusline = StatuslineConfig{}
		if err := a.store.SaveSettings(settings); err != nil {
			return a.extendedError("statusline uninstall", opts, err)
		}
		return 0
	case "test":
		line := a.statuslineFromInput(map[string]any{"email": "test@example.com", "remaining_percent": 42, "plan": "Google AI Pro", "reset_at": time.Now().Add(time.Hour).UTC().Format(time.RFC3339)})
		if opts.JSON {
			return a.extendedResult("statusline test", opts, map[string]string{"text": line}, nil)
		}
		fmt.Fprintln(a.Out, line)
		return 0
	default:
		return a.extendedError("statusline", opts, fmt.Errorf("unknown subcommand %q", sub))
	}
}
