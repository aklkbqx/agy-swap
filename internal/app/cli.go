package app

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/term"
)

type Application struct {
	Version             string
	BuildID             string
	In                  io.Reader
	Out, Err            io.Writer
	lineReader          *bufio.Reader
	lineReaderMu        sync.Mutex
	paths               Paths
	store               *Store
	credentials         *Credentials
	vault               AccountVault
	http                *HTTPService
	quota               *QuotaService
	color               bool
	p                   palette
	stdinTTY, stdoutTTY bool
	loginCommand        func(context.Context) *exec.Cmd
	loginTimeout        time.Duration
	loginPollInterval   time.Duration
}

const (
	defaultLoginTimeout      = 120 * time.Second
	defaultLoginPollInterval = 1500 * time.Millisecond
	loginStopGracePeriod     = 2 * time.Second
)

func New(version string, in io.Reader, out, errOut io.Writer) (*Application, error) {
	paths, err := defaultPaths()
	if err != nil {
		return nil, err
	}
	store := NewStore(paths)
	httpService := NewHTTPService(errOut)
	stdoutTTY := writerTerminal(out)
	var lineReader *bufio.Reader
	if in != nil {
		lineReader = bufio.NewReader(in)
	}
	app := &Application{Version: version, BuildID: "unknown", In: in, Out: out, Err: errOut, lineReader: lineReader, paths: paths, store: store, credentials: NewCredentials(paths), vault: NewAccountVault(), http: httpService, quota: NewQuotaService(httpService, store), stdinTTY: readerTerminal(in), stdoutTTY: stdoutTTY, color: stdoutTTY && os.Getenv("NO_COLOR") == ""}
	app.quota.SetVault(app.vault)
	app.p = makePalette(app.color)
	return app, nil
}
func readerTerminal(reader io.Reader) bool {
	file, ok := reader.(*os.File)
	return ok && term.IsTerminal(int(file.Fd()))
}
func writerTerminal(writer io.Writer) bool {
	file, ok := writer.(*os.File)
	return ok && term.IsTerminal(int(file.Fd()))
}

