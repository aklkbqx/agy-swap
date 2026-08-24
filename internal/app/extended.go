package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

type extendedOptions struct {
	JSON            bool
	Force           bool
	Refresh         bool
	Once            bool
	Apply           bool
	Merge           bool
	IncludeSecrets  bool
	PassphraseStdin bool
	Passphrase      string
	Output          string
	Account         string
	Target          string
	Profile         string
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
		if arg == "--json" || arg == "--force" || arg == "--refresh" || arg == "--once" || arg == "--apply" || arg == "--merge" || arg == "--include-secrets" || arg == "--passphrase-stdin" {
			continue
		}
		if strings.HasPrefix(arg, "--") {
			i++
			continue
		}
		switch arg {
		case "doctor", "config", "alias", "tag", "profile", "bind", "unbind", "recommend", "statusline", "watch", "history", "stats", "forecast", "backup", "metrics", "completion", "account", "target", "run":
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
			if value != "" && !oneOf(value, "claude", "gemini", "gpt") {
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
	case "--passphrase", "--output", "--account", "--target", "--profile", "--family", "--tag", "--mode", "--interval", "--limit", "--threshold":
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
		return a.writeJSON(extendedEnvelope{Schema: stateSchema, Command: command, Error: err.Error()})
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

func (a *Application) loadSettings() (AppSettings, error) { return a.store.LoadSettings() }

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
		migrated, skipped := 0, 0
		for _, email := range accounts.Order {
			account := accounts.ByEmail[email]
			token := getString(account, "token_data")
			if token == "" {
				skipped++
				continue
			}
			if a.saveAccountSecret(ctx, account, token) {
				migrated++
			} else {
				skipped++
			}
		}
		if err := a.store.Save(accounts); err != nil {
			return a.extendedError("account migrate", opts, err)
		}
		data := map[string]any{"migrated": migrated, "skipped": skipped, "remaining_plaintext": skipped}
		if opts.JSON {
			return a.extendedResult("account migrate", opts, data, nil)
		}
		fmt.Fprintf(a.Out, "Migrated %d account(s); %d remain in legacy storage.\n", migrated, skipped)
		return 0
	default:
		return a.extendedError("account", opts, fmt.Errorf("unknown subcommand %q", sub))
	}
}

func mustSettings(store *Store) AppSettings { settings, _ := store.LoadSettings(); return settings }

func (a *Application) cmdConfig(opts extendedOptions, positional []string) int {
	sub := "show"
	if len(positional) > 0 {
		sub = positional[0]
	}
	settings, err := a.loadSettings()
	if err != nil {
		return a.extendedError("config", opts, err)
	}
	switch sub {
	case "show":
		if opts.JSON {
			return a.extendedResult("config show", opts, settings, nil)
		}
		data, _ := json.MarshalIndent(settings, "", "  ")
		fmt.Fprintln(a.Out, string(data))
		return 0
	case "path":
		if opts.JSON {
			return a.extendedResult("config path", opts, map[string]any{"path": a.paths.Settings}, nil)
		}
		fmt.Fprintln(a.Out, a.paths.Settings)
		return 0
	case "reset":
		if !opts.Force {
			return a.extendedError("config reset", opts, errors.New("rerun with --force to reset configuration"))
		}
		if err := a.store.SaveSettings(defaultSettings()); err != nil {
			return a.extendedError("config reset", opts, err)
		}
		if opts.JSON {
			return a.extendedResult("config reset", opts, defaultSettings(), nil)
		}
		fmt.Fprintln(a.Out, "Configuration reset.")
		return 0
	case "set":
		if len(positional) < 3 {
			return a.extendedError("config set", opts, errors.New("usage: config set KEY VALUE"))
		}
		if err := setConfigValue(&settings, positional[1], positional[2]); err != nil {
			return a.extendedError("config set", opts, err)
		}
		if err := a.store.SaveSettings(settings); err != nil {
			return a.extendedError("config set", opts, err)
		}
		if opts.JSON {
			return a.extendedResult("config set", opts, settings, nil)
		}
		fmt.Fprintf(a.Out, "Set %s.\n", positional[1])
		return 0
	default:
		return a.extendedError("config", opts, fmt.Errorf("unknown subcommand %q", sub))
	}
}

func setConfigValue(settings *AppSettings, key, value string) error {
	parseBool := func() (bool, error) {
		parsed, err := strconv.ParseBool(value)
		if err != nil {
			return false, errors.New("value must be true or false")
		}
		return parsed, nil
	}
	parseInt := func(min, max int) (int, error) {
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed < min || parsed > max {
			return 0, fmt.Errorf("value must be between %d and %d", min, max)
		}
		return parsed, nil
	}
	switch key {
	case "policy.name":
		settings.Policy.Name = value
	case "policy.prefer_family":
		if value != "" && !oneOf(value, "claude", "gemini", "gpt") {
			return errors.New("invalid policy family")
		}
		settings.Policy.PreferFamily = value
	case "policy.min_remaining_pct":
		parsed, err := parseInt(0, 100)
		if err != nil {
			return err
		}
		settings.Policy.MinRemainingPct = parsed
	case "policy.allow_apply":
		parsed, err := parseBool()
		if err != nil {
			return err
		}
		settings.Policy.AllowApply = parsed
	case "notifications.enabled":
		parsed, err := parseBool()
		if err != nil {
			return err
		}
		settings.Notifications.Enabled = parsed
	case "notifications.threshold":
		parsed, err := parseInt(0, 100)
		if err != nil {
			return err
		}
		settings.Notifications.Threshold = parsed
	case "notifications.reset":
		parsed, err := parseBool()
		if err != nil {
			return err
		}
		settings.Notifications.Reset = parsed
	case "notifications.auth_failure":
		parsed, err := parseBool()
		if err != nil {
			return err
		}
		settings.Notifications.AuthFailure = parsed
	case "notifications.stale":
		parsed, err := parseBool()
		if err != nil {
			return err
		}
		settings.Notifications.Stale = parsed
	case "notifications.cooldown_seconds":
		parsed, err := parseInt(0, 604800)
		if err != nil {
			return err
		}
		settings.Notifications.CooldownSeconds = parsed
	case "history.enabled":
		parsed, err := parseBool()
		if err != nil {
			return err
		}
		settings.History.Enabled = parsed
	case "history.retention_days":
		parsed, err := parseInt(1, 3650)
		if err != nil {
			return err
		}
		settings.History.RetentionDays = parsed
	case "history.max_bytes":
		parsed, err := parseInt(64*1024, 256*1024*1024)
		if err != nil {
			return err
		}
		settings.History.MaxBytes = parsed
	default:
		return fmt.Errorf("unknown configuration key %q", key)
	}
	return nil
}

func (a *Application) cmdAlias(opts extendedOptions, positional []string) int {
	sub := "list"
	if len(positional) > 0 {
		sub = positional[0]
	}
	settings, err := a.loadSettings()
	if err != nil {
		return a.extendedError("alias", opts, err)
	}
	switch sub {
	case "list":
		if opts.JSON {
			return a.extendedResult("alias list", opts, settings.Aliases, nil)
		}
		keys := make([]string, 0, len(settings.Aliases))
		for key := range settings.Aliases {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			fmt.Fprintf(a.Out, "%s -> %s\n", key, settings.Aliases[key])
		}
		return 0
	case "set":
		if len(positional) < 3 {
			return a.extendedError("alias set", opts, errors.New("usage: alias set NAME ACCOUNT"))
		}
		if err := validateAliasName(positional[1]); err != nil {
			return a.extendedError("alias set", opts, err)
		}
		settings.Aliases[positional[1]] = positional[2]
		if err := a.store.SaveSettings(settings); err != nil {
			return a.extendedError("alias set", opts, err)
		}
		if opts.JSON {
			return a.extendedResult("alias set", opts, map[string]string{"name": positional[1], "target": positional[2]}, nil)
		}
		fmt.Fprintf(a.Out, "Alias %s -> %s\n", positional[1], positional[2])
		return 0
	case "remove", "rm":
		if len(positional) < 2 {
			return a.extendedError("alias remove", opts, errors.New("usage: alias remove NAME"))
		}
		delete(settings.Aliases, positional[1])
		if err := a.store.SaveSettings(settings); err != nil {
			return a.extendedError("alias remove", opts, err)
		}
		return 0
	default:
		return a.extendedError("alias", opts, fmt.Errorf("unknown subcommand %q", sub))
	}
}

func (a *Application) cmdTag(opts extendedOptions, positional []string) int {
	sub := "list"
	if len(positional) > 0 {
		sub = positional[0]
	}
	settings, err := a.loadSettings()
	if err != nil {
		return a.extendedError("tag", opts, err)
	}
	accounts, err := a.store.Load(false)
	if err != nil {
		return a.extendedError("tag", opts, err)
	}
	switch sub {
	case "list":
		if opts.JSON {
			return a.extendedResult("tag list", opts, settings.Tags, nil)
		}
		keys := make([]string, 0, len(settings.Tags))
		for key := range settings.Tags {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			fmt.Fprintf(a.Out, "%s: %s\n", key, strings.Join(settings.Tags[key], ", "))
		}
		return 0
	case "set":
		if len(positional) < 3 {
			return a.extendedError("tag set", opts, errors.New("usage: tag set ACCOUNT TAG [TAG...]"))
		}
		email, resolveErr := resolveConfiguredTarget(positional[1], accounts, settings)
		if resolveErr != nil || email == "" {
			if resolveErr == nil {
				resolveErr = errors.New("account not found")
			}
			return a.extendedError("tag set", opts, resolveErr)
		}
		tags := uniqueClean(positional[2:])
		settings.Tags[email] = tags
		if err := a.store.SaveSettings(settings); err != nil {
			return a.extendedError("tag set", opts, err)
		}
		if opts.JSON {
			return a.extendedResult("tag set", opts, map[string]any{"email": email, "tags": tags}, nil)
		}
		fmt.Fprintf(a.Out, "%s: %s\n", email, strings.Join(tags, ", "))
		return 0
	case "remove", "rm":
		if len(positional) < 2 {
			return a.extendedError("tag remove", opts, errors.New("usage: tag remove ACCOUNT"))
		}
		email, _ := resolveConfiguredTarget(positional[1], accounts, settings)
		delete(settings.Tags, email)
		if err := a.store.SaveSettings(settings); err != nil {
			return a.extendedError("tag remove", opts, err)
		}
		return 0
	default:
		return a.extendedError("tag", opts, fmt.Errorf("unknown subcommand %q", sub))
	}
}

func uniqueClean(values []string) []string {
	seen := map[string]bool{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = cleanText(value)
		if value != "" && !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	sort.Strings(result)
	return result
}

func containsStringValue(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func (a *Application) cmdProfile(opts extendedOptions, positional []string) int {
	sub := "list"
	if len(positional) > 0 {
		sub = positional[0]
	}
	settings, err := a.loadSettings()
	if err != nil {
		return a.extendedError("profile", opts, err)
	}
	accounts, err := a.store.Load(false)
	if err != nil {
		return a.extendedError("profile", opts, err)
	}
	switch sub {
	case "list":
		if opts.JSON {
			return a.extendedResult("profile list", opts, settings.Profiles, nil)
		}
		keys := make([]string, 0, len(settings.Profiles))
		for key := range settings.Profiles {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			profile := settings.Profiles[key]
			fmt.Fprintf(a.Out, "%s -> %s", key, profile.Account)
			if profile.Family != "" {
				fmt.Fprintf(a.Out, " [%s]", profile.Family)
			}
			fmt.Fprintln(a.Out)
		}
		return 0
	case "show":
		if len(positional) < 2 {
			return a.extendedError("profile show", opts, errors.New("usage: profile show NAME"))
		}
		profile, ok := settings.Profiles[positional[1]]
		if !ok {
			return a.extendedError("profile show", opts, errors.New("profile not found"))
		}
		if opts.JSON {
			return a.extendedResult("profile show", opts, profile, nil)
		}
		data, _ := json.MarshalIndent(profile, "", "  ")
		fmt.Fprintln(a.Out, string(data))
		return 0
	case "set":
		if len(positional) < 3 {
			return a.extendedError("profile set", opts, errors.New("usage: profile set NAME ACCOUNT [--family FAMILY]"))
		}
		name := cleanText(positional[1])
		if err := validateAliasName(name); err != nil {
			return a.extendedError("profile set", opts, err)
		}
		email, resolveErr := resolveConfiguredTarget(positional[2], accounts, settings)
		if resolveErr != nil || email == "" {
			if resolveErr == nil {
				resolveErr = errors.New("account not found")
			}
			return a.extendedError("profile set", opts, resolveErr)
		}
		profile := settings.Profiles[name]
		profile.Account = email
		if opts.Family != "" {
			profile.Family = opts.Family
		}
		if profile.Policy == "" {
			profile.Policy = settings.Policy.Name
		}
		if opts.Threshold >= 0 {
			profile.NotifyThreshold = opts.Threshold
		}
		settings.Profiles[name] = profile
		if err := a.store.SaveSettings(settings); err != nil {
			return a.extendedError("profile set", opts, err)
		}
		if opts.JSON {
			return a.extendedResult("profile set", opts, map[string]any{"name": name, "profile": profile}, nil)
		}
		fmt.Fprintf(a.Out, "%s -> %s\n", name, email)
		return 0
	case "remove", "rm":
		if len(positional) < 2 {
			return a.extendedError("profile remove", opts, errors.New("usage: profile remove NAME"))
		}
		delete(settings.Profiles, positional[1])
		if err := a.store.SaveSettings(settings); err != nil {
			return a.extendedError("profile remove", opts, err)
		}
		return 0
	default:
		return a.extendedError("profile", opts, fmt.Errorf("unknown subcommand %q", sub))
	}
}

func cleanBindingPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	if absolute, err := filepath.Abs(path); err == nil {
		return filepath.Clean(absolute)
	}
	return filepath.Clean(path)
}

func (a *Application) cmdBind(opts extendedOptions, positional []string) int {
	sub := "list"
	if len(positional) > 0 {
		sub = positional[0]
	}
	settings, err := a.loadSettings()
	if err != nil {
		return a.extendedError("bind", opts, err)
	}
	switch sub {
	case "list":
		if opts.JSON {
			return a.extendedResult("bind list", opts, settings.Bindings, nil)
		}
		for _, binding := range settings.Bindings {
			fmt.Fprintf(a.Out, "%s -> %s (%s)\n", binding.Path, binding.Profile, binding.Mode)
		}
		return 0
	case "set":
		if len(positional) < 3 {
			return a.extendedError("bind set", opts, errors.New("usage: bind set PATH PROFILE [--mode MODE]"))
		}
		path := cleanBindingPath(positional[1])
		if path == "" {
			return a.extendedError("bind set", opts, errors.New("path is required"))
		}
		mode := firstString(opts.Mode, "prompt")
		if !oneOf(mode, "prompt", "recommend", "disabled") {
			return a.extendedError("bind set", opts, errors.New("mode must be prompt, recommend, or disabled"))
		}
		replaced := false
		for i := range settings.Bindings {
			if settings.Bindings[i].Path == path {
				settings.Bindings[i] = Binding{Path: path, Profile: positional[2], Mode: mode}
				replaced = true
			}
		}
		if !replaced {
			settings.Bindings = append(settings.Bindings, Binding{Path: path, Profile: positional[2], Mode: mode})
		}
		if err := a.store.SaveSettings(settings); err != nil {
			return a.extendedError("bind set", opts, err)
		}
		if opts.JSON {
			return a.extendedResult("bind set", opts, Binding{Path: path, Profile: positional[2], Mode: mode}, nil)
		}
		fmt.Fprintf(a.Out, "%s -> %s (%s)\n", path, positional[2], mode)
		return 0
	case "resolve":
		if len(positional) < 2 {
			return a.extendedError("bind resolve", opts, errors.New("usage: bind resolve PATH"))
		}
		path := cleanBindingPath(positional[1])
		best := Binding{}
		for _, binding := range settings.Bindings {
			if (path == binding.Path || strings.HasPrefix(path, binding.Path+string(filepath.Separator))) && len(binding.Path) > len(best.Path) {
				best = binding
			}
		}
		if best.Path == "" {
			return a.extendedError("bind resolve", opts, errors.New("no binding matches path"))
		}
		if opts.JSON {
			return a.extendedResult("bind resolve", opts, best, nil)
		}
		fmt.Fprintf(a.Out, "%s -> %s (%s)\n", best.Path, best.Profile, best.Mode)
		return 0
	case "remove", "rm":
		if len(positional) < 2 {
			return a.extendedError("bind remove", opts, errors.New("usage: bind remove PATH"))
		}
		path := cleanBindingPath(positional[1])
		filtered := settings.Bindings[:0]
		for _, binding := range settings.Bindings {
			if binding.Path != path {
				filtered = append(filtered, binding)
			}
		}
		settings.Bindings = filtered
		if err := a.store.SaveSettings(settings); err != nil {
			return a.extendedError("bind remove", opts, err)
		}
		return 0
	default:
		return a.extendedError("bind", opts, fmt.Errorf("unknown subcommand %q", sub))
	}
}

func (a *Application) cmdUnbind(opts extendedOptions, positional []string) int {
	positional = append([]string{"remove"}, positional...)
	return a.cmdBind(opts, positional)
}

type recommendation struct {
	Email     string   `json:"email"`
	Name      string   `json:"name"`
	Score     int      `json:"score"`
	Ready     bool     `json:"ready"`
	Remaining float64  `json:"remaining_pct"`
	Wait      string   `json:"wait,omitempty"`
	Family    string   `json:"family,omitempty"`
	Reasons   []string `json:"reasons"`
}

func accountRemaining(account Account, family string) (float64, bool, string) {
	groups := quotaGroupHealths(account)
	var best *quotaGroupHealth
	for i := range groups {
		group := &groups[i]
		if family == "gemini" && group.id != "gemini" {
			continue
		}
		if (family == "claude" || family == "gpt") && group.id != "third_party" {
			continue
		}
		if best == nil || group.fraction > best.fraction {
			best = group
		}
	}
	if best != nil {
		return best.fraction * 100, true, best.id
	}
	return 0, false, ""
}

func (a *Application) buildRecommendations(ctx context.Context, accounts *Accounts, settings AppSettings, profileName, family, tag string) []recommendation {
	active := a.activeEmail(ctx, accounts, a.credentials.Current(ctx))
	if profileName != "" {
		if profile, ok := settings.Profiles[profileName]; ok {
			if family == "" {
				family = profile.Family
			}
			if active == "" {
				active = profile.Account
			}
		}
	}
	if family == "" {
		family = settings.Policy.PreferFamily
	}
	result := make([]recommendation, 0, accounts.Len())
	now := time.Now().UTC()
	for _, email := range accounts.Order {
		if tag != "" && !containsStringValue(settings.Tags[email], tag) {
			continue
		}
		account := accounts.ByEmail[email]
		remaining, known, group := accountRemaining(account, family)
		wait, waitKnown := accountCooldown(account, now, family)
		item := recommendation{Email: email, Name: getString(account, "name"), Remaining: remaining, Family: group, Reasons: []string{}}
		item.Ready = !waitKnown || wait <= 0
		if known {
			item.Reasons = append(item.Reasons, fmt.Sprintf("%s has %.1f%% remaining", group, remaining))
			if remaining < float64(settings.Policy.MinRemainingPct) {
				item.Reasons = append(item.Reasons, "below policy reserve")
			}
		} else {
			item.Reasons = append(item.Reasons, "no recent quota snapshot")
		}
		if waitKnown && wait > 0 {
			item.Wait = formatDuration(wait.Seconds())
			item.Reasons = append(item.Reasons, "cooldown until "+item.Wait)
		}
		if strings.EqualFold(email, active) {
			item.Score += 15
			item.Reasons = append(item.Reasons, "active account preference")
		}
		if item.Ready {
			item.Score += 40
		}
		if known {
			item.Score += int(remaining)
		}
		if remaining < float64(settings.Policy.MinRemainingPct) {
			item.Score -= 30
		}
		if family != "" && group == family {
			item.Score += 5
		}
		result = append(result, item)
	}
	sort.SliceStable(result, func(i, j int) bool {
		if result[i].Score != result[j].Score {
			return result[i].Score > result[j].Score
		}
		return result[i].Email < result[j].Email
	})
	return result
}

func (a *Application) cmdRecommend(ctx context.Context, opts extendedOptions, positional []string) int {
	settings, err := a.loadSettings()
	if err != nil {
		return a.extendedError("recommend", opts, err)
	}
	accounts, err := a.store.Load(true)
	if err != nil {
		return a.extendedError("recommend", opts, err)
	}
	if accounts.Len() == 0 {
		return a.extendedError("recommend", opts, errors.New("no accounts found"))
	}
	profile := opts.Profile
	if profile == "" && len(positional) > 0 {
		profile = positional[0]
	}
	if opts.Refresh {
		a.quota.Refresh(ctx, accounts, true, nil)
	}
	recommendations := a.buildRecommendations(ctx, accounts, settings, profile, opts.Family, opts.Tag)
	if len(recommendations) == 0 {
		return a.extendedError("recommend", opts, errors.New("no recommendation available"))
	}
	if opts.Apply {
		if !settings.Policy.AllowApply {
			return a.extendedError("recommend", opts, errors.New("policy.allow_apply is false; set it explicitly before applying a recommendation"))
		}
		chosen := accounts.ByEmail[recommendations[0].Email]
		token, tokenErr := a.accountToken(ctx, chosen)
		if tokenErr != nil || !a.credentials.Apply(ctx, token, recommendations[0].Email) {
			if tokenErr == nil {
				tokenErr = errors.New("credential apply failed")
			}
			return a.extendedError("recommend", opts, tokenErr)
		}
	}
	if opts.JSON {
		return a.extendedResult("recommend", opts, map[string]any{"profile": profile, "selected": recommendations[0], "candidates": recommendations, "applied": opts.Apply}, nil)
	}
	for i, item := range recommendations {
		marker := " "
		if i == 0 {
			marker = "*"
		}
		fmt.Fprintf(a.Out, "%s %s  %-30s  %3.0f%%  %s\n", marker, item.Email, item.Name, item.Remaining, strings.Join(item.Reasons, "; "))
	}
	return 0
}
