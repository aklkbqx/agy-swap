package app

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

func withoutEnvKey(env []string, key string) []string {
	prefix := key + "="
	out := make([]string, 0, len(env))
	for _, entry := range env {
		if strings.HasPrefix(entry, prefix) {
			continue
		}
		out = append(out, entry)
	}
	return out
}

func (a *Application) newLoginCommand(ctx context.Context) *exec.Cmd {
	var cmd *exec.Cmd
	if a.loginCommand != nil {
		cmd = a.loginCommand(ctx)
	} else {
		agyPath := a.resolveAgyExecutable()
		if agyPath == "" {
			agyPath = "agy"
		}
		cmd = exec.CommandContext(ctx, agyPath)
	}
	baseEnv := cmd.Env
	if len(baseEnv) == 0 {
		baseEnv = os.Environ()
	}
	if runtime.GOOS == "darwin" {
		baseEnv = withoutEnvKey(baseEnv, "WSL_INTEROP")
	}
	baseEnv = append(baseEnv, "AGY_SWAP_SKIP_SYNC=1", "AGY_CLI_FORCE_FILE_TOKEN_STORAGE=1")
	cmd.Env = baseEnv
	return cmd
}

func (a *Application) loginWaitTimeout() time.Duration {
	if a.loginTimeout > 0 {
		return a.loginTimeout
	}
	return defaultLoginTimeout
}

func (a *Application) loginPollDelay() time.Duration {
	if a.loginPollInterval > 0 {
		return a.loginPollInterval
	}
	return defaultLoginPollInterval
}

// waitLoginCommand waits for agy to exit and gives the terminal back to
// agy-swap before anything else can use it.
func waitLoginCommand(command *exec.Cmd) error {
	err := command.Wait()
	reclaimLoginTerminal(command)
	return err
}

func stopLoginCommand(command *exec.Cmd, done <-chan error) {
	if command == nil || command.Process == nil {
		return
	}
	select {
	case waitErr := <-done:
		_ = waitErr
		return
	default:
	}
	if stopLoginProcessGroup(command) {
		if waitForLoginCommand(done, loginStopGracePeriod) {
			return
		}
		killLoginCommandTree(command)
		_ = waitForLoginCommand(done, time.Second)
		return
	}
	if err := command.Process.Signal(os.Interrupt); err == nil && waitForLoginCommand(done, loginStopGracePeriod) {
		return
	}
	killLoginCommandTree(command)
	_ = waitForLoginCommand(done, time.Second)
}

func waitForLoginCommand(done <-chan error, timeout time.Duration) bool {
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case waitErr := <-done:
		_ = waitErr
		return true
	case <-timer.C:
		return false
	}
}

func newAccount(email, name, token string) Account {
	return Account{"email": email, "name": firstString(cleanText(name), "Google User"), "token_data": token, "quota_schema": quotaSchema}
}

func (a *Application) cmdAdd(ctx context.Context, args cliArgs) int {
	stopUpdateCheck := a.startBackgroundUpdateCheck(ctx)
	defer stopUpdateCheck()
	if args.token == "-" {
		raw, err := io.ReadAll(io.LimitReader(a.In, maxTokenBytes+1))
		if err != nil || len(raw) > maxTokenBytes {
			fmt.Fprintf(a.Err, "%sToken exceeds %d bytes.%s\n", a.p.Red, maxTokenBytes, a.p.Reset)
			return 1
		}
		token := strings.TrimSpace(string(raw))
		if token == "" {
			fmt.Fprintf(a.Err, "%sEmpty token provided.%s\n", a.p.Red, a.p.Reset)
			return 1
		}
		return a.saveTokenAccount(ctx, token)
	}
	return a.addLoginFlow(ctx)
}

func (a *Application) restoreSessionBackup(ctx context.Context, fileSession, mirrorKeyring bool, backupSecure string, snapshot fileSnapshot) {
	if lock, err := acquireFileLock(a.paths.SessionLock); err == nil {
		defer func() { _ = lock.Close() }()
	}
	if !restoreFiles(snapshot) {
		fmt.Fprintln(a.Err, "Session file rollback failed; restore from your local backup")
	}
	if fileSession {
		a.credentials.ensureKeyringBypassMarker()
		if !mirrorKeyring {
			return
		}
	}
	rollbackCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	var secureOK bool
	if backupSecure != "" {
		secureOK = a.credentials.Set(rollbackCtx, backupSecure)
	} else {
		secureOK = a.credentials.Delete(rollbackCtx)
	}
	if !secureOK && !fileSession {
		fmt.Fprintln(a.Err, "Session vault rollback failed; the previous secure credential could not be restored")
	}
}