func (a *Application) newLoginCommand(ctx context.Context) *exec.Cmd {
	if a.loginCommand != nil {
		return a.loginCommand(ctx)
	}
	return exec.CommandContext(ctx, "agy")
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

func stopLoginCommand(command *exec.Cmd, done <-chan error) {
	if command == nil || command.Process == nil {
		return
	}
	select {
	case <-done:
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
	case <-done:
		return true
	case <-timer.C:
		return false
	}
}

type cliArgs struct {
	command, account, token, family, duration, group                            string
	verbose, refresh, force, help, version                                      bool
	legacyAdd, legacyList, legacyNext, legacySwitch, legacyStatus, legacyLogout bool
	legacyRemove, stringSwitch                                                  string
	updateCheck                                                                 bool
}

func (a *Application) Run(ctx context.Context, argv []string) int {
	if len(argv) > 0 && argv[0] == "__update-finalize" {
		return a.runUpdateFinalizer(ctx, argv[1:])
	}
	if isExtendedCommand(argv) {
		return a.runExtended(ctx, argv)
	}
	args, err := parseCLI(argv)
	if err != nil {
		fmt.Fprintf(a.Err, "agy-swap: error: %v\n", err)
		a.printUsage(a.Err)
		return 2
	}
	if args.command == "version" {
		args.version = true
	}
	if args.version {
		build := strings.TrimSpace(a.BuildID)
		if build == "" || build == "unknown" {
			fmt.Fprintf(a.Out, "agy-swap v%s\n", a.Version)
		} else {
			fmt.Fprintf(a.Out, "agy-swap v%s (%s)\n", a.Version, build)
		}
		return 0
	}
	if args.help {
		a.printUsage(a.Out)
		return 0
	}
	command := args.command
	if args.legacyAdd || args.token != "" {
		command = "add"
	} else if args.legacyList {
		command = "list"
	} else if args.legacyLogout {
		command = "logout"
	} else if args.legacyNext {
		command = "next"
	} else if args.stringSwitch != "" {
		command = "switch"
		args.account = args.stringSwitch
	} else if args.legacySwitch {
		command = "switch"
	} else if args.legacyRemove != "" {
		command = "remove"
		args.account = args.legacyRemove
	} else if args.legacyStatus {
		command = "status"
	}
	if command == "" {
		if a.stdinTTY && a.stdoutTTY {
			return a.cmdInteractive(ctx)
		}
		command = "list"
	}
	var code int
	switch command {
	case "add":
		code = a.cmdAdd(ctx, args)
	case "list":
		code = a.cmdList(ctx, args)
	case "limits":
		code = a.cmdLimits(ctx, args)
	case "limit":
		code = a.cmdSetLimit(ctx, args)
	case "logout":
		code = a.cmdLogout(ctx)
	case "next":
		code = a.cmdNext(ctx, args)
	case "switch":
		code = a.cmdSwitch(ctx, args)
	case "remove":
		code = a.cmdRemove(ctx, args)
	case "status":
		code = a.cmdStatus(ctx)
	case "update":
		code = a.cmdUpdate(ctx, args)
	default:
		fmt.Fprintf(a.Err, "agy-swap: error: unknown command %q\n", command)
		return 2
	}
	return code
}

func (a *Application) printUsage(w io.Writer) {
	fmt.Fprintln(w, `usage: agy-swap [-h] [-v] [--add] [--token -] [--list] [--next]
                [--family {claude,gemini,gpt}] [--switch-to ACCOUNT]
                [--switch] [--remove ACCOUNT] [--status] [--logout]
                [--verbose]
                {add,list,limits,logout,next,switch,remove,status,update,version,limit,
                 doctor,config,alias,tag,profile,bind,recommend,statusline,watch,
                 history,stats,forecast,backup,metrics,completion,account,target,run} ...

Minimal Multi-Account Switcher for Google Antigravity CLI (agy)

commands:
  add       Log in & add a Google account
  list      List all saved accounts
  limits    Show account quota limits
  logout    Logout of active session
  next      Rotate to an account with available quota
  switch    Switch to an account
  remove    Remove a saved account
  status    Show active account details
  update    Update agy-swap to the latest version
  version   Show the installed version and build provenance
  limit     Manage a manual quota cooldown`)
	fmt.Fprintln(w, `
extended commands:
  doctor             Check storage, credentials, vault, and platform readiness
  config             Manage versioned configuration (show/set/reset)
  alias/tag          Create account aliases and searchable tags
  profile/bind        Define project profiles and directory bindings
  recommend          Explain the safest account choice; --apply is opt-in
  statusline          Render or install a JSON-driven statusline
  watch/history       Poll quota, notify on thresholds, and inspect bounded history
  stats/forecast      Summarize history and show reset forecasts
  backup/metrics      Export/import backups and expose local Prometheus metrics
  target              Check experimental CLI/IDE target adapters
  run now             Launch a configured CLI target immediately
  completion          Print bash, zsh, fish, or PowerShell completion

Most extended commands support --json. Secret backup passphrases can be read
from stdin with --passphrase-stdin.`)
}

func parseCLI(argv []string) (cliArgs, error) {
	var result cliArgs
	result.group = "claude"
	if len(argv) == 0 {
		return result, nil
	}
	known := map[string]bool{"add": true, "list": true, "limits": true, "logout": true, "next": true, "switch": true, "remove": true, "status": true, "update": true, "version": true, "limit": true}
	i := 0
	if known[argv[0]] {
		result.command = argv[0]
		i = 1
		if result.command == "limit" {
			if i >= len(argv) || argv[i] != "set" {
				return result, fmt.Errorf("limit requires the 'set' subcommand")
			}
			i++
		}
	}
	for i < len(argv) {
		arg := argv[i]
		take := func() (string, error) {
			i++
			if i >= len(argv) {
				return "", fmt.Errorf("argument %s requires a value", arg)
			}
			return argv[i], nil
		}
		switch arg {
		case "-h", "--help":
			result.help = true
		case "-v", "--version":
			result.version = true
		case "--verbose":
			result.verbose = true
		case "--refresh":
			result.refresh = true
		case "--force":
			result.force = true
		case "--add":
			result.legacyAdd = true
		case "--list":
			result.legacyList = true
		case "--next", "--auto-rotate":
			result.legacyNext = true
		case "--switch":
			result.legacySwitch = true
		case "--status":
			result.legacyStatus = true
		case "--logout":
			result.legacyLogout = true
		case "--token":
			value, err := take()
			if err != nil {
				return result, err
			}
			if value != "-" {
				return result, fmt.Errorf("invalid choice for --token: %q (choose from '-')", value)
			}
			result.token = value
		case "--family":
			value, err := take()
			if err != nil {
				return result, err
			}
			if !oneOf(value, "claude", "gemini", "gpt") {
				return result, fmt.Errorf("invalid family %q", value)
			}
			result.family = value
		case "--group":
			value, err := take()
			if err != nil {
				return result, err
			}
			if !oneOf(value, "claude", "gemini", "gpt") {
				return result, fmt.Errorf("invalid group %q", value)
			}
			result.group = value
		case "--switch-to":
			value, err := take()
			if err != nil {
				return result, err
			}
			result.stringSwitch = value
		case "--remove":
			value, err := take()
			if err != nil {
				return result, err
			}
			result.legacyRemove = value
		default:
			if strings.HasPrefix(arg, "-") {
				return result, fmt.Errorf("unrecognized argument: %s", arg)
			}
			switch result.command {
			case "update":
				if arg != "check" || result.updateCheck {
					return result, fmt.Errorf("unexpected argument: %s", arg)
				}
				result.updateCheck = true
			case "switch", "remove":
				if result.account != "" {
					return result, fmt.Errorf("too many arguments")
				}
				result.account = arg
			case "limit":
				if result.account == "" {
					result.account = arg
				} else if result.duration == "" {
					result.duration = arg
				} else {
					return result, fmt.Errorf("too many arguments")
				}
			default:
				return result, fmt.Errorf("unexpected argument: %s", arg)
			}
		}
		i++
	}
	if result.command == "limit" && (result.account == "" || result.duration == "") {
		return result, fmt.Errorf("limit set requires ACCOUNT and DURATION")
	}
	return result, nil
}

func (a *Application) spinner(message string) func() {
	if !a.stdoutTTY {
		return func() {}
	}
	done := make(chan struct{})
	var once sync.Once
	var worker sync.WaitGroup
	worker.Add(1)
	go func() {
		defer worker.Done()
		frames := []string{"·  ", "•  ", "●  ", "•  "}
		ticker := time.NewTicker(120 * time.Millisecond)
		defer ticker.Stop()
		i := 0
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				fmt.Fprintf(a.Out, "\r%s%s%s %s", a.p.Orange, frames[i%len(frames)], a.p.Reset, message)
				i++
			}
		}
	}()
	return func() {
		once.Do(func() {
			close(done)
			worker.Wait()
			fmt.Fprint(a.Out, "\r\x1b[K")
		})
	}
}

