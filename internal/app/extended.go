package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

type extendedOptions struct {
	Help            bool
	JSON            bool
	Force           bool
	Refresh         bool
	Once            bool
	Apply           bool
	Merge           bool
	Open            bool
	IncludeSecrets  bool
	PassphraseStdin bool
	Passphrase      string
	Output          string
	Dir             string
	Account         string
	Target          string
	Profile         string
	Policy          string
	Reserves        []string
	ReservesSet     bool
	Family          string
	Tag             string
	Mode            string
	RunArgs         []string
	Interval        time.Duration
	Limit           int
	Threshold       int
}

func isExtendedCommand(argv []string) bool {
	for i := 0; i < len(argv); i++ {
		arg := argv[i]
		if arg == "--" {
			return false
		}
		if strings.HasPrefix(arg, "-") {
			if extendedOptionTakesValue(arg) {
				i++
			}
			continue
		}
		switch arg {
		case "doctor", "config", "alias", "tag", "profile", "bind", "unbind", "recommend", "statusline", "watch", "history", "stats", "forecast", "backup", "metrics", "completion", "account", "target", "run", "install-agy", "install", "agy", "donate", "sponsor":
			return true
		default:
			return false
		}
	}
	return false
}

func parseExtended(argv []string) (string, extendedOptions, []string, error) {
	if len(argv) == 0 {
		return "", extendedOptions{}, nil, errors.New("missing command")
	}
	argv = normalizeExtendedArgv(argv)
	command := argv[0]
	opts := extendedOptions{Interval: 60 * time.Second, Limit: 20, Threshold: -1}
	var positional []string
	for i := 1; i < len(argv); i++ {
		arg := argv[i]
		take := func() (string, error) {
			if i+1 >= len(argv) {
				return "", fmt.Errorf("%s requires a value", arg)
			}
			i++
			return argv[i], nil
		}
		switch arg {
		case "-h", "--help":
			opts.Help = true
		case "--json":
			opts.JSON = true
		case "--force":
			opts.Force = true
		case "--refresh":
			opts.Refresh = true
		case "--once":
			opts.Once = true
		case "--apply":
			opts.Apply = true
		case "--merge":
			opts.Merge = true
		case "--open":
			opts.Open = true
		case "--include-secrets":
			opts.IncludeSecrets = true
		case "--passphrase":
			value, err := take()
			if err != nil {
				return "", opts, nil, err
			}
			opts.Passphrase = value
		case "--passphrase-stdin":
			opts.PassphraseStdin = true
		case "--output":
			value, err := take()
			if err != nil {
				return "", opts, nil, err
			}
			opts.Output = value
		case "--dir", "-d":
			value, err := take()
			if err != nil {
				return "", opts, nil, err
			}
			opts.Dir = value
		case "--account":
			value, err := take()
			if err != nil {
				return "", opts, nil, err
			}
			opts.Account = value
		case "--target":
			value, err := take()
			if err != nil {
				return "", opts, nil, err
			}
			opts.Target = cleanText(value)
		case "--profile":
			value, err := take()
			if err != nil {
				return "", opts, nil, err
			}
			opts.Profile = value
		case "--family":
			value, err := take()
			if err != nil {
				return "", opts, nil, err
			}
			if value != "" && !isQuotaFamily(value) {
				return "", opts, nil, fmt.Errorf("invalid family %q", value)
			}
			opts.Family = value
		case "--tag":
			value, err := take()
			if err != nil {
				return "", opts, nil, err
			}
			opts.Tag = cleanText(value)
		case "--mode":
			value, err := take()
			if err != nil {
				return "", opts, nil, err
			}
			opts.Mode = value
		case "--policy", "--reserve":
			value, err := take()
			if err != nil {
				return "", opts, nil, err
			}
			if arg == "--policy" {
				opts.Policy = value
			} else {
				opts.ReservesSet = true
				if value != "" {
					opts.Reserves = strings.Split(value, ",")
				}
			}
		case "--interval":
			value, err := take()
			if err != nil {
				return "", opts, nil, err
			}
			interval, parseErr := time.ParseDuration(value)
			if parseErr != nil || interval < time.Second || interval > 24*time.Hour {
				return "", opts, nil, errors.New("interval must be between 1s and 24h")
			}
			opts.Interval = interval
		case "--limit":
			value, err := take()
			if err != nil {
				return "", opts, nil, err
			}
			limit, parseErr := strconv.Atoi(value)
			if parseErr != nil || limit < 1 || limit > 10000 {
				return "", opts, nil, errors.New("limit must be between 1 and 10000")
			}
			opts.Limit = limit
		case "--threshold":
			value, err := take()
			if err != nil {
				return "", opts, nil, err
			}
			threshold, parseErr := strconv.Atoi(value)
			if parseErr != nil || threshold < 0 || threshold > 100 {
				return "", opts, nil, errors.New("threshold must be between 0 and 100")
			}
			opts.Threshold = threshold
		case "--":
			opts.RunArgs = append([]string(nil), argv[i+1:]...)
			i = len(argv)
		default:
			if strings.HasPrefix(arg, "-") {
				return "", opts, nil, fmt.Errorf("unknown option %s", arg)
			}
			positional = append(positional, arg)
		}
	}
	return command, opts, positional, nil
}

