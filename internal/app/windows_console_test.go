//go:build windows

package app

import (
	"errors"
	"os"
	"sync/atomic"
	"testing"
	"time"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"
)

func TestWindowsInputRecordABISize(t *testing.T) {
	if got := unsafe.Sizeof(windowsInputRecord{}); got != 20 {
		t.Fatalf("sizeof(windowsInputRecord) = %d, want 20", got)
	}
}

func TestWindowsConsoleDecoderHandlesTextSurrogatesAndVirtualKeys(t *testing.T) {
	var dec windowsConsoleInputDecoder

	// Key-up and focus events produce no bytes.
	if got := dec.feedRecord(makeKeyRecord(false, 1, 'y', 'y')); len(got) != 0 {
		t.Fatalf("key-up produced %q", got)
	}
	if got := dec.feedRecord(windowsInputRecord{EventType: windowsFocusEvent}); len(got) != 0 {
		t.Fatalf("focus event produced %q", got)
	}

	// Thai character (3 UTF-8 bytes from 1 UTF-16 code unit).
	if got := string(dec.feedRecord(makeKeyRecord(true, 1, 0, 'ก'))); got != "ก" {
		t.Fatalf("Thai rune = %q, want %q", got, "ก")
	}

	// Surrogate pair across two KEY_EVENT records (e.g. Windows Terminal emoji input).
	high, low := utf16.EncodeRune('🚀')
	if got := dec.feedRecord(makeKeyRecord(true, 1, 0, uint16(high))); len(got) != 0 {
		t.Fatalf("high surrogate alone emitted %q", got)
	}
	if got := string(dec.feedRecord(makeKeyRecord(true, 1, 0, uint16(low)))); got != "🚀" {
		t.Fatalf("surrogate pair = %q, want 🚀", got)
	}

	// Legacy virtual key fallback when UnicodeChar is 0.
	if got := string(dec.feedRecord(makeKeyRecord(true, 1, vkUp, 0))); got != "\x1b[A" {
		t.Fatalf("VK_UP = %q, want \\x1b[A", got)
	}
	if got := string(dec.feedRecord(makeKeyRecord(true, 1, vkDelete, 0))); got != "\x1b[3~" {
		t.Fatalf("VK_DELETE = %q, want \\x1b[3~", got)
	}
}

func TestWindowsConsolePollLeavesEventUntouchedWhenPaused(t *testing.T) {
	queue := []windowsInputRecord{
		{EventType: windowsFocusEvent},
		makeKeyRecord(false, 1, 'A', 'a'),
		makeKeyRecord(true, 1, 'Y', 'y'),
	}
	poller := &fakeWindowsConsoleSource{records: queue}
	var dec windowsConsoleInputDecoder
	var paused atomic.Bool

	// Drains non-key / key-up events and returns 'y'.
	b, err := dec.readByteWithTimeout(poller, 80*time.Millisecond, &paused)
	if err != nil || b != 'y' {
		t.Fatalf("readByteWithTimeout = (%q, %v), want ('y', nil)", b, err)
	}

	// Queue another 'y', then pause before reading: event must remain in queue for readLine / agy.
	poller.records = []windowsInputRecord{makeKeyRecord(true, 1, 'Y', 'y')}
	paused.Store(true)
	_, err = dec.readByteWithTimeout(poller, 80*time.Millisecond, &paused)
	if !errors.Is(err, errInputPaused) {
		t.Fatalf("paused err = %v, want errInputPaused", err)
	}
	if len(poller.records) != 1 {
		t.Fatalf("paused poll consumed console record; remaining = %d, want 1", len(poller.records))
	}

	// Empty queue times out cleanly instead of blocking forever like os.Stdin.Read.
	paused.Store(false)
	poller.records = nil
	start := time.Now()
	_, err = dec.readByteWithTimeout(poller, 35*time.Millisecond, &paused)
	if !errors.Is(err, os.ErrDeadlineExceeded) {
		t.Fatalf("timeout err = %v, want os.ErrDeadlineExceeded", err)
	}
	if elapsed := time.Since(start); elapsed > 500*time.Millisecond {
		t.Fatalf("timeout took too long: %v", elapsed)
	}
}

