package app

import (
	"context"
	"os"
	"strings"
	"time"
)

type tuiController struct {
	app               *Application
	state             *tuiState
	outFile           *os.File
	ctx               func() context.Context
	current           func() string
	demoNotice        func()
	suspend           func(action func() int) int
	startJob          func(kind, label string, work func(context.Context) tuiJobResult)
	startRefresh      func(force, quiet bool)
	invalidateRefresh func()
	performSwitch     func()
	confirmSwitch     func()
	finish            func()
}

func (c *tuiController) setView(view tuiView) {
	c.app.beginTUIView(c.state, view)
	c.state.message = ""
	if view == tuiViewDashboard {
		c.state.messageType = "info"
	}
	c.app.renderTUI(c.state, c.outFile)
}

func (c *tuiController) openConfirm(kind, subject string) {
	c.state.openDialog(confirmDialog(kind, subject, subject))
	c.app.renderTUI(c.state, c.outFile)
}

func (c *tuiController) openHelp() {
	frameWidth, _ := tuiFrameBox(c.state.width)
	c.state.openHelp(c.app.tuiHelpLines(frameWidth - 2))
}

func (c *tuiController) selectedProfile() string {
	if c.state.profileIndex >= 0 && c.state.profileIndex < len(c.state.profileNames) {
		return c.state.profileNames[c.state.profileIndex]
	}
	return ""
}

func (c *tuiController) startDoctor(refresh bool) {
	if c.app.demo {
		c.demoNotice()
		return
	}
	c.startJob("doctor", "Running health check", func(jobCtx context.Context) tuiJobResult {
		checks, healthy := c.app.tuiDoctorSnapshot(jobCtx, refresh)
		return tuiJobResult{message: "Health check complete", doctorChecks: checks, doctorHealthy: healthy}
	})
}

func (c *tuiController) startBackupExport(path, passphrase string, includeSecrets bool) {
	if c.app.demo {
		c.demoNotice()
		return
	}
	target := firstString(strings.TrimSpace(path), "agy-swap-backup.json")
	c.state.backupPath = target
	c.startJob("backup-export", "Writing backup", func(jobCtx context.Context) tuiJobResult {
		message, err := c.app.tuiExportBackup(jobCtx, target, passphrase, includeSecrets)
		return tuiJobResult{message: message, err: err}
	})
}

func (c *tuiController) startBackupImport(path, passphrase string, merge bool) {
	if c.app.demo {
		c.demoNotice()
		return
	}
	target := strings.TrimSpace(path)
	c.state.backupPath = target
	c.invalidateRefresh()
	c.startJob("backup-import", "Importing backup", func(jobCtx context.Context) tuiJobResult {
		message, err := c.app.tuiImportBackup(jobCtx, target, passphrase, merge)
		return tuiJobResult{message: message, err: err}
	})
}

func (c *tuiController) startBackupVerify(path, passphrase string) {
	if c.app.demo {
		c.demoNotice()
		return
	}
	target := strings.TrimSpace(path)
	c.state.backupPath = target
	c.startJob("backup-verify", "Verifying backup", func(context.Context) tuiJobResult {
		message, err := c.app.tuiVerifyBackup(target, passphrase)
		return tuiJobResult{message: message, err: err}
	})
}

func (c *tuiController) performDelete(email string) {
	c.invalidateRefresh()
	fresh, loadErr := c.app.store.Load(false)
	if loadErr != nil {
		c.state.message, c.state.messageType = loadErr.Error(), "error"
		return
	}
	ref := getString(fresh.ByEmail[email], "secret_ref")
	fresh.Delete(email)
	if saveErr := c.app.store.Save(fresh); saveErr != nil {
		c.state.message, c.state.messageType = saveErr.Error(), "error"
		return
	}
	if ref != "" && c.app.vault != nil {
		_ = c.app.vault.Delete(c.ctx(), ref)
	}
	c.state.setAccounts(fresh)
	c.state.active = c.app.activeHint(c.state.accounts, c.current())
	c.state.message, c.state.messageType = "Removed account "+email, "success"
	c.state.beginAnimation("success", 360*time.Millisecond)
}

