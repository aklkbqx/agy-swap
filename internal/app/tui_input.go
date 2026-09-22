package app

import (
	"errors"
	"io"
	"sync/atomic"
	"time"
	"unicode"
	"unicode/utf8"
)

var errInputPaused = errors.New("terminal input is paused")

const tuiInputPollTimeout = 120 * time.Millisecond
const tuiEscapeTimeout = 90 * time.Millisecond

// readTerminalKey decodes the small, portable key vocabulary used by the TUI.
// It accepts both Unix CSI/SS3 sequences and the legacy Windows console
// prefixes. Unknown escape sequences are treated as escape/alt input rather
// than silently disappearing, which keeps cancel actions reliable.
func readTerminalKey(reader io.Reader) string {
	return readTerminalKeyPaused(reader, nil)
}

func readTerminalKeyPaused(reader io.Reader, paused *atomic.Bool) string {
	first, err := readInputByteWithTimeout(reader, tuiInputPollTimeout, paused)
	if err != nil {
		return ""
	}
	switch first {
	case 0:
		second, err := readInputByteWithTimeout(reader, tuiEscapeTimeout, paused)
		if err != nil {
			return ""
		}
		switch second {
		case 'H':
			return "up"
		case 'P':
			return "down"
		case 'K':
			return "left"
		case 'M':
			return "right"
		case 'S':
			return "delete"
		}
		return ""
	case 0x1b:
		return readEscapeSequence(reader, paused)
	case '\r', '\n':
		return "enter"
	case '\t':
		return "tab"
	case 0x7f, 0x08:
		return "backspace"
	case 0x03:
		return "ctrl-c"
	case 0x04:
		return "ctrl-d"
	case 0x15:
		return "ctrl-u"
	case 0x17:
		return "ctrl-w"
	case 0x0b:
		return "ctrl-k"
	default:
		buf := []byte{first}
		for !utf8.FullRune(buf) && len(buf) < utf8.UTFMax {
			next, err := readInputByteWithTimeout(reader, tuiEscapeTimeout, paused)
			if err != nil {
				return ""
			}
			buf = append(buf, next)
		}
		if !utf8.Valid(buf) {
			return ""
		}
		return string(buf)
	}
}

func readEscapeSequence(reader io.Reader, paused *atomic.Bool) string {
	second, err := readInputByteWithTimeout(reader, tuiEscapeTimeout, paused)
	if err != nil {
		if errors.Is(err, errInputPaused) {
			return ""
		}
		return "esc"
	}
	if second == '[' {
		sequence := make([]byte, 0, 4)
		for len(sequence) < 8 {
			b, readErr := readInputByteWithTimeout(reader, tuiEscapeTimeout, paused)
			if readErr != nil {
				if errors.Is(readErr, errInputPaused) {
					return ""
				}
				return "esc"
			}
			sequence = append(sequence, b)
			if b >= 0x40 && b <= 0x7e {
				break
			}
		}
		return decodeCSI(sequence)
	}
	if second == 'O' {
		third, readErr := readInputByteWithTimeout(reader, tuiEscapeTimeout, paused)
		if readErr != nil {
			if errors.Is(readErr, errInputPaused) {
				return ""
			}
			return "esc"
		}
		switch third {
		case 'A':
			return "up"
		case 'B':
			return "down"
		case 'C':
			return "right"
		case 'D':
			return "left"
		case 'H':
			return "home"
		case 'F':
			return "end"
		default:
			return "esc"
		}
	}
	// Preserve the fact that this was an Alt chord. The controller currently
	// ignores unknown Alt commands, but tests and future commands can extend it.
	return "alt-" + string(second)
}

func decodeCSI(sequence []byte) string {
	if len(sequence) == 0 {
		return "esc"
	}
	final := sequence[len(sequence)-1]
	switch final {
	case 'Z':
		return "shift-tab"
	case 'A':
		return "up"
	case 'B':
		return "down"
	case 'C':
		return "right"
	case 'D':
		return "left"
	case 'H':
		return "home"
	case 'F':
		return "end"
	case '~':
		value := string(sequence[:len(sequence)-1])
		switch value {
		case "1", "7":
			return "home"
		case "4", "8":
			return "end"
		case "3":
			return "delete"
		case "5":
			return "page-up"
		case "6":
			return "page-down"
		}
	}
	return "esc"
}

func waitUntilInputIdle(busy *atomic.Bool, timeout time.Duration) {
	if busy == nil {
		return
	}
	deadline := time.Now().Add(timeout)
	for busy.Load() && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
}

func readInputByte(reader io.Reader) (byte, error) {
	var b [1]byte
	_, err := io.ReadFull(reader, b[:])
	return b[0], err
}

func printableKey(key string) bool {
	r, size := utf8.DecodeRuneInString(key)
	return size == len(key) && r != utf8.RuneError && unicode.IsPrint(r)
}

func removeLastRune(value string) string {
	_, size := utf8.DecodeLastRuneInString(value)
	return value[:len(value)-size]
}
