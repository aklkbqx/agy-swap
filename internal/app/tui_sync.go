package app

import (
	"context"
	"os"
	"strings"
	"sync"
	"time"
)

// The TUI notices two things without reading any file: Antigravity writing to
// its logs (it is being used, so the active account's quota is moving), and
// another agy-swap process changing accounts.json. Both look only at file
// sizes and modification times.

const (
	tuiActivityPoll  = 2 * time.Second
	tuiActivityList  = time.Minute     // how often the log files are listed again
	tuiActivityQuiet = 3 * time.Second // the active account is read once writing pauses this long
	// tuiActivityGap is the shortest time between two reads that activity
	// asks for, so a busy Antigravity costs at most one call every 10 s.
	tuiActivityGap = 10 * time.Second
)

// fileStamp is what a file looked like: enough to see that it changed.
type fileStamp struct {
	mod  time.Time
	size int64
}

func stampOf(path string) (fileStamp, bool) {
	info, err := os.Stat(path)
	if err != nil {
		return fileStamp{}, false
	}
	return fileStamp{info.ModTime(), info.Size()}, true
}

// fileWatch reports changes to one file.
type fileWatch struct {
	path string
	last fileStamp
	seen bool
}

// changed says the file differs from the last look, and remembers it.
func (w *fileWatch) changed() bool {
	stamp, ok := stampOf(w.path)
	if !ok {
		return false
	}
	before, seen := w.last, w.seen
	w.last, w.seen = stamp, true
	return seen && stamp != before
}

// tuiActivity follows Antigravity's newest log files.
type tuiActivity struct {
	pollMu   sync.Mutex
	mu       sync.Mutex
	list     func() []string
	files    map[string]fileStamp
	listedAt time.Time
	// at is when a log last grew; read is when the active account was read
	// because of it.
	at, read time.Time
}

func newTUIActivity(scanner *LogScanner) *tuiActivity {
	return &tuiActivity{list: func() []string {
		if scanner == nil {
			return nil
		}
		var paths []string
		for i, candidate := range scanner.selected() {
			if i == 5 {
				break // the newest few are where Antigravity writes now
			}
			paths = append(paths, candidate.path)
		}
		return paths
	}}
}

// poll looks at the log files and notes activity when one changed.
func (w *tuiActivity) poll(now time.Time) {
	w.pollMu.Lock()
	defer w.pollMu.Unlock()
	if w.files == nil || now.Sub(w.listedAt) >= tuiActivityList {
		files := map[string]fileStamp{}
		atChanged := false
		for _, path := range w.list() {
			if stamp, ok := stampOf(path); ok {
				files[path] = stamp
				if old, known := w.files[path]; w.files != nil && (!known || old != stamp) {
					atChanged = true
				}
			}
		}
		w.files, w.listedAt = files, now
		if atChanged {
			w.mu.Lock()
			w.at = now
			w.mu.Unlock()
		}
		return
	}
	atChanged := false
	for path, old := range w.files {
		if stamp, ok := stampOf(path); ok && stamp != old {
			w.files[path] = stamp
			atChanged = true
		}
	}
	if atChanged {
		w.mu.Lock()
		w.at = now
		w.mu.Unlock()
	}
}

// due says the active account should be read: Antigravity wrote since the
// last such read, has paused for a moment, and the last read is not too recent.
func (w *tuiActivity) due(now time.Time) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.at.After(w.read) && now.Sub(w.at) >= tuiActivityQuiet && now.Sub(w.read) >= tuiActivityGap
}

func (w *tuiActivity) markRead(now time.Time) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.read = now
}

// dueEmails are the accounts whose quota should be read now.
func (s *tuiState) dueEmails(now time.Time, tried map[string]time.Time, backoff func(email string) time.Time, activity bool) []string {
	if s.accounts == nil {
		return nil
	}
	var due []string
	for _, email := range s.accounts.Order {
		account := s.accounts.ByEmail[email]
		if getString(account, "token_data") == "" && getString(account, "secret_ref") == "" {
			continue // nothing to read with
		}
		active := strings.EqualFold(email, s.active)
		if active && activity && !backoff(email).After(now) {
			due = append(due, email)
			continue
		}
		if !quotaDueAt(account, active, tried[email], backoff(email)).After(now) {
			due = append(due, email)
		}
	}
	return due
}

type tuiAccountProgressEvent struct {
	revision     uint64
	email        string
	account      Account
	vaultPresent *bool
	index        int
	total        int
}

func (tuiAccountProgressEvent) tuiEvent() {}

type tuiRefreshCoordinator struct {
	app             *Application
	state           *tuiState
	workerCtx       context.Context
	events          chan<- tuiEvent
	render          func()
	accountsChanged func()

	refreshing   bool
	refreshQuiet bool
	revision     uint64
	cancelFn     context.CancelFunc
}

func newTUIRefreshCoordinator(workerCtx context.Context, a *Application, state *tuiState, events chan<- tuiEvent, render, accountsChanged func()) *tuiRefreshCoordinator {
	return &tuiRefreshCoordinator{
		app:             a,
		state:           state,
		workerCtx:       workerCtx,
		events:          events,
		render:          render,
		accountsChanged: accountsChanged,
	}
}