func (c *tuiController) toggleTier() {
	email, _, ok := c.state.selectedAccount()
	if !ok {
		return
	}
	c.invalidateRefresh()
	fresh, loadErr := c.app.store.Load(false)
	if loadErr != nil {
		c.state.message, c.state.messageType = loadErr.Error(), "error"
		return
	}
	account := fresh.ByEmail[email]
	if account == nil {
		c.state.message, c.state.messageType = "Account changed outside TUI; refresh and try again", "error"
		return
	}
	if getMap(account["quota_snapshot"]) != nil {
		c.state.message, c.state.messageType = "Tier is synced from Google", "info"
		return
	}
	plan := "Pro"
	if getString(account, "tier_source") == "manual" && getString(account, "plan") == "Pro" {
		plan = "Free"
	} else if getString(account, "tier_source") == "manual" && getString(account, "plan") == "Free" {
		plan = ""
	}
	if plan == "" {
		delete(account, "plan")
		delete(account, "tier")
		delete(account, "tier_source")
		delete(account, "is_pro")
	} else {
		account["plan"], account["tier"], account["tier_source"], account["is_pro"] = plan, plan, "manual", plan == "Pro"
	}
	if saveErr := c.app.store.Save(fresh); saveErr != nil {
		c.state.message, c.state.messageType = saveErr.Error(), "error"
		return
	}
	c.state.setAccounts(fresh)
	c.state.message, c.state.messageType = "Set tier for "+email+" to "+firstString(plan, "Unknown"), "success"
	c.state.beginAnimation("success", 360*time.Millisecond)
}

func (c *tuiController) searchKey(key string) {
	switch key {
	case "esc":
		c.state.cancelSearch()
	case "enter":
		c.state.mode = tuiBrowse
		c.state.selectedBefore = ""
		c.state.clampSelection()
		if c.state.search != "" && len(c.state.visibleEmails()) == 0 {
			c.state.message, c.state.messageType = "No matching accounts", "info"
		}
	case "backspace":
		if len(c.state.search) > 0 {
			c.state.search = removeLastRune(c.state.search)
			c.state.clampSelection()
		}
	case "ctrl-u":
		c.state.search = ""
		c.state.clampSelection()
	case "ctrl-w":
		c.state.search = strings.TrimRight(c.state.search, " \t")
		if index := strings.LastIndexAny(c.state.search, " \t"); index >= 0 {
			c.state.search = c.state.search[:index]
		} else {
			c.state.search = ""
		}
		c.state.clampSelection()
	default:
		if printableKey(key) {
			c.state.search += strings.ToLower(key)
			c.state.clampSelection()
		}
	}
}

func (c *tuiController) runAddAccount() {
	c.app.inTUISuspend = true
	code := c.suspend(func() int { return c.app.addLoginFlow(c.ctx()) })
	c.app.inTUISuspend = false
	if code == 0 {
		c.startRefresh(true, true)
	}
}

