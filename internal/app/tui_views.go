package app

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

// tuiAction is intentionally data-only. The event loop owns the side effects,
// which keeps the palette renderable in tests and makes every action share the
// same dispatcher as keyboard shortcuts.
type tuiAction struct {
	ID          string
	Label       string
	Description string
	Shortcut    string
	Section     string
	Enabled     bool
}

func tuiActions(state *tuiState) []tuiAction {
	hasAccounts := state != nil && state.accounts != nil && state.accounts.Len() > 0
	return []tuiAction{
		{ID: "dashboard", Label: "Dashboard", Description: "Return to the account dashboard", Shortcut: "g", Section: "Navigate", Enabled: true},
		{ID: "quota", Label: "Quota overview", Description: "Inspect usage and reset windows", Shortcut: "v", Section: "Navigate", Enabled: hasAccounts},
		{ID: "profiles", Label: "Profiles", Description: "Create and manage named account profiles", Shortcut: "p", Section: "Navigate", Enabled: true},
		{ID: "history", Label: "History", Description: "Review account switches and quota events", Shortcut: "h", Section: "Navigate", Enabled: true},
		{ID: "settings", Label: "Settings", Description: "Edit policy, notifications, and retention", Shortcut: "s", Section: "Navigate", Enabled: true},
		{ID: "doctor", Label: "Run health check", Description: "Check storage, vault, session, and platform", Shortcut: "o", Section: "Tools", Enabled: true},
		{ID: "backup", Label: "Backup & restore", Description: "Export, import, or verify account data", Shortcut: "b", Section: "Tools", Enabled: true},
		{ID: "recommend", Label: "Recommend best account", Description: "Score accounts using policy, tags, and cooldowns", Shortcut: "", Section: "Tools", Enabled: hasAccounts},
		{ID: "forecast", Label: "Forecast quota", Description: "Show remaining capacity and confidence", Shortcut: "", Section: "Tools", Enabled: hasAccounts},
		{ID: "watch-once", Label: "Run notification check", Description: "Check thresholds once and notify when configured", Shortcut: "", Section: "Tools", Enabled: hasAccounts},
		{ID: "metrics", Label: "Export metrics", Description: "Print a Prometheus-compatible snapshot", Shortcut: "", Section: "Tools", Enabled: true},
		{ID: "run-now", Label: "Run AGY now", Description: "Launch the configured Antigravity CLI immediately", Shortcut: "", Section: "Tools", Enabled: true},
		{ID: "statusline-install", Label: "Install statusline hint", Description: "Save the statusline integration command", Shortcut: "", Section: "Integrations", Enabled: true},
		{ID: "completion", Label: "Print shell completion", Description: "Generate bash completion for setup", Shortcut: "", Section: "Integrations", Enabled: true},
		{ID: "add-account", Label: "Add account", Description: "Sign in and save another account", Shortcut: "a", Section: "Accounts", Enabled: true},
		{ID: "switch-account", Label: "Switch selected account", Description: "Make the selected account active", Shortcut: "enter", Section: "Accounts", Enabled: hasAccounts},
		{ID: "next-account", Label: "Choose next available", Description: "Pick the next healthy account", Shortcut: "n", Section: "Accounts", Enabled: hasAccounts},
		{ID: "refresh", Label: "Refresh quota", Description: "Fetch fresh usage from the provider", Shortcut: "r", Section: "Accounts", Enabled: hasAccounts},
		{ID: "edit-tags", Label: "Edit account tags", Description: "Add searchable labels to the selected account", Shortcut: "e", Section: "Accounts", Enabled: hasAccounts},
		{ID: "toggle-tier", Label: "Toggle manual tier", Description: "Set or clear a local tier override", Shortcut: "t", Section: "Accounts", Enabled: hasAccounts},
		{ID: "migrate-vault", Label: "Migrate secrets to OS vault", Description: "Move legacy plaintext tokens into secure storage", Shortcut: "m", Section: "Security", Enabled: hasAccounts},
		{ID: "profile-create", Label: "Create profile", Description: "Save a named account preset", Shortcut: "c", Section: "Profiles", Enabled: hasAccounts},
		{ID: "profile-edit", Label: "Edit selected profile", Description: "Change the selected profile", Shortcut: "e", Section: "Profiles", Enabled: state != nil && state.view == tuiViewProfiles && len(state.profileNames) > 0},
		{ID: "profile-remove", Label: "Remove selected profile", Description: "Delete the selected profile", Shortcut: "d", Section: "Profiles", Enabled: state != nil && state.view == tuiViewProfiles && len(state.profileNames) > 0},
		{ID: "history-clear", Label: "Clear history", Description: "Remove local action history", Shortcut: "c", Section: "History", Enabled: state != nil && state.view == tuiViewHistory && len(state.history) > 0},
		{ID: "history-export", Label: "Export history", Description: "Write local history as JSON", Shortcut: "x", Section: "History", Enabled: state != nil && state.view == tuiViewHistory && len(state.history) > 0},
		{ID: "settings-edit", Label: "Edit settings", Description: "Change policy and notification defaults", Shortcut: "e", Section: "Settings", Enabled: state != nil && state.view == tuiViewSettings},
		{ID: "settings-reset", Label: "Reset settings", Description: "Restore safe default policy and retention", Shortcut: "", Section: "Settings", Enabled: state != nil && state.view == tuiViewSettings},
		{ID: "alias-create", Label: "Create alias", Description: "Name an account or profile", Shortcut: "a", Section: "Settings", Enabled: state != nil && state.view == tuiViewSettings},
		{ID: "binding-create", Label: "Create project binding", Description: "Bind a folder to a profile", Shortcut: "b", Section: "Settings", Enabled: state != nil && state.view == tuiViewSettings},
		{ID: "target-create", Label: "Register target", Description: "Add a compatible CLI target", Shortcut: "t", Section: "Settings", Enabled: state != nil && state.view == tuiViewSettings},
		{ID: "backup-export", Label: "Export backup", Description: "Write metadata-only or encrypted backup", Shortcut: "x", Section: "Backup", Enabled: state != nil && state.view == tuiViewBackup},
		{ID: "backup-import", Label: "Import backup", Description: "Restore accounts and settings", Shortcut: "i", Section: "Backup", Enabled: state != nil && state.view == tuiViewBackup},
		{ID: "backup-verify", Label: "Verify backup", Description: "Check a backup file without importing", Shortcut: "v", Section: "Backup", Enabled: state != nil && state.view == tuiViewBackup},
		{ID: "update-check", Label: "Check for update", Description: "See whether a matching release asset is available", Shortcut: "", Section: "System", Enabled: true},
		{ID: "update", Label: "Install latest update", Description: "Download, verify, and install the latest release", Shortcut: "u", Section: "System", Enabled: true},
		{ID: "quit", Label: "Quit", Description: "Close the interactive console", Shortcut: "q", Section: "System", Enabled: true},
	}
}

