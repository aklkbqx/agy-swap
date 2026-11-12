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
	vaultCache  map[string]bool
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
	email      string
	message    string
	kind       string
	fresh      *Accounts
	vaultCache map[string]bool
	current    string
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
	// From here to the end every frame goes through frames, so the loop never
	// waits for the terminal. It is closed before the screen is handed back.
	var frames *tuiFrameWriter
	defer func() {
		frames.close()
		if raw {
			origTerm.Restore(false)
		}
		leaveScreen()
	}()

	state := newTUIState(accounts, current)
	frames = newTUIFrameWriter(a.Out, true)
	state.frames = frames
	state.startedAt = time.Now() // the screen wipes in once
	state.vault = a.vault
	state.vaultCache = a.warmVaultCache(ctx, accounts)
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
	events := make(chan tuiEvent, 256)
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
				default:
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

	var resolvedToken, resolvedEmail string
	startActiveResolve := func() {
		local := localActiveEmail(state.accounts, current)
		if local != "" || current == "" {
			state.active = local
			state.resolvingToken = ""
			resolvedToken, resolvedEmail = current, local
			return
		}
		if current == resolvedToken && resolvedEmail != "" && state.accounts != nil {
			if _, ok := state.accounts.Get(resolvedEmail); ok {
				state.active = resolvedEmail
				state.resolvingToken = ""
				return
			}
		}
		if state.resolvingToken == current {
			return
		}
		token := current
		snapshot := copyAccounts(state.accounts)
		state.resolvingToken = token
		go func() {
			email := a.activeHint(snapshot, token)
			if email == "" {
				resolveCtx, cancel := context.WithTimeout(workerCtx, 5*time.Second)
				email = a.activeEmail(resolveCtx, snapshot, token)
				cancel()
			}
			select {
			case events <- tuiActiveEvent{token: token, email: email}:
			case <-workerCtx.Done():
			}
		}()
	}
	accountsWatch := &fileWatch{path: a.paths.Accounts}
	accountsWatch.changed()
	refreshCoord := newTUIRefreshCoordinator(
		workerCtx,
		a,
		state,
		events,
		func() { a.renderTUI(state, outFile) },
		func() { accountsWatch.changed() },
	)
	startRefreshFor := refreshCoord.startFor
	startRefresh := refreshCoord.start
	invalidateRefresh := refreshCoord.invalidate

	var frameTimer *time.Timer
	var frameC <-chan time.Time
	armFrame := func() {
		now := time.Now()
		moving := state.moving(now)
		if !state.animation.active && !state.toastActive(now) && !moving && !state.ambient && !state.frameOwed {
			frameC = nil
			if frameTimer != nil {
				frameTimer.Stop()
			}
			return
		}
		interval := 50 * time.Millisecond
		switch {
		case moving || state.frameOwed:
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
		frames.flush() // the terminal has everything before another program takes it
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
			refreshCoord.setWorkerContext(workerCtx)
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
		state.repaintWhole() // the screen was left, so the next frame is painted whole
		fresh, loadErr := a.store.Load(false)
		if loadErr == nil {
			state.setAccountsWithVaultCache(fresh, a.warmVaultCache(ctx, fresh))
		}
		current = a.credentials.Current(ctx)
		state.current = current
		resolvedToken, resolvedEmail = "", ""
		state.active = localActiveEmail(state.accounts, current)
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
			var vaultCache map[string]bool
			if loadErr != nil {
				fresh = nil
			} else if fresh != nil {
				vaultCache = a.warmVaultCache(workerCtx, fresh)
			}
			cur := a.credentials.Current(workerCtx)
			select {
			case events <- tuiSwitchEvent{email: email, message: message, kind: kind, fresh: fresh, vaultCache: vaultCache, current: cur}:
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

	var stagedEvent tuiEvent
	eventsBurst := 0
	for {
		// A render can start motion (a new target, a dialog, a toast); keep
		// frames coming until it settles.
		if frameC == nil && (state.moving(time.Now()) || state.ambient || state.frameOwed) {
			armFrame()
		}
		var event tuiEvent
		if stagedEvent != nil {
			event = stagedEvent
			stagedEvent = nil
			eventsBurst++
		} else if eventsBurst < 8 {
			select {
			case ev := <-events:
				event = ev
				eventsBurst++
			default:
			}
		}
		if event == nil {
			eventsBurst = 0
			select {
			case <-tuiCtx.Done():
				finish()
				return 0
			case <-resizeTicker.C:
				width, height, sizeErr := termSize(outFile)
				if sizeErr == nil && (width != state.width || height != state.height) {
					a.renderTUI(state, outFile)
				}
				continue
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
				continue
			case now := <-syncTicker.C:
				if now.Minute() != lastMinute {
					lastMinute = now.Minute()
					a.renderTUI(state, outFile)
				}
				if refreshCoord.busy() || switching || a.demo || a.quota == nil {
					continue
				}
				if accountsWatch.changed() {
					startRefreshFor([]string{}, false, true, false) // another process changed the accounts
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
					startRefreshFor(due, false, true, busy)
				}
				continue
			case <-frameC:
				now := time.Now()
				state.expireToast(now)
				state.advanceAnimation(now)
				if state.frameTick() {
					a.renderTUI(state, outFile)
				}
				armFrame()
				continue
			case ev := <-events:
				event = ev
			}
		}
		switch value := event.(type) {
		case tuiCredentialEvent:
			credentialPolling = false
			if value.token != current {
				current = value.token
				state.current = current
				resolvedToken, resolvedEmail = "", ""
				state.active = localActiveEmail(state.accounts, current)
				startActiveResolve()
				a.renderTUI(state, outFile)
			}
		case tuiActivityPollEvent:
			activityPolling = false
			if refreshCoord.busy() || switching || a.demo || a.quota == nil || !value.busy {
				continue
			}
			if due := state.dueEmails(value.now, tried, a.quota.BackoffUntil, true); len(due) > 0 {
				for _, email := range due {
					tried[email] = value.now
				}
				activity.markRead(value.now)
				startRefreshFor(due, false, true, true)
			}
		case tuiSwitchEvent:
			switching = false
			accountsWatch.changed()
			if value.fresh != nil {
				state.setAccountsWithVaultCache(value.fresh, value.vaultCache)
			}
			current = value.current
			state.current = current
			if value.kind == "error" {
				resolvedToken, resolvedEmail = "", ""
				state.active = localActiveEmail(state.accounts, current)
			} else {
				resolvedToken, resolvedEmail = current, value.email
				state.active = value.email
			}
			startActiveResolve()
			state.message, state.messageType = "", "info"
			state.showToast(value.message, value.kind)
			a.renderTUI(state, outFile)
			armFrame()
		case tuiAccountProgressEvent:
			if !refreshCoord.applyProgress(value) {
				continue
			}
			a.renderTUI(state, outFile)
			armFrame()
		case tuiAccountsEvent:
			if !refreshCoord.applyDone(value) {
				// A slower refresh must never overwrite a newer frame.
				continue
			}
			startActiveResolve()
			a.renderTUI(state, outFile)
			armFrame()
		case tuiActiveEvent:
			if value.token == current {
				resolvedToken, resolvedEmail = value.token, value.email
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
					state.setAccountsWithVaultCache(fresh, a.warmVaultCache(ctx, fresh))
				}
				current = a.credentials.Current(ctx)
				state.current = current
				resolvedToken, resolvedEmail = "", ""
				startActiveResolve()
			}
			a.renderTUI(state, outFile)
			armFrame()
		case tuiKeyEvent:
			value, stagedEvent = coalescePointerKeyEvent(value, events)
			key := strings.ToLower(value.key)
			if steps, wheelCol, wheelRow, nextAfterWheel := coalesceWheelKeyEvent(value, events); steps != 0 || strings.HasPrefix(key, "wheel-") {
				if stagedEvent == nil {
					stagedEvent = nextAfterWheel
				}
				if steps != 0 {
					// The wheel scrolls the view under the pointer when it does not
					// all fit; otherwise it moves the selection, as it always has.
					line, x := state.pointerAt(wheelCol, wheelRow)
					scrolled := false
					if hit, ok := state.hitAt(line, x, hitScroll, hitScrollbar); ok && wheelRow > 0 {
						scrolled = state.scrollBy(hit.key, 3*steps)
					}
					if !scrolled {
						stepDir := 1
						count := steps
						if steps < 0 {
							stepDir = -1
							count = -steps
						}
						for range count {
							switch {
							case state.mode == tuiPalette:
								state.movePalette(stepDir)
							case state.mode != tuiBrowse:
							case state.view == tuiViewProfiles:
								state.moveProfile(stepDir)
							case state.view == tuiViewHistory:
								state.moveHistory(stepDir)
							default:
								state.move(stepDir)
							}
						}
					}
					a.renderTUI(state, outFile)
					armFrame()
				}
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
					shape:         func(shape string) { frames.send("\x1b]22;" + shape + "\x1b\\") },
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

func coalesceWheelKeyEvent(first tuiKeyEvent, events <-chan tuiEvent) (dir, col, row int, staged tuiEvent) {
	dir, col, row = wheelEvent(strings.ToLower(first.key))
	if dir == 0 {
		return 0, 0, 0, nil
	}
	for {
		select {
		case next := <-events:
			if nextKey, ok := next.(tuiKeyEvent); ok {
				nextDir, nextCol, nextRow := wheelEvent(strings.ToLower(nextKey.key))
				if nextDir != 0 && nextCol == col && nextRow == row {
					dir += nextDir
					continue
				}
			}
			return dir, col, row, next
		default:
			return dir, col, row, nil
		}
	}
}

func coalescePointerKeyEvent(first tuiKeyEvent, events <-chan tuiEvent) (tuiKeyEvent, tuiEvent) {
	lower := strings.ToLower(first.key)
	var prefix string
	switch {
	case strings.HasPrefix(lower, "mouse:move:"):
		prefix = "mouse:move:"
	case strings.HasPrefix(lower, "mouse:drag:"):
		prefix = "mouse:drag:"
	default:
		return first, nil
	}
	latest := first
	for {
		select {
		case next := <-events:
			if nextKey, ok := next.(tuiKeyEvent); ok && strings.HasPrefix(strings.ToLower(nextKey.key), prefix) {
				latest = nextKey
				continue
			}
			return latest, next
		default:
			return latest, nil
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