func newAccount(email, name, token string) Account {
	return Account{"email": email, "name": firstString(cleanText(name), "Google User"), "token_data": token, "quota_schema": quotaSchema}
}

func (a *Application) readLine(prompt string) string {
	fmt.Fprint(a.Out, prompt)
	a.lineReaderMu.Lock()
	defer a.lineReaderMu.Unlock()
	if a.In == nil {
		return ""
	}
	if a.lineReader == nil {
		a.lineReader = bufio.NewReader(a.In)
	}
	line, _ := a.lineReader.ReadString('\n')
	return strings.TrimSpace(line)
}

func parseYesNo(input string, defaultYes bool) (bool, bool) {
	switch strings.ToLower(strings.TrimSpace(input)) {
	case "":
		return defaultYes, true
	case "y", "yes":
		return true, true
	case "n", "no":
		return false, true
	default:
		return false, false
	}
}

func (a *Application) cmdAdd(ctx context.Context, args cliArgs) int {
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
		decoded := decodeToken(token)
		inner := tokenObject(decoded)
		if inner == nil || getString(inner, "access_token") == "" {
			fmt.Fprintf(a.Err, "%sCould not parse token.%s\n", a.p.Red, a.p.Reset)
			return 1
		}
		stop := a.spinner("Fetching Google profile & avatar...")
		userinfo := a.http.userInfo(ctx, getString(inner, "access_token"))
		stop()
		email, name := "", "Google User"
		if userinfo != nil {
			email = normalizeEmail(getString(userinfo, "email"))
			name = cleanText(firstString(userinfo["name"], "Google User"))
		} else {
			email = extractVerifiedEmail(token)
			if email == "" && !a.stdinTTY {
				fmt.Fprintf(a.Err, "%sGoogle userinfo unavailable and no verified email claim in token. In non-interactive mode, a token with a verified email claim is required.%s\n", a.p.Red, a.p.Reset)
				return 1
			}
			if email == "" {
				email = normalizeEmail(a.readLine("Enter email address manually: "))
			}
		}
		if email == "" {
			fmt.Fprintf(a.Err, "%sEmail address is required.%s\n", a.p.Red, a.p.Reset)
			return 1
		}
		if !tokenMatchesEmail(token, email) {
			fmt.Fprintf(a.Err, "%sToken identity does not match the selected Google account.%s\n", a.p.Red, a.p.Reset)
			return 1
		}
		accounts, err := a.store.Load(false)
		if err != nil {
			return a.storeError(err)
		}
		account := newAccount(email, name, token)
		_ = a.saveAccountSecret(ctx, account, token)
		accounts.Set(email, account)
		if err := a.store.Save(accounts); err != nil {
			return a.storeError(err)
		}
		fmt.Fprintf(a.Out, "%s✓ Saved account '%s'.%s\n", a.p.Green, email, a.p.Reset)
		return 0
	}
	return a.addLoginFlow(ctx)
}