func normalizeExtendedArgv(argv []string) []string {
	commandIndex := 0
	for commandIndex < len(argv) {
		arg := argv[commandIndex]
		if !strings.HasPrefix(arg, "-") {
			break
		}
		if extendedOptionTakesValue(arg) {
			commandIndex += 2
		} else {
			commandIndex++
		}
	}
	if commandIndex <= 0 || commandIndex >= len(argv) {
		return argv
	}
	result := make([]string, 0, len(argv))
	result = append(result, argv[commandIndex])
	result = append(result, argv[:commandIndex]...)
	result = append(result, argv[commandIndex+1:]...)
	return result
}

func extendedOptionTakesValue(arg string) bool {
	switch arg {
	case "--passphrase", "--output", "--dir", "-d", "--account", "--target", "--profile", "--family", "--tag", "--mode", "--interval", "--limit", "--threshold", "--policy", "--reserve":
		return true
	default:
		return false
	}
}

type extendedEnvelope struct {
	Schema   int      `json:"schema_version"`
	Command  string   `json:"command"`
	OK       bool     `json:"ok"`
	Data     any      `json:"data,omitempty"`
	Warnings []string `json:"warnings,omitempty"`
	Error    string   `json:"error,omitempty"`
}

func (a *Application) extendedResult(command string, opts extendedOptions, data any, warnings []string) int {
	if opts.JSON {
		return a.writeJSON(extendedEnvelope{Schema: stateSchema, Command: command, OK: true, Data: data, Warnings: warnings})
	}
	return 0
}

func (a *Application) extendedError(command string, opts extendedOptions, err error) int {
	if opts.JSON {
		a.writeJSON(extendedEnvelope{Schema: stateSchema, Command: command, Error: err.Error()})
		return 1
	}
	fmt.Fprintf(a.Err, "agy-swap: %s: %v\n", command, err)
	return 1
}

func (a *Application) writeJSON(value any) int {
	encoder := json.NewEncoder(a.Out)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		fmt.Fprintf(a.Err, "agy-swap: cannot write JSON: %v\n", err)
		return 1
	}
	return 0
}

func (a *Application) runExtended(ctx context.Context, argv []string) int {
	command, opts, positional, err := parseExtended(argv)
	if err != nil {
		return a.extendedError(firstString(command, "command"), opts, err)
	}
	if opts.Help {
		a.printUsage(a.Out)
		return 0
	}
	switch command {
	case "completion", "donate", "sponsor", "history", "stats", "config", "alias", "tag", "profile", "bind", "unbind", "target":
	default:
		a.reconcileActiveSession(ctx)
	}
	var code int
	switch command {
	case "doctor":
		code = a.cmdDoctor(ctx, opts)
	case "config":
		code = a.cmdConfig(opts, positional)
	case "alias":
		code = a.cmdAlias(opts, positional)
	case "tag":
		code = a.cmdTag(opts, positional)
	case "profile":
		code = a.cmdProfile(opts, positional)
	case "bind":
		code = a.cmdBind(opts, positional)
	case "unbind":
		code = a.cmdUnbind(opts, positional)
	case "recommend":
		code = a.cmdRecommend(ctx, opts, positional)
	case "statusline":
		code = a.cmdStatusline(ctx, opts, positional)
	case "watch":
		code = a.cmdWatch(ctx, opts, positional)
	case "history":
		code = a.cmdHistory(opts, positional)
	case "stats":
		code = a.cmdHistoryStats(opts)
	case "forecast":
		code = a.cmdForecast(ctx, opts, positional)
	case "backup":
		code = a.cmdBackup(ctx, opts, positional)
	case "metrics":
		code = a.cmdMetrics(ctx, opts, positional)
	case "completion":
		code = a.cmdCompletion(opts, positional)
	case "account":
		code = a.cmdAccount(ctx, opts, positional)
	case "target":
		code = a.cmdTarget(opts, positional)
	case "run":
		code = a.cmdRunNow(ctx, opts, positional)
	case "install-agy", "install", "agy":
		code = a.cmdInstallAgy(ctx, opts, positional)
	case "donate", "sponsor":
		code = a.cmdDonate(ctx, opts, positional)
	default:
		return a.extendedError(command, opts, fmt.Errorf("unknown command %q", command))
	}
	return code
}