func (s *tuiState) paletteActions() []tuiAction {
	if s == nil {
		return nil
	}
	query := strings.ToLower(strings.TrimSpace(s.paletteQuery))
	result := make([]tuiAction, 0)
	for _, action := range tuiActions(s) {
		if query != "" {
			text := strings.ToLower(action.Label + " " + action.Description + " " + action.Section)
			if !strings.Contains(text, query) {
				continue
			}
		}
		result = append(result, action)
	}
	if len(result) == 0 {
		s.paletteIndex = 0
	} else if s.paletteIndex >= len(result) {
		s.paletteIndex = len(result) - 1
	} else if s.paletteIndex < 0 {
		s.paletteIndex = 0
	}
	return result
}

func (s *tuiState) beginPalette() {
	s.paletteQuery = ""
	s.paletteIndex = 0
	s.mode = tuiPalette
}

func (s *tuiState) movePalette(delta int) {
	items := s.paletteActions()
	if len(items) == 0 {
		return
	}
	index := (s.paletteIndex + delta) % len(items)
	if index < 0 {
		index += len(items)
	}
	// Do not make disabled actions dead ends when a user searches for one.
	for i := 0; i < len(items); i++ {
		candidate := items[index]
		if candidate.Enabled {
			s.paletteIndex = index
			return
		}
		index = (index + map[bool]int{true: 1, false: -1}[delta >= 0]) % len(items)
		if index < 0 {
			index += len(items)
		}
	}
}