func (a *Application) addLoginFlow(ctx context.Context) int {
	lock, err := acquireFileLock(a.paths.SessionLock)
	if err != nil {
		return a.storeError(err)
	}
	defer lock.Close()
	current := a.credentials.Current(ctx)
	backupSecure := a.credentials.Secure(ctx)
	snapshot, _ := snapshotFiles(a.paths.OAuthToken, a.paths.OAuthCredentials, a.paths.GoogleAccounts)
	restore := func() {
		_ = restoreFiles(snapshot)
		if backupSecure != "" {
			_ = a.credentials.Set(ctx, backupSecure)
		} else {
			_ = a.credentials.Delete(ctx)
		}
	}
	if current != "" {
		accounts, _ := a.store.Load(false)
		active := a.activeEmail(ctx, accounts, current)
		_, saved := accounts.Get(strings.ToLower(active))
		if !saved {
			label := "Unknown account"
			if info := a.http.userInfo(ctx, getString(tokenObject(decodeToken(current)), "access_token")); info != nil {
				email := normalizeEmail(getString(info, "email"))
				name := firstString(info["name"], "Google User")
				if email != "" {
					label = fmt.Sprintf("%s <%s>", name, email)
				}
			}
			fmt.Fprintf(a.Out, "\n%s⚠ Active session detected: %s%s%s\n%sThis account is not yet saved in agy-swap.%s\n", a.p.Yellow, a.p.Bold, label, a.p.Reset, a.p.Gray, a.p.Reset)
			for {
				save, valid := parseYesNo(a.readLine(a.p.Cyan+"Save this account? [y/N] (type 'n' to login a different account): "+a.p.Reset), false)
				if !valid {
					fmt.Fprintln(a.Out, "Please answer y or n.")
					continue
				}
				if save {
					return a.saveTokenAccount(ctx, current)
				}
				break
			}
		}
	}
	fmt.Fprintf(a.Out, "\n%sAdd / Login Google Account%s\n%s1. A browser window will open to authenticate with Google.%s\n%s2. Complete login in Google Antigravity.%s\n", a.p.Bold, a.p.Reset, a.p.Gray, a.p.Reset, a.p.Gray, a.p.Reset)
	for {
		if a.readLine(a.p.Cyan+"Press Enter when ready to start login (no password is required here)..."+a.p.Reset) == "" {
			break
		}
		fmt.Fprintln(a.Out, "Please press Enter without entering a password; authentication happens in the browser.")
	}
	if !a.credentials.clearUnlocked(ctx) {
		fmt.Fprintln(a.Err, "Could not clear the active session before login")
		return 1
	}
	fmt.Fprintf(a.Out, "\n%sLaunching interactive 'agy' login...%s\n", a.p.Green, a.p.Reset)
	fmt.Fprintf(a.Out, "%sComplete every prompt shown here and in the browser. agy-swap will continue after the credential is saved.%s\n\n", a.p.Yellow, a.p.Reset)
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
	go func() { commandDone <- command.Wait() }()
	timeout := time.NewTimer(a.loginWaitTimeout())
	defer timeout.Stop()
	poll := time.NewTicker(a.loginPollDelay())
	defer poll.Stop()
	var token string
	for {
		select {
		case <-ctx.Done():
			stopLoginCommand(command, commandDone)
			restore()
			return 1
		case commandErr := <-commandDone:
			token = a.credentials.Current(ctx)
			if token != "" && token != current {
				return a.saveTokenAccount(ctx, token)
			}
			restore()
			if ctx.Err() != nil {
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
				return a.saveTokenAccount(ctx, token)
			}
		case <-timeout.C:
			stopLoginCommand(command, commandDone)
			restore()
			fmt.Fprintf(a.Out, "\n%sTimed out waiting for login (120s).%s\n", a.p.Red, a.p.Reset)
			if current != "" {
				fmt.Fprintf(a.Out, "%sRestored previous token.%s\n", a.p.Gray, a.p.Reset)
			}
			return 1
		}
	}
}

func (a *Application) saveTokenAccount(ctx context.Context, token string) int {
	inner := tokenObject(decodeToken(token))
	if inner == nil || getString(inner, "access_token") == "" {
		fmt.Fprintf(a.Out, "\n%sInvalid token structure.%s\n", a.p.Red, a.p.Reset)
		return 1
	}
	stop := a.spinner("Fetching Google profile & avatar...")
	info := a.http.userInfo(ctx, getString(inner, "access_token"))
	stop()
	email, name := "", "Google User"
	if info != nil {
		email = normalizeEmail(getString(info, "email"))
		name = firstString(info["name"], name)
	} else {
		email = extractVerifiedEmail(token)
		if email == "" && a.stdinTTY {
			email = normalizeEmail(a.readLine("Enter account email manually: "))
		}
	}
	if email == "" {
		fmt.Fprintln(a.Err, "A valid email is required")
		return 1
	}
	if !tokenMatchesEmail(token, email) {
		fmt.Fprintf(a.Err, "%sToken identity does not match the selected Google account.%s\n", a.p.Red, a.p.Reset)
		return 1
	}
	accounts, err := a.store.Load(false)
	if err != nil {
		return a.storeError(err)
	}
	account := newAccount(email, name, token)
	_ = a.saveAccountSecret(ctx, account, token)
	accounts.Set(email, account)
	if err := a.store.Save(accounts); err != nil {
		return a.storeError(err)
	}
	fmt.Fprintf(a.Out, "\n%s✓ Successfully saved account: %s <%s>%s [%s]\n", a.p.Green, name, email, a.p.Reset, tierBadge(accounts.ByEmail[email], a.p))
	return 0
}

