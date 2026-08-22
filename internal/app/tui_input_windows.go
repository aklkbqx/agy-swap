//go:build windows

package app

import (
	"io"
	"os"
	"time"
)

// Windows terminals expose deadline support differently from Unix PTYs. Use
// the native file deadline when available and retain a simple reader fallback
// for console implementations that do not expose it.
func readInputByteWithTimeout(reader io.Reader, timeout time.Duration) (byte, error) {
	file, ok := reader.(*os.File)
	if !ok {
		return readInputByte(reader)
	}
	if err := file.SetReadDeadline(time.Now().Add(timeout)); err == nil {
		defer file.SetReadDeadline(time.Time{})
	}
	return readInputByte(reader)
}