func (s *tuiState) selectedPaletteAction() (tuiAction, bool) {
	items := s.paletteActions()
	if len(items) == 0 || s.paletteIndex < 0 || s.paletteIndex >= len(items) {
		return tuiAction{}, false
	}
	item := items[s.paletteIndex]
	return item, item.Enabled
}

func formField(form *tuiFormState, key string) string {
	if form == nil {
		return ""
	}
	for _, field := range form.Fields {
		if field.Key == key {
			return field.Value
		}
	}
	return ""
}

func formBool(form *tuiFormState, key string) bool {
	value := strings.ToLower(strings.TrimSpace(formField(form, key)))
	return value == "on" || value == "true" || value == "yes" || value == "1"
}

func (s *tuiState) formKey(key string) (submit, cancel bool) {
	if s.form == nil {
		return false, true
	}
	form := s.form
	if len(form.Fields) == 0 {
		return key == "enter", key == "esc"
	}
	if key == "esc" {
		return false, true
	}
	if key == "up" || key == "shift-tab" {
		form.Index = (form.Index - 1 + len(form.Fields)) % len(form.Fields)
		return false, false
	}
	if key == "down" || key == "tab" {
		form.Index = (form.Index + 1) % len(form.Fields)
		return false, false
	}
	field := &form.Fields[form.Index]
	if key == "left" || key == "right" {
		if len(field.Options) > 0 {
			index := 0
			for i, option := range field.Options {
				if option == field.Value {
					index = i
					break
				}
			}
			delta := 1
			if key == "left" {
				delta = -1
			}
			index = (index + delta + len(field.Options)) % len(field.Options)
			field.Value = field.Options[index]
		}
		return false, false
	}
	if key == "enter" {
		if form.Index < len(form.Fields)-1 {
			form.Index++
			return false, false
		}
		return true, false
	}
	if key == "backspace" {
		if len(field.Value) > 0 {
			field.Value = field.Value[:len(field.Value)-1]
		}
		return false, false
	}
	if key == "ctrl-u" {
		field.Value = ""
		return false, false
	}
	if key == "ctrl-w" {
		field.Value = strings.TrimRight(field.Value, " \t")
		if index := strings.LastIndexAny(field.Value, " \t"); index >= 0 {
			field.Value = field.Value[:index]
		} else {
			field.Value = ""
		}
		return false, false
	}
	if len(key) == 1 && key[0] >= 32 && key[0] != 127 && len(field.Options) == 0 {
		field.Value += key
	}
	return false, false
}

func (a *Application) tuiPaletteLines(state *tuiState, width int) []string {
	items := state.paletteActions()
	lines := []string{"ACTION PALETTE", "", a.p.Cyan + ":" + tuiText(state.paletteQuery) + "▌" + a.p.Reset, ""}
	if len(items) == 0 {
		return append(lines, a.p.Gray+"No matching actions"+a.p.Reset)
	}
	start := maxInt(0, state.paletteIndex-4)
	end := minInt(len(items), start+9)
	if end-start < 9 {
		start = maxInt(0, end-9)
	}
	lastSection := ""
	for index := start; index < end; index++ {
		item := items[index]
		if item.Section != lastSection {
			lines = append(lines, a.p.Gray+tuiText(item.Section)+a.p.Reset)
			lastSection = item.Section
		}
		marker := "  "
		if index == state.paletteIndex {
			marker = a.p.Orange + "> " + a.p.Reset
		}
		color := a.p.White
		if !item.Enabled {
			color = a.p.DarkGray
		}
		shortcut := ""
		if item.Shortcut != "" {
			shortcut = a.p.Gray + " [" + item.Shortcut + "]" + a.p.Reset
		}
		lines = append(lines, marker+color+tuiText(item.Label)+a.p.Reset+shortcut)
		lines = append(lines, "   "+a.p.Gray+tuiText(item.Description)+a.p.Reset)
	}
	lines = append(lines, "", a.p.Gray+"↑↓ Move  Enter Run  Type Filter  Esc Close"+a.p.Reset)
	for i := range lines {
		lines[i] = truncateVisible(lines[i], maxInt(18, width-8), a.p)
	}
	return lines
}

