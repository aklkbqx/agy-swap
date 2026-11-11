package app

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
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
type tuiCredentialEvent struct{ token string }
type tuiActivityPollEvent struct {
	busy bool
	now  time.Time
}
type tuiSwitchEvent struct {
	email   string
	message string
	kind    string
	fresh   *Accounts
	current string
}

type tuiJobResult struct {
	message       string
	err           error
	doctorChecks  []doctorCheck
	doctorHealthy bool
}

type tuiEvent interface{ tuiEvent() }

func (tuiKeyEvent) tuiEvent()          {}
func (tuiAccountsEvent) tuiEvent()     {}
func (tuiActiveEvent) tuiEvent()       {}
func (tuiResizeEvent) tuiEvent()       {}
func (tuiJobEvent) tuiEvent()          {}
func (tuiCredentialEvent) tuiEvent()   {}
func (tuiActivityPollEvent) tuiEvent() {}
func (tuiSwitchEvent) tuiEvent()       {}

func (a *Application) cmdInteractive(rootCtx context.Context) int {
	tuiCtx := rootCtx
	ctx := tuiCtx
	inFile, inOK := a.In.(*os.File)
	outFile, outOK := a.Out.(*os.File)
	if !inOK || !outOK || !term.IsTerminal(int(inFile.Fd())) || !term.IsTerminal(int(outFile.Fd())) {
		return a.cmdList(ctx, cliArgs{})
	}
	sigintGuard := make(chan os.Signal, 1)
	signal.Notify(sigintGuard, os.Interrupt)
	defer signal.Stop(sigintGuard)
	accounts, err := a.store.Load(false)
	if err != nil {
		return a.storeError(err)
	}
	current := a.credentials.Current(ctx)
	origTerm := captureTerminalState(inFile, outFile)
	if _, err := term.MakeRaw(int(inFile.Fd())); err != nil {
		return a.storeError(err)
	}
	enableTUIRawOutput(outFile)
	raw := true
	// The mouse reports buttons, drags and, for hover, every move (1003).
	mouseOn, mouseOff := "\x1b[?1000h\x1b[?1002h\x1b[?1003h\x1b[?1006h", "\x1b[?1006l\x1b[?1003l\x1b[?1002l\x1b[?1000l"
	if !tuiMouseEnabled() {
		mouseOn, mouseOff = "", ""
	}
	enterScreen := func() { fmt.Fprint(a.Out, "\x1b[?1049h\x1b[?25l"+mouseOn+"\x1b[H") }
	if mouseOn != "" && tuiPointerShapes() {
		mouseOff = "\x1b]22;default\x1b\\" + mouseOff // give the pointer back as it was
	}
	leaveScreen := func() { fmt.Fprint(a.Out, mouseOff+"\x1b[?1049l\x1b[?25h") }
	enterScreen()
	defer func() {
		if raw {
			origTerm.Restore(false)
		}
		leaveScreen()
	}()

	state := newTUIState(accounts, current)
	state.startedAt = time.Now() // the screen wipes in once
	state.vault = a.vault
	var initialSplitOffset, initialStackOffset int
	if initialSettings, err := a.loadSettings(); err == nil {
		state.settings = initialSettings
		state.settingsLoaded = true
		state.splitOffset = initialSettings.UI.SplitOffset
		initialSplitOffset = initialSettings.UI.SplitOffset
		state.stackOffset = initialSettings.UI.StackOffset
		initialStackOffset = initialSettings.UI.StackOffset
	}
	defer func() {
		if (state.splitOffset != initialSplitOffset || state.stackOffset != initialStackOffset) && a.store != nil {
			if s, err := a.loadSettings(); err == nil {
				s.UI.SplitOffset = state.splitOffset
				s.UI.StackOffset = state.stackOffset
				_ = a.store.SaveSettings(s)
			}
		}
	}()
	events := make(chan tuiEvent, 32)
	done := make(chan struct{})
	workerCtx, cancelWorkers := context.WithCancel(tuiCtx)
	var stopRecreatedTUI func()
	defer func() {
		cancelWorkers()
		if stopRecreatedTUI != nil {
			stopRecreatedTUI()
		}
	}()
	var inputPaused atomic.Bool
	var inputBusy atomic.Bool
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
			inputBusy.Store(true)
			key := readTerminalKeyPaused(inFile, &inputPaused)
			inputBusy.Store(false)
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
		snapshot := copyAccounts(state.accounts)
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
	// startRefresh reloads quota in the background. A quiet refresh (on start,
	// on the timer, after adding an account) shows nothing unless it fails;
	// only a refresh the user asked for shows its progress and result.
	refreshQuiet := false
	// startRefreshFor reads the quota of these accounts (every account when
	// emails is nil, none when it is empty: then it only reloads the store).
	startRefreshFor := func(emails []string, force, quiet bool) {
		if refreshing {
			return
		}
		refreshRevision++
		revision := refreshRevision
		refreshing = true
		refreshQuiet = quiet
		if !quiet {
			state.refreshing = true
			state.refreshStartedAt = time.Now()
			state.beginAnimation("refresh", 0)
			a.renderTUI(state, outFile)
		}
		go func() {
			if revision == 1 && !a.demo {
				a.reconcileActiveSession(workerCtx)
			}
			fresh, loadErr := a.store.Load(!a.demo)
			errs := map[string]string{}
			if loadErr == nil && fresh.Len() > 0 && !a.demo {
				switch {
				case emails == nil:
					errs = a.quota.Refresh(workerCtx, fresh, force, nil)
				case len(emails) > 0:
					errs = a.quota.RefreshEmails(workerCtx, fresh, emails)
				}
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
	startRefresh := func(force, quiet bool) { startRefreshFor(nil, force, quiet) }
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
		now := time.Now()
		moving := state.moving(now)
		if !state.animation.active && !state.toastActive(now) && !moving && !state.ambient {
			frameC = nil
			if frameTimer != nil {
				frameTimer.Stop()
			}
			return
		}
		interval := 50 * time.Millisecond
		switch {
		case moving:
			interval = tuiFrameInterval
		case state.ambient && !state.animation.active && !state.toastActive(now):
			interval = tuiAmbientFrame // only the dot breathes
		}
		if frameTimer == nil {
			frameTimer = time.NewTimer(interval)
		} else {
			if !frameTimer.Stop() {
				select {
				case <-frameTimer.C:
				default:
				}
			}
			frameTimer.Reset(interval)
		}
		frameC = frameTimer.C
	}

	resizeTicker := time.NewTicker(250 * time.Millisecond)
	defer resizeTicker.Stop()
	credentialTicker := time.NewTicker(1500 * time.Millisecond)
	defer credentialTicker.Stop()
	// Every second the sync timer reads the accounts that are due (see
	// quotaDueAt), repaints when the minute turns so countdowns move, and
	// notices Antigravity's activity and other agy-swap processes.
	syncTicker := time.NewTicker(time.Second)
	defer syncTicker.Stop()
	activity := newTUIActivity(nil)
	if a.store != nil {
		activity = newTUIActivity(a.store.scanner)
	}
	accountsWatch := &fileWatch{path: a.paths.Accounts}
	accountsWatch.changed()
	tried := map[string]time.Time{}
	var activityPolled time.Time
	lastMinute := time.Now().Minute()
	defer func() {
		if frameTimer != nil {
			frameTimer.Stop()
		}
	}()
	a.renderTUI(state, outFile)
	var draggingSplit bool
	// draggingThumb names the scroll view whose thumb is being dragged, and
	// thumbGrab and thumbTop where the thumb was caught and where its view starts.
	var draggingThumb string
	var thumbGrab, thumbTop int
	var lastClickTime time.Time
	var lastClickCol, lastClickRow int
	startActiveResolve()
	startRefresh(false, true)
	armFrame()
	demoNotice := func() {
		state.message, state.messageType = errDemoUnavailable.Error(), "info"
	}

	suspend := func(action func() int) int {
		if a.demo {
			demoNotice()
			return 1
		}
		invalidateRefresh()
		inputPaused.Store(true)
		waitUntilInputIdle(&inputBusy, tuiInputPollTimeout+150*time.Millisecond)
		resetPlatformInputState()
		a.resetTerminalLineReader()
		defer inputPaused.Store(false)
		origTerm.Restore(false)
		raw = false
		leaveScreen()
		suspendCtx, stopInterrupt := signal.NotifyContext(context.WithoutCancel(tuiCtx), os.Interrupt, syscall.SIGTERM)
		ctx = suspendCtx
		code := action()
		interrupted := suspendCtx.Err() != nil
		stopInterrupt()
		if tuiCtx.Err() != nil && interrupted {
			if stopRecreatedTUI != nil {
				stopRecreatedTUI()
			}
			tuiCtx, stopRecreatedTUI = signal.NotifyContext(context.WithoutCancel(tuiCtx), syscall.SIGTERM)
			workerCtx, cancelWorkers = context.WithCancel(tuiCtx)
		}
		ctx = tuiCtx
		switch {
		case code == 0:
			state.message, state.messageType = "Completed", "success"
		case interrupted:
			state.message, state.messageType = "Cancelled", "info"
		default:
			state.message, state.messageType = "Action failed", "error"
		}
		if _, stateErr := term.MakeRaw(int(inFile.Fd())); stateErr == nil {
			raw = true
		}
		enableTUIRawOutput(outFile)
		enterScreen()
		state.lastFrame = nil // the screen was left, so the next frame is painted whole
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

	// performSwitch switches in place: the dashboard stays on screen, with a
	// status line while the session is written, and a toast with the result.
	var switching bool
	var credentialPolling bool
	var activityPolling bool
	performSwitch := func() {
		email, account, ok := state.selectedAccount()
		if !ok || switching {
			return
		}
		switching = true
		invalidateRefresh()
		accountSnap := copyAccount(account)
		state.message, state.messageType = "Switching to "+email+"…", "info"
		a.renderTUI(state, outFile)
		go func() {
			message, kind := a.tuiSwitchAccount(workerCtx, email, accountSnap)
			fresh, loadErr := a.store.Load(false)
			if loadErr != nil {
				fresh = nil
			}
			cur := a.credentials.Current(workerCtx)
			select {
			case events <- tuiSwitchEvent{email: email, message: message, kind: kind, fresh: fresh, current: cur}:
			case <-workerCtx.Done():
			}
		}()
	}

	confirmSwitch := func() {
		state.beginSwitchConfirm()
		a.renderTUI(state, outFile)
	}

	ctrl := &tuiController{
		app:               a,
		state:             state,
		outFile:           outFile,
		ctx:               func() context.Context { return ctx },
		current:           func() string { return current },
		demoNotice:        demoNotice,
		suspend:           suspend,
		startJob:          startJob,
		startRefresh:      startRefresh,
		invalidateRefresh: invalidateRefresh,
		accountsChanged:   func() { accountsWatch.changed() },
		performSwitch:     performSwitch,
		confirmSwitch:     confirmSwitch,
		finish:            finish,
	}

	for {
		// A render can start motion (a new target, a dialog, a toast); keep
		// frames coming until it settles.
		if frameC == nil && (state.moving(time.Now()) || state.ambient) {
			armFrame()
		}
		select {
		case <-tuiCtx.Done():
			finish()
			return 0
		case <-resizeTicker.C:
			width, height, sizeErr := termSize(outFile)
			if sizeErr == nil && (width != state.width || height != state.height) {
				a.renderTUI(state, outFile)
			}
		case <-credentialTicker.C:
			if credentialPolling || switching {
				continue
			}
			credentialPolling = true
			go func() {
				tok := a.credentials.Current(workerCtx)
				select {
				case events <- tuiCredentialEvent{token: tok}:
				case <-workerCtx.Done():
				}
			}()
		case now := <-syncTicker.C:
			if now.Minute() != lastMinute {
				lastMinute = now.Minute()
				a.renderTUI(state, outFile)
			}
			if refreshing || switching || a.demo || a.quota == nil {
				continue
			}
			if accountsWatch.changed() {
				startRefreshFor([]string{}, false, true) // another process changed the accounts
				continue
			}
			if !activityPolling && now.Sub(activityPolled) >= tuiActivityPoll {
				activityPolling = true
				activityPolled = now
				go func(pollTime time.Time) {
					activity.poll(pollTime)
					busy := activity.due(pollTime)
					select {
					case events <- tuiActivityPollEvent{busy: busy, now: pollTime}:
					case <-workerCtx.Done():
					}
				}(now)
			}
			busy := activity.due(now)
			if due := state.dueEmails(now, tried, a.quota.BackoffUntil, busy); len(due) > 0 {
				for _, email := range due {
					tried[email] = now
				}
				if busy {
					activity.markRead(now)
				}
				startRefreshFor(due, false, true)
			}
		case <-frameC:
			now := time.Now()
			state.expireToast(now)
			state.advanceAnimation(now)
			a.renderTUI(state, outFile)
			armFrame()
		case event := <-events:
			switch value := event.(type) {
			case tuiCredentialEvent:
				credentialPolling = false
				if value.token != current {
					current = value.token
					state.current = current
					state.active = a.activeHint(state.accounts, current)
					startActiveResolve()
					a.renderTUI(state, outFile)
				}
			case tuiActivityPollEvent:
				activityPolling = false
				if refreshing || switching || a.demo || a.quota == nil || !value.busy {
					continue
				}
				if due := state.dueEmails(value.now, tried, a.quota.BackoffUntil, true); len(due) > 0 {
					for _, email := range due {
						tried[email] = value.now
					}
					activity.markRead(value.now)
					startRefreshFor(due, false, true)
				}
			case tuiSwitchEvent:
				switching = false
				accountsWatch.changed()
				if value.fresh != nil {
					state.setAccounts(value.fresh)
				}
				current = value.current
				state.current = current
				if value.kind == "error" {
					state.active = a.activeHint(state.accounts, current)
				} else {
					state.active = value.email
				}
				startActiveResolve()
				state.message, state.messageType = "", "info"
				state.showToast(value.message, value.kind)
				a.renderTUI(state, outFile)
				armFrame()
			case tuiAccountsEvent:
				if value.revision != refreshRevision {
					// A slower refresh must never overwrite a newer frame.
					continue
				}
				refreshing = false
				state.refreshing = false
				accountsWatch.changed() // this refresh's own save is not another process
				state.setAccounts(value.accounts)
				state.quotaErrors = value.quotaErrors
				if message, kind, animate := refreshNotice(refreshQuiet, value.quotaErrors); message != "" {
					state.message, state.messageType = message, kind
					if animate {
						state.beginAnimation(map[string]string{"success": "success", "warning": "error"}[kind], 360*time.Millisecond)
					}
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
					accountsWatch.changed()
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
				if dir, wheelCol, wheelRow := wheelEvent(key); dir != 0 {
					// The wheel scrolls the view under the pointer when it does not
					// all fit; otherwise it moves the selection, as it always has.
					line, x := state.pointerAt(wheelCol, wheelRow)
					scrolled := false
					if hit, ok := state.hitAt(line, x, hitScroll, hitScrollbar); ok && wheelRow > 0 {
						scrolled = state.scrollBy(hit.key, 3*dir)
					}
					if !scrolled {
						switch {
						case state.mode == tuiPalette:
							state.movePalette(dir)
						case state.mode != tuiBrowse:
						case state.view == tuiViewProfiles:
							state.moveProfile(dir)
						case state.view == tuiViewHistory:
							state.moveHistory(dir)
						default:
							state.move(dir)
						}
					}
					a.renderTUI(state, outFile)
					armFrame()
					continue
				}
				if strings.HasPrefix(key, "mouse:") {
					if a.tuiPointer(state, key, &tuiPointerState{
						dragging: &draggingSplit, thumb: &draggingThumb, grab: &thumbGrab, thumbTop: &thumbTop,
						lastTime: &lastClickTime, lastCol: &lastClickCol, lastRow: &lastClickRow,
						press: func(k string) {
							select {
							case events <- tuiKeyEvent{key: k}:
							default:
							}
						},
						confirmSwitch: confirmSwitch,
						shape:         func(shape string) { fmt.Fprint(outFile, "\x1b]22;"+shape+"\x1b\\") },
					}) {
						a.renderTUI(state, outFile)
						armFrame()
					}
					continue
				}
				if ctrl.handleKeyEvent(value.key, key, armFrame) {
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
	if matched := matchAccountByToken(context.Background(), accounts, a.vault, current); matched != "" {
		return matched
	}
	candidate := a.credentials.StoredActiveEmail()
	for _, email := range accounts.Order {
		if strings.EqualFold(email, candidate) {
			return email
		}
	}
	return ""
}
