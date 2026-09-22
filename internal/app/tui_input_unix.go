//go:build !windows

package app

import (
	"io"
	"os"
	"sync/atomic"
	"time"

	"golang.org/x/sys/unix"
)

// readInputByteWithTimeout uses poll(2) for terminals. SetReadDeadline is not
// consistently supported by macOS pseudo-terminals, while poll gives the
// input loop a reliable cancellation boundary for bare Escape and quit.
func readInputByteWithTimeout(reader io.Reader, timeout time.Duration, paused *atomic.Bool) (byte, error) {
	if paused != nil && paused.Load() {
		return 0, errInputPaused
	}
	file, ok := reader.(*os.File)
	if !ok {
		return readInputByte(reader)
	}
	fd := int32(file.Fd())
	pollFD := []unix.PollFd{{Fd: fd, Events: unix.POLLIN}}
	millis := int(timeout / time.Millisecond)
	if millis < 1 {
		millis = 1
	}
	ready, err := unix.Poll(pollFD, millis)
	if err != nil {
		return 0, err
	}
	// A pause requested while this poll was waiting must leave the byte in
	// the terminal buffer for the process that is about to own stdin.
	if paused != nil && paused.Load() {
		return 0, errInputPaused
	}
	if ready == 0 {
		return 0, os.ErrDeadlineExceeded
	}
	if paused != nil && paused.Load() {
		return 0, errInputPaused
	}
	return readInputByte(reader)
}
