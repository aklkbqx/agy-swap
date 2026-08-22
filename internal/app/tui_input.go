package app

import (
	"io"
	"os"
	"time"
)

func readTerminalKey(reader io.Reader) string {
	var first [1]byte
	if _, err := io.ReadFull(reader, first[:]); err != nil {
		return ""
	}
	switch first[0] {
	case 0, 0xe0:
		var second [1]byte
		if _, err := io.ReadFull(reader, second[:]); err != nil {
			return ""
		}
		switch second[0] {
		case 'H':
			return "up"
		case 'P':
			return "down"
		case 'S':
			return "delete"
		}
		return ""
	case 0x1b:
		if file, ok := reader.(*os.File); ok {
			_ = file.SetReadDeadline(time.Now().Add(75 * time.Millisecond))
			defer file.SetReadDeadline(time.Time{})
		}
		var second [1]byte
		if _, err := reader.Read(second[:]); err != nil {
			return "esc"
		}
		if second[0] != '[' && second[0] != 'O' {
			return ""
		}
		var third [1]byte
		if _, err := reader.Read(third[:]); err != nil {
			return ""
		}
		switch third[0] {
		case 'A':
			return "up"
		case 'B':
			return "down"
		case '3':
			var fourth [1]byte
			if _, err := reader.Read(fourth[:]); err == nil && fourth[0] == '~' {
				return "delete"
			}
		}
		return ""
	case '\r', '\n':
		return "enter"
	case 0x7f, 0x08:
		return "backspace"
	case 0x03:
		return "ctrl-c"
	case 0x04:
		return "ctrl-d"
	default:
		return string(first[:])
	}
}
