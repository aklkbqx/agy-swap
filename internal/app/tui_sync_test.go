package app

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestAFileWatchSeesAnotherWriteButNotTheFirstLook(t *testing.T) {
	path := filepath.Join(t.TempDir(), "accounts.json")
	if err := os.WriteFile(path, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	w := &fileWatch{path: path}
	if w.changed() {
		t.Fatal("the first look is not a change")
	}
	if err := os.WriteFile(path, []byte(`{"a":1}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if !w.changed() || w.changed() {
		t.Fatal("a write is one change")
	}
}

func TestAntigravityWritingToItsLogMakesTheActiveAccountDueOncePaused(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cli.log")
	if err := os.WriteFile(path, []byte("start\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	w := &tuiActivity{list: func() []string { return []string{path} }}
	now := time.Date(2030, 1, 1, 12, 0, 0, 0, time.UTC)
	w.poll(now)
	if w.due(now.Add(time.Minute)) {
		t.Fatal("logs that did not change are not activity")
	}
	if err := os.WriteFile(path, []byte("start\nrequest\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	w.poll(now.Add(2 * time.Second))
	if w.due(now.Add(3 * time.Second)) {
		t.Fatal("the read waits for writing to pause")
	}
	if !w.due(now.Add(2*time.Second + tuiActivityQuiet)) {
		t.Fatal("activity followed by a pause makes the active account due")
	}
	w.read = now.Add(2*time.Second + tuiActivityQuiet)
	if w.due(now.Add(time.Minute)) {
		t.Fatal("one burst of activity is one read")
	}
	if err := os.WriteFile(path, []byte("start\nrequest\nmore\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	w.poll(w.read.Add(time.Second))
	if w.due(w.read.Add(time.Second + tuiActivityQuiet)) {
		t.Fatal("activity reads are at least tuiActivityGap apart")
	}
	if !w.due(w.read.Add(tuiActivityGap)) {
		t.Fatal("after the gap the next burst is read")
	}
}

func TestDueEmailsFollowTheScheduleAndActivity(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	fresh := scheduledAccount(now, 0.5, 0.5, now.Add(48*time.Hour))
	fresh["token_data"] = "t"
	stale := scheduledAccount(now.Add(-3*time.Minute), 0.5, 0.5, now.Add(48*time.Hour))
	stale["token_data"] = "t"
	accounts := NewAccounts()
	accounts.Set("active@example.com", fresh)
	accounts.Set("stale@example.com", stale)
	accounts.Set("none@example.com", Account{"email": "none@example.com"})
	state := newTUIState(accounts, "")
	state.active = "active@example.com"
	none := func(string) time.Time { return time.Time{} }
	if got := state.dueEmails(now, nil, none, false); len(got) != 1 || got[0] != "stale@example.com" {
		t.Fatalf("due = %v, want only the stale account", got)
	}
	if got := state.dueEmails(now, nil, none, true); len(got) != 2 || got[0] != "active@example.com" {
		t.Fatalf("with activity due = %v, want the active account too", got)
	}
	blocked := func(email string) time.Time { return now.Add(time.Minute) }
	if got := state.dueEmails(now, nil, blocked, true); len(got) != 0 {
		t.Fatalf("backing off, due = %v", got)
	}
}