func (a *Application) cmdList(ctx context.Context, args cliArgs) int {
	accounts, err := a.store.Load(true)
	if err != nil {
		return a.storeError(err)
	}
	if accounts.Len() == 0 {
		fmt.Fprintf(a.Out, "%sNo accounts added yet. Run 'agy-swap add' to save a Google account.%s\n", a.p.Gray, a.p.Reset)
		return 0
	}
	current := a.credentials.Current(ctx)
	active := a.activeEmail(ctx, accounts, current)
	now := time.Now().UTC()
	fmt.Fprintf(a.Out, "\n%sMANAGED ACCOUNTS%s\n%s%s%s\n", a.p.Bold, a.p.Reset, a.p.Gray, strings.Repeat("─", 65), a.p.Reset)
	hasQuota, hasLimits := false, false
	for _, email := range accounts.Order {
		account := accounts.ByEmail[email]
		hasQuota = hasQuota || len(quotaGroups(account)) > 0
		hasLimits = hasLimits || len(activeLimits(account, now)) > 0
	}
	if hasQuota {
		fmt.Fprintf(a.Out, "%sQuota comes from Google; grouped models share weekly and optional 5-hour limits.%s\n", a.p.Gray, a.p.Reset)
	} else if hasLimits {
		fmt.Fprintf(a.Out, "%sGoogle usage unavailable; cooldown bars below come from recent local errors.%s\n", a.p.Gray, a.p.Reset)
	}
	for i, email := range accounts.Order {
		account := accounts.ByEmail[email]
		isActive := strings.EqualFold(active, email)
		marker := a.p.Gray + "○" + a.p.Reset
		activeLabel := ""
		if isActive {
			marker = a.p.Green + "●" + a.p.Reset
			activeLabel = " " + a.p.Green + "(Active)" + a.p.Reset
		}
		fmt.Fprintf(a.Out, " %s [%d] %s %s%s%s %s<%s>%s%s\n", marker, i+1, avatar(getString(account, "name"), email, a.color), a.p.Bold, getString(account, "name"), a.p.Reset, a.p.Gray, email, a.p.Reset, activeLabel)
		fmt.Fprintf(a.Out, "       ↳ Status: %s\n", accountStatus(account, a.p, now))
		if len(quotaGroups(account)) > 0 {
			for _, rg := range quotaGroups(account) {
				group := getMap(rg)
				fmt.Fprintf(a.Out, "       ↳ %s\n", getString(group, "name"))
				for _, rb := range getSlice(group["buckets"]) {
					bucket := getMap(rb)
					fmt.Fprintf(a.Out, "          %s: %s\n", getString(bucket, "name"), formatQuotaBar(bucket, a.p, now, 12))
				}
			}
		} else {
			for _, limit := range activeLimits(account, now) {
				if bar := formatCooldownBar(limit.Limit, a.p, now, 12); bar != "" {
					fmt.Fprintf(a.Out, "       ↳ %s: %s · resets in %s\n", getString(limit.Limit, "model"), bar, formatDuration(limit.Remaining.Seconds()))
				}
			}
		}
		if args.verbose {
			if age, ok := quotaAge(account, now); ok {
				fmt.Fprintf(a.Out, "       ↳ Google quota synced %s ago\n", formatDuration(age.Seconds()))
			}
			for _, limit := range activeLimits(account, now) {
				observed, _ := parseUTC(getString(limit.Limit, "observed_at"))
				source := "manual"
				if getString(limit.Limit, "source") != "manual" {
					source = "local log " + firstString(limit.Limit["source_file"], "(unknown file)")
				}
				fmt.Fprintf(a.Out, "       ↳ %s: observed %s · source %s\n", getString(limit.Limit, "model"), observed.Format("2006-01-02 15:04:05 UTC"), source)
			}
		}
	}
	fmt.Fprintf(a.Out, "%s%s%s\n\n", a.p.Gray, strings.Repeat("─", 65), a.p.Reset)
	return 0
}

