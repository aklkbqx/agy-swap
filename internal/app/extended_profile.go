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
		if value != "" && !isQuotaFamily(value) {
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
	case "ui.split_offset", "split_offset":
		parsed, err := parseInt(-40, 40)
		if err != nil {
			return err
		}
		settings.UI.SplitOffset = parsed
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
			return a.extendedError("profile set", opts, errors.New("usage: profile set NAME ACCOUNT [--family FAMILY] [--policy sticky|balanced|round-robin] [--reserve ACCOUNT,...] [--threshold PERCENT]"))
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
		if opts.Policy != "" {
			profile.Policy = opts.Policy
		}
		if opts.ReservesSet {
			profile.ReserveAccounts = nil
			for _, target := range opts.Reserves {
				reserve, err := resolveConfiguredTarget(target, accounts, settings)
				if err != nil || reserve == "" {
					return a.extendedError("profile set", opts, errors.New("reserve account not found"))
				}
				profile.ReserveAccounts = append(profile.ReserveAccounts, reserve)
			}
		}
		if opts.Family != "" {
			profile.Family = opts.Family
		}
		if profile.Policy == "" {
			profile.Policy = settings.Policy.Name
		}
		if opts.Threshold >= 0 {
			profile.NotifyThreshold = opts.Threshold
			profile.NotifyThresholdSet = true
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
		if _, ok := settings.Profiles[positional[2]]; !ok {
			return a.extendedError("bind set", opts, errors.New("profile not found"))
		}
		mode := firstString(opts.Mode, "prompt")
		if !oneOf(mode, "prompt", "recommend", "auto", "disabled") {
			return a.extendedError("bind set", opts, errors.New("mode must be prompt, recommend, auto, or disabled"))
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
		best := resolveBinding(settings, path)
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
	var limiting *quotaGroupHealth
	for i := range groups {
		group := &groups[i]
		if !poolInFamily(account, group.id, family) {
			continue
		}
		if limiting == nil || group.fraction < limiting.fraction {
			limiting = group
		}
	}
	if limiting != nil {
		return limiting.fraction * 100, true, limiting.id
	}
	return 0, false, ""
}

func (a *Application) buildRecommendations(ctx context.Context, accounts *Accounts, settings AppSettings, profileName, family, tag string) []recommendation {
	active := ""
	if a.credentials != nil {
		active = a.activeEmail(ctx, accounts, a.credentials.Current(ctx))
	}
	return rankAccounts(accounts, settings, active, profileName, family, tag)
}

// rankAccounts orders accounts for a policy relative to the active account:
// ready accounts first, then profile primaries before reserves, then score.
func rankAccounts(accounts *Accounts, settings AppSettings, active, profileName, family, tag string) []recommendation {
	policy := settings.Policy.Name
	profile, hasProfile := settings.Profiles[profileName]
	if hasProfile {
		if family == "" {
			family = profile.Family
		}
		if profile.Policy != "" {
			policy = profile.Policy
		}
	}
	if family == "" {
		family = settings.Policy.PreferFamily
	}
	result := make([]recommendation, 0, accounts.Len())
	now := time.Now().UTC()
	activeIndex := -1
	for i, email := range accounts.Order {
		if strings.EqualFold(email, active) {
			activeIndex = i
		}
	}
	for index, email := range accounts.Order {
		if tag != "" && !containsStringValue(settings.Tags[email], tag) {
			continue
		}
		if hasProfile && len(profile.ReserveAccounts) > 0 && email != profile.Account && !containsStringValue(profile.ReserveAccounts, email) {
			continue
		}
		account := accounts.ByEmail[email]
		remaining, known, group := accountRemaining(account, family)
		wait, waitKnown := accountCooldown(account, now, family)
		age, ageKnown := quotaAge(account, now)
		_, needsVerify := accountEligibility(account)
		item := recommendation{Email: email, Name: getString(account, "name"), Remaining: remaining, Family: group, Reasons: []string{}}
		item.Ready = !needsVerify && known && ageKnown && age <= 2*time.Minute && (!waitKnown || wait <= 0) && remaining > 0 && remaining >= float64(settings.Policy.MinRemainingPct)
		if needsVerify {
			item.Reasons = append(item.Reasons, "needs Google verification")
		}
		if known {
			item.Reasons = append(item.Reasons, fmt.Sprintf("%s has %.1f%% remaining", group, remaining))
		} else {
			item.Reasons = append(item.Reasons, "quota unknown")
		}
		if !ageKnown || age > 2*time.Minute {
			item.Reasons = append(item.Reasons, "quota needs refresh")
		}
		if remaining < float64(settings.Policy.MinRemainingPct) {
			item.Reasons = append(item.Reasons, "below policy reserve")
		}
		if waitKnown && wait > 0 {
			item.Wait = formatDuration(wait.Seconds())
			item.Reasons = append(item.Reasons, "cooldown for "+item.Wait)
		}
		item.Score = int(remaining)
		if needsVerify {
			item.Score -= 1000
		}
		switch policy {
		case "sticky":
			preferred := active
			if hasProfile {
				preferred = profile.Account
			}
			if strings.EqualFold(email, preferred) {
				item.Score += 200
				item.Reasons = append(item.Reasons, "sticky account preference")
			}
		case "round-robin":
			item.Score = accounts.Len() - (index-activeIndex-1+accounts.Len())%accounts.Len()
			item.Reasons = append(item.Reasons, "next in account rotation")
		}
		if hasProfile && containsStringValue(profile.ReserveAccounts, email) {
			item.Reasons = append(item.Reasons, "reserve account")
		}
		result = append(result, item)
	}
	sort.SliceStable(result, func(i, j int) bool {
		if result[i].Ready != result[j].Ready {
			return result[i].Ready
		}
		if hasProfile {
			ir, jr := containsStringValue(profile.ReserveAccounts, result[i].Email), containsStringValue(profile.ReserveAccounts, result[j].Email)
			if ir != jr {
				return !ir
			}
		}
		return result[i].Score > result[j].Score
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
	if profile != "" {
		if _, ok := settings.Profiles[profile]; !ok {
			return a.extendedError("recommend", opts, errors.New("profile not found"))
		}
	}
	if opts.Refresh || opts.Apply {
		failures := a.quota.Refresh(ctx, accounts, true, nil)
		if failure := failures["store"]; failure != "" {
			return a.extendedError("recommend", opts, errors.New(failure))
		}
		for email := range failures {
			delete(accounts.ByEmail[email], "quota_snapshot")
		}
	}
	recommendations := a.buildRecommendations(ctx, accounts, settings, profile, opts.Family, opts.Tag)
	if len(recommendations) == 0 {
		return a.extendedError("recommend", opts, errors.New("no recommendation available"))
	}
	if opts.Apply {
		if !settings.Policy.AllowApply {
			return a.extendedError("recommend", opts, errors.New("policy.allow_apply is false; set it explicitly before applying a recommendation"))
		}
		if !recommendations[0].Ready {
			return a.extendedError("recommend", opts, errors.New("no eligible account with fresh quota above the policy reserve"))
		}
		chosen := accounts.ByEmail[recommendations[0].Email]
		token, tokenErr := a.accountToken(ctx, chosen)
		if tokenErr != nil || !a.applyAccount(ctx, token, recommendations[0].Email) {
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