func accountSummary(account Account, active bool) map[string]any {
	result := map[string]any{
		"email":      getString(account, "email"),
		"name":       getString(account, "name"),
		"active":     active,
		"secret_ref": getString(account, "secret_ref"),
		"has_token":  getString(account, "token_data") != "" || getString(account, "secret_ref") != "",
	}
	if snapshot := getMap(account["quota_snapshot"]); snapshot != nil {
		result["quota"] = snapshot
	}
	if limits := getMap(account["quota_limits"]); len(limits) > 0 {
		result["cooldowns"] = limits
	}
	if ineligible, ok := accountEligibility(account); ok {
		result["eligibility"] = map[string]any{
			"state":   "verify_required",
			"reason":  ineligible.Reason,
			"message": ineligible.Message,
			"has_url": ineligible.URL != "",
		}
	}
	return result
}

func (a *Application) allAccountSummaries(ctx context.Context, accounts *Accounts) []map[string]any {
	active := a.activeEmail(ctx, accounts, a.credentials.Current(ctx))
	result := make([]map[string]any, 0, accounts.Len())
	for _, email := range accounts.Order {
		result = append(result, accountSummary(accounts.ByEmail[email], strings.EqualFold(active, email)))
	}
	return result
}

func (a *Application) loadSettings() (AppSettings, error) {
	if a == nil || a.store == nil {
		return defaultSettings(), nil
	}
	return a.store.LoadSettings()
}

func resolveConfiguredTarget(target string, accounts *Accounts, settings AppSettings) (string, error) {
	target = strings.TrimSpace(target)
	seen := map[string]bool{}
	for i := 0; i < 8; i++ {
		if next, ok := settings.Aliases[target]; ok {
			if seen[target] {
				return "", errors.New("alias cycle detected")
			}
			seen[target] = true
			target = next
			continue
		}
		if profile, ok := settings.Profiles[target]; ok {
			target = profile.Account
			continue
		}
		break
	}
	return resolveTarget(target, accounts)
}

func validateAliasName(name string) error {
	if name == "" || strings.ContainsAny(name, " /\\\t\n") || len(name) > 64 {
		return errors.New("alias name must be 1-64 characters without spaces or slashes")
	}
	return nil
}

func (a *Application) cmdAccount(ctx context.Context, opts extendedOptions, positional []string) int {
	sub := "list"
	if len(positional) > 0 {
		sub = positional[0]
	}
	accounts, err := a.store.Load(true)
	if err != nil {
		return a.extendedError("account "+sub, opts, err)
	}
	switch sub {
	case "list":
		data := a.allAccountSummaries(ctx, accounts)
		if opts.JSON {
			return a.extendedResult("account list", opts, data, nil)
		}
		for i, item := range data {
			fmt.Fprintf(a.Out, "%d  %s  %s\n", i+1, item["email"], item["name"])
		}
		return 0
	case "show":
		if len(positional) < 2 {
			return a.extendedError("account show", opts, errors.New("usage: account show ACCOUNT"))
		}
		email, resolveErr := resolveConfiguredTarget(positional[1], accounts, mustSettings(a.store))
		if resolveErr != nil || email == "" {
			if resolveErr == nil {
				resolveErr = errors.New("account not found")
			}
			return a.extendedError("account show", opts, resolveErr)
		}
		data := accountSummary(accounts.ByEmail[email], strings.EqualFold(email, a.activeEmail(ctx, accounts, a.credentials.Current(ctx))))
		if opts.JSON {
			return a.extendedResult("account show", opts, data, nil)
		}
		b, _ := json.MarshalIndent(data, "", "  ")
		fmt.Fprintln(a.Out, string(b))
		return 0
	case "migrate":
		if !opts.Force {
			return a.extendedError("account migrate", opts, errors.New("migration changes token storage; rerun with --force"))
		}
		migrated, skipped, remaining := 0, 0, 0
		var replaced []string
		for _, email := range accounts.Order {
			account := accounts.ByEmail[email]
			token := getString(account, "token_data")
			if token == "" {
				skipped++
				continue
			}
			old, ok := a.saveAccountSecret(ctx, account, token)
			if !ok {
				skipped++
				remaining++
				continue
			}
			migrated++
			if old != "" {
				replaced = append(replaced, old)
			}
		}
		if err := a.store.Save(accounts); err != nil {
			return a.extendedError("account migrate", opts, err)
		}
		a.deleteReplacedSecrets(ctx, replaced)
		data := map[string]any{"migrated": migrated, "skipped": skipped, "remaining_plaintext": remaining}
		if opts.JSON {
			return a.extendedResult("account migrate", opts, data, nil)
		}
		fmt.Fprintf(a.Out, "Migrated %d account(s); %d remain in legacy storage.\n", migrated, remaining)
		return 0
	default:
		return a.extendedError("account", opts, fmt.Errorf("unknown subcommand %q", sub))
	}
}

func mustSettings(store *Store) AppSettings { settings, _ := store.LoadSettings(); return settings }