func (a *Application) tuiFormLines(state *tuiState, width int) []string {
	form := state.form
	if form == nil {
		return []string{"FORM", "", "No form is active."}
	}
	lines := []string{a.p.Bold + tuiText(form.Title) + a.p.Reset}
	if form.Description != "" {
		lines = append(lines, a.p.Gray+tuiText(form.Description)+a.p.Reset)
	}
	lines = append(lines, "")
	for index, field := range form.Fields {
		marker := "  "
		if index == form.Index {
			marker = a.p.Orange + "> " + a.p.Reset
		}
		value := field.Value
		if field.Secret && value != "" {
			value = strings.Repeat("•", maxInt(6, len([]rune(value))))
		}
		if value == "" {
			value = a.p.DarkGray + "(type a value)" + a.p.Reset
		}
		lines = append(lines, marker+a.p.Cyan+tuiText(field.Label)+a.p.Reset+"  "+fitVisible(value, maxInt(10, width-26), a.p))
		if index == form.Index && field.Help != "" {
			lines = append(lines, "   "+a.p.Gray+tuiText(field.Help)+a.p.Reset)
		}
	}
	lines = append(lines, "", a.p.Gray+"↑↓ Field  ←→ Choose  Enter Next/Save  Esc Cancel"+a.p.Reset)
	for i := range lines {
		lines[i] = truncateVisible(lines[i], maxInt(18, width-8), a.p)
	}
	return lines
}

func (a *Application) tuiConfirmActionLines(state *tuiState, width int) []string {
	return []string{
		a.p.Bold + tuiText(firstString(state.confirmTitle, "Confirm action")) + a.p.Reset,
		"",
		"This action may change local configuration or data.",
		"",
		a.p.Yellow + "[y] Confirm    [n] Cancel    [Esc] Back" + a.p.Reset,
		" ",
	}
}

func (a *Application) tuiViewBody(state *tuiState, width, height int) []string {
	g := newTUIGeometry(width, height+8)
	g.bodyRows = maxInt(1, height)
	rows := a.tuiViewRows(state, g.contentWidth, g.bodyRows)
	framed := make([]string, 0, len(rows))
	for _, row := range rows {
		framed = append(framed, frameRow(row, g, a.p))
	}
	return fitBodyLines(framed, g, a.p)
}

func (a *Application) tuiViewRows(state *tuiState, width, height int) []string {
	title, subtitle := tuiViewTitle(state.view), tuiViewSubtitle(state.view)
	rows := []string{a.tuiSectionTitle(title)}
	if subtitle != "" {
		rows = append(rows, a.p.Gray+tuiText(subtitle)+a.p.Reset)
	}
	rows = append(rows, a.p.DarkGray+strings.Repeat("─", maxInt(1, width))+a.p.Reset)
	remaining := maxInt(1, height-len(rows))
	var content []string
	switch state.view {
	case tuiViewQuota:
		content = a.tuiQuotaViewRows(state, width, remaining)
	case tuiViewProfiles:
		content = a.tuiProfileViewRows(state, width, remaining)
	case tuiViewHistory:
		content = a.tuiHistoryViewRows(state, width, remaining)
	case tuiViewSettings:
		content = a.tuiSettingsViewRows(state, width, remaining)
	case tuiViewDoctor:
		content = a.tuiDoctorViewRows(state, width, remaining)
	case tuiViewBackup:
		content = a.tuiBackupViewRows(state, width, remaining)
	}
	return append(rows, content...)
}