func (a *Application) activeEmail(ctx context.Context, accounts *Accounts, current string) string {
	claimed := localActiveEmail(accounts, current)
	if claimed != "" {
		return claimed
	}
	for _, email := range accounts.Order {
		if token, err := a.accountToken(ctx, accounts.ByEmail[email]); err == nil && token == current {
			return email
		}
	}
	inner := tokenObject(decodeToken(current))
	if inner != nil {
		if info := a.http.userInfo(ctx, getString(inner, "access_token")); info != nil {
			claimed = normalizeEmail(getString(info, "email"))
		}
	}
	if claimed != "" {
		for _, email := range accounts.Order {
			if strings.EqualFold(email, claimed) {
				return email
			}
		}
	}
	return claimed
}

func localActiveEmail(accounts *Accounts, current string) string {
	if current == "" {
		return ""
	}
	for _, email := range accounts.Order {
		if getString(accounts.ByEmail[email], "token_data") == current {
			return email
		}
	}
	claimed := extractVerifiedEmail(current)
	if claimed != "" {
		for _, email := range accounts.Order {
			if strings.EqualFold(email, claimed) {
				return email
			}
		}
	}
	return claimed
}

func resolveTarget(target string, accounts *Accounts) (string, error) {
	target = strings.TrimSpace(target)
	if target == "" || accounts.Len() == 0 {
		return "", nil
	}
	if number, err := strconv.Atoi(target); err == nil {
		if number >= 1 && number <= accounts.Len() {
			return accounts.Order[number-1], nil
		}
		return "", nil
	}
	for _, email := range accounts.Order {
		if strings.EqualFold(email, target) {
			return email, nil
		}
	}
	var matches []string
	for _, email := range accounts.Order {
		if strings.Contains(strings.ToLower(email), strings.ToLower(target)) {
			matches = append(matches, email)
		}
	}
	if len(matches) > 1 {
		return "", fmt.Errorf("%w: Account target '%s' matches: %s", errAmbiguous, target, strings.Join(matches, ", "))
	}
	if len(matches) == 1 {
		return matches[0], nil
	}
	return "", nil
}

func accountCooldown(account Account, now time.Time, family string) (time.Duration, bool) {
	if wait, matched := quotaWait(account, now, family); matched {
		return wait, true
	}
	var maxWait time.Duration
	found := false
	for _, limit := range activeLimits(account, now) {
		if family == "" || getString(limit.Limit, "family") == family {
			found = true
			if limit.Remaining > maxWait {
				maxWait = limit.Remaining
			}
		}
	}
	return maxWait, found
}
func selectNext(accounts *Accounts, active, family string) (Account, int) {
	activeIndex := -1
	for i, email := range accounts.Order {
		if active != "" && strings.EqualFold(email, active) {
			activeIndex = i
			break
		}
	}
	type candidate struct {
		account Account
		wait    time.Duration
		known   bool
	}
	list := make([]candidate, 0, accounts.Len())
	now := time.Now()
	for offset := 1; offset <= accounts.Len(); offset++ {
		account := accounts.ByEmail[accounts.Order[(activeIndex+offset)%accounts.Len()]]
		wait, known := accountCooldown(account, now, family)
		list = append(list, candidate{account, wait, known})
	}
	for _, item := range list {
		if item.known && item.wait == 0 {
			return item.account, 0
		}
	}
	for _, item := range list {
		if !item.known {
			return item.account, -1
		}
	}
	best := list[0]
	for _, item := range list[1:] {
		if item.wait < best.wait {
			best = item
		}
	}
	return best.account, 1
}

func (a *Application) cmdNext(ctx context.Context, args cliArgs) int {
	accounts, err := a.store.Load(true)
	if err != nil {
		return a.storeError(err)
	}
	if accounts.Len() == 0 {
		fmt.Fprintf(a.Out, "%sNo accounts found. Add one with 'agy-swap add' first.%s\n", a.p.Gray, a.p.Reset)
		return 1
	}
	_ = a.quota.Refresh(ctx, accounts, false, nil)
	lock, err := acquireFileLock(a.paths.SessionLock)
	if err != nil {
		return a.storeError(err)
	}
	defer lock.Close()
	current := a.credentials.Current(ctx)
	active := a.activeEmail(ctx, accounts, current)
	next, state := selectNext(accounts, active, args.family)
	label := ""
	if args.family != "" {
		label = strings.Title(args.family) + " "
	}
	reason := "available " + label + "quota"
	if state == 1 {
		wait, _ := accountCooldown(next, time.Now(), args.family)
		fmt.Fprintf(a.Out, "%sAll accounts have an observed %slimit; selecting the shortest wait (%s).%s\n", a.p.Yellow, label, formatDuration(wait.Seconds()), a.p.Reset)
		reason = "shortest observed " + label + "limit"
	} else if state == -1 {
		fmt.Fprintf(a.Out, "%sNo account has confirmed available %squota; selecting the next account with unverified usage.%s\n", a.p.Yellow, label, a.p.Reset)
		reason = "unverified " + label + "quota"
	}
	fmt.Fprintf(a.Out, "Auto-rotating to account with %s: %s%s%s %s<%s>%s...\n", reason, a.p.Bold, getString(next, "name"), a.p.Reset, a.p.Gray, getString(next, "email"), a.p.Reset)
	token, tokenErr := a.accountToken(ctx, next)
	if tokenErr == nil && a.credentials.applyUnlocked(ctx, token, getString(next, "email")) {
		fmt.Fprintf(a.Out, "%s✓ Successfully auto-rotated to %s.%s\n", a.p.Green, getString(next, "email"), a.p.Reset)
		return 0
	}
	fmt.Fprintf(a.Err, "%s✕ Failed to rotate account.%s\n", a.p.Red, a.p.Reset)
	return 1
}

