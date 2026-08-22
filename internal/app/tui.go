package app

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"golang.org/x/term"
)

type tuiKeyEvent struct{ key string }
type tuiAccountsEvent struct {
	accounts    *Accounts
	quotaErrors map[string]string
}
type tuiActiveEvent struct {
	token string
	email string
}
type tuiResizeEvent struct{ width, height int }

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
	events := make(chan any, 24)
	done := make(chan struct{})
	var closeDone sync.Once
	finish := func() {
		closeDone.Do(func() { close(done) })
	}

	go func() {
		for {
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
		local := localActiveEmail(state.accounts, current)
		if local != "" || current == "" {
			state.active = local
			state.resolvingToken = ""
			return
		}
		if state.resolvingToken == current {
			return
		}
		token := current
		snapshot := state.accounts
		state.resolvingToken = token
		go func() {
			email := a.activeEmail(ctx, snapshot, token)
			select {
			case events <- tuiActiveEvent{token: token, email: email}:
			case <-ctx.Done():
			}
		}()
	}

	refreshing := false
	startRefresh := func(force bool) {
		if refreshing {
			return
		}
		refreshing = true
		state.refreshing = true
		state.beginAnimation("refresh", 0)
		a.renderTUI(state, outFile)
		go func() {
			fresh, loadErr := a.store.Load(true)
			errs := map[string]string{}
			if loadErr == nil && fresh.Len() > 0 {
				errs = a.quota.Refresh(ctx, fresh, force, nil)
			}
			if loadErr != nil {
				errs["store"] = loadErr.Error()
			}
			select {
			case events <- tuiAccountsEvent{fresh, errs}:
			case <-ctx.Done():
			}
		}()
	}

	var frameTimer *time.Timer
	var frameC <-chan time.Time
	armFrame := func() {
		if !state.animation.active {
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
	a.renderTUI(state, outFile)
	startRefresh(false)
	startActiveResolve()
	armFrame()

	suspend := func(action func() int) {
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
		state.active = localActiveEmail(state.accounts, current)
		startActiveResolve()
		a.renderTUI(state, outFile)
	}

	performDelete := func(email string) {
		fresh, loadErr := a.store.Load(false)
		if loadErr != nil {
			state.message, state.messageType = loadErr.Error(), "error"
			return
		}
		fresh.Delete(email)
		if saveErr := a.store.Save(fresh); saveErr != nil {
			state.message, state.messageType = saveErr.Error(), "error"
			return
		}
		state.setAccounts(fresh)
		state.message, state.messageType = "Removed account "+email, "success"
		state.beginAnimation("success", 360*time.Millisecond)
	}

	toggleTier := func() {
		email, account, ok := state.selectedAccount()
		if !ok {
			return
		}
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
		default:
			if len(key) == 1 && key[0] >= 32 && key[0] != 127 {
				state.search += strings.ToLower(key)
				state.clampSelection()
			}
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
				state.active = localActiveEmail(state.accounts, current)
				startActiveResolve()
				a.renderTUI(state, outFile)
			}
		case <-quotaTicker.C:
			if !refreshing {
				state.message, state.messageType = "Background sync…", "info"
				startRefresh(false)
			}
		case <-frameC:
			if state.advanceAnimation(time.Now()) {
				a.renderTUI(state, outFile)
				armFrame()
			} else {
				a.renderTUI(state, outFile)
				armFrame()
			}
		case event := <-events:
			switch value := event.(type) {
			case tuiAccountsEvent:
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
				state.width, state.height = value.width, value.height
				a.renderTUI(state, outFile)
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

				switch key {
				case "q", "esc", "ctrl-c", "ctrl-d":
					finish()
					return 0
				case "?":
					state.mode = tuiHelp
				case "/":
					state.beginSearch()
				case "up", "k":
					state.move(-1)
				case "down", "j":
					state.move(1)
				case "r":
					state.message, state.messageType = "Refreshing quota…", "info"
					startRefresh(true)
				case "a":
					suspend(func() int { return a.addLoginFlow(ctx) })
				case "d", "delete", "backspace":
					if email, _, ok := state.selectedAccount(); ok {
						state.mode, state.confirmEmail = tuiConfirmDelete, email
					}
				case "t":
					toggleTier()
				case "n":
					suspend(func() int { return a.cmdNext(ctx, cliArgs{}) })
				case "l":
					suspend(func() int { return a.cmdLogout(ctx) })
				case "enter":
					if email, account, ok := state.selectedAccount(); ok {
						suspend(func() int {
							fmt.Fprintf(a.Out, "Switching to %s%s%s %s<%s>%s…\n", a.p.Bold, getString(account, "name"), a.p.Reset, a.p.Gray, email, a.p.Reset)
							if a.credentials.Apply(ctx, getString(account, "token_data"), email) {
								return 0
							}
							return 1
						})
					}
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
			}
		}
	}
}