func TestWindowsCookedModeSanitizerClearsRawAndNewlineFlags(t *testing.T) {
	// Simulate raw input mode left by taskkill'd agy.exe or TUI mouse tracking:
	// ECHO, LINE_INPUT, PROCESSED_INPUT cleared; VIRTUAL_TERMINAL_INPUT, MOUSE, WINDOW set.
	rawIn := uint32(windows.ENABLE_VIRTUAL_TERMINAL_INPUT | windows.ENABLE_MOUSE_INPUT | windows.ENABLE_WINDOW_INPUT | windows.ENABLE_EXTENDED_FLAGS)
	cookedIn := sanitizeWindowsCookedInputMode(rawIn)
	wantInBits := uint32(windows.ENABLE_PROCESSED_INPUT | windows.ENABLE_LINE_INPUT | windows.ENABLE_ECHO_INPUT)
	if cookedIn&wantInBits != wantInBits {
		t.Fatalf("cookedIn = 0x%04x, missing cooked input bits 0x%04x", cookedIn, wantInBits)
	}
	if cookedIn&windows.ENABLE_VIRTUAL_TERMINAL_INPUT != 0 {
		t.Fatalf("cookedIn = 0x%04x, still has ENABLE_VIRTUAL_TERMINAL_INPUT", cookedIn)
	}
	if cookedIn&windows.ENABLE_MOUSE_INPUT != 0 {
		t.Fatalf("cookedIn = 0x%04x, still has ENABLE_MOUSE_INPUT", cookedIn)
	}
	if cookedIn&windows.ENABLE_WINDOW_INPUT != 0 {
		t.Fatalf("cookedIn = 0x%04x, still has ENABLE_WINDOW_INPUT", cookedIn)
	}

	// Simulate raw output mode left by taskkill'd agy.exe:
	// DISABLE_NEWLINE_AUTO_RETURN set (0x0008 -> staircase output).
	rawOut := uint32(windows.ENABLE_PROCESSED_OUTPUT | windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING | windows.DISABLE_NEWLINE_AUTO_RETURN)
	cookedOut := sanitizeWindowsCookedOutputMode(rawOut)
	if cookedOut&windows.DISABLE_NEWLINE_AUTO_RETURN != 0 {
		t.Fatalf("cookedOut = 0x%04x, still has DISABLE_NEWLINE_AUTO_RETURN", cookedOut)
	}
	if cookedOut&windows.ENABLE_PROCESSED_OUTPUT == 0 {
		t.Fatalf("cookedOut = 0x%04x, missing ENABLE_PROCESSED_OUTPUT", cookedOut)
	}
}

func TestWindowsRealConsoleSnapshotRestoresBothHandles(t *testing.T) {
	inH := windows.Handle(os.Stdin.Fd())
	outH := windows.Handle(os.Stdout.Fd())
	var origIn, origOut uint32
	if windows.GetConsoleMode(inH, &origIn) != nil || windows.GetConsoleMode(outH, &origOut) != nil {
		t.Skip("no real Windows console attached to stdio")
	}
	defer func() {
		_ = windows.SetConsoleMode(inH, origIn)
		_ = windows.SetConsoleMode(outH, origOut)
	}()

	snap := captureTerminalState(os.Stdin, os.Stdout)

	// Corrupt both handles the exact way taskkill'd agy.exe does on Windows ConPTY.
	dirtyIn := (origIn &^ (windows.ENABLE_ECHO_INPUT | windows.ENABLE_LINE_INPUT | windows.ENABLE_PROCESSED_INPUT)) | windows.ENABLE_VIRTUAL_TERMINAL_INPUT
	dirtyOut := origOut | windows.DISABLE_NEWLINE_AUTO_RETURN
	if err := windows.SetConsoleMode(inH, dirtyIn); err != nil {
		t.Fatalf("SetConsoleMode dirtyIn: %v", err)
	}
	if err := windows.SetConsoleMode(outH, dirtyOut); err != nil {
		t.Fatalf("SetConsoleMode dirtyOut: %v", err)
	}

	snap.Restore(false)

	var gotIn, gotOut uint32
	if err := windows.GetConsoleMode(inH, &gotIn); err != nil {
		t.Fatal(err)
	}
	if err := windows.GetConsoleMode(outH, &gotOut); err != nil {
		t.Fatal(err)
	}
	wantIn := windows.ENABLE_PROCESSED_INPUT | windows.ENABLE_LINE_INPUT | windows.ENABLE_ECHO_INPUT
	if gotIn&uint32(wantIn) != uint32(wantIn) {
		t.Fatalf("restored input mode 0x%04x missing cooked flags 0x%04x", gotIn, wantIn)
	}
	if gotOut&windows.DISABLE_NEWLINE_AUTO_RETURN != 0 {
		t.Fatalf("restored output mode 0x%04x still has DISABLE_NEWLINE_AUTO_RETURN", gotOut)
	}
}

func TestWindowsRealConsolePollTimesOutCleanlyWithoutBlockingStdin(t *testing.T) {
	inH := windows.Handle(os.Stdin.Fd())
	var origIn uint32
	if windows.GetConsoleMode(inH, &origIn) != nil {
		t.Skip("no real Windows console attached to stdin")
	}
	var paused atomic.Bool
	done := make(chan error, 1)
	go func() {
		_, err := readInputByteWithTimeout(os.Stdin, 40*time.Millisecond, &paused)
		done <- err
	}()
	select {
	case err := <-done:
		if !errors.Is(err, os.ErrDeadlineExceeded) {
			t.Fatalf("readInputByteWithTimeout(os.Stdin) = %v, want os.ErrDeadlineExceeded", err)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("readInputByteWithTimeout(os.Stdin) blocked indefinitely on real Windows console")
	}
}

type fakeWindowsConsoleSource struct {
	records []windowsInputRecord
}

func (f *fakeWindowsConsoleSource) wait(timeoutMs uint32) (uint32, error) {
	if len(f.records) > 0 {
		return windows.WAIT_OBJECT_0, nil
	}
	time.Sleep(time.Duration(timeoutMs) * time.Millisecond)
	return uint32(windows.WAIT_TIMEOUT), nil
}

func (f *fakeWindowsConsoleSource) peek() (windowsInputRecord, bool, error) {
	if len(f.records) == 0 {
		return windowsInputRecord{}, false, nil
	}
	return f.records[0], true, nil
}

func (f *fakeWindowsConsoleSource) read() (windowsInputRecord, bool, error) {
	if len(f.records) == 0 {
		return windowsInputRecord{}, false, nil
	}
	rec := f.records[0]
	f.records = f.records[1:]
	return rec, true, nil
}
