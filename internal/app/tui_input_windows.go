//go:build windows

package app

import (
	"io"
	"os"
	"sync/atomic"
	"time"
)

// Windows terminals expose deadline support differently from Unix PTYs. Use
// the native file deadline when available and retain a simple reader fallback
// for console implementations that do not expose it.
func readInputByteWithTimeout(reader io.Reader, timeout time.Duration, paused *atomic.Bool) (byte, error) {
	if paused != nil && paused.Load() {
		return 0, errInputPaused
	}
	file, ok := reader.(*os.File)
	if !ok {
		return readInputByte(reader)
	}
	if err := file.SetReadDeadline(time.Now().Add(timeout)); err == nil {
		defer file.SetReadDeadline(time.Time{})
	}
	if paused != nil && paused.Load() {
		return 0, errInputPaused
	}
	return readInputByte(reader)
}
