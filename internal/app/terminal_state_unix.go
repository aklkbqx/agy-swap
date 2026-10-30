//go:build !windows

package app

import (
	"fmt"
	"io"
	"os"

	"golang.org/x/term"
)

const childTerminalResetSeq = "\x1b[?1006l\x1b[?1003l\x1b[?1002l\x1b[?1000l\x1b[?2004l\x1b[?1049l\x1b[?25h"

type terminalSnapshot struct {
	inFD   int
	hasIn  bool
	state  *term.State
	out    io.Writer
	hasOut bool
}

func captureTerminalState(in io.Reader, out io.Writer) *terminalSnapshot {
	snap := &terminalSnapshot{out: out}
	if inFile, ok := in.(*os.File); ok {
		fd := int(inFile.Fd())
		if term.IsTerminal(fd) {
			if st, err := term.GetState(fd); err == nil {
				snap.inFD = fd
				snap.hasIn = true
				snap.state = st
			}
		}
	}
	if outFile, ok := out.(*os.File); ok && term.IsTerminal(int(outFile.Fd())) {
		snap.hasOut = true
	}
	return snap
}

func (s *terminalSnapshot) Restore(resetChildModes bool) {
	if s == nil {
		return
	}
	if s.hasIn && s.state != nil {
		_ = term.Restore(s.inFD, s.state)
	}
	if resetChildModes && s.hasOut && s.out != nil {
		_, _ = fmt.Fprint(s.out, childTerminalResetSeq)
	}
}

func enableTUIRawOutput(io.Writer) {}

func ensureCookedTerminal(io.Reader, io.Writer) {}

func resetPlatformInputState() {}
