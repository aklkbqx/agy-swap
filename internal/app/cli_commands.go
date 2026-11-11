package app

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

func (a *Application) cmdList(ctx context.Context, args cliArgs) int {
	accounts, err := a.store.Load(true)
	if err != nil {
		return a.storeError(err)
	}
	if accounts.Len() == 0 {
		fmt.Fprintf(a.Out, "%sNo accounts added yet. Run 'agy-swap add' to save a Google account.%s\n", a.p.Gray, a.p.Reset)
		return 0
	}
	if args.refresh {
		for email, failure := range a.quota.Refresh(ctx, accounts, true, nil) {
			fmt.Fprintf(a.Err, "Quota refresh failed for %s: %s\n", email, failure)
		}
	}
	return a.renderAccountList(ctx, accounts, args)
}

func (a *Application) renderAccountList(ctx context.Context, accounts *Accounts, args cliArgs) int {
	if accounts.Len() == 0 {
		fmt.Fprintf(a.Out, "%sNo accounts added yet. Run 'agy-swap add' to save a Google account.%s\n", a.p.Gray, a.p.Reset)
		return 0
	}
	current := a.credentials.Current(ctx)
	active := a.activeEmail(ctx, accounts, current)
	now := a.renderTime()
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
		_, verify := accountEligibility(account)
		if !verify && len(quotaGroups(account)) > 0 {
			for _, rg := range quotaGroups(account) {
				group := getMap(rg)
				fmt.Fprintf(a.Out, "       ↳ %s\n", getString(group, "name"))
				for _, rb := range getSlice(group["buckets"]) {
					bucket := getMap(rb)
					fmt.Fprintf(a.Out, "          %-6s: %s\n", quotaBucketLabel(bucket), formatQuotaBar(bucket, a.p, now, 12))
				}
			}
		} else if !verify {
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
	currentHash := hashToken(current)
	for _, email := range accounts.Order {
		acc := accounts.ByEmail[email]
		if token, err := a.accountToken(ctx, acc); err == nil {
			if token == current || (currentHash != "" && hashToken(token) == currentHash) {
				if getString(acc, "token_hash") == "" && currentHash != "" {
					acc["token_hash"] = currentHash
					_ = a.store.Save(accounts)
				}
				return email
			}
		}
	}
	currentRefresh := tokenRefreshToken(current)
	currentRefreshHash := hashToken(currentRefresh)
	if currentRefresh != "" {
		matchedEmail := ""
		for _, email := range accounts.Order {
			acc := accounts.ByEmail[email]
			if token, err := a.accountToken(ctx, acc); err == nil && tokenRefreshToken(token) == currentRefresh {
				if matchedEmail != "" && !strings.EqualFold(matchedEmail, email) {
					matchedEmail = ""
					break
				}
				matchedEmail = email
			}
		}
		if matchedEmail != "" {
			acc := accounts.ByEmail[matchedEmail]
			if getString(acc, "refresh_hash") == "" && currentRefreshHash != "" {
				acc["refresh_hash"] = currentRefreshHash
				_ = a.store.Save(accounts)
			}
			return matchedEmail
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
	if current == "" || accounts == nil {
		return ""
	}
	currentHash := hashToken(current)
	for _, email := range accounts.Order {
		acc := accounts.ByEmail[email]
		if getString(acc, "token_data") == current {
			return email
		}
		if currentHash != "" && getString(acc, "token_hash") == currentHash {
			return email
		}
	}
	claimed := extractEmailHint(current)
	if claimed != "" {
		for _, email := range accounts.Order {
			if strings.EqualFold(email, claimed) {
				return email
			}
		}
		return claimed
	}
	currentRefresh := tokenRefreshToken(current)
	currentRefreshHash := hashToken(currentRefresh)
	if currentRefresh != "" {
		matchedEmail := ""
		for _, email := range accounts.Order {
			acc := accounts.ByEmail[email]
			matched := false
			if currentRefreshHash != "" && getString(acc, "refresh_hash") == currentRefreshHash {
				matched = true
			} else if stored := getString(acc, "token_data"); stored != "" && tokenRefreshToken(stored) == currentRefresh {
				matched = true
			}
			if matched {
				if matchedEmail != "" && !strings.EqualFold(matchedEmail, email) {
					return ""
				}
				matchedEmail = email
			}
		}
		if matchedEmail != "" {
			return matchedEmail
		}
	}
	return ""
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
	maxWait, found := quotaWait(account, now, family)
	for _, limit := range activeLimits(account, now) {
		if limitFamily := getString(limit.Limit, "family"); limitFamily == "" || familiesOverlap(account, limitFamily, family) {
			found = true
			if limit.Remaining > maxWait {
				maxWait = limit.Remaining
			}
		}
	}
	return maxWait, found
}
func selectNext(accounts *Accounts, active, family string) (Account, int) {
	if accounts.Len() == 0 {
		return nil, -1
	}
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
		if _, verify := accountEligibility(account); verify {
			continue
		}
		wait, known := accountCooldown(account, now, family)
		list = append(list, candidate{account, wait, known})
	}
	if len(list) == 0 {
		for offset := 1; offset <= accounts.Len(); offset++ {
			account := accounts.ByEmail[accounts.Order[(activeIndex+offset)%accounts.Len()]]
			wait, known := accountCooldown(account, now, family)
			list = append(list, candidate{account, wait, known})
		}
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

// nextEligible picks the best-ranked ready account other than the active one.
// Balanced ranking can put the active account first, and next must move away
// from it; it is kept only when no other account is ready.
func nextEligible(ranked []recommendation, active string) (recommendation, bool) {
	for _, item := range ranked {
		if item.Ready && !strings.EqualFold(item.Email, active) {
			return item, true
		}
	}
	if len(ranked) > 0 && ranked[0].Ready {
		return ranked[0], true
	}
	return recommendation{}, false
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
	settings, err := a.loadSettings()
	if err != nil {
		return a.storeError(err)
	}
	failures := a.quota.Refresh(ctx, accounts, true, nil)
	if failure := failures["store"]; failure != "" {
		return a.storeError(errors.New(failure))
	}
	for email, failure := range failures {
		fmt.Fprintf(a.Err, "Quota refresh failed for %s: %s\n", email, failure)
	}
	// Explicit next advances the rotation; recommendation sticky policy is for reuse.
	if settings.Policy.Name == "sticky" {
		settings.Policy.Name = "round-robin"
	}
	active := a.activeEmail(ctx, accounts, a.credentials.Current(ctx))
	choice, ok := nextEligible(rankAccounts(accounts, settings, active, "", args.family, ""), active)
	if !ok {
		fmt.Fprintln(a.Err, "No eligible account with fresh quota; use switch ACCOUNT for an explicit override.")
		return 1
	}
	next := accounts.ByEmail[choice.Email]
	lock, err := acquireFileLock(a.paths.SessionLock)
	if err != nil {
		return a.storeError(err)
	}
	defer func() { _ = lock.Close() }()
	fmt.Fprintf(a.Out, "Rotating to %s...\n", getString(next, "email"))
	token, tokenErr := a.accountToken(ctx, next)
	if tokenErr == nil && a.credentials.applyUnlocked(ctx, token, getString(next, "email")) {
		a.recordSwitch(getString(next, "email"))
		fmt.Fprintf(a.Out, "%s✓ Successfully auto-rotated to %s.%s\n", a.p.Green, getString(next, "email"), a.p.Reset)
		return 0
	}
	fmt.Fprintf(a.Err, "%s✕ Failed to rotate account.%s\n", a.p.Red, a.p.Reset)
	return 1
}

func (a *Application) cmdSwitch(ctx context.Context, args cliArgs) int {
	accounts, err := a.store.Load(false)
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
	fmt.Fprintf(a.Out, "Switching to %s%s%s %s<%s>%s...\n", a.p.Bold, getString(account, "name"), a.p.Reset, a.p.Gray, email, a.p.Reset)
	if a.applyAccount(ctx, token, email) {
		fmt.Fprintf(a.Out, "%s✓ Successfully switched to %s.%s\n", a.p.Green, email, a.p.Reset)
		if _, verify := accountEligibility(account); verify {
			fmt.Fprintf(a.Out, "%s⚠ %s needs Google verification before Antigravity can use it. Run 'agy-swap verify %s'.%s\n", a.p.Yellow, email, email, a.p.Reset)
		}
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
		if args.limitClear && !args.groupSet {
			delete(account, "quota_limits")
		} else {
			delete(limits, key)
			if len(limits) == 0 {
				delete(account, "quota_limits")
			} else {
				account["quota_limits"] = limits
			}
		}
		if err := a.store.Save(accounts); err != nil {
			return a.storeError(err)
		}
		fmt.Fprintf(a.Out, "%s✓ Cleared rate limit cooldown for '%s'.%s\n", a.p.Green, email, a.p.Reset)
		return 0
	}
	now := time.Now().UTC()
	name := titleWord(args.group)
	if args.group == "gpt" {
		name = "GPT"
	}
	limits[key] = map[string]any{"model": name, "family": args.group, "reset_at": isoTime(now.Add(duration)), "observed_at": isoTime(now), "source": "manual"}
	account["quota_limits"] = limits
	if err := a.store.Save(accounts); err != nil {
		return a.storeError(err)
	}
	fmt.Fprintf(a.Out, "%s✓ Set %s rate limit cooldown for '%s' (%s).%s\n", a.p.Green, titleWord(args.group), email, now.Add(duration).Format("15:04:05 UTC"), a.p.Reset)
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
	code := a.renderAccountList(ctx, accounts, args)
	if len(failures) > 0 {
		fmt.Fprintf(a.Err, "%sUsage refresh failed for %d account(s); cached data kept.%s\n", a.p.Yellow, len(failures), a.p.Reset)
	}
	return code
}

func (a *Application) cmdRemove(ctx context.Context, args cliArgs) int {
	accounts, err := a.store.Load(false)
	if err != nil {
		return a.storeError(err)
	}
	if accounts.Len() == 0 {
		fmt.Fprintf(a.Out, "%sNo accounts found.%s\n", a.p.Gray, a.p.Reset)
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
	email, _ := resolveConfiguredTarget(args.account, accounts, settings)
	if email == "" {
		fmt.Fprintf(a.Err, "%sAccount '%s' not found.%s\n", a.p.Red, args.account, a.p.Reset)
		return 1
	}
	answer, readErr := a.readLine(ctx, fmt.Sprintf("Are you sure you want to remove '%s'? [y/N]: ", email))
	if readErr != nil {
		fmt.Fprintln(a.Out, "Cancelled.")
		return 1
	}
	if strings.ToLower(answer) == "y" {
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