func (a *Application) addLoginFlow(ctx context.Context) int {
	// Snapshot active session under a brief lock, then release before interactive
	// prompts and browser login so other terminals are not blocked for up to 120s.
	lock, err := acquireFileLock(a.paths.SessionLock)
	if err != nil {
		return a.storeError(err)
	}
	current := a.credentials.Current(ctx)
	fileSession := a.credentials.useFileSessionStore()
	mirrorKeyring := a.credentials.shouldMirrorOSKeyring()
	backupSecure := ""
	if !fileSession || mirrorKeyring {
		backupSecure = a.credentials.Secure(ctx)
	}
	snapshot, snapErr := snapshotFiles(a.credentials.sessionFilePaths(true)...)
	_ = lock.Close()
	if snapErr != nil {
		return a.storeError(snapErr)
	}
	restore := func() {
		a.restoreSessionBackup(ctx, fileSession, mirrorKeyring, backupSecure, snapshot)
	}
	if current != "" || a.hasAnySurfaceSession() {
		accounts, err := a.store.Load(false)
		if err != nil {
			return a.storeError(err)
		}
		candidate := current
		saved := false
		if hasOAuthSession(candidate) {
			active := a.activeEmail(ctx, accounts, candidate)
			_, saved = accounts.Get(strings.ToLower(active))
		} else {
			candidate = ""
		}
		if candidate == "" || saved {
			// The CLI file is empty or holds a saved account, while the IDE may
			// be signed in to one that is not saved yet; offer that one instead.
			if unsaved := a.unsavedSurfaceSession(ctx, accounts); unsaved != "" {
				candidate, saved = unsaved, false
			}
		}
		if candidate != "" && !saved {
			label := "Unknown account"
			if info := a.http.userInfo(ctx, getString(tokenObject(decodeToken(candidate)), "access_token")); info != nil {
				email := normalizeEmail(getString(info, "email"))
				name := firstString(info["name"], "Google User")
				if email != "" {
					label = fmt.Sprintf("%s <%s>", name, email)
				}
			}
			fmt.Fprintf(a.Out, "\n%s⚠ Active session detected: %s%s%s\n%sThis account is not yet saved in agy-swap.%s\n", a.p.Yellow, a.p.Bold, label, a.p.Reset, a.p.Gray, a.p.Reset)
			for {
				answer, readErr := a.readLine(ctx, a.p.Cyan+"Save this account? [y/N] (type 'n' to login a different account): "+a.p.Reset)
				if readErr != nil {
					restore()
					fmt.Fprintf(a.Out, "%sLogin cancelled.%s\n", a.p.Yellow, a.p.Reset)
					return 1
				}
				save, valid := parseYesNo(answer, false)
				if !valid {
					fmt.Fprintln(a.Out, "Please answer y or n.")
					continue
				}
				if save {
					code, savedEmail, savedToken := a.saveTokenAccountDetail(ctx, candidate)
					if code == 0 && savedEmail != "" && savedToken != "" {
						_ = a.credentials.Apply(ctx, savedToken, savedEmail)
					}
					return code
				}
				break
			}
		}
	}
	if a.loginCommand == nil && a.resolveAgyExecutable() == "" {
		fmt.Fprintf(a.Out, "\n%s⚠ Antigravity CLI ('agy') was not found on this machine.%s\n", a.p.Yellow, a.p.Reset)
		for {
			answer, readErr := a.readLine(ctx, a.p.Cyan+"Install Antigravity CLI ('agy') now? [Y/n]: "+a.p.Reset)
			if readErr != nil {
				restore()
				fmt.Fprintf(a.Out, "%sLogin cancelled.%s\n", a.p.Yellow, a.p.Reset)
				return 1
			}
			installNow, valid := parseYesNo(answer, true)
			if !valid {
				fmt.Fprintln(a.Out, "Please answer y or n.")
				continue
			}
			if installNow {
				if code := a.cmdInstallAgy(ctx, extendedOptions{}, nil); code != 0 {
					restore()
					return code
				}
			}
			break
		}
	}
	fmt.Fprintf(a.Out, "\n%sAdd / Login Google Account%s\n%s1. A browser window will open to authenticate with Google.%s\n%s2. Complete login in Google Antigravity.%s\n", a.p.Bold, a.p.Reset, a.p.Gray, a.p.Reset, a.p.Gray, a.p.Reset)
	for {
		resp, readErr := a.readLine(ctx, a.p.Cyan+"Press Enter when ready to start login (no password is required here; 'q' to cancel)..."+a.p.Reset)
		if readErr != nil {
			restore()
			fmt.Fprintf(a.Out, "%sLogin cancelled.%s\n", a.p.Yellow, a.p.Reset)
			return 1
		}
		if resp == "" {
			break
		}
		lower := strings.ToLower(resp)
		if lower == "q" || lower == "cancel" || lower == "exit" || lower == "n" || lower == "no" {
			restore()
			fmt.Fprintln(a.Out, "Login cancelled.")
			return 0
		}
		fmt.Fprintln(a.Out, "Please press Enter to start login, or type 'q' to return.")
	}
	clearLock, clearLockErr := acquireFileLock(a.paths.SessionLock)
	if clearLockErr != nil {
		restore()
		return a.storeError(clearLockErr)
	}
	cleared := a.credentials.clearUnlocked(ctx)
	_ = clearLock.Close()
	if !cleared {
		restore()
		fmt.Fprintln(a.Err, "Could not clear the active session before login")
		return 1
	}
	finishLoginSave := func(newToken string) int {
		code, savedEmail, savedToken := a.saveTokenAccountDetail(ctx, newToken)
		if code != 0 {
			restore()
			return code
		}
		if savedAccounts, loadErr := a.store.Load(false); loadErr == nil {
			activeEmail := firstString(savedEmail, a.activeEmail(ctx, savedAccounts, savedToken))
			if _, verify := accountEligibility(savedAccounts.ByEmail[activeEmail]); verify {
				if current != "" {
					restore()
					fmt.Fprintf(a.Out, "%sKept your previous active session while %s awaits Google verification.%s\n", a.p.Gray, activeEmail, a.p.Reset)
				} else {
					fmt.Fprintf(a.Out, "%sNote: %s is the active session, and Antigravity cannot use it until verification finishes.%s\n", a.p.Yellow, activeEmail, a.p.Reset)
				}
				return code
			}
			if activeEmail != "" && savedToken != "" {
				_ = a.credentials.Apply(ctx, savedToken, activeEmail)
			}
		}
		return code
	}
	fmt.Fprintf(a.Out, "\n%sLaunching interactive 'agy' login...%s\n", a.p.Green, a.p.Reset)
	fmt.Fprintf(a.Out, "%sComplete every prompt shown here and in the browser. agy-swap will continue after the credential is saved.%s\n", a.p.Yellow, a.p.Reset)
	fmt.Fprintf(a.Out, "%sPress Ctrl+C to cancel at any time.%s\n\n", a.p.Gray, a.p.Reset)
	loginTerm := captureTerminalState(a.In, a.Out)
	restoreLoginTerm := func() {
		loginTerm.Restore(true)
		a.resetTerminalLineReader()
	}
	command := a.newLoginCommand(ctx)
	command.Stdin = a.In
	command.Stdout = a.Out
	command.Stderr = a.Err
	prepareLoginCommand(command)
	if err := command.Start(); err != nil {
		restore()
		fmt.Fprintf(a.Err, "\n%sCould not launch 'agy': %v%s\n", a.p.Red, err, a.p.Reset)
		return 1
	}
	commandDone := make(chan error, 1)
	go func() { commandDone <- waitLoginCommand(command) }()
	timeout := time.NewTimer(a.loginWaitTimeout())
	defer timeout.Stop()
	poll := time.NewTicker(a.loginPollDelay())
	defer poll.Stop()
	var token string
	for {
		select {
		case <-ctx.Done():
			stopLoginCommand(command, commandDone)
			restoreLoginTerm()
			restore()
			fmt.Fprintf(a.Out, "\n%sLogin cancelled.%s\n", a.p.Yellow, a.p.Reset)
			return 1
		case commandErr := <-commandDone:
			restoreLoginTerm()
			token = a.credentials.Current(ctx)
			if token != "" && token != current {
				return finishLoginSave(token)
			}
			restore()
			if ctx.Err() != nil {
				fmt.Fprintf(a.Out, "\n%sLogin cancelled.%s\n", a.p.Yellow, a.p.Reset)
				return 1
			}
			if commandErr != nil {
				fmt.Fprintf(a.Err, "\n%s'agy' exited before saving a login credential: %v%s\n", a.p.Red, commandErr, a.p.Reset)
			} else {
				fmt.Fprintf(a.Err, "\n%s'agy' exited before saving a login credential.%s\n", a.p.Red, a.p.Reset)
			}
			return 1
		case <-poll.C:
			token = a.credentials.Current(ctx)
			if token != "" && token != current {
				stopLoginCommand(command, commandDone)
				restoreLoginTerm()
				return finishLoginSave(token)
			}
		case <-timeout.C:
			stopLoginCommand(command, commandDone)
			restoreLoginTerm()
			restore()
			fmt.Fprintf(a.Out, "\n%sTimed out waiting for login (120s).%s\n", a.p.Red, a.p.Reset)
			if current != "" {
				fmt.Fprintf(a.Out, "%sAttempted to restore the previous session; any rollback errors are reported above.%s\n", a.p.Gray, a.p.Reset)
			}
			return 1
		}
	}
}

