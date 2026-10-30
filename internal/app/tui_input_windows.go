//go:build windows

package app

import (
	"encoding/binary"
	"io"
	"os"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf16"
	"unicode/utf8"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	windowsKeyEvent   uint16 = 0x0001
	windowsFocusEvent uint16 = 0x0010

	vkLeft   uint16 = 0x25
	vkUp     uint16 = 0x26
	vkRight  uint16 = 0x27
	vkDown   uint16 = 0x28
	vkDelete uint16 = 0x2E
)

var (
	kernel32DLL        = windows.NewLazySystemDLL("kernel32.dll")
	procPeekConsoleInW = kernel32DLL.NewProc("PeekConsoleInputW")
	procReadConsoleInW = kernel32DLL.NewProc("ReadConsoleInputW")

	windowsInputDecoderMu     sync.Mutex
	windowsSharedInputDecoder windowsConsoleInputDecoder
)

// windowsInputRecord matches Win32 INPUT_RECORD (20 bytes across x86/amd64/arm64).
type windowsInputRecord struct {
	EventType uint16
	_         uint16
	Event     [16]byte
}

func makeKeyRecord(keyDown bool, repeat uint16, vkey uint16, unicodeChar uint16) windowsInputRecord {
	var rec windowsInputRecord
	rec.EventType = windowsKeyEvent
	if keyDown {
		binary.LittleEndian.PutUint32(rec.Event[0:4], 1)
	}
	binary.LittleEndian.PutUint16(rec.Event[4:6], repeat)
	binary.LittleEndian.PutUint16(rec.Event[6:8], vkey)
	binary.LittleEndian.PutUint16(rec.Event[10:12], unicodeChar)
	return rec
}

type windowsConsoleEventSource interface {
	wait(timeoutMs uint32) (uint32, error)
	peek() (windowsInputRecord, bool, error)
	read() (windowsInputRecord, bool, error)
}

type realWindowsConsoleSource struct {
	handle windows.Handle
}

func (s realWindowsConsoleSource) wait(timeoutMs uint32) (uint32, error) {
	return windows.WaitForSingleObject(s.handle, timeoutMs)
}

func (s realWindowsConsoleSource) peek() (windowsInputRecord, bool, error) {
	var rec windowsInputRecord
	var count uint32
	r1, _, err := procPeekConsoleInW.Call(
		uintptr(s.handle),
		uintptr(unsafe.Pointer(&rec)),
		1,
		uintptr(unsafe.Pointer(&count)),
	)
	if r1 == 0 {
		return windowsInputRecord{}, false, err
	}
	return rec, count > 0, nil
}

func (s realWindowsConsoleSource) read() (windowsInputRecord, bool, error) {
	var rec windowsInputRecord
	var count uint32
	r1, _, err := procReadConsoleInW.Call(
		uintptr(s.handle),
		uintptr(unsafe.Pointer(&rec)),
		1,
		uintptr(unsafe.Pointer(&count)),
	)
	if r1 == 0 {
		return windowsInputRecord{}, false, err
	}
	return rec, count > 0, nil
}

type windowsConsoleInputDecoder struct {
	pending          []byte
	highSurrogate    rune
	hasHighSurrogate bool
}

func (d *windowsConsoleInputDecoder) reset() {
	d.pending = d.pending[:0]
	d.highSurrogate = 0
	d.hasHighSurrogate = false
}

