package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
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
	if err != nil && (command == "agy" || command == "agy.exe" || opts.Target == "" || opts.Target == "agy") {
		if resolved := a.resolveAgyExecutable(); resolved != "" {
			path, err = resolved, nil
		}
	}
	if err != nil {
		return a.extendedError("run now", opts, fmt.Errorf("%s is not available: %w (run 'agy-swap install-agy' to install Antigravity CLI)", command, err))
	}
	if err := a.prepareRunAccount(ctx, opts.Account, settings); err != nil {
		if errors.Is(err, context.Canceled) && !opts.JSON {
			fmt.Fprintln(a.Out, "Cancelled.")
			return 1
		}
		return a.extendedError("run now", opts, err)
	}
	if opts.Account == "" {
		if err := a.prepareBoundRun(ctx, settings, opts.Profile); err != nil {
			if errors.Is(err, context.Canceled) && !opts.JSON {
				fmt.Fprintln(a.Out, "Cancelled.")
				return 1
			}
			return a.extendedError("run now", opts, err)
		}
	}

	// Let the foreground child own SIGINT directly (instead of CommandContext
	// sending SIGKILL) while forwarding SIGTERM/cancellation cleanly and
	// restoring terminal state + harvesting refreshed session tokens afterward.
	runTerm := captureTerminalState(a.In, a.Out)
	defer func() {
		runTerm.Restore(true)
		a.resetTerminalLineReader()
	}()
	sigintGuard := make(chan os.Signal, 1)
	signal.Notify(sigintGuard, os.Interrupt)
	defer signal.Stop(sigintGuard)

	cmd := exec.Command(path, opts.RunArgs...)
	cmd.Stdin = a.In
	cmd.Stdout = a.Out
	cmd.Stderr = a.Err
	if err := cmd.Start(); err != nil {
		return a.extendedError("run now", opts, fmt.Errorf("%s failed to start: %w", command, err))
	}
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			if cmd.Process != nil {
				_ = cmd.Process.Signal(syscall.SIGTERM)
			}
		case <-done:
		}
	}()
	waitErr := cmd.Wait()
	close(done)
	a.reconcileActiveSession(context.Background())
	if waitErr != nil {
		if ctx.Err() != nil {
			return a.extendedError("run now", opts, ctx.Err())
		}
		var exitErr *exec.ExitError
		if errors.As(waitErr, &exitErr) {
			return exitErr.ExitCode()
		}
		return a.extendedError("run now", opts, fmt.Errorf("%s exited: %w", command, waitErr))
	}
	return 0
}

func resolveBinding(settings AppSettings, path string) Binding {
	path = cleanBindingPath(path)
	best := Binding{}
	bestLen := 0
	for _, binding := range settings.Bindings {
		canonical := cleanBindingPath(binding.Path)
		if canonical == "" {
			continue
		}
		rel, err := filepath.Rel(canonical, path)
		if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && len(canonical) > bestLen {
			best = binding
			bestLen = len(canonical)
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
		answer, readErr := a.readLine(ctx, "Switch to this account before running? [y/N]: ")
		if readErr != nil {
			return readErr
		}
		yes, valid := parseYesNo(answer, false)
		if !valid || !yes {
			return nil
		}
	}
	return a.prepareRunAccount(ctx, email, settings)
}
