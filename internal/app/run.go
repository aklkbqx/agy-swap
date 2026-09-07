package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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
	if !a.applyAccount(ctx, token, email) {
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
	if opts.Account == "" {
		if err := a.prepareBoundRun(ctx, settings, opts.Profile); err != nil {
			return a.extendedError("run now", opts, err)
		}
	}

	cmd := exec.CommandContext(ctx, path, opts.RunArgs...)
	cmd.Stdin = a.In
	cmd.Stdout = a.Out
	cmd.Stderr = a.Err
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return a.extendedError("run now", opts, ctx.Err())
		}
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return exitErr.ExitCode()
		}
		return a.extendedError("run now", opts, fmt.Errorf("%s exited: %w", command, err))
	}
	return 0
}

func resolveBinding(settings AppSettings, path string) Binding {
	path = cleanBindingPath(path)
	best := Binding{}
	for _, binding := range settings.Bindings {
		rel, err := filepath.Rel(binding.Path, path)
		if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && len(binding.Path) > len(best.Path) {
			best = binding
		}
	}
	return best
}

// Directory bindings are evaluated by run now. Merely listing or inspecting
// accounts never changes the session. An explicit --account takes precedence.
func (a *Application) prepareBoundRun(ctx context.Context, settings AppSettings, profileName string) error {
	path, err := os.Getwd()
	if err != nil {
		return err
	}
	binding := resolveBinding(settings, path)
	if profileName != "" {
		binding = Binding{Profile: profileName, Mode: "prompt"}
	}
	if binding.Profile == "" || binding.Mode == "disabled" {
		return nil
	}
	if _, ok := settings.Profiles[binding.Profile]; !ok {
		return errors.New("bound profile not found")
	}
	accounts, err := a.store.Load(true)
	if err != nil {
		return err
	}
	failures := a.quota.Refresh(ctx, accounts, true, nil)
	if failure := failures["store"]; failure != "" {
		return errors.New(failure)
	}
	for email := range failures {
		delete(accounts.ByEmail[email], "quota_snapshot")
	}
	choices := a.buildRecommendations(ctx, accounts, settings, binding.Profile, "", "")
	if len(choices) == 0 || !choices[0].Ready {
		return errors.New("bound profile has no eligible account with fresh quota")
	}
	email := choices[0].Email
	fmt.Fprintf(a.Out, "Project profile %s recommends %s.\n", binding.Profile, email)
	if binding.Mode == "recommend" {
		return nil
	}
	if binding.Mode == "auto" {
		if !settings.Policy.AllowApply {
			return errors.New("automatic binding requires policy.allow_apply=true")
		}
	} else {
		if !a.stdinTTY {
			return errors.New("binding needs confirmation; use --account explicitly or configure auto mode")
		}
		yes, valid := parseYesNo(a.readLine("Switch to this account before running? [y/N]: "), false)
		if !valid || !yes {
			return nil
		}
	}
	return a.prepareRunAccount(ctx, email, settings)
}
