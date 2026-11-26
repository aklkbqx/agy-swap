//go:build !windows

package app

import (
	"context"
	"errors"
	"io"
	"os"
	"sync/atomic"
	"time"

	"golang.org/x/sys/unix"
	"golang.org/x/term"
)

func isTerminalClosedErr(err error) bool {
	return errors.Is(err, io.EOF) || errors.Is(err, os.ErrClosed) || errors.Is(err, unix.EIO) || errors.Is(err, unix.EBADF)
}

// readInputByteWithTimeout uses poll(2) for terminals. SetReadDeadline is not
// consistently supported by macOS pseudo-terminals, while poll gives the
// input loop a reliable cancellation boundary for bare Escape and quit.
func readInputByteWithTimeout(reader io.Reader, timeout time.Duration, paused *atomic.Bool) (byte, error) {
	if paused != nil && paused.Load() {
		return 0, errInputPaused
	}
	file, ok := reader.(*os.File)
	if !ok {
		b, err := readInputByte(reader)
		if isTerminalClosedErr(err) {
			return 0, io.EOF
		}
		return b, err
	}
	fd := int32(file.Fd())
	deadline := time.Now().Add(timeout)
	for {
		if paused != nil && paused.Load() {
			return 0, errInputPaused
		}
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return 0, os.ErrDeadlineExceeded
		}
		millis := int((remaining + time.Millisecond - 1) / time.Millisecond)
		if millis < 1 {
			millis = 1
		}
		pollFD := []unix.PollFd{{Fd: fd, Events: unix.POLLIN}}
		ready, err := unix.Poll(pollFD, millis)
		if err != nil {
			if errors.Is(err, unix.EINTR) {
				continue
			}
			if isTerminalClosedErr(err) {
				return 0, io.EOF
			}
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
		if pollFD[0].Revents&(unix.POLLERR|unix.POLLNVAL|unix.POLLHUP) != 0 && pollFD[0].Revents&unix.POLLIN == 0 {
			return 0, io.EOF
		}
		b, readErr := readInputByte(reader)
		if isTerminalClosedErr(readErr) {
			return 0, io.EOF
		}
		return b, readErr
	}
}

func waitTerminalInputReady(ctx context.Context, reader io.Reader) (bool, error) {
	file, ok := reader.(*os.File)
	if !ok || !term.IsTerminal(int(file.Fd())) {
		return false, nil
	}
	fd := int32(file.Fd())
	for {
		if err := ctx.Err(); err != nil {
			return true, err
		}
		pollFD := []unix.PollFd{{Fd: fd, Events: unix.POLLIN}}
		ready, err := unix.Poll(pollFD, 25)
		if err != nil {
			if errors.Is(err, unix.EINTR) {
				continue
			}
			return true, err
		}
		if ready > 0 {
			return true, ctx.Err()
		}
	}
}
