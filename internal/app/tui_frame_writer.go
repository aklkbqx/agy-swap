package app

import (
	"io"
	"strings"
	"sync"
)

// tuiFrameWriter sends frames to the terminal away from the interactive loop.
// A terminal takes output at its own pace, and a write waits for it; done in
// the loop, that wait holds back the keyboard, the mouse and every timer.
// The loop hands each frame here and moves on. When the terminal falls
// behind, only the newest frame is kept, and it is compared with what the
// terminal last received, so a frame that was never sent costs nothing.
type tuiFrameWriter struct {
	out io.Writer
	// synchronized wraps each frame in DEC private mode 2026, so a terminal
	// that knows it shows the frame at once; others ignore the markers.
	synchronized bool

	mu      sync.Mutex
	changed *sync.Cond
	frame   []string // the newest frame not sent yet
	width   int
	waiting bool     // frame holds a frame to send
	control []string // sequences to send before the next frame, in order
	repaint bool     // the next frame is painted whole
	writing bool     // a write is with the terminal now
	closed  bool
	done    chan struct{}

	// Only the writing goroutine reads and sets these.
	shown      []string // what the terminal last received
	shownWidth int
}

func newTUIFrameWriter(out io.Writer, synchronized bool) *tuiFrameWriter {
	w := &tuiFrameWriter{out: out, synchronized: synchronized, done: make(chan struct{})}
	w.changed = sync.NewCond(&w.mu)
	go w.run()
	return w
}

// submit replaces any frame still waiting with this one and returns at once.
// The caller must not change lines afterwards.
func (w *tuiFrameWriter) submit(lines []string, width int) {
	if w == nil {
		return
	}
	w.mu.Lock()
	if !w.closed {
		w.frame, w.width, w.waiting = lines, width, true
		w.changed.Broadcast()
	}
	w.mu.Unlock()
}

// send queues a control sequence (a pointer shape, entering the screen) to go
// out before the next frame.
func (w *tuiFrameWriter) send(sequence string) {
	if w == nil || sequence == "" {
		return
	}
	w.mu.Lock()
	if !w.closed {
		w.control = append(w.control, sequence)
		w.changed.Broadcast()
	}
	w.mu.Unlock()
}

// repaintNext makes the next frame a whole paint: the screen was left to
// another program, so nothing the terminal showed can be assumed.
func (w *tuiFrameWriter) repaintNext() {
	if w == nil {
		return
	}
	w.mu.Lock()
	w.repaint = true
	w.mu.Unlock()
}

// busy says the terminal has not yet taken everything it was given.
func (w *tuiFrameWriter) busy() bool {
	if w == nil {
		return false
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	return !w.closed && (w.writing || w.waiting || len(w.control) > 0)
}

// flush waits until the terminal has taken everything submitted so far.
func (w *tuiFrameWriter) flush() {
	if w == nil {
		return
	}
	w.mu.Lock()
	for !w.closed && (w.writing || w.waiting || len(w.control) > 0) {
		w.changed.Wait()
	}
	w.mu.Unlock()
}

// close drops what was not sent yet, waits for a write the terminal is still
// taking and stops the writer. Nothing is written afterwards, so the caller
// can hand the terminal back.
func (w *tuiFrameWriter) close() {
	if w == nil {
		return
	}
	w.mu.Lock()
	if !w.closed {
		w.closed = true
		w.frame, w.waiting, w.control = nil, false, nil
		w.changed.Broadcast()
	}
	w.mu.Unlock()
	<-w.done
}

func (w *tuiFrameWriter) run() {
	defer close(w.done)
	for {
		w.mu.Lock()
		for !w.closed && !w.waiting && len(w.control) == 0 {
			w.changed.Wait()
		}
		if w.closed {
			w.mu.Unlock()
			return
		}
		frame, width, hasFrame := w.frame, w.width, w.waiting
		control, repaint := w.control, w.repaint
		w.frame, w.waiting, w.control, w.repaint = nil, false, nil, false
		w.writing = true
		w.mu.Unlock()

		var output strings.Builder
		for _, sequence := range control {
			output.WriteString(sequence)
		}
		if repaint || (hasFrame && width != w.shownWidth) {
			w.shown = nil // a new size, like a screen that was left, is painted whole
		}
		if hasFrame {
			if diff := tuiPaintDelta(w.shown, frame, width); diff != "" {
				if w.synchronized {
					output.WriteString("\x1b[?2026h")
					output.WriteString(diff)
					output.WriteString("\x1b[?2026l")
				} else {
					output.WriteString(diff)
				}
			}
			w.shown, w.shownWidth = frame, width
		}
		if output.Len() > 0 {
			_, _ = ioWriteString(w.out, output.String())
		}

		w.mu.Lock()
		w.writing = false
		w.changed.Broadcast()
		w.mu.Unlock()
	}
}