func (d *windowsConsoleInputDecoder) feedRecord(rec windowsInputRecord) []byte {
	if rec.EventType != windowsKeyEvent {
		return nil
	}
	keyDown := binary.LittleEndian.Uint32(rec.Event[0:4]) != 0
	if !keyDown {
		return nil
	}
	repeat := int(binary.LittleEndian.Uint16(rec.Event[4:6]))
	if repeat < 1 {
		repeat = 1
	}
	vkey := binary.LittleEndian.Uint16(rec.Event[6:8])
	uchar := rune(binary.LittleEndian.Uint16(rec.Event[10:12]))

	if uchar == 0 {
		seq := windowsVirtualKeySequence(vkey)
		if seq == "" {
			return nil
		}
		out := make([]byte, 0, len(seq)*repeat)
		for range repeat {
			out = append(out, seq...)
		}
		return out
	}

	if d.hasHighSurrogate {
		high := d.highSurrogate
		d.highSurrogate = 0
		d.hasHighSurrogate = false
		combined := utf16.DecodeRune(high, uchar)
		var out []byte
		out = utf8.AppendRune(out, combined)
		return out
	}
	if utf16.IsSurrogate(uchar) {
		// High surrogate (0xD800–0xDBFF): wait for low surrogate in next KEY_EVENT.
		if uchar >= 0xD800 && uchar <= 0xDBFF {
			d.highSurrogate = uchar
			d.hasHighSurrogate = true
			return nil
		}
		uchar = utf8.RuneError
	}

	var one [utf8.UTFMax]byte
	n := utf8.EncodeRune(one[:], uchar)
	out := make([]byte, 0, n*repeat)
	for range repeat {
		out = append(out, one[:n]...)
	}
	return out
}

func windowsVirtualKeySequence(vkey uint16) string {
	switch vkey {
	case vkUp:
		return "\x1b[A"
	case vkDown:
		return "\x1b[B"
	case vkRight:
		return "\x1b[C"
	case vkLeft:
		return "\x1b[D"
	case vkDelete:
		return "\x1b[3~"
	default:
		return ""
	}
}

func (d *windowsConsoleInputDecoder) readByteWithTimeout(src windowsConsoleEventSource, timeout time.Duration, paused *atomic.Bool) (byte, error) {
	if paused != nil && paused.Load() {
		return 0, errInputPaused
	}
	if len(d.pending) > 0 {
		b := d.pending[0]
		d.pending = d.pending[1:]
		return b, nil
	}
	deadline := time.Now().Add(timeout)
	for {
		if paused != nil && paused.Load() {
			return 0, errInputPaused
		}
		_, hasEvent, err := src.peek()
		if err != nil {
			return 0, err
		}
		if hasEvent {
			if paused != nil && paused.Load() {
				return 0, errInputPaused
			}
			rec, ok, readErr := src.read()
			if readErr != nil {
				return 0, readErr
			}
			if ok {
				if emitted := d.feedRecord(rec); len(emitted) > 0 {
					d.pending = append(d.pending, emitted[1:]...)
					return emitted[0], nil
				}
			}
			continue
		}

		remaining := time.Until(deadline)
		if remaining <= 0 {
			return 0, os.ErrDeadlineExceeded
		}
		waitMs := uint32(remaining / time.Millisecond)
		if waitMs < 1 {
			waitMs = 1
		}
		if waitMs > 20 {
			waitMs = 20
		}
		event, waitErr := src.wait(waitMs)
		if waitErr != nil {
			return 0, waitErr
		}
		if event == uint32(windows.WAIT_TIMEOUT) && time.Now().After(deadline) {
			return 0, os.ErrDeadlineExceeded
		}
	}
}

// Windows console handles are non-overlapped in Go's os package, so
// os.File.SetReadDeadline returns os.ErrNoDeadline and os.File.Read holds
// fdMutex indefinitely until a key press. Poll the Win32 console input queue
// directly when backed by a console handle so TUI suspend never deadlocks or
// steals prompt input.
func readInputByteWithTimeout(reader io.Reader, timeout time.Duration, paused *atomic.Bool) (byte, error) {
	if paused != nil && paused.Load() {
		return 0, errInputPaused
	}
	file, ok := reader.(*os.File)
	if !ok {
		return readInputByte(reader)
	}
	handle := windows.Handle(file.Fd())
	var mode uint32
	if windows.GetConsoleMode(handle, &mode) == nil {
		windowsInputDecoderMu.Lock()
		defer windowsInputDecoderMu.Unlock()
		return windowsSharedInputDecoder.readByteWithTimeout(realWindowsConsoleSource{handle: handle}, timeout, paused)
	}
	if err := file.SetReadDeadline(time.Now().Add(timeout)); err == nil {
		defer func() { _ = file.SetReadDeadline(time.Time{}) }()
	}
	if paused != nil && paused.Load() {
		return 0, errInputPaused
	}
	return readInputByte(reader)
}