func tuiViewTitle(view tuiView) string {
	switch view {
	case tuiViewQuota:
		return "QUOTA OVERVIEW"
	case tuiViewProfiles:
		return "PROFILES"
	case tuiViewHistory:
		return "HISTORY"
	case tuiViewSettings:
		return "SETTINGS"
	case tuiViewDoctor:
		return "HEALTH CHECK"
	case tuiViewBackup:
		return "BACKUP & RESTORE"
	default:
		return "DASHBOARD"
	}
}

func tuiViewSubtitle(view tuiView) string {
	switch view {
	case tuiViewQuota:
		return "See remaining capacity and reset windows for every saved account."
	case tuiViewProfiles:
		return "Profiles make repeatable account selection simple."
	case tuiViewHistory:
		return "Recent switches, refreshes, and health events stay local."
	case tuiViewSettings:
		return "Local policy, notifications, retention, and integration settings."
	case tuiViewDoctor:
		return "A safe read-only check of the local installation."
	case tuiViewBackup:
		return "Move this setup between machines without remembering CLI flags."
	default:
		return ""
	}
}

func (a *Application) tuiQuotaViewRows(state *tuiState, width, height int) []string {
	rows := []string{}
	if state.accounts == nil || state.accounts.Len() == 0 {
		return []string{a.p.Yellow + "No accounts yet." + a.p.Reset + "  Press a to add one."}
	}
	for _, email := range state.visibleEmails() {
		if len(rows) >= height {
			break
		}
		account := state.accounts.ByEmail[email]
		name := firstString(tuiText(getString(account, "name")), "Google User")
		health := accountHealthCompact(account, time.Now())
		color := a.tuiHealthColor(tuiHealthToneForGroups(quotaGroupHealths(account), account, time.Now()))
		rows = append(rows, color+"● "+tuiText(name)+a.p.Reset+"  "+a.p.Gray+tuiText(email)+a.p.Reset+"  "+color+tuiText(health)+a.p.Reset)
	}
	if email, _, ok := state.selectedAccount(); ok && len(rows) < height {
		rows = append(rows, "", a.p.Bold+"Selected account"+a.p.Reset+"  "+tuiText(email))
		for _, detail := range a.tuiDetailLines(state, width, maxInt(1, height-len(rows))) {
			if len(rows) >= height {
				break
			}
			rows = append(rows, detail)
		}
	}
	return rows
}

func (a *Application) tuiProfileViewRows(state *tuiState, width, height int) []string {
	if len(state.profileNames) == 0 {
		return []string{a.p.Gray + "No profiles yet." + a.p.Reset, "", "Press c to create a profile for the selected account."}
	}
	rows := make([]string, 0, height)
	for index, name := range state.profileNames {
		if len(rows) >= height {
			break
		}
		profile := state.settings.Profiles[name]
		marker := "  "
		if index == state.profileIndex {
			marker = a.p.Orange + "> " + a.p.Reset
		}
		family := profile.Family
		if family == "" {
			family = "any model family"
		}
		rows = append(rows, marker+a.p.Bold+tuiText(name)+a.p.Reset+"  "+a.p.Gray+tuiText(profile.Account)+" · "+tuiText(family)+a.p.Reset)
	}
	if state.profileIndex >= 0 && state.profileIndex < len(state.profileNames) && len(rows) < height {
		name := state.profileNames[state.profileIndex]
		profile := state.settings.Profiles[name]
		rows = append(rows, "", a.p.Cyan+"Selected profile"+a.p.Reset, "  account: "+tuiText(profile.Account), "  policy: "+tuiText(firstString(profile.Policy, state.settings.Policy.Name)))
	}
	return rows
}