func (a *Application) cmdSwitch(ctx context.Context, args cliArgs) int {
	accounts, err := a.store.Load(true)
	if err != nil {
		return a.storeError(err)
	}
	if accounts.Len() == 0 {
		fmt.Fprintf(a.Out, "%sNo accounts found. Add one with 'agy-swap add' first.%s\n", a.p.Gray, a.p.Reset)
		return 1
	}
	if args.account == "" {
		if a.stdinTTY {
			return a.cmdInteractive(ctx)
		}
		fmt.Fprintf(a.Err, "%sInteractive mode requires TTY.%s\n", a.p.Red, a.p.Reset)
		return 1
	}
	settings, _ := a.loadSettings()
	email, err := resolveConfiguredTarget(args.account, accounts, settings)
	if err != nil {
		fmt.Fprintf(a.Err, "%s%s%s\n", a.p.Red, strings.TrimPrefix(err.Error(), errAmbiguous.Error()+": "), a.p.Reset)
		return 1
	}
	if email == "" {
		fmt.Fprintf(a.Err, "%sAccount '%s' not found.%s\n", a.p.Red, args.account, a.p.Reset)
		return 1
	}
	account := accounts.ByEmail[email]
	token, tokenErr := a.accountToken(ctx, account)
	if tokenErr != nil {
		fmt.Fprintf(a.Err, "%s✕ Failed to read account credential: %v.%s\n", a.p.Red, tokenErr, a.p.Reset)
		return 1
	}
	if a.credentials.Current(ctx) == token {
		fmt.Fprintf(a.Out, "Already using %s%s%s %s<%s>%s.\n", a.p.Bold, getString(account, "name"), a.p.Reset, a.p.Gray, email, a.p.Reset)
		return 0
	}
	fmt.Fprintf(a.Out, "Switching to %s%s%s %s<%s>%s...\n", a.p.Bold, getString(account, "name"), a.p.Reset, a.p.Gray, email, a.p.Reset)
	if a.credentials.Apply(ctx, token, email) {
		fmt.Fprintf(a.Out, "%s✓ Successfully switched to %s.%s\n", a.p.Green, email, a.p.Reset)
		return 0
	}
	fmt.Fprintf(a.Err, "%s✕ Failed to switch account.%s\n", a.p.Red, a.p.Reset)
	return 1
}

func (a *Application) cmdSetLimit(_ context.Context, args cliArgs) int {
	accounts, err := a.store.Load(true)
	if err != nil {
		return a.storeError(err)
	}
	if accounts.Len() == 0 {
		fmt.Fprintf(a.Out, "%sNo accounts found.%s\n", a.p.Gray, a.p.Reset)
		return 1
	}
	settings, _ := a.loadSettings()
	email, err := resolveConfiguredTarget(args.account, accounts, settings)
	if err != nil || email == "" {
		fmt.Fprintf(a.Err, "%sAccount '%s' not found.%s\n", a.p.Red, args.account, a.p.Reset)
		return 1
	}
	duration, ok := parseDuration(args.duration)
	if !ok {
		fmt.Fprintf(a.Err, "%sInvalid duration format or duration exceeds 7 days.%s\n", a.p.Red, a.p.Reset)
		return 1
	}
	account := accounts.ByEmail[email]
	limits := getMap(account["quota_limits"])
	if limits == nil {
		limits = make(map[string]any)
	}
	key := "manual:" + args.group
	if duration == 0 {
		delete(limits, key)
		if len(limits) == 0 {
			delete(account, "quota_limits")
		} else {
			account["quota_limits"] = limits
		}
		if err := a.store.Save(accounts); err != nil {
			return a.storeError(err)
		}
		fmt.Fprintf(a.Out, "%s✓ Cleared rate limit cooldown for '%s'.%s\n", a.p.Green, email, a.p.Reset)
		return 0
	}
	now := time.Now().UTC()
	name := strings.Title(args.group)
	if args.group == "gpt" {
		name = "GPT"
	}
	limits[key] = map[string]any{"model": name, "family": args.group, "reset_at": isoTime(now.Add(duration)), "observed_at": isoTime(now), "source": "manual"}
	account["quota_limits"] = limits
	if err := a.store.Save(accounts); err != nil {
		return a.storeError(err)
	}
	fmt.Fprintf(a.Out, "%s✓ Set %s rate limit cooldown for '%s' (%s).%s\n", a.p.Green, strings.Title(args.group), email, now.Add(duration).Format("15:04:05 UTC"), a.p.Reset)
	return 0
}