func (c *tuiController) submitForm() {
	if c.state.form == nil {
		c.state.mode = tuiBrowse
		return
	}
	form := c.state.form
	kind := form.Kind
	if err := c.app.allowDemoAction(kind); err != nil {
		c.state.form = nil
		c.state.mode = tuiBrowse
		c.demoNotice()
		return
	}
	if kind == "backup-export" {
		c.state.form = nil
		c.state.mode = tuiBrowse
		c.startBackupExport(formField(form, "path"), formField(form, "passphrase"), formBool(form, "encrypted"))
		return
	}
	if kind == "backup-import" {
		c.state.form = nil
		c.state.mode = tuiBrowse
		c.startBackupImport(formField(form, "path"), formField(form, "passphrase"), formBool(form, "merge"))
		return
	}
	if kind == "backup-verify" {
		c.state.form = nil
		c.state.mode = tuiBrowse
		c.startBackupVerify(formField(form, "path"), formField(form, "passphrase"))
		return
	}
	if kind == "history-export" {
		path := strings.TrimSpace(formField(form, "path"))
		c.state.form = nil
		c.state.mode = tuiBrowse
		c.suspend(func() int { return c.app.cmdHistory(extendedOptions{Output: path}, []string{"export", path}) })
		return
	}
	message, err := c.app.applyTUIForm(c.ctx(), c.state)
	previousView := form.PreviousView
	c.state.form = nil
	c.state.mode = tuiBrowse
	if err != nil {
		c.state.message, c.state.messageType = err.Error(), "error"
	} else {
		c.state.message, c.state.messageType = message, "success"
		c.state.beginAnimation("success", 360*time.Millisecond)
	}
	c.app.beginTUIView(c.state, previousView)
	if err != nil {
		c.state.message, c.state.messageType = err.Error(), "error"
	} else {
		c.state.message, c.state.messageType = message, "success"
	}
}

func (c *tuiController) runAction(id string) {
	if c.app.demo {
		switch id {
		case "dashboard", "quota", "profiles", "history", "settings", "help", "quit", "switch-account", "refresh", "edit-tags", "toggle-tier", "profile-create", "profile-edit", "profile-remove", "history-clear", "settings-edit", "settings-reset", "alias-create", "split-widen", "split-narrow", "split-reset":
		default:
			c.demoNotice()
			return
		}
	}
	switch id {
	case "dashboard":
		c.setView(tuiViewDashboard)
	case "profiles":
		c.setView(tuiViewProfiles)
	case "history":
		c.setView(tuiViewHistory)
	case "settings":
		c.setView(tuiViewSettings)
	case "doctor":
		c.setView(tuiViewDoctor)
		c.startDoctor(false)
	case "backup":
		c.setView(tuiViewBackup)
	case "recommend":
		c.suspend(func() int { return c.app.cmdRecommend(c.ctx(), extendedOptions{}, nil) })
	case "forecast":
		c.suspend(func() int { return c.app.cmdForecast(c.ctx(), extendedOptions{}, nil) })
	case "watch-once":
		c.suspend(func() int {
			return c.app.cmdWatch(c.ctx(), extendedOptions{Once: true, Threshold: -1, Interval: time.Minute}, nil)
		})
	case "metrics":
		c.suspend(func() int { return c.app.cmdMetrics(c.ctx(), extendedOptions{}, []string{"render"}) })
	case "run-now":
		c.suspend(func() int { return c.app.cmdRunNow(c.ctx(), extendedOptions{}, []string{"now"}) })
	case "statusline-install":
		c.suspend(func() int { return c.app.cmdStatusline(c.ctx(), extendedOptions{}, []string{"install"}) })
	case "completion":
		c.suspend(func() int { return c.app.cmdCompletion(extendedOptions{}, []string{"bash"}) })
	case "update-check":
		c.suspend(func() int { return c.app.cmdUpdate(c.ctx(), cliArgs{updateCheck: true}) })
	case "update":
		c.openConfirm("update", "")
	case "install-agy":
		c.suspend(func() int { return c.app.cmdInstallAgy(c.ctx(), extendedOptions{}, nil) })
	case "donate":
		c.app.tuiOpenDonate(c.ctx(), c.state)
	case "add-account":
		c.setView(tuiViewDashboard)
		c.runAddAccount()
	case "verify":
		c.app.tuiVerifySelected(c.ctx(), c.state)
	case "switch-account":
		c.confirmSwitch()
	case "next-account":
		c.suspend(func() int { return c.app.cmdNext(c.ctx(), cliArgs{}) })
	case "refresh":
		c.state.message, c.state.messageType = "Refreshing quota…", "info"
		c.startRefresh(true, false)
	case "edit-tags":
		c.app.beginTUIForm(c.state, "tags")
	case "toggle-tier":
		c.toggleTier()
	case "migrate-vault":
		c.suspend(func() int { return c.app.cmdAccount(c.ctx(), extendedOptions{Force: true}, []string{"migrate"}) })
	case "profile-create":
		c.app.beginTUIForm(c.state, "profile-create")
	case "profile-edit":
		if len(c.state.profileNames) > 0 {
			c.app.beginTUIForm(c.state, "profile-edit")
		}
	case "profile-remove":
		if len(c.state.profileNames) > 0 {
			c.openConfirm("profile-remove", c.selectedProfile())
		}
	case "history-clear":
		if len(c.state.history) > 0 {
			c.openConfirm("history-clear", "")
		}
	case "history-export":
		if len(c.state.history) > 0 {
			c.app.beginTUIForm(c.state, "history-export")
		}
	case "settings-edit":
		c.app.beginTUIForm(c.state, "settings")
	case "settings-reset":
		c.openConfirm("settings-reset", "")
	case "alias-create":
		c.app.beginTUIForm(c.state, "alias")
	case "binding-create":
		c.app.beginTUIForm(c.state, "binding")
	case "target-create":
		c.app.beginTUIForm(c.state, "target")
	case "backup-export":
		c.app.beginTUIForm(c.state, "backup-export")
	case "backup-import":
		c.app.beginTUIForm(c.state, "backup-import")
	case "backup-verify":
		c.app.beginTUIForm(c.state, "backup-verify")
	case "split-widen":
		c.state.adjustSplit(4)
		c.state.message, c.state.messageType = c.state.splitMessage(), "info"
	case "split-narrow":
		c.state.adjustSplit(-4)
		c.state.message, c.state.messageType = c.state.splitMessage(), "info"
	case "split-reset":
		c.state.resetSplit()
		c.state.message, c.state.messageType = "Split reset to default", "info"
	case "help":
		c.openHelp()
	case "quit":
		c.state.quitRequested = true
		c.finish()
	}
}