func (a *Application) tuiHistoryViewRows(state *tuiState, width, height int) []string {
	if len(state.history) == 0 {
		return []string{a.p.Gray + "No local history yet." + a.p.Reset, "", "History is written as actions happen."}
	}
	rows := make([]string, 0, height)
	for index, event := range state.history {
		if len(rows) >= height {
			break
		}
		marker := "  "
		if index == state.historyIndex {
			marker = a.p.Orange + "> " + a.p.Reset
		}
		at := event.At
		if parsed, err := parseUTC(event.At); err == nil {
			at = parsed.Local().Format("2006-01-02 15:04")
		}
		target := event.Email
		if target == "" {
			target = "system"
		}
		rows = append(rows, marker+a.p.Gray+at+a.p.Reset+"  "+a.p.Cyan+fitVisible(event.Kind, 16, a.p)+a.p.Reset+"  "+tuiText(target))
	}
	return rows
}

func (a *Application) tuiSettingsViewRows(state *tuiState, width, height int) []string {
	if !state.settingsLoaded {
		return []string{a.p.Yellow + "Loading settings…" + a.p.Reset}
	}
	s := state.settings
	rows := []string{
		"Policy",
		"  name: " + tuiText(s.Policy.Name),
		"  preferred family: " + firstString(tuiText(s.Policy.PreferFamily), "any"),
		fmt.Sprintf("  minimum remaining: %d%% · auto apply: %s", s.Policy.MinRemainingPct, onOff(s.Policy.AllowApply)),
		"Notifications",
		"  enabled: " + onOff(s.Notifications.Enabled) + " · threshold: " + strconv.Itoa(s.Notifications.Threshold) + "%",
		"  reset: " + onOff(s.Notifications.Reset) + " · auth: " + onOff(s.Notifications.AuthFailure) + " · stale: " + onOff(s.Notifications.Stale),
		"History",
		"  enabled: " + onOff(s.History.Enabled) + " · retention: " + strconv.Itoa(s.History.RetentionDays) + " days · max: " + strconv.Itoa(s.History.MaxBytes) + " bytes",
		"Integrations",
		fmt.Sprintf("  aliases: %d · tags: %d · bindings: %d · targets: %d", len(s.Aliases), len(s.Tags), len(s.Bindings), len(s.Targets)),
	}
	if len(rows) > height {
		rows = rows[:height]
	}
	return rows
}

func (a *Application) tuiDoctorViewRows(state *tuiState, width, height int) []string {
	if len(state.doctorChecks) == 0 {
		return []string{a.p.Gray + "Press r or Enter to run the health check." + a.p.Reset}
	}
	rows := make([]string, 0, height)
	for _, check := range state.doctorChecks {
		if len(rows) >= height {
			break
		}
		symbol, color := "✓", a.p.Green
		if check.Status == "warning" {
			symbol, color = "!", a.p.Yellow
		}
		if check.Status == "error" {
			symbol, color = "✕", a.p.Red
		}
		rows = append(rows, color+symbol+a.p.Reset+" "+a.p.Cyan+tuiText(check.Name)+a.p.Reset+"  "+tuiText(check.Message))
	}
	if len(rows) < height {
		status := "healthy"
		color := a.p.Green
		if !state.doctorHealthy {
			status, color = "needs attention", a.p.Red
		}
		rows = append(rows, "", color+"Overall: "+status+a.p.Reset)
	}
	return rows
}

func (a *Application) tuiBackupViewRows(state *tuiState, width, height int) []string {
	path := state.backupPath
	if path == "" {
		path = "agy-swap-backup.json"
	}
	rows := []string{
		"Export a metadata-only backup with x.",
		"Export an encrypted backup with e.",
		"Import a backup with i, or verify one with v.",
		"",
		"Metadata-only backups never contain account tokens.",
		"Encrypted exports require a passphrase of 8+ characters.",
		"",
		"Last path: " + tuiText(path),
	}
	if state.job != nil && state.job.Done && state.job.Message != "" {
		rows = append(rows, "", a.p.Green+"✓ "+tuiText(state.job.Message)+a.p.Reset)
	}
	if len(rows) > height {
		rows = rows[:height]
	}
	return rows
}