func (r *tuiRefreshCoordinator) setWorkerContext(ctx context.Context) {
	r.workerCtx = ctx
}

func (r *tuiRefreshCoordinator) busy() bool {
	return r.refreshing
}

func (r *tuiRefreshCoordinator) cancelCurrent() {
	if r.cancelFn != nil {
		r.cancelFn()
		r.cancelFn = nil
	}
}

func (r *tuiRefreshCoordinator) start(force, quiet bool) {
	r.startFor(nil, force, quiet, true)
}

func (r *tuiRefreshCoordinator) startFor(emails []string, force, quiet, syncLogs bool) {
	if r.refreshing {
		// A background refresh never interrupts an active run, and pressing r
		// twice while a manual refresh is already running keeps the first run.
		// Only a manual refresh during a quiet background read cancels the
		// partial/non-forced background read so the user's request runs now.
		if quiet || !r.refreshQuiet {
			return
		}
		r.cancelCurrent()
	}
	r.revision++
	revision := r.revision
	reqCtx, cancel := context.WithCancel(r.workerCtx)
	r.cancelFn = cancel
	r.refreshing = true
	r.refreshQuiet = quiet
	r.state.refreshDone, r.state.refreshTotal = 0, 0
	if !quiet {
		r.state.refreshing = true
		r.state.refreshStartedAt = time.Now()
		r.state.beginAnimation("refresh", 0)
		if r.render != nil {
			r.render()
		}
	}
	go func() {
		defer cancel()
		if revision == 1 && !r.app.demo {
			r.app.reconcileActiveSession(reqCtx)
		}
		if reqCtx.Err() != nil {
			return
		}
		fresh, loadErr := r.app.store.Load(!r.app.demo && syncLogs)
		errs := map[string]string{}
		if loadErr == nil && fresh != nil && fresh.Len() > 0 && !r.app.demo && r.app.quota != nil {
			onProgress := func(index, total int, account Account, status string) {
				if reqCtx.Err() != nil {
					return
				}
				email := normalizeEmail(getString(account, "email"))
				if email == "" {
					return
				}
				var vp *bool
				if ref := getString(account, "secret_ref"); ref != "" && getString(account, "token_data") == "" && r.app.vault != nil {
					_, present := r.app.vault.Get(reqCtx, ref)
					vp = &present
				}
				select {
				case r.events <- tuiAccountProgressEvent{
					revision:     revision,
					email:        email,
					account:      account,
					vaultPresent: vp,
					index:        index,
					total:        total,
				}:
				case <-reqCtx.Done():
				}
			}
			switch {
			case emails == nil:
				errs = r.app.quota.Refresh(reqCtx, fresh, force, onProgress)
			case len(emails) > 0:
				errs = r.app.quota.RefreshEmailsWithProgress(reqCtx, fresh, emails, onProgress)
			}
		}
		if reqCtx.Err() != nil {
			return
		}
		if loadErr != nil {
			errs["store"] = loadErr.Error()
		}
		var vaultCache map[string]bool
		if loadErr == nil && fresh != nil {
			vaultCache = r.app.warmVaultCache(reqCtx, fresh)
		}
		if reqCtx.Err() != nil {
			return
		}
		select {
		case r.events <- tuiAccountsEvent{accounts: fresh, vaultCache: vaultCache, quotaErrors: errs, revision: revision}:
		case <-reqCtx.Done():
		}
	}()
}

func (r *tuiRefreshCoordinator) invalidate() {
	r.cancelCurrent()
	r.revision++
	r.refreshing = false
	r.state.refreshing = false
	r.state.refreshDone, r.state.refreshTotal = 0, 0
}

func (r *tuiRefreshCoordinator) applyProgress(ev tuiAccountProgressEvent) bool {
	if ev.revision != r.revision || !r.refreshing {
		return false
	}
	r.state.refreshDone, r.state.refreshTotal = ev.index, ev.total
	if r.state.accounts != nil && ev.email != "" && ev.account != nil {
		r.state.accounts.Set(ev.email, ev.account)
		if ev.vaultPresent != nil {
			if ref := getString(ev.account, "secret_ref"); ref != "" {
				if r.state.vaultCache == nil {
					r.state.vaultCache = map[string]bool{}
				}
				r.state.vaultCache[ref] = *ev.vaultPresent
			}
		}
	}
	return true
}

func (r *tuiRefreshCoordinator) applyDone(ev tuiAccountsEvent) bool {
	if ev.revision != r.revision {
		return false
	}
	r.cancelCurrent()
	r.refreshing = false
	r.state.refreshing = false
	r.state.refreshDone, r.state.refreshTotal = 0, 0
	if r.accountsChanged != nil {
		r.accountsChanged()
	}
	r.state.setAccountsWithVaultCache(ev.accounts, ev.vaultCache)
	r.state.quotaErrors = ev.quotaErrors
	if message, kind, animate := refreshNotice(r.refreshQuiet, ev.quotaErrors); message != "" {
		r.state.message, r.state.messageType = message, kind
		if animate {
			r.state.beginAnimation(map[string]string{"success": "success", "warning": "error"}[kind], 360*time.Millisecond)
		}
	}
	return true
}