func (a *Application) cmdLimits(ctx context.Context, args cliArgs) int {
	accounts, err := a.store.Load(true)
	if err != nil {
		return a.storeError(err)
	}
	var failures map[string]string
	if accounts.Len() > 0 {
		failures = a.quota.Refresh(ctx, accounts, args.refresh, nil)
	}
	code := a.cmdList(ctx, args)
	if len(failures) > 0 {
		fmt.Fprintf(a.Err, "%sUsage refresh failed for %d account(s); cached data kept.%s\n", a.p.Yellow, len(failures), a.p.Reset)
	}
	return code
}

func (a *Application) cmdRemove(_ context.Context, args cliArgs) int {
	accounts, err := a.store.Load(true)
	if err != nil {
		return a.storeError(err)
	}
	if accounts.Len() == 0 {
		fmt.Fprintf(a.Out, "%sNo accounts found.%s\n", a.p.Gray, a.p.Reset)
		return 1
	}
	if args.account == "" {
		if a.stdinTTY {
			return a.cmdInteractive(context.Background())
		}
		fmt.Fprintf(a.Err, "%sInteractive mode requires TTY.%s\n", a.p.Red, a.p.Reset)
		return 1
	}
	settings, _ := a.loadSettings()
	email, _ := resolveConfiguredTarget(args.account, accounts, settings)
	if email == "" {
		fmt.Fprintf(a.Err, "%sAccount '%s' not found.%s\n", a.p.Red, args.account, a.p.Reset)
		return 1
	}
	if strings.ToLower(a.readLine(fmt.Sprintf("Are you sure you want to remove '%s'? [y/N]: ", email))) == "y" {
		ref := getString(accounts.ByEmail[email], "secret_ref")
		accounts.Delete(email)
		if err := a.store.Save(accounts); err != nil {
			return a.storeError(err)
		}
		if ref != "" && a.vault != nil {
			_ = a.vault.Delete(context.Background(), ref)
		}
		fmt.Fprintf(a.Out, "%s✓ Removed account '%s'.%s\n", a.p.Green, email, a.p.Reset)
	}
	return 0
}

func (a *Application) cmdStatus(ctx context.Context) int {
	current := a.credentials.Current(ctx)
	if current == "" {
		fmt.Fprintf(a.Out, "%sStatus: Not logged in%s\n", a.p.Gray, a.p.Reset)
		return 0
	}
	accounts, err := a.store.Load(true)
	if err != nil {
		return a.storeError(err)
	}
	email := a.activeEmail(ctx, accounts, current)
	if account, ok := accounts.Get(email); ok {
		fmt.Fprintf(a.Out, "Active Account: %s %s● %s%s %s<%s>%s\n", avatar(getString(account, "name"), email, a.color), a.p.Green, getString(account, "name"), a.p.Reset, a.p.Gray, email, a.p.Reset)
		fmt.Fprintf(a.Out, "Status: %s\n", accountStatus(account, a.p, time.Now()))
		return 0
	}
	if email == "" {
		email = "Unknown Session"
	}
	fmt.Fprintf(a.Out, "Active Account: %s● %s%s %s(Unsaved in agy-swap)%s\n", a.p.Yellow, email, a.p.Reset, a.p.Gray, a.p.Reset)
	return 0
}

func (a *Application) cmdLogout(ctx context.Context) int {
	current := a.credentials.Current(ctx)
	if !a.credentials.Clear(ctx) {
		fmt.Fprintf(a.Err, "%s✕ Could not clear every active credential store.%s\n", a.p.Red, a.p.Reset)
		return 1
	}
	if current != "" {
		fmt.Fprintf(a.Out, "%s✓ Logged out and removed active OAuth credentials.%s\n", a.p.Green, a.p.Reset)
	} else {
		fmt.Fprintf(a.Out, "%sAlready logged out; removed any stale credential files.%s\n", a.p.Gray, a.p.Reset)
	}
	return 0
}

func (a *Application) storeError(err error) int {
	if errors.Is(err, errStoreConflict) {
		fmt.Fprintf(a.Err, "%s%s%s\n", a.p.Red, err, a.p.Reset)
	} else {
		fmt.Fprintf(a.Err, "%sStore error: %v%s\n", a.p.Red, err, a.p.Reset)
	}
	return 1
}
