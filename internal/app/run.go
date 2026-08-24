package app

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

var builtinRunTargets = map[string]string{
	"agy":    "agy",
	"gemini": "gemini",
	"claude": "claude",
	"gpt":    "gpt",
}

func resolveRunTarget(settings AppSettings, name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "agy"
	}
	if target, ok := settings.Targets[name]; ok {
		if !target.Enabled {
			return "", fmt.Errorf("target %q is disabled", name)
		}
		command := strings.TrimSpace(target.Command)
		if command == "" {
			return "", fmt.Errorf("target %q has no executable command", name)
		}
		return command, nil
	}
	if command, ok := builtinRunTargets[name]; ok {
		return command, nil
	}
	return "", fmt.Errorf("unknown target %q; configure it with 'agy-swap target set'", name)
}

func (a *Application) prepareRunAccount(ctx context.Context, target string, settings AppSettings) error {
	if strings.TrimSpace(target) == "" {
		return nil
	}
	accounts, err := a.store.Load(true)
	if err != nil {
		return err
	}
	email, err := resolveConfiguredTarget(target, accounts, settings)
	if err != nil {
		return err
	}
	if email == "" {
		return errors.New("account not found")
	}
	account := accounts.ByEmail[email]
	token, err := a.accountToken(ctx, account)
	if err != nil {
		return fmt.Errorf("read account credential: %w", err)
	}
	if a.credentials.Current(ctx) == token {
		return nil
	}
	if !a.credentials.Apply(ctx, token, email) {
		return fmt.Errorf("failed to switch to %s", email)
	}
	fmt.Fprintf(a.Out, "✓ Switched to %s before running.\n", email)
	return nil
}

func (a *Application) cmdRunNow(ctx context.Context, opts extendedOptions, positional []string) int {
	if len(positional) == 0 || positional[0] != "now" {
		return a.extendedError("run", opts, errors.New("usage: run now [--account ACCOUNT] [--target NAME] [-- AGY_ARGS...]"))
	}
	if len(positional) > 1 {
		return a.extendedError("run", opts, errors.New("arguments for agy must follow --"))
	}
	settings, err := a.loadSettings()
	if err != nil {
		return a.extendedError("run now", opts, err)
	}
	command, err := resolveRunTarget(settings, opts.Target)
	if err != nil {
		return a.extendedError("run now", opts, err)
	}
	path, err := exec.LookPath(command)
	if err != nil {
		return a.extendedError("run now", opts, fmt.Errorf("%s is not available: %w", command, err))
	}
	if err := a.prepareRunAccount(ctx, opts.Account, settings); err != nil {
		return a.extendedError("run now", opts, err)
	}

	cmd := exec.CommandContext(ctx, path, opts.RunArgs...)
	cmd.Stdin = a.In
	cmd.Stdout = a.Out
	cmd.Stderr = a.Err
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return a.extendedError("run now", opts, ctx.Err())
		}
		return a.extendedError("run now", opts, fmt.Errorf("%s exited: %w", command, err))
	}
	return 0
}