func onOff(value bool) string {
	if value {
		return "on"
	}
	return "off"
}

func (a *Application) beginTUIView(state *tuiState, view tuiView) {
	state.view = view
	state.mode = tuiBrowse
	state.form = nil
	state.paletteQuery = ""
	switch view {
	case tuiViewProfiles:
		state.settings, _ = a.loadSettings()
		state.settingsLoaded = true
		state.profileNames = state.profileNames[:0]
		for name := range state.settings.Profiles {
			state.profileNames = append(state.profileNames, name)
		}
		sort.Strings(state.profileNames)
		if state.profileIndex >= len(state.profileNames) {
			state.profileIndex = maxInt(0, len(state.profileNames)-1)
		}
	case tuiViewSettings:
		settings, err := a.loadSettings()
		if err != nil {
			state.settingsLoaded = false
			state.message, state.messageType = err.Error(), "error"
		} else {
			state.settings, state.settingsLoaded = settings, true
		}
	case tuiViewHistory:
		events, err := a.readHistory(80)
		if err != nil {
			state.message, state.messageType = err.Error(), "error"
		} else {
			state.history = events
			state.historyIndex = minInt(state.historyIndex, maxInt(0, len(events)-1))
		}
	case tuiViewDoctor:
		state.doctorChecks = nil
		state.doctorHealthy = false
	case tuiViewBackup:
		if state.backupPath == "" {
			state.backupPath = "agy-swap-backup.json"
		}
	}
}

