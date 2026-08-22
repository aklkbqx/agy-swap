//go:build !windows

package app

import (
	"io"
	"os"
	"time"

	"golang.org/x/sys/unix"
)

// readInputByteWithTimeout uses poll(2) for terminals. SetReadDeadline is not
// consistently supported by macOS pseudo-terminals, while poll gives the
// input loop a reliable cancellation boundary for bare Escape and quit.
func readInputByteWithTimeout(reader io.Reader, timeout time.Duration) (byte, error) {
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
	if ready == 0 {
		return 0, os.ErrDeadlineExceeded
	}
	return readInputByte(reader)
}
