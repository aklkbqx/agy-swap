package app

import (
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