func (a *Application) saveTokenAccount(ctx context.Context, token string) int {
	code, _, _ := a.saveTokenAccountDetail(ctx, token)
	return code
}

func (a *Application) saveTokenAccountDetail(ctx context.Context, token string) (int, string, string) {
	inner := tokenObject(decodeToken(token))
	if inner == nil || getString(inner, "access_token") == "" {
		fmt.Fprintf(a.Out, "\n%sInvalid token structure.%s\n", a.p.Red, a.p.Reset)
		return 1, "", ""
	}
	stop := a.spinner("Verifying Google account...")
	access, refreshed, err := a.http.accessTokenData(ctx, token)
	if err != nil {
		stop()
		fmt.Fprintln(a.Err, "Could not refresh credential:", err)
		return 1, "", ""
	}
	info := a.http.userInfo(ctx, access)
	stop()
	if info == nil || !verifiedUserInfo(info) {
		fmt.Fprintln(a.Err, "Google userinfo could not verify the account; retry when Google is reachable.")
		return 1, "", ""
	}
	token = refreshed
	email, name := normalizeEmail(getString(info, "email")), cleanText(firstString(info["name"], "Google User"))
	if email == "" {
		fmt.Fprintln(a.Err, "Google returned no valid account email")
		return 1, "", ""
	}
	if !tokenMatchesEmail(token, email) {
		fmt.Fprintf(a.Err, "%sToken identity does not match the selected Google account.%s\n", a.p.Red, a.p.Reset)
		return 1, "", ""
	}
	accounts, err := a.store.Load(false)
	if err != nil {
		return a.storeError(err), "", ""
	}
	account, exists := accounts.Get(email)
	if !exists {
		account = newAccount(email, name, token)
	} else {
		account["name"] = name
		account["token_data"] = token
	}
	oldRef, saved := a.saveAccountSecret(ctx, account, token)
	if !saved {
		fmt.Fprintln(a.Err, "Vault unavailable; token stored in the private accounts file.")
	}
	accounts.Set(email, account)
	if err := a.store.Save(accounts); err != nil {
		return a.storeError(err), "", ""
	}
	if saved {
		a.deleteReplacedSecrets(ctx, []string{oldRef})
	}
	fmt.Fprintf(a.Out, "\n%s✓ Successfully saved account: %s <%s>%s [%s]\n", a.p.Green, name, email, a.p.Reset, tierBadge(accounts.ByEmail[email], a.p))
	_, _ = a.checkNewAccount(ctx, email)
	return 0, email, token
}
