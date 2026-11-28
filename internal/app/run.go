package app

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

var (
	autoSwapNow = func() time.Time { return time.Now().UTC() }

	autoSwapWaitSleep = func(ctx context.Context, d time.Duration) {
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
)

type runChildOutcome struct {
	Trigger string // "", "quota", "auth"
	Family  string
	WaitErr error
}

func isRealAgyBinary(execPath string) bool {
	base := strings.ToLower(filepath.Base(strings.TrimSpace(execPath)))
	return base == "agy" || base == "agy.exe" || base == ".agy.real"
}

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
	if len(positional) > 0 && positional[0] == "now" {
		positional = positional[1:]
	}
	if len(positional) > 0 {
		return a.extendedError("run", opts, errors.New("usage: run [now] [--account ACCOUNT] [--target NAME] [--auto-swap] [--handoff clean-summary|resume] [-- AGY_ARGS...]"))
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

	autoSwapActive := opts.AutoSwap || settings.AutoSwap.Enabled
	handoffMode := opts.Handoff
	if handoffMode == "" {
		handoffMode = settings.AutoSwap.HandoffMode
	}
	if handoffMode == "" {
		handoffMode = "clean-summary"
	}

	// For supervised agy runs, unwrap any agy-swap session-guard shell wrapper so
	// we spawn the real binary directly with AGY_SWAP_SKIP_SYNC=1 and
	// AGY_CLI_FORCE_FILE_TOKEN_STORAGE=1, never injecting --dangerously-skip-permissions.
	isAgyTarget := opts.Target == "" || opts.Target == "agy" || command == "agy" || command == "agy.exe"
	execPath := path
	if autoSwapActive && isAgyTarget {
		execPath = unwrapManagedAgyBinary(a.paths.Home, path)
	}

	// Let the foreground child own SIGINT directly while forwarding cancellation
	// cleanly via cross-OS process group / Job Object draining and restoring
	// terminal state + harvesting refreshed session tokens afterward.
	runTerm := captureTerminalState(a.In, a.Out)
	defer func() {
		runTerm.Restore(true)
		a.resetTerminalLineReader()
	}()
	sigintGuard := make(chan os.Signal, 1)
	signal.Notify(sigintGuard, os.Interrupt)
	defer signal.Stop(sigintGuard)

	currentArgs := append([]string(nil), opts.RunArgs...)
	interactiveTTY := a.stdinTTY && a.stdoutTTY
	cwd, _ := os.Getwd()

	for attempt := 0; ; attempt++ {
		launchTime := autoSwapNow()
		var sessionEmail string
		if autoSwapActive {
			if accsBefore, loadErr := a.store.Load(false); loadErr == nil {
				sessionEmail = a.activeEmail(ctx, accsBefore, a.credentials.Current(ctx))
			}
		}

		childArgs := append([]string(nil), currentArgs...)
		sessionLogPath := ""
		createdSessionLog := false
		if autoSwapActive && interactiveTTY && isAgyTarget {
			if explicitLog, hasExplicit := hasLogFileFlag(childArgs); hasExplicit && explicitLog != "" {
				sessionLogPath = explicitLog
			} else if isRealAgyBinary(execPath) && strings.TrimSpace(a.paths.Home) != "" {
				logDir := filepath.Join(a.paths.Home, ".gemini", "antigravity-cli", "log")
				if os.MkdirAll(logDir, 0o700) == nil {
					sessionLogPath = filepath.Join(logDir, fmt.Sprintf("cli-swap-%d-%d-%d.log", os.Getpid(), attempt, launchTime.UnixNano()))
					childArgs = append([]string{"--log-file", sessionLogPath}, childArgs...)
					createdSessionLog = true
				}
			}
		}

		childOutcome := a.runManagedChild(ctx, execPath, childArgs, autoSwapActive, launchTime, sessionEmail, sessionLogPath)
		if createdSessionLog && sessionLogPath != "" {
			if info, statErr := os.Stat(sessionLogPath); statErr == nil && info.Size() == 0 {
				_ = os.Remove(sessionLogPath)
			}
		}
		a.reconcileActiveSession(context.Background())

		if !autoSwapActive || childOutcome.Trigger == "" || ctx.Err() != nil {
			return a.finishRunExit(ctx, command, opts, childOutcome.WaitErr)
		}

		accounts, loadErr := a.store.Load(true)
		if loadErr != nil {
			return a.finishRunExit(ctx, command, opts, childOutcome.WaitErr)
		}
		if sessionEmail == "" {
			sessionEmail = a.activeEmail(ctx, accounts, a.credentials.Current(ctx))
		}

		// Handle expired OAuth/session errors by attempting an in-place refresh first.
		refreshedSameAccount := false
		if childOutcome.Trigger == "auth" && sessionEmail != "" && a.quota != nil {
			failures := a.quota.RefreshEmails(ctx, accounts, []string{sessionEmail})
			if failures[sessionEmail] == "" {
				if reloaded, rErr := a.store.Load(false); rErr == nil {
					if acc := reloaded.ByEmail[sessionEmail]; acc != nil {
						if tok, tErr := a.accountToken(ctx, acc); tErr == nil && tok != "" && a.applyAccountWithMode(ctx, tok, sessionEmail, "manual") {
							refreshedSameAccount = true
						}
					}
				}
			}
		}

		var targetEmail string
		var adopted bool
		if refreshedSameAccount {
			targetEmail = sessionEmail
		} else {
			targetFamily := firstString(opts.Family, childOutcome.Family)
			outcome := a.resolveOrAttemptAutoSwap(ctx, sessionEmail, opts.Profile, targetFamily, autoSwapNow())
			for !outcome.Swapped && outcome.WaitSeconds > 0 && outcome.WaitSeconds <= 1800 &&
				(outcome.Status == "dwell_wait" || outcome.Status == "hourly_budget_exceeded" || outcome.Status == "no_candidate") {
				waitDur := time.Duration(outcome.WaitSeconds) * time.Second
				fmt.Fprintf(a.Err, "agy-swap: waiting %s (%s) before auto-swapping... (Ctrl+C to stop)\n", formatDuration(waitDur.Seconds()), outcome.Status)
				remaining := waitDur
				adoptedEarly := false
				for remaining > 0 {
					slice := min(remaining, 5*time.Second)
					autoSwapWaitSleep(ctx, slice)
					if ctx.Err() != nil {
						return a.finishRunExit(ctx, command, opts, childOutcome.WaitErr)
					}
					remaining -= slice
					if accsCheck, errCheck := a.store.Load(false); errCheck == nil {
						diskNow := a.activeEmail(ctx, accsCheck, a.credentials.Current(ctx))
						if sessionEmail != "" && diskNow != "" && !strings.EqualFold(diskNow, sessionEmail) {
							adoptedEarly = true
							break
						}
					}
				}
				outcome = a.resolveOrAttemptAutoSwap(ctx, sessionEmail, opts.Profile, targetFamily, autoSwapNow())
				if adoptedEarly && outcome.Swapped {
					break
				}
			}
			if !outcome.Swapped {
				fmt.Fprintf(a.Err, "agy-swap: auto-swap skipped (%s): %s\n", outcome.Status, outcome.Reason)
				return a.finishRunExit(ctx, command, opts, childOutcome.WaitErr)
			}
			targetEmail = outcome.TargetEmail
			adopted = outcome.Adopted
		}

		convID := extractConversationArg(currentArgs)
		if convID == "" {
			convID = findLatestConversationID(a.paths.Home, cwd)
		}
		var handoffFile string
		if handoffMode == "resume" {
			fmt.Fprintln(a.Err, "agy-swap: warning: --handoff=resume reuses conversation trajectory across accounts.")
		} else {
			brief, briefErr := buildCleanHandoffMarkdown(a.paths.Home, convID, cwd, defaultHandoffMaxBytes)
			if briefErr == nil {
				if written, writeErr := writeHandoffBrief(a.paths.Home, cwd, brief); writeErr == nil {
					handoffFile = written
				}
			}
		}
		switch {
		case refreshedSameAccount:
			fmt.Fprintf(a.Err, "agy-swap: refreshed session for %s; relaunching (%s)...\n", targetEmail, handoffMode)
		case adopted:
			fmt.Fprintf(a.Err, "agy-swap: adopted %s; relaunching session (%s)...\n", targetEmail, handoffMode)
		default:
			fmt.Fprintf(a.Err, "agy-swap: switched to %s; relaunching session (%s)...\n", targetEmail, handoffMode)
		}
		currentArgs = buildHandoffRelaunchArgs(opts.RunArgs, handoffMode, handoffFile, convID)
	}
}

func (a *Application) finishRunExit(ctx context.Context, command string, opts extendedOptions, waitErr error) int {
	if waitErr == nil {
		return 0
	}
	if ctx.Err() != nil {
		return a.extendedError("run now", opts, ctx.Err())
	}
	var exitErr *exec.ExitError
	if errors.As(waitErr, &exitErr) {
		return exitErr.ExitCode()
	}
	return a.extendedError("run now", opts, fmt.Errorf("%s exited: %w", command, waitErr))
}

func (a *Application) runManagedChild(ctx context.Context, execPath string, args []string, watchQuota bool, launchTime time.Time, sessionEmail, sessionLogPath string) runChildOutcome {
	cmd := exec.Command(execPath, args...)
	if watchQuota {
		cmd.Env = buildSupervisedAgyEnv(os.Environ(), runtime.GOOS)
	}
	interactiveTTY := a.stdinTTY && a.stdoutTTY
	handle := configureManagedChild(cmd, interactiveTTY)
	defer handle.close()

	var triggerMu sync.Mutex
	var triggerKind, triggerFamily string
	setTrigger := func(kind, family string) bool {
		triggerMu.Lock()
		defer triggerMu.Unlock()
		if triggerKind == "" {
			triggerKind = kind
			triggerFamily = family
			return true
		}
		if triggerFamily == "" && family != "" {
			triggerFamily = family
		}
		return false
	}

	var wg sync.WaitGroup
	var outPipe, errPipe io.ReadCloser
	var pipeErr error

	cmd.Stdin = a.In
	if watchQuota && !interactiveTTY {
		outPipe, pipeErr = cmd.StdoutPipe()
		if pipeErr == nil {
			errPipe, pipeErr = cmd.StderrPipe()
		}
		if pipeErr != nil {
			cmd.Stdout = a.Out
			cmd.Stderr = a.Err
			outPipe, errPipe = nil, nil
		}
	} else {
		cmd.Stdout = a.Out
		cmd.Stderr = a.Err
	}

	if err := cmd.Start(); err != nil {
		return runChildOutcome{WaitErr: err}
	}
	handle.attachStarted(cmd)

	teeScan := func(r io.ReadCloser, dst io.Writer) {
		defer wg.Done()
		scanner := bufio.NewScanner(r)
		scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
		for scanner.Scan() {
			line := scanner.Text()
			if !strings.Contains(line, "llm_monitor.go") && !strings.Contains(line, "runner.go") {
				if isQuotaExhaustedOutputLine(line) {
					setTrigger("quota", inferExhaustedFamilyFromLine(line))
				} else if isAuthExpiredOutputLine(line) {
					setTrigger("auth", "")
				}
			}
			_, _ = io.WriteString(dst, line+"\n")
		}
	}
	if outPipe != nil {
		wg.Add(1)
		go teeScan(outPipe, a.Out)
	}
	if errPipe != nil {
		wg.Add(1)
		go teeScan(errPipe, a.Err)
	}

	waitCh := make(chan error, 1)
	go func() {
		wg.Wait()
		waitCh <- cmd.Wait()
	}()

	// For interactive TTY sessions where we do not intercept stdout/stderr pipes,
	// tail the child's dedicated --log-file (fast 300ms reaction) or fall back
	// to scanning Antigravity logs scoped to sessionEmail.
	var logStop chan struct{}
	if watchQuota && interactiveTTY {
		logStop = make(chan struct{})
		go func() {
			if sessionLogPath != "" {
				a.watchChildLogFile(ctx, logStop, cmd, handle, sessionLogPath, launchTime, setTrigger)
				return
			}
			ticker := time.NewTicker(500 * time.Millisecond)
			defer ticker.Stop()
			scanner := NewLogScanner(a.paths)
			for {
				select {
				case <-logStop:
					return
				case <-ctx.Done():
					return
				case <-ticker.C:
					if hit, fam := detectFreshQuotaExhaustion(scanner, sessionEmail, launchTime); hit {
						setTrigger("quota", fam)
						handle.signalGraceful(cmd)
						return
					}
				}
			}
		}()
	}

	var waitErr error
	select {
	case waitErr = <-waitCh:
	case <-ctx.Done():
		waitErr = drainManagedChild(cmd, handle, waitCh, 5*time.Second)
	}
	if logStop != nil {
		close(logStop)
	}
	triggerMu.Lock()
	kind, fam := triggerKind, triggerFamily
	triggerMu.Unlock()
	return runChildOutcome{Trigger: kind, Family: fam, WaitErr: waitErr}
}

func (a *Application) watchChildLogFile(
	ctx context.Context,
	stopCh <-chan struct{},
	cmd *exec.Cmd,
	handle *managedChildHandle,
	logPath string,
	launchTime time.Time,
	setTrigger func(kind, family string) bool,
) {
	ticker := time.NewTicker(300 * time.Millisecond)
	defer ticker.Stop()

	var offset int64
	var partial string
	seenAuthLoaded := false
	currentLabelFamily := ""

	sleepSettle := func() {
		timer := time.NewTimer(350 * time.Millisecond)
		defer timer.Stop()
		select {
		case <-stopCh:
		case <-ctx.Done():
		case <-timer.C:
		}
	}

	for {
		select {
		case <-stopCh:
			return
		case <-ctx.Done():
			return
		case <-ticker.C:
			f, err := os.Open(logPath)
			if err != nil {
				continue
			}
			info, statErr := f.Stat()
			if statErr != nil || info.Size() <= offset {
				_ = f.Close()
				continue
			}
			if _, seekErr := f.Seek(offset, io.SeekStart); seekErr != nil {
				_ = f.Close()
				continue
			}
			chunk, readErr := io.ReadAll(io.LimitReader(f, 2*1024*1024))
			_ = f.Close()
			if readErr != nil || len(chunk) == 0 {
				continue
			}
			offset += int64(len(chunk))
			text := partial + string(chunk)
			lines := strings.Split(text, "\n")
			if !bytes.HasSuffix(chunk, []byte("\n")) {
				partial = lines[len(lines)-1]
				lines = lines[:len(lines)-1]
			} else {
				partial = ""
			}
			for _, line := range lines {
				if strings.Contains(line, "keyringAuth: loaded token") || strings.Contains(line, "applyAuthResult:") {
					seenAuthLoaded = true
				}
				if containsASCIIFold(line, "label") && strings.Contains(line, "=") {
					if m, ok := detectModel(labelPattern, line); ok {
						currentLabelFamily = m.Family
					}
				} else if containsASCIIFold(line, "resolving model") {
					if m, ok := detectModel(resolvedPattern, line); ok {
						currentLabelFamily = m.Family
					}
				}
				if !isRealAgyLogErrorLine(line) {
					continue
				}
				if isQuotaExhaustedOutputLine(line) {
					fam := firstString(inferExhaustedFamilyFromLine(line), currentLabelFamily)
					setTrigger("quota", fam)
					sleepSettle()
					handle.signalGraceful(cmd)
					return
				}
				if (seenAuthLoaded || time.Since(launchTime) > 5*time.Second) && isAuthExpiredOutputLine(line) {
					setTrigger("auth", "")
					sleepSettle()
					handle.signalGraceful(cmd)
					return
				}
			}
		}
	}
}

func detectFreshQuotaExhaustion(scanner *LogScanner, sessionEmail string, since time.Time) (bool, string) {
	if scanner == nil {
		return false, ""
	}
	_, evidence, _ := scanner.Scan()
	for email, byFamily := range evidence {
		if sessionEmail != "" && !strings.EqualFold(email, sessionEmail) {
			continue
		}
		for _, rec := range byFamily {
			if rec.State != "limited" {
				continue
			}
			if obs, err := parseUTC(rec.ObservedAt); err == nil && !obs.Before(since) {
				return true, rec.Family
			}
		}
	}
	return false, ""
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
	targets := scopedEligibleEmails(accounts, settings, binding.Profile, "")
	var failures map[string]string
	if len(targets) > 0 {
		failures = a.quota.RefreshEmails(ctx, accounts, targets)
	}
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
