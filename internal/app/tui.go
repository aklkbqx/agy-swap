package app

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/term"
)

type tuiKeyEvent struct{ key string }
type tuiAccountsEvent struct {
	accounts    *Accounts
	quotaErrors map[string]string
	revision    uint64
}
type tuiActiveEvent struct {
	token string
	email string
}
type tuiResizeEvent struct{ width, height int }
type tuiJobEvent struct {
	id            uint64
	kind          string
	message       string
	err           error
	doctorChecks  []doctorCheck
	doctorHealthy bool
}

type tuiJobResult struct {
	message       string
	err           error
	doctorChecks  []doctorCheck
	doctorHealthy bool
}

type tuiEvent interface{ tuiEvent() }

func (tuiKeyEvent) tuiEvent()      {}
func (tuiAccountsEvent) tuiEvent() {}
func (tuiActiveEvent) tuiEvent()   {}
func (tuiResizeEvent) tuiEvent()   {}
func (tuiJobEvent) tuiEvent()      {}

func (a *Application) cmdInteractive(ctx context.Context) int {
	inFile, inOK := a.In.(*os.File)
	outFile, outOK := a.Out.(*os.File)
	if !inOK || !outOK || !term.IsTerminal(int(inFile.Fd())) || !term.IsTerminal(int(outFile.Fd())) {
		return a.cmdList(ctx, cliArgs{})
	}
	accounts, err := a.store.Load(false)
	if err != nil {
		return a.storeError(err)
	}
	current := a.credentials.Current(ctx)
	oldState, err := term.MakeRaw(int(inFile.Fd()))
	if err != nil {
		return a.storeError(err)
	}
	raw := true
	enterScreen := func() { fmt.Fprint(a.Out, "\x1b[?1049h\x1b[?25l\x1b[H") }
	leaveScreen := func() { fmt.Fprint(a.Out, "\x1b[?1049l\x1b[?25h") }
	enterScreen()
	defer func() {
		if raw {
			_ = term.Restore(int(inFile.Fd()), oldState)
		}
		leaveScreen()
	}()

	state := newTUIState(accounts, current)
	state.active = a.activeHint(accounts, current)
	events := make(chan tuiEvent, 32)
	done := make(chan struct{})
	workerCtx, cancelWorkers := context.WithCancel(ctx)
	defer cancelWorkers()
	var inputPaused atomic.Bool
	var closeDone sync.Once
	finish := func() {
		closeDone.Do(func() { close(done) })
		cancelWorkers()
	}

	go func() {
		for {
			if inputPaused.Load() {
				select {
				case <-done:
					return
				case <-time.After(20 * time.Millisecond):
				}
				continue
			}
			// readTerminalKey polls the terminal fd, so this worker remains
			// cancellable even when the terminal is idle.
			key := readTerminalKey(inFile)
			if key == "" {
				select {
				case <-done:
					return
				case <-time.After(10 * time.Millisecond):
				}
				continue
			}
			select {
			case events <- tuiKeyEvent{key: key}:
			case <-done:
				return
			}
		}
	}()

	startActiveResolve := func() {
		local := a.activeHint(state.accounts, current)
		if local != "" || current == "" {
			state.active = local
			state.resolvingToken = ""
			return
		}
		if state.resolvingToken == current {
			return
		}
		state.active = ""
		token := current
		snapshot := state.accounts
		state.resolvingToken = token
		go func() {
			resolveCtx, cancel := context.WithTimeout(workerCtx, 5*time.Second)
			email := a.activeEmail(resolveCtx, snapshot, token)
			cancel()
			select {
			case events <- tuiActiveEvent{token: token, email: email}:
			case <-workerCtx.Done():
			}
		}()
	}
	refreshing := false
	var refreshRevision uint64
	startRefresh := func(force bool) {
		if refreshing {
			return
		}
		refreshRevision++
		revision := refreshRevision
		refreshing = true
		state.refreshing = true
		state.beginAnimation("refresh", 0)
		a.renderTUI(state, outFile)
		go func() {
			fresh, loadErr := a.store.Load(true)
			errs := map[string]string{}
			if loadErr == nil && fresh.Len() > 0 {
				errs = a.quota.Refresh(workerCtx, fresh, force, nil)
			}
			if loadErr != nil {
				errs["store"] = loadErr.Error()
			}
			select {
			case events <- tuiAccountsEvent{accounts: fresh, quotaErrors: errs, revision: revision}:
			case <-workerCtx.Done():
			}
		}()
	}
	invalidateRefresh := func() {
		// Mutating actions must invalidate an in-flight snapshot. Otherwise a
		// slower quota response could repaint accounts that were just changed.
		refreshRevision++
		refreshing = false
		state.refreshing = false
	}

	var frameTimer *time.Timer
	var frameC <-chan time.Time
	armFrame := func() {
		if !state.animation.active && !state.toastActive(time.Now()) {
			frameC = nil
			if frameTimer != nil {
				frameTimer.Stop()
			}
			return
		}
		if frameTimer == nil {
			frameTimer = time.NewTimer(80 * time.Millisecond)
		} else {
			if !frameTimer.Stop() {
				select {
				case <-frameTimer.C:
				default:
				}
			}
			frameTimer.Reset(80 * time.Millisecond)
		}
		frameC = frameTimer.C
	}

	resizeTicker := time.NewTicker(250 * time.Millisecond)
	defer resizeTicker.Stop()
	credentialTicker := time.NewTicker(1500 * time.Millisecond)
	defer credentialTicker.Stop()
	quotaTicker := time.NewTicker(tuiAutoRefresh)
	defer quotaTicker.Stop()
	defer func() {
		if frameTimer != nil {
			frameTimer.Stop()
		}
	}()
	a.renderTUI(state, outFile)
	startActiveResolve()
	startRefresh(false)
	armFrame()

	suspend := func(action func() int) int {
		invalidateRefresh()
		inputPaused.Store(true)
		defer inputPaused.Store(false)
		_ = term.Restore(int(inFile.Fd()), oldState)
		raw = false
		leaveScreen()
		code := action()
		if code == 0 {
			state.message, state.messageType = "Completed", "success"
		} else {
			state.message, state.messageType = "Action failed", "error"
		}
		newState, stateErr := term.MakeRaw(int(inFile.Fd()))
		if stateErr == nil {
			oldState = newState
			raw = true
		}
		enterScreen()
		fresh, loadErr := a.store.Load(false)
		if loadErr == nil {
			state.setAccounts(fresh)
		}
		current = a.credentials.Current(ctx)
		state.current = current
		state.active = a.activeHint(state.accounts, current)
		startActiveResolve()
		a.renderTUI(state, outFile)
		return code
	}

	var jobID uint64
	startJob := func(kind, label string, work func(context.Context) tuiJobResult) {
		jobID++
		id := jobID
		state.job = &tuiJobState{ID: id, Kind: kind, Label: label, Started: time.Now()}
		state.message = ""
		state.beginAnimation("job", 0)
		a.renderTUI(state, outFile)
		go func() {
			result := work(workerCtx)
			select {
			case events <- tuiJobEvent{id: id, kind: kind, message: result.message, err: result.err, doctorChecks: result.doctorChecks, doctorHealthy: result.doctorHealthy}:
			case <-workerCtx.Done():
			}
		}()
	}

	setView := func(view tuiView) {
		a.beginTUIView(state, view)
		state.message = ""
		if view == tuiViewDashboard {
			state.messageType = "info"
		}
		a.renderTUI(state, outFile)
	}

	beginConfirmAction := func(kind, title string) {
		state.mode = tuiConfirmAction
		state.confirmAction = kind
		state.confirmTitle = title
		a.renderTUI(state, outFile)
	}

	startDoctor := func(refresh bool) {
		startJob("doctor", "Running health check", func(jobCtx context.Context) tuiJobResult {
			checks, healthy := a.tuiDoctorSnapshot(jobCtx, refresh)
			return tuiJobResult{message: "Health check complete", doctorChecks: checks, doctorHealthy: healthy}
		})
	}

	startBackupExport := func(path, passphrase string, includeSecrets bool) {
		target := firstString(strings.TrimSpace(path), "agy-swap-backup.json")
		state.backupPath = target
		startJob("backup-export", "Writing backup", func(jobCtx context.Context) tuiJobResult {
			message, err := a.tuiExportBackup(jobCtx, target, passphrase, includeSecrets)
			return tuiJobResult{message: message, err: err}
		})
	}

	startBackupImport := func(path, passphrase string, merge bool) {
		target := strings.TrimSpace(path)
		state.backupPath = target
		invalidateRefresh()
		startJob("backup-import", "Importing backup", func(jobCtx context.Context) tuiJobResult {
			message, err := a.tuiImportBackup(jobCtx, target, passphrase, merge)
			return tuiJobResult{message: message, err: err}
		})
	}

	startBackupVerify := func(path, passphrase string) {
		target := strings.TrimSpace(path)
		state.backupPath = target
		startJob("backup-verify", "Verifying backup", func(context.Context) tuiJobResult {
			message, err := a.tuiVerifyBackup(target, passphrase)
			return tuiJobResult{message: message, err: err}
		})
	}

	performDelete := func(email string) {
		invalidateRefresh()
		fresh, loadErr := a.store.Load(false)
		if loadErr != nil {
			state.message, state.messageType = loadErr.Error(), "error"
			return
		}
		ref := getString(fresh.ByEmail[email], "secret_ref")
		fresh.Delete(email)
		if saveErr := a.store.Save(fresh); saveErr != nil {
			state.message, state.messageType = saveErr.Error(), "error"
			return
		}
		if ref != "" && a.vault != nil {
			_ = a.vault.Delete(ctx, ref)
		}
		state.setAccounts(fresh)
		state.active = a.activeHint(state.accounts, current)
		state.message, state.messageType = "Removed account "+email, "success"
		state.beginAnimation("success", 360*time.Millisecond)
	}

	toggleTier := func() {
		email, account, ok := state.selectedAccount()
		if !ok {
			return
		}
		invalidateRefresh()
		fresh, loadErr := a.store.Load(false)
		if loadErr != nil {
			state.message, state.messageType = loadErr.Error(), "error"
			return
		}
		account = fresh.ByEmail[email]
		if account == nil {
			state.message, state.messageType = "Account changed outside TUI; refresh and try again", "error"
			return
		}
		if getMap(account["quota_snapshot"]) != nil {
			state.message, state.messageType = "Tier is synced from Google", "info"
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
		if saveErr := a.store.Save(fresh); saveErr != nil {
			state.message, state.messageType = saveErr.Error(), "error"
			return
		}
		state.setAccounts(fresh)
		state.message, state.messageType = "Set tier for "+email+" to "+firstString(plan, "Unknown"), "success"
		state.beginAnimation("success", 360*time.Millisecond)
	}

	searchKey := func(key string) {
		switch key {
		case "esc":
			state.cancelSearch()
		case "enter":
			state.mode = tuiBrowse
			state.selectedBefore = ""
			state.clampSelection()
			if state.search != "" && len(state.visibleEmails()) == 0 {
				state.message, state.messageType = "No matching accounts", "info"
			}
		case "backspace":
			if len(state.search) > 0 {
				state.search = state.search[:len(state.search)-1]
				state.clampSelection()
			}
		case "ctrl-u":
			state.search = ""
			state.clampSelection()
		case "ctrl-w":
			state.search = strings.TrimRight(state.search, " \t")
			if index := strings.LastIndexAny(state.search, " \t"); index >= 0 {
				state.search = state.search[:index]
			} else {
				state.search = ""
			}
			state.clampSelection()
		default:
			if len(key) == 1 && key[0] >= 32 && key[0] != 127 {
				state.search += strings.ToLower(key)
				state.clampSelection()
			}
		}
	}

	performSwitch := func() {
		email, account, ok := state.selectedAccount()
		if !ok {
			return
		}
		alreadyUsing := false
		var switchErr error
		code := suspend(func() int {
			token, tokenErr := a.accountToken(ctx, account)
			if tokenErr != nil {
				switchErr = tokenErr
				return 1
			}
			if a.credentials.Current(ctx) == token {
				alreadyUsing = true
				return 0
			}
			if a.credentials.Apply(ctx, token, email) {
				return 0
			}
			return 1
		})
		if code == 0 {
			if alreadyUsing {
				state.showToast("Already using "+email, "info")
			} else {
				state.showToast("Switched to "+email, "success")
			}
		} else if switchErr != nil {
			state.showToast("Switch failed: "+switchErr.Error(), "error")
		} else {
			state.showToast("Could not switch to "+email, "error")
		}
		a.renderTUI(state, outFile)
		armFrame()
	}

	submitForm := func() {
		if state.form == nil {
			state.mode = tuiBrowse
			return
		}
		form := state.form
		kind := form.Kind
		if kind == "backup-export" {
			state.form = nil
			state.mode = tuiBrowse
			startBackupExport(formField(form, "path"), formField(form, "passphrase"), formBool(form, "encrypted"))
			return
		}
		if kind == "backup-import" {
			state.form = nil
			state.mode = tuiBrowse
			startBackupImport(formField(form, "path"), formField(form, "passphrase"), formBool(form, "merge"))
			return
		}
		if kind == "backup-verify" {
			state.form = nil
			state.mode = tuiBrowse
			startBackupVerify(formField(form, "path"), formField(form, "passphrase"))
			return
		}
		if kind == "history-export" {
			path := strings.TrimSpace(formField(form, "path"))
			state.form = nil
			state.mode = tuiBrowse
			suspend(func() int { return a.cmdHistory(extendedOptions{Output: path}, []string{"export", path}) })
			return
		}
		message, err := a.applyTUIForm(ctx, state)
		previousView := form.PreviousView
		state.form = nil
		state.mode = tuiBrowse
		if err != nil {
			state.message, state.messageType = err.Error(), "error"
		} else {
			state.message, state.messageType = message, "success"
			state.beginAnimation("success", 360*time.Millisecond)
		}
		a.beginTUIView(state, previousView)
		// beginTUIView refreshes cached rows, so restore the action result after
		// it has completed.
		if err != nil {
			state.message, state.messageType = err.Error(), "error"
		} else {
			state.message, state.messageType = message, "success"
		}
	}

	runAction := func(id string) {
		switch id {
		case "dashboard":
			setView(tuiViewDashboard)
		case "quota":
			setView(tuiViewQuota)
		case "profiles":
			setView(tuiViewProfiles)
		case "history":
			setView(tuiViewHistory)
		case "settings":
			setView(tuiViewSettings)
		case "doctor":
			setView(tuiViewDoctor)
			startDoctor(false)
		case "backup":
			setView(tuiViewBackup)
		case "recommend":
			suspend(func() int { return a.cmdRecommend(ctx, extendedOptions{}, nil) })
		case "forecast":
			suspend(func() int { return a.cmdForecast(ctx, extendedOptions{}, nil) })
		case "watch-once":
			suspend(func() int { return a.cmdWatch(ctx, extendedOptions{Once: true}, nil) })
		case "metrics":
			suspend(func() int { return a.cmdMetrics(ctx, extendedOptions{}, []string{"render"}) })
		case "run-now":
			suspend(func() int { return a.cmdRunNow(ctx, extendedOptions{}, []string{"now"}) })
		case "statusline-install":
			suspend(func() int { return a.cmdStatusline(ctx, extendedOptions{}, []string{"install"}) })
		case "completion":
			suspend(func() int { return a.cmdCompletion(extendedOptions{}, []string{"bash"}) })
		case "update-check":
			suspend(func() int { return a.cmdUpdate(ctx, cliArgs{updateCheck: true}) })
		case "update":
			beginConfirmAction("update", "Download and install the latest release")
		case "add-account":
			setView(tuiViewDashboard)
			suspend(func() int { return a.addLoginFlow(ctx) })
		case "switch-account":
			performSwitch()
		case "next-account":
			suspend(func() int { return a.cmdNext(ctx, cliArgs{}) })
		case "refresh":
			state.message, state.messageType = "Refreshing quota…", "info"
			startRefresh(true)
		case "edit-tags":
			a.beginTUIForm(state, "tags")
		case "toggle-tier":
			toggleTier()
		case "migrate-vault":
			suspend(func() int { return a.cmdAccount(ctx, extendedOptions{Force: true}, []string{"migrate"}) })
		case "profile-create":
			a.beginTUIForm(state, "profile-create")
		case "profile-edit":
			if len(state.profileNames) > 0 {
				a.beginTUIForm(state, "profile-edit")
			}
		case "profile-remove":
			if len(state.profileNames) > 0 {
				beginConfirmAction("profile-remove", "Remove selected profile")
			}
		case "history-clear":
			if len(state.history) > 0 {
				beginConfirmAction("history-clear", "Clear local history")
			}
		case "history-export":
			if len(state.history) > 0 {
				a.beginTUIForm(state, "history-export")
			}
		case "settings-edit":
			a.beginTUIForm(state, "settings")
		case "settings-reset":
			beginConfirmAction("settings-reset", "Reset settings")
		case "alias-create":
			a.beginTUIForm(state, "alias")
		case "binding-create":
			a.beginTUIForm(state, "binding")
		case "target-create":
			a.beginTUIForm(state, "target")
		case "backup-export":
			a.beginTUIForm(state, "backup-export")
		case "backup-import":
			a.beginTUIForm(state, "backup-import")
		case "backup-verify":
			a.beginTUIForm(state, "backup-verify")
		case "help":
			state.mode = tuiHelp
		case "quit":
			state.quitRequested = true
			finish()
		}
	}

	for {
		select {
		case <-ctx.Done():
			finish()
			return 0
		case <-resizeTicker.C:
			width, height, sizeErr := termSize(outFile)
			if sizeErr == nil && (width != state.width || height != state.height) {
				a.renderTUI(state, outFile)
			}
		case <-credentialTicker.C:
			newToken := a.credentials.Current(ctx)
			if newToken != current {
				current = newToken
				state.current = current
				state.active = a.activeHint(state.accounts, current)
				startActiveResolve()
				a.renderTUI(state, outFile)
			}
		case <-quotaTicker.C:
			if !refreshing {
				state.message, state.messageType = "Background sync…", "info"
				startRefresh(false)
			}
		case <-frameC:
			now := time.Now()
			state.expireToast(now)
			if state.advanceAnimation(now) {
				a.renderTUI(state, outFile)
				armFrame()
			} else {
				a.renderTUI(state, outFile)
				armFrame()
			}
		case event := <-events:
			switch value := event.(type) {
			case tuiAccountsEvent:
				if value.revision != refreshRevision {
					// A slower refresh must never overwrite a newer frame.
					continue
				}
				refreshing = false
				state.refreshing = false
				state.setAccounts(value.accounts)
				state.quotaErrors = value.quotaErrors
				if len(value.quotaErrors) > 0 {
					state.message, state.messageType = fmt.Sprintf("Usage refresh completed with %d warning(s)", len(value.quotaErrors)), "error"
					state.beginAnimation("error", 360*time.Millisecond)
				} else {
					state.message, state.messageType = "Usage refreshed", "success"
					state.beginAnimation("success", 360*time.Millisecond)
				}
				startActiveResolve()
				a.renderTUI(state, outFile)
				armFrame()
			case tuiActiveEvent:
				if value.token == current {
					state.active, state.resolvingToken = value.email, ""
					a.renderTUI(state, outFile)
				}
			case tuiResizeEvent:
				state.width, state.height = maxInt(28, value.width), maxInt(12, value.height)
				a.renderTUI(state, outFile)
			case tuiJobEvent:
				if state.job == nil || state.job.ID != value.id {
					continue
				}
				state.job.Done = true
				state.job.Message = value.message
				state.job.Error = ""
				if value.err != nil {
					state.job.Error = value.err.Error()
					state.message, state.messageType = value.err.Error(), "error"
					state.beginAnimation("error", 360*time.Millisecond)
				} else {
					state.message, state.messageType = value.message, "success"
					state.beginAnimation("success", 360*time.Millisecond)
				}
				if value.kind == "doctor" {
					state.doctorChecks, state.doctorHealthy = value.doctorChecks, value.doctorHealthy
				}
				if value.kind == "backup-import" && value.err == nil {
					fresh, loadErr := a.store.Load(false)
					if loadErr == nil {
						state.setAccounts(fresh)
					}
					current = a.credentials.Current(ctx)
					state.current = current
					startActiveResolve()
				}
				a.renderTUI(state, outFile)
				armFrame()
			case tuiKeyEvent:
				key := strings.ToLower(value.key)
				if state.mode == tuiHelp {
					state.mode = tuiBrowse
					a.renderTUI(state, outFile)
					continue
				}
				if state.mode == tuiSearch {
					searchKey(key)
					a.renderTUI(state, outFile)
					continue
				}
				if state.mode == tuiConfirmDelete {
					if key == "y" || key == "enter" {
						email := state.confirmEmail
						state.mode, state.confirmEmail = tuiBrowse, ""
						performDelete(email)
					} else if key == "n" || key == "esc" {
						state.mode, state.confirmEmail = tuiBrowse, ""
						state.message, state.messageType = "Delete canceled", "info"
					}
					a.renderTUI(state, outFile)
					armFrame()
					continue
				}
				if state.mode == tuiConfirmAction {
					if key == "y" || key == "enter" {
						action := state.confirmAction
						state.mode, state.confirmAction, state.confirmTitle = tuiBrowse, "", ""
						switch action {
						case "profile-remove":
							if state.profileIndex >= 0 && state.profileIndex < len(state.profileNames) {
								name := state.profileNames[state.profileIndex]
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
							if err := os.Remove(a.paths.History); err != nil && !os.IsNotExist(err) {
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
								state.message, state.messageType = "Settings reset", "success"
								a.beginTUIView(state, tuiViewSettings)
							}
						case "update":
							suspend(func() int { return a.cmdUpdate(ctx, cliArgs{}) })
						}
					} else if key == "n" || key == "esc" {
						state.mode, state.confirmAction, state.confirmTitle = tuiBrowse, "", ""
						state.message, state.messageType = "Action canceled", "info"
					}
					a.renderTUI(state, outFile)
					continue
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
							state.paletteQuery = state.paletteQuery[:len(state.paletteQuery)-1]
							state.paletteIndex = 0
						}
					case "enter":
						if action, ok := state.selectedPaletteAction(); ok {
							state.mode = tuiBrowse
							runAction(action.ID)
							if state.quitRequested {
								return 0
							}
						}
					default:
						if len(key) == 1 && key[0] >= 32 && key[0] != 127 {
							state.paletteQuery += key
							state.paletteIndex = 0
						}
					}
					a.renderTUI(state, outFile)
					continue
				}
				if state.mode == tuiForm {
					submit, cancel := state.formKey(key)
					if cancel {
						previous := tuiViewDashboard
						if state.form != nil {
							previous = state.form.PreviousView
						}
						state.form, state.mode = nil, tuiBrowse
						state.message, state.messageType = "Edit canceled", "info"
						state.view = previous
					} else if submit {
						submitForm()
					}
					a.renderTUI(state, outFile)
					armFrame()
					continue
				}

				switch key {
				case "q", "esc", "ctrl-c", "ctrl-d":
					finish()
					return 0
				case "ctrl-k", ":":
					state.beginPalette()
				case "?":
					state.mode = tuiHelp
				case "/":
					state.beginSearch()
				case "up", "k":
					switch state.view {
					case tuiViewProfiles:
						state.moveProfile(-1)
					case tuiViewHistory:
						state.moveHistory(-1)
					case tuiViewDashboard, tuiViewQuota:
						state.move(-1)
					}
				case "down", "j":
					switch state.view {
					case tuiViewProfiles:
						state.moveProfile(1)
					case tuiViewHistory:
						state.moveHistory(1)
					case tuiViewDashboard, tuiViewQuota:
						state.move(1)
					}
				case "page-up":
					if state.view == tuiViewProfiles {
						state.moveProfile(-5)
					} else if state.view == tuiViewHistory {
						state.moveHistory(-5)
					} else {
						state.move(-maxInt(1, len(state.visibleEmails())/2))
					}
				case "page-down":
					if state.view == tuiViewProfiles {
						state.moveProfile(5)
					} else if state.view == tuiViewHistory {
						state.moveHistory(5)
					} else {
						state.move(maxInt(1, len(state.visibleEmails())/2))
					}
				case "home":
					if state.view == tuiViewProfiles {
						state.profileIndex = 0
					} else if state.view == tuiViewHistory {
						state.historyIndex = 0
					} else {
						state.moveToBoundary(false)
					}
				case "end":
					if state.view == tuiViewProfiles {
						state.profileIndex = maxInt(0, len(state.profileNames)-1)
					} else if state.view == tuiViewHistory {
						state.historyIndex = maxInt(0, len(state.history)-1)
					} else {
						state.moveToBoundary(true)
					}
				case "r":
					if state.view == tuiViewDoctor {
						startDoctor(false)
					} else if state.view == tuiViewDashboard || state.view == tuiViewQuota {
						state.message, state.messageType = "Refreshing quota…", "info"
						startRefresh(true)
					}
				case "a":
					if state.view == tuiViewSettings {
						a.beginTUIForm(state, "alias")
					} else {
						suspend(func() int { return a.addLoginFlow(ctx) })
					}
				case "d", "delete", "backspace":
					if state.view == tuiViewProfiles && len(state.profileNames) > 0 {
						beginConfirmAction("profile-remove", "Remove selected profile")
					} else if (state.view == tuiViewDashboard || state.view == tuiViewQuota) && state.mode == tuiBrowse {
						if email, _, ok := state.selectedAccount(); ok {
							state.mode, state.confirmEmail = tuiConfirmDelete, email
						}
					}
				case "t":
					if state.view == tuiViewSettings {
						a.beginTUIForm(state, "target")
					} else {
						toggleTier()
					}
				case "n":
					suspend(func() int { return a.cmdNext(ctx, cliArgs{}) })
				case "l":
					suspend(func() int { return a.cmdLogout(ctx) })
				case "enter":
					if state.view == tuiViewDashboard || state.view == tuiViewQuota {
						performSwitch()
					} else if state.view == tuiViewProfiles {
						if len(state.profileNames) > 0 {
							a.beginTUIForm(state, "profile-edit")
						}
					} else if state.view == tuiViewSettings {
						a.beginTUIForm(state, "settings")
					} else if state.view == tuiViewDoctor {
						startDoctor(false)
					}
				case "p":
					setView(tuiViewProfiles)
				case "h":
					setView(tuiViewHistory)
				case "s":
					setView(tuiViewSettings)
				case "o":
					setView(tuiViewDoctor)
					startDoctor(false)
				case "b":
					if state.view == tuiViewSettings {
						a.beginTUIForm(state, "binding")
					} else if state.view == tuiViewDashboard {
						setView(tuiViewBackup)
					} else {
						setView(tuiViewDashboard)
					}
				case "v":
					if state.view == tuiViewBackup {
						a.beginTUIForm(state, "backup-verify")
					} else {
						setView(tuiViewQuota)
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
					if state.view == tuiViewSettings {
						a.beginTUIForm(state, "settings")
					} else if state.view == tuiViewProfiles {
						if len(state.profileNames) > 0 {
							a.beginTUIForm(state, "profile-edit")
						}
					} else if state.view == tuiViewDashboard || state.view == tuiViewQuota {
						a.beginTUIForm(state, "tags")
					}
				case "c":
					if state.view == tuiViewProfiles {
						a.beginTUIForm(state, "profile-create")
					} else if state.view == tuiViewHistory && len(state.history) > 0 {
						beginConfirmAction("history-clear", "Clear local history")
					}
				case "m":
					suspend(func() int { return a.cmdAccount(ctx, extendedOptions{Force: true}, []string{"migrate"}) })
				case "u":
					beginConfirmAction("update", "Download and install the latest release")
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
				if state.quitRequested {
					return 0
				}
			}
		}
	}
}

func (a *Application) activeHint(accounts *Accounts, current string) string {
	if accounts == nil {
		return ""
	}
	local := localActiveEmail(accounts, current)
	if local != "" || current == "" || a.credentials == nil {
		return local
	}
	candidate := a.credentials.StoredActiveEmail()
	for _, email := range accounts.Order {
		if strings.EqualFold(email, candidate) {
			return email
		}
	}
	return ""
}
