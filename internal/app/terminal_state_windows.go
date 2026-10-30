//go:build windows

package app

import (
	"fmt"
	"io"
	"os"

	"golang.org/x/sys/windows"
)

const childTerminalResetSeq = "\x1b[?1006l\x1b[?1003l\x1b[?1002l\x1b[?1000l\x1b[?2004l\x1b[?1049l\x1b[?25h"

type terminalSnapshot struct {
	inHandle  windows.Handle
	hasIn     bool
	inMode    uint32
	outHandle windows.Handle
	hasOut    bool
	out       io.Writer
	outMode   uint32
}

func sanitizeWindowsCookedInputMode(mode uint32) uint32 {
	cooked := mode | windows.ENABLE_PROCESSED_INPUT | windows.ENABLE_LINE_INPUT | windows.ENABLE_ECHO_INPUT
	cooked &^= windows.ENABLE_VIRTUAL_TERMINAL_INPUT
	return cooked
}

func sanitizeWindowsCookedOutputMode(mode uint32) uint32 {
	cooked := mode | windows.ENABLE_PROCESSED_OUTPUT | windows.ENABLE_WRAP_AT_EOL_OUTPUT
	cooked &^= windows.DISABLE_NEWLINE_AUTO_RETURN
	return cooked
}

func captureTerminalState(in io.Reader, out io.Writer) *terminalSnapshot {
	snap := &terminalSnapshot{out: out}
	if inFile, ok := in.(*os.File); ok {
		h := windows.Handle(inFile.Fd())
		var mode uint32
		if windows.GetConsoleMode(h, &mode) == nil {
			snap.inHandle = h
			snap.hasIn = true
			snap.inMode = sanitizeWindowsCookedInputMode(mode)
		}
	}
	if outFile, ok := out.(*os.File); ok {
		h := windows.Handle(outFile.Fd())
		var mode uint32
		if windows.GetConsoleMode(h, &mode) == nil {
			snap.outHandle = h
			snap.hasOut = true
			snap.outMode = sanitizeWindowsCookedOutputMode(mode)
		}
	}
	return snap
}

func (s *terminalSnapshot) Restore(resetChildModes bool) {
	if s == nil {
		return
	}
	if s.hasIn {
		_ = windows.SetConsoleMode(s.inHandle, s.inMode)
	}
	if s.hasOut {
		if resetChildModes && s.out != nil {
			// Ensure VT sequences are interpreted while closing any child alt-screen/mouse modes.
			_ = windows.SetConsoleMode(s.outHandle, s.outMode|windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING)
			_, _ = fmt.Fprint(s.out, childTerminalResetSeq)
		}
		_ = windows.SetConsoleMode(s.outHandle, s.outMode)
	}
}

func enableTUIRawOutput(out io.Writer) {
	outFile, ok := out.(*os.File)
	if !ok {
		return
	}
	h := windows.Handle(outFile.Fd())
	var mode uint32
	if windows.GetConsoleMode(h, &mode) != nil {
		return
	}
	_ = windows.SetConsoleMode(h, (mode|windows.ENABLE_PROCESSED_OUTPUT|windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING)&^windows.DISABLE_NEWLINE_AUTO_RETURN)
}

func ensureCookedTerminal(in io.Reader, out io.Writer) {
	if inFile, ok := in.(*os.File); ok {
		h := windows.Handle(inFile.Fd())
		var mode uint32
		if windows.GetConsoleMode(h, &mode) == nil {
			want := sanitizeWindowsCookedInputMode(mode)
			if mode != want {
				_ = windows.SetConsoleMode(h, want)
			}
		}
	}
	if outFile, ok := out.(*os.File); ok {
		h := windows.Handle(outFile.Fd())
		var mode uint32
		if windows.GetConsoleMode(h, &mode) == nil {
			want := sanitizeWindowsCookedOutputMode(mode)
			if mode != want {
				_ = windows.SetConsoleMode(h, want)
			}
		}
	}
}

func resetPlatformInputState() {
	windowsInputDecoderMu.Lock()
	windowsSharedInputDecoder.reset()
	windowsInputDecoderMu.Unlock()
}
