package app

import (
	"io"
	"time"
)

const tuiInputPollTimeout = 120 * time.Millisecond
const tuiEscapeTimeout = 90 * time.Millisecond

// readTerminalKey decodes the small, portable key vocabulary used by the TUI.
// It accepts both Unix CSI/SS3 sequences and the legacy Windows console
// prefixes. Unknown escape sequences are treated as escape/alt input rather
// than silently disappearing, which keeps cancel actions reliable.
func readTerminalKey(reader io.Reader) string {
	first, err := readInputByteWithTimeout(reader, tuiInputPollTimeout)
	if err != nil {
		return ""
	}
	switch first {
	case 0, 0xe0:
		second, err := readInputByte(reader)
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
		return readEscapeSequence(reader)
	case '\r', '\n':
		return "enter"
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
	default:
		return string(first)
	}
}

func readEscapeSequence(reader io.Reader) string {
	second, err := readInputByteWithTimeout(reader, tuiEscapeTimeout)
	if err != nil {
		return "esc"
	}
	if second == '[' {
		sequence := make([]byte, 0, 4)
		for len(sequence) < 8 {
			b, readErr := readInputByteWithTimeout(reader, tuiEscapeTimeout)
			if readErr != nil {
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
		third, readErr := readInputByteWithTimeout(reader, tuiEscapeTimeout)
		if readErr != nil {
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

func readInputByte(reader io.Reader) (byte, error) {
	var b [1]byte
	_, err := io.ReadFull(reader, b[:])
	return b[0], err
}