func (c *tuiController) handleKeyEvent(rawKey, key string, armFrame func()) bool {
	state := c.state
	a := c.app
	outFile := c.outFile

	state.keyFlash, state.keyFlashAt = key, time.Now()
	if state.mode == tuiSearch {
		c.searchKey(rawKey)
		a.renderTUI(state, outFile)
		return false
	}
	if state.mode == tuiDialogOpen {
		subject := ""
		if state.dialog != nil {
			subject = state.dialog.Subject
		}
		kind, action, done := state.dialogKey(key)
		switch {
		case !done || kind == "help":
		case action != "confirm":
			state.message, state.messageType = "Canceled", "info"
		default:
			switch kind {
			case "profile-remove":
				if name := subject; name != "" {
					settings, loadErr := a.loadSettings()
					if loadErr != nil {
						state.message, state.messageType = loadErr.Error(), "error"
					} else {
						delete(settings.Profiles, name)
						if saveErr := a.store.SaveSettings(settings); saveErr != nil {
							state.message, state.messageType = saveErr.Error(), "error"
						} else {
							state.message, state.messageType = "Removed profile "+name, "success"
							a.beginTUIView(state, tuiViewProfiles)
						}
					}
				}
			case "history-clear":
				if err := a.clearHistory(); err != nil {
					state.message, state.messageType = err.Error(), "error"
				} else {
					state.history = nil
					state.historyIndex = 0
					state.message, state.messageType = "History cleared", "success"
				}
			case "settings-reset":
				if saveErr := a.store.SaveSettings(defaultSettings()); saveErr != nil {
					state.message, state.messageType = saveErr.Error(), "error"
				} else {
					state.splitOffset, state.stackOffset = 0, 0
					state.message, state.messageType = "Settings reset", "success"
					a.beginTUIView(state, tuiViewSettings)
				}
			case "update":
				c.suspend(func() int { return a.cmdUpdate(c.ctx(), cliArgs{}) })
			case "switch":
				c.performSwitch()
			case "delete":
				c.performDelete(subject)
			}
		}
		a.renderTUI(state, outFile)
		armFrame()
		return false
	}
	if state.mode == tuiPalette {
		switch key {
		case "esc":
			state.mode = tuiBrowse
		case "up", "k":
			state.movePalette(-1)
		case "down", "j":
			state.movePalette(1)
		case "page-up":
			state.movePalette(-5)
		case "page-down":
			state.movePalette(5)
		case "backspace":
			if len(state.paletteQuery) > 0 {
				state.paletteQuery = removeLastRune(state.paletteQuery)
				state.paletteIndex = 0
			}
		case "enter":
			if action, ok := state.selectedPaletteAction(); ok {
				state.mode = tuiBrowse
				c.runAction(action.ID)
				if state.quitRequested {
					return true
				}
			}
		default:
			if printableKey(key) {
				state.paletteQuery += key
				state.paletteIndex = 0
			}
		}
		a.renderTUI(state, outFile)
		return false
	}
	if state.mode == tuiForm {
		submit, cancel := state.formKey(rawKey)
		if cancel {
			previous := tuiViewDashboard
			if state.form != nil {
				previous = state.form.PreviousView
			}
			state.form, state.mode = nil, tuiBrowse
			state.message, state.messageType = "Edit canceled", "info"
			state.view = previous
		} else if submit {
			c.submitForm()
		}
		a.renderTUI(state, outFile)
		armFrame()
		return false
	}

	switch key {
	case "q", "esc", "ctrl-c", "ctrl-d":
		c.finish()
		return true
	case "ctrl-k", ":":
		state.beginPalette()
	case "?":
		c.openHelp()
	case "/":
		state.beginSearch()
	case "up", "k":
		switch state.view {
		case tuiViewProfiles:
			state.moveProfile(-1)
		case tuiViewHistory:
			state.moveHistory(-1)
		case tuiViewDashboard:
			state.move(-1)
		}
	case "down", "j":
		switch state.view {
		case tuiViewProfiles:
			state.moveProfile(1)
		case tuiViewHistory:
			state.moveHistory(1)
		case tuiViewDashboard:
			state.move(1)
		}
	case "page-up":
		switch state.view {
		case tuiViewProfiles:
			state.moveProfile(-5)
		case tuiViewHistory:
			state.moveHistory(-5)
		default:
			state.move(-maxInt(1, len(state.visibleEmails())/2))
		}
	case "page-down":
		switch state.view {
		case tuiViewProfiles:
			state.moveProfile(5)
		case tuiViewHistory:
			state.moveHistory(5)
		default:
			state.move(maxInt(1, len(state.visibleEmails())/2))
		}
	case "home":
		switch state.view {
		case tuiViewProfiles:
			state.profileIndex = 0
		case tuiViewHistory:
			state.historyIndex = 0
		default:
			state.moveToBoundary(false)
		}
	case "end":
		switch state.view {
		case tuiViewProfiles:
			state.profileIndex = maxInt(0, len(state.profileNames)-1)
		case tuiViewHistory:
			state.historyIndex = maxInt(0, len(state.history)-1)
		default:
			state.moveToBoundary(true)
		}
	case "r":
		switch state.view {
		case tuiViewDoctor:
			c.startDoctor(false)
		case tuiViewDashboard:
			state.message, state.messageType = "Refreshing quota…", "info"
			c.startRefresh(true, false)
		}
	case "a":
		if state.view == tuiViewSettings {
			a.beginTUIForm(state, "alias")
		} else {
			c.runAddAccount()
		}
	case "d", "delete", "backspace":
		if state.view == tuiViewProfiles && len(state.profileNames) > 0 {
			c.openConfirm("profile-remove", c.selectedProfile())
		} else if state.view == tuiViewDashboard && state.mode == tuiBrowse {
			if email, account, ok := state.selectedAccount(); ok {
				state.openDialog(confirmDialog("delete", email, tuiText(firstString(account["name"], email))))
			}
		}
	case "t":
		if state.view == tuiViewSettings {
			a.beginTUIForm(state, "target")
		} else {
			c.toggleTier()
		}
	case "n":
		c.suspend(func() int { return a.cmdNext(c.ctx(), cliArgs{}) })
	case "l":
		c.suspend(func() int { return a.cmdLogout(c.ctx()) })
	case "enter":
		switch state.view {
		case tuiViewDashboard:
			c.confirmSwitch()
		case tuiViewProfiles:
			if len(state.profileNames) > 0 {
				a.beginTUIForm(state, "profile-edit")
			}
		case tuiViewSettings:
			a.beginTUIForm(state, "settings")
		case tuiViewDoctor:
			c.startDoctor(false)
		}
	case "p":
		c.setView(tuiViewProfiles)
	case "g":
		c.setView(tuiViewDashboard)
	case "h":
		c.setView(tuiViewHistory)
	case "s":
		c.setView(tuiViewSettings)
	case "o":
		c.setView(tuiViewDoctor)
		c.startDoctor(false)
	case "b":
		switch state.view {
		case tuiViewSettings:
			a.beginTUIForm(state, "binding")
		case tuiViewDashboard:
			c.setView(tuiViewBackup)
		default:
			c.setView(tuiViewDashboard)
		}
	case "w":
		if state.view == tuiViewDashboard {
			a.tuiVerifySelected(c.ctx(), state)
		}
	case "v":
		if state.view == tuiViewBackup {
			a.beginTUIForm(state, "backup-verify")
		}
	case "x":
		if state.view == tuiViewBackup {
			a.beginTUIForm(state, "backup-export")
		} else if state.view == tuiViewHistory && len(state.history) > 0 {
			a.beginTUIForm(state, "history-export")
		}
	case "i":
		if state.view == tuiViewBackup {
			a.beginTUIForm(state, "backup-import")
		}
	case "e":
		switch state.view {
		case tuiViewSettings:
			a.beginTUIForm(state, "settings")
		case tuiViewProfiles:
			if len(state.profileNames) > 0 {
				a.beginTUIForm(state, "profile-edit")
			}
		case tuiViewDashboard:
			a.beginTUIForm(state, "tags")
		}
	case "c":
		if state.view == tuiViewProfiles {
			a.beginTUIForm(state, "profile-create")
		} else if state.view == tuiViewHistory && len(state.history) > 0 {
			c.openConfirm("history-clear", "")
		}
	case "m":
		c.suspend(func() int { return a.cmdAccount(c.ctx(), extendedOptions{Force: true}, []string{"migrate"}) })
	case "u":
		c.openConfirm("update", "")
	case "]", ">", "+", "alt-right", "alt-f":
		state.adjustSplit(2)
		state.message, state.messageType = state.splitMessage(), "info"
	case "}", "ctrl-right":
		state.adjustSplit(6)
		state.message, state.messageType = state.splitMessage(), "info"
	case "[", "<", "-", "alt-left", "alt-b":
		state.adjustSplit(-2)
		state.message, state.messageType = state.splitMessage(), "info"
	case "{", "ctrl-left":
		state.adjustSplit(-6)
		state.message, state.messageType = state.splitMessage(), "info"
	case "=":
		state.resetSplit()
		state.message, state.messageType = "Split reset to default", "info"
	default:
		if len(key) == 1 && key[0] >= '1' && key[0] <= '9' {
			index := int(key[0] - '1')
			emails := state.visibleEmails()
			if index < len(emails) {
				state.selectedEmail = emails[index]
				state.beginAnimation("focus", 140*time.Millisecond)
			}
		}
	}
	a.renderTUI(state, outFile)
	armFrame()
	return state.quitRequested
}
