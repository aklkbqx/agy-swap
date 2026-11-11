package app

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"golang.org/x/term"
)

type Application struct {
	Version             string
	demo                bool
	renderClock         func() time.Time
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
	openURL             func(context.Context, string) error
	loginTimeout        time.Duration
	loginPollInterval   time.Duration
	inTUISuspend        bool
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
	app := &Application{Version: version, BuildID: "unknown", In: in, Out: out, Err: errOut, lineReader: lineReader, paths: paths, store: store, credentials: NewCredentials(paths), vault: NewAccountVault(paths), http: httpService, quota: NewQuotaService(httpService, store), stdinTTY: readerTerminal(in), stdoutTTY: stdoutTTY, color: stdoutTTY && os.Getenv("NO_COLOR") == ""}
	app.quota.SetVault(app.vault)
	if os.Getenv("AGY_SWAP_QUOTA_DEBUG") == "1" {
		app.quota.SetDiagnostics(errOut)
	}
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

type cliArgs struct {
	command, account, token, family, duration, group                            string
	verbose, refresh, force, help, version                                      bool
	legacyAdd, legacyList, legacyNext, legacySwitch, legacyStatus, legacyLogout bool
	legacyRemove, stringSwitch                                                  string
	updateCheck, limitClear, groupSet                                           bool
}

func (a *Application) Run(ctx context.Context, argv []string) int {
	if len(argv) > 0 && argv[0] == "__update-finalize" {
		return a.runUpdateFinalizer(ctx, argv[1:])
	}
	if len(argv) > 0 && (argv[0] == "--sync-session" || argv[0] == "session-sync" || argv[0] == "__session-sync") {
		a.reconcileActiveSession(ctx)
		return 0
	}
	if isExtendedCommand(argv) {
		cmdCtx, stopSignals := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
		defer stopSignals()
		return a.runExtended(cmdCtx, argv)
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
		fmt.Fprintf(a.Out, "agy-swap v%s\n", a.displayVersion())
		return 0
	}
	if args.help {
		a.printUsage(a.Out)
		return 0
	}
	command := args.command
	switch {
	case args.legacyAdd || args.token != "":
		command = "add"
	case args.legacyList:
		command = "list"
	case args.legacyLogout:
		command = "logout"
	case args.legacyNext:
		command = "next"
	case args.stringSwitch != "":
		command = "switch"
		args.account = args.stringSwitch
	case args.legacySwitch:
		command = "switch"
	case args.legacyRemove != "":
		command = "remove"
		args.account = args.legacyRemove
	case args.legacyStatus:
		command = "status"
	}
	if command == "" {
		if a.stdinTTY && a.stdoutTTY {
			return a.cmdInteractive(ctx)
		}
		command = "list"
	}
	a.reconcileActiveSession(ctx)
	if (command == "switch" || command == "remove") && args.account == "" && a.stdinTTY && a.stdoutTTY {
		if command == "switch" {
			return a.cmdSwitch(ctx, args)
		}
		return a.cmdRemove(ctx, args)
	}
	cmdCtx, stopSignals := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stopSignals()
	ctx = cmdCtx
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
	case "verify":
		code = a.cmdVerify(ctx, args)
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
                [--family {claude,gemini,gpt,argon}] [--switch-to ACCOUNT]
                [--switch] [--remove ACCOUNT] [--status] [--logout]
                [--verbose]
                {add,list,limits,logout,next,switch,remove,status,update,version,limit,
                 doctor,config,alias,tag,profile,bind,recommend,statusline,watch,
                 history,stats,forecast,backup,metrics,completion,account,target,run,donate} ...

Minimal Multi-Account Switcher for Google Antigravity CLI (agy)

commands:
  add       Log in & add a Google account
  list      List all saved accounts
  limits    Refresh and show live quota usage across accounts
  logout    Logout of active session
  next      Rotate to an account with available quota
  switch    Switch to an account
  remove    Remove a saved account
  status    Show active account details
  verify    Open Google's verification page for an account
  update    Update agy-swap to the latest version
  version   Show the installed version and build provenance
  limit     Set or clear a manual quota cooldown (limit set|clear)`)
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
  install-agy         Download, verify, and install the Antigravity CLI (agy)
  donate              Show GitHub Sponsors support link (--open)
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
	known := map[string]bool{"add": true, "list": true, "limits": true, "logout": true, "next": true, "switch": true, "remove": true, "status": true, "update": true, "version": true, "limit": true, "verify": true}
	i := 0
	if known[argv[0]] {
		result.command = argv[0]
		i = 1
		if result.command == "limit" {
			if i >= len(argv) || (argv[i] != "set" && argv[i] != "clear") {
				return result, fmt.Errorf("limit requires the 'set' or 'clear' subcommand")
			}
			if argv[i] == "clear" {
				result.limitClear = true
				result.duration = "0s"
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
			if !isQuotaFamily(value) {
				return result, fmt.Errorf("invalid family %q", value)
			}
			result.family = value
		case "--group":
			value, err := take()
			if err != nil {
				return result, err
			}
			if !isQuotaFamily(value) {
				return result, fmt.Errorf("invalid group %q", value)
			}
			result.group = value
			result.groupSet = true
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
			case "switch", "remove", "verify":
				if result.account != "" {
					return result, fmt.Errorf("too many arguments")
				}
				result.account = arg
			case "limit":
				switch {
				case result.account == "":
					result.account = arg
				case !result.limitClear && result.duration == "":
					result.duration = arg
				default:
					return result, fmt.Errorf("too many arguments")
				}
			default:
				return result, fmt.Errorf("unexpected argument: %s", arg)
			}
		}
		i++
	}
	if result.command == "limit" {
		if result.limitClear && result.account == "" {
			return result, fmt.Errorf("limit clear requires ACCOUNT")
		}
		if !result.limitClear && (result.account == "" || result.duration == "") {
			return result, fmt.Errorf("limit set requires ACCOUNT and DURATION")
		}
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

func (a *Application) resetTerminalLineReader() {
	a.lineReaderMu.Lock()
	defer a.lineReaderMu.Unlock()
	if a.In != nil && readerTerminal(a.In) && a.lineReader != nil {
		a.lineReader.Reset(a.In)
	}
}

// readLine prints prompt and reads one line from a.In. When ctx is cancelled
// (Ctrl+C) before a line arrives it returns ctx.Err() at once so callers stop
// prompting and return cleanly.
func (a *Application) readLine(ctx context.Context, prompt string) (string, error) {
	ensureCookedTerminal(a.In, a.Out)
	fmt.Fprint(a.Out, prompt)
	if a.In == nil {
		return "", nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		fmt.Fprintln(a.Out)
		return "", err
	}
	if line, handled, err := a.readTerminalLine(ctx); handled {
		if err != nil {
			fmt.Fprintln(a.Out)
			return "", err
		}
		return line, nil
	}
	type lineResult struct {
		line string
		err  error
	}
	result := make(chan lineResult, 1)
	go func() {
		line, err := a.readLineBlocking()
		result <- lineResult{line: line, err: err}
	}()
	select {
	case res := <-result:
		if res.err != nil {
			fmt.Fprintln(a.Out)
			return "", res.err
		}
		return res.line, nil
	case <-ctx.Done():
		fmt.Fprintln(a.Out)
		return "", ctx.Err()
	}
}

func (a *Application) readTerminalLine(ctx context.Context) (string, bool, error) {
	if !readerTerminal(a.In) {
		return "", false, nil
	}
	a.lineReaderMu.Lock()
	defer a.lineReaderMu.Unlock()
	if a.lineReader == nil {
		a.lineReader = bufio.NewReader(a.In)
	}
	var line []byte
	for {
		if a.lineReader.Buffered() == 0 {
			handled, err := waitTerminalInputReady(ctx, a.In)
			if !handled {
				return "", false, nil
			}
			if err != nil {
				return "", true, err
			}
		}
		b, err := a.lineReader.ReadByte()
		if err != nil {
			break
		}
		if b == 0x03 {
			return "", true, context.Canceled
		}
		if b == '\n' {
			break
		}
		if b == '\r' {
			if a.lineReader.Buffered() > 0 {
				if next, peekErr := a.lineReader.Peek(1); peekErr == nil && len(next) == 1 && next[0] == '\n' {
					_, _ = a.lineReader.ReadByte()
				}
			}
			break
		}
		line = append(line, b)
	}
	return strings.TrimSpace(string(line)), true, nil
}

func (a *Application) readLineBlocking() (string, error) {
	a.lineReaderMu.Lock()
	defer a.lineReaderMu.Unlock()
	if a.lineReader == nil {
		a.lineReader = bufio.NewReader(a.In)
	}
	var line []byte
	for {
		b, err := a.lineReader.ReadByte()
		if err != nil {
			break
		}
		if b == 0x03 {
			return "", context.Canceled
		}
		if b == '\n' {
			break
		}
		if b == '\r' {
			if a.lineReader.Buffered() > 0 {
				if next, peekErr := a.lineReader.Peek(1); peekErr == nil && len(next) == 1 && next[0] == '\n' {
					_, _ = a.lineReader.ReadByte()
				}
			}
			break
		}
		line = append(line, b)
	}
	return strings.TrimSpace(string(line)), nil
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

func (a *Application) storeError(err error) int {
	if errors.Is(err, errStoreConflict) {
		fmt.Fprintf(a.Err, "%s%s%s\n", a.p.Red, err, a.p.Reset)
	} else {
		fmt.Fprintf(a.Err, "%sStore error: %v%s\n", a.p.Red, err, a.p.Reset)
	}
	return 1
}

func (a *Application) renderTime() time.Time {
	if a.renderClock != nil {
		return a.renderClock()
	}
	return time.Now()
}
