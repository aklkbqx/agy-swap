package app

import (
	"reflect"
	"sync"
	"testing"
	"time"
)

// gatedTerminal records what it is sent. With a gate, each write waits for the
// gate to open, like a terminal that has stopped reading.
type gatedTerminal struct {
	mu      sync.Mutex
	writes  []string
	gate    chan struct{}
	started chan struct{}
}

func (g *gatedTerminal) Write(p []byte) (int, error) {
	if g.started != nil {
		select {
		case g.started <- struct{}{}:
		default:
		}
	}
	if g.gate != nil {
		<-g.gate
	}
	g.mu.Lock()
	g.writes = append(g.writes, string(p))
	g.mu.Unlock()
	return len(p), nil
}

func (g *gatedTerminal) sent() []string {
	g.mu.Lock()
	defer g.mu.Unlock()
	return append([]string(nil), g.writes...)
}

func TestFrameWriterKeepsOnlyTheNewestFrameWhileTheTerminalIsBusy(t *testing.T) {
	terminal := &gatedTerminal{gate: make(chan struct{}), started: make(chan struct{}, 8)}
	writer := newTUIFrameWriter(terminal, false)
	first := []string{"alpha", "one"}
	newest := []string{"alpha", "three"}
	writer.submit(first, 10)
	<-terminal.started // the terminal now holds the first frame and takes nothing more

	submitted := make(chan struct{})
	go func() {
		writer.submit([]string{"alpha", "two"}, 10)
		writer.submit(newest, 10)
		close(submitted)
	}()
	select {
	case <-submitted:
	case <-time.After(2 * time.Second):
		t.Fatal("submitting a frame waited for the terminal")
	}
	if !writer.busy() {
		t.Fatal("the writer reports idle while the terminal still holds a frame")
	}
	close(terminal.gate)
	writer.flush()
	if writer.busy() {
		t.Fatal("the writer reports busy after everything was written")
	}
	want := []string{tuiPaint(first, 10), tuiPaintDelta(first, newest, 10)}
	if got := terminal.sent(); !reflect.DeepEqual(got, want) {
		t.Fatalf("the terminal got %q, want the first frame and then only the change to the newest %q", got, want)
	}
	writer.close()
}

func TestFrameWriterRepaintsWholeOnRequestAndOnANewWidth(t *testing.T) {
	terminal := &gatedTerminal{}
	writer := newTUIFrameWriter(terminal, false)
	defer writer.close()
	frame := []string{"alpha", "one"}
	writer.submit(frame, 10)
	writer.flush()
	writer.submit(frame, 10) // nothing changed: nothing is sent
	writer.flush()
	writer.repaintNext()
	writer.submit(frame, 10)
	writer.flush()
	writer.submit(frame, 12)
	writer.flush()
	want := []string{tuiPaint(frame, 10), tuiPaint(frame, 10), tuiPaint(frame, 12)}
	if got := terminal.sent(); !reflect.DeepEqual(got, want) {
		t.Fatalf("the terminal got %q, want three whole paints %q", got, want)
	}
}

func TestFrameWriterSendsControlBeforeTheFrameInOneSynchronizedWrite(t *testing.T) {
	terminal := &gatedTerminal{gate: make(chan struct{}), started: make(chan struct{}, 8)}
	writer := newTUIFrameWriter(terminal, true)
	writer.submit([]string{"alpha"}, 10)
	<-terminal.started
	writer.send("\x1b]22;pointer\x1b\\")
	writer.submit([]string{"beta"}, 10)
	close(terminal.gate)
	writer.flush()
	got := terminal.sent()
	want := []string{
		"\x1b[?2026h" + tuiPaint([]string{"alpha"}, 10) + "\x1b[?2026l",
		"\x1b]22;pointer\x1b\\" + "\x1b[?2026h" + tuiPaintDelta([]string{"alpha"}, []string{"beta"}, 10) + "\x1b[?2026l",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("the terminal got %q, want %q", got, want)
	}
	writer.close()
}

func TestFrameWriterWritesNothingAfterClose(t *testing.T) {
	terminal := &gatedTerminal{}
	writer := newTUIFrameWriter(terminal, false)
	writer.submit([]string{"alpha"}, 10)
	writer.flush()
	writer.close()
	writer.submit([]string{"beta"}, 10)
	writer.send("late")
	writer.flush() // returns at once on a closed writer
	writer.close() // closing twice is harmless
	if got := terminal.sent(); len(got) != 1 {
		t.Fatalf("the terminal got %q after close, want only the first frame", got)
	}
}

func TestTimerFrameIsSkippedWhileTheTerminalIsBusyAndOwedAfterwards(t *testing.T) {
	terminal := &gatedTerminal{gate: make(chan struct{}), started: make(chan struct{}, 8)}
	state := newTUIState(NewAccounts(), "")
	state.frames = newTUIFrameWriter(terminal, false)
	defer state.frames.close()
	if !state.frameTick() {
		t.Fatal("a timer frame was skipped although the terminal is idle")
	}
	state.frames.submit([]string{"alpha"}, 10)
	<-terminal.started
	if state.frameTick() {
		t.Fatal("a timer frame was drawn while the terminal still holds the last one")
	}
	if !state.frameOwed {
		t.Fatal("the skipped frame was not remembered, so the screen could stop on a stale frame")
	}
	close(terminal.gate)
	state.frames.flush()
	if !state.frameTick() {
		t.Fatal("the owed frame was not drawn once the terminal caught up")
	}
	a := &Application{p: makePalette(false)}
	a.renderTUI(state, nil)
	if state.frameOwed {
		t.Fatal("drawing a frame did not settle the owed one")
	}
}