func (a *Application) beginTUIForm(state *tuiState, kind string) {
	settings := state.settings
	if !state.settingsLoaded {
		settings, _ = a.loadSettings()
		state.settings = settings
		state.settingsLoaded = true
	}
	form := &tuiFormState{Kind: kind, PreviousView: state.view}
	switch kind {
	case "profile-create", "profile-edit":
		name, account, family, threshold := "", state.active, "", ""
		if kind == "profile-edit" && state.profileIndex >= 0 && state.profileIndex < len(state.profileNames) {
			name = state.profileNames[state.profileIndex]
			profile := settings.Profiles[name]
			account, family = profile.Account, profile.Family
			if profile.NotifyThreshold > 0 {
				threshold = strconv.Itoa(profile.NotifyThreshold)
			}
		}
		form.Title = "PROFILE"
		form.Description = "Give a saved account a short, repeatable name."
		form.Fields = []tuiFormField{
			{Key: "name", Label: "Name", Value: name, Help: "Letters, numbers, dots, dashes, or underscores."},
			{Key: "account", Label: "Account", Value: account, Help: "Email, alias, or another saved profile."},
			{Key: "family", Label: "Family", Value: family, Options: []string{"", "claude", "gemini", "gpt"}},
			{Key: "threshold", Label: "Notify %", Value: threshold, Help: "Optional 0–100 threshold; leave blank for default."},
		}
	case "tags":
		email, _, ok := state.selectedAccount()
		if !ok {
			return
		}
		form.Title = "ACCOUNT TAGS"
		form.Description = "Separate tags with commas; they stay local to this machine."
		form.Fields = []tuiFormField{{Key: "tags", Label: "Tags", Value: strings.Join(settings.Tags[email], ", ")}}
	case "settings":
		family := settings.Policy.PreferFamily
		form.Title = "SETTINGS"
		form.Description = "Use arrows for choices, then Enter to save."
		form.Fields = []tuiFormField{
			{Key: "policy.name", Label: "Policy", Value: settings.Policy.Name, Options: []string{"sticky", "balanced", "round-robin"}},
			{Key: "policy.prefer_family", Label: "Family", Value: family, Options: []string{"", "claude", "gemini", "gpt"}},
			{Key: "policy.min_remaining_pct", Label: "Min %", Value: strconv.Itoa(settings.Policy.MinRemainingPct)},
			{Key: "policy.allow_apply", Label: "Auto apply", Value: onOff(settings.Policy.AllowApply), Options: []string{"off", "on"}},
			{Key: "notifications.enabled", Label: "Notify", Value: onOff(settings.Notifications.Enabled), Options: []string{"off", "on"}},
			{Key: "notifications.threshold", Label: "Alert %", Value: strconv.Itoa(settings.Notifications.Threshold)},
			{Key: "notifications.reset", Label: "Reset alerts", Value: onOff(settings.Notifications.Reset), Options: []string{"off", "on"}},
			{Key: "notifications.auth_failure", Label: "Auth alerts", Value: onOff(settings.Notifications.AuthFailure), Options: []string{"off", "on"}},
			{Key: "notifications.stale", Label: "Stale alerts", Value: onOff(settings.Notifications.Stale), Options: []string{"off", "on"}},
			{Key: "notifications.cooldown_seconds", Label: "Alert cooldown", Value: strconv.Itoa(settings.Notifications.CooldownSeconds)},
			{Key: "history.enabled", Label: "History", Value: onOff(settings.History.Enabled), Options: []string{"off", "on"}},
			{Key: "history.retention_days", Label: "Keep days", Value: strconv.Itoa(settings.History.RetentionDays)},
			{Key: "history.max_bytes", Label: "Max bytes", Value: strconv.Itoa(settings.History.MaxBytes)},
		}
	case "alias":
		form.Title, form.Description = "ALIAS", "Give an account or profile a short command name."
		form.Fields = []tuiFormField{{Key: "name", Label: "Name"}, {Key: "target", Label: "Target", Value: state.active}}
	case "binding":
		form.Title, form.Description = "PROJECT BINDING", "Bind a folder to a named profile."
		form.Fields = []tuiFormField{{Key: "path", Label: "Path"}, {Key: "profile", Label: "Profile"}, {Key: "mode", Label: "Mode", Value: "prompt", Options: []string{"prompt", "recommend", "disabled"}}}
	case "target":
		form.Title, form.Description = "TARGET", "Register a compatible CLI executable."
		form.Fields = []tuiFormField{{Key: "name", Label: "Name"}, {Key: "command", Label: "Command"}}
	case "history-export":
		form.Title, form.Description = "EXPORT HISTORY", "Write the local event log as JSON."
		form.Fields = []tuiFormField{{Key: "path", Label: "Path", Value: "agy-swap-history.json"}}
	case "backup-export":
		form.Title, form.Description = "EXPORT BACKUP", "Metadata-only is safe to share; encrypted includes secrets."
		form.Fields = []tuiFormField{{Key: "path", Label: "Path", Value: firstString(state.backupPath, "agy-swap-backup.json")}, {Key: "encrypted", Label: "Encrypted", Value: "off", Options: []string{"off", "on"}}, {Key: "passphrase", Label: "Passphrase", Secret: true, Help: "Required when encrypted is on."}}
	case "backup-import":
		form.Title, form.Description = "IMPORT BACKUP", "Import replaces accounts unless Merge is on."
		form.Fields = []tuiFormField{{Key: "path", Label: "Path"}, {Key: "merge", Label: "Merge", Value: "on", Options: []string{"off", "on"}}, {Key: "passphrase", Label: "Passphrase", Secret: true}}
	case "backup-verify":
		form.Title, form.Description = "VERIFY BACKUP", "Encrypted backups need their passphrase."
		form.Fields = []tuiFormField{{Key: "path", Label: "Path"}, {Key: "passphrase", Label: "Passphrase", Secret: true}}
	}
	if len(form.Fields) == 0 {
		return
	}
	state.form = form
	state.mode = tuiForm
}
