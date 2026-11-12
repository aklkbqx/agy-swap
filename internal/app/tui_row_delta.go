package app

import (
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"
)

// A changed row is usually changed in one place: a selection bar, a value, a
// cell of the refresh light. Sending the whole row again costs a terminal that
// reads slowly most of a frame, so the screen sends only the part between the
// last space the two rows share before the change and the first space they
// share after it, with the styling in force there restored.
//
// Cutting at a shared space keeps this safe: every cell before the cut is the
// same in both rows and already on screen, and nothing joined to a neighbour
// (a combining mark, half of a wide rune) can sit across a space.

// rowStyle is the styling in force at a point in a row: the colour and style
// codes since the last reset, in order. A plain colour code replaces an
// earlier one of its kind, so the list stays short.
type rowStyle struct {
	codes []string
	kinds []byte
}

// rowStyleLimit is the most codes a cut may have to replay. A row that needs
// more is sent whole.
const rowStyleLimit = 12

// sgrKind sorts a colour or style code: 'r' a reset, 'f' a plain foreground
// colour, 'b' a plain background colour, 'o' any other styling, and 0 an
// escape that is not styling at all.
func sgrKind(code string) byte {
	if len(code) < 3 || code[len(code)-1] != 'm' || code[1] != '[' {
		return 0
	}
	body := code[2 : len(code)-1]
	if body == "" || body == "0" {
		return 'r'
	}
	first, rest, compound := strings.Cut(body, ";")
	switch first {
	case "38", "48":
		kind := byte('f')
		if first == "48" {
			kind = 'b'
		}
		switch fields := strings.Count(rest, ";") + 1; {
		case strings.HasPrefix(rest, "2;") && fields == 4, strings.HasPrefix(rest, "5;") && fields == 2:
			return kind
		}
		return 'o'
	}
	if compound || len(first) < 2 || len(first) > 3 {
		return 'o'
	}
	switch tens := first[:len(first)-1]; {
	case first[len(first)-1] == '8':
		return 'o'
	case tens == "3" || tens == "9":
		return 'f'
	case tens == "4" || tens == "10":
		return 'b'
	}
	return 'o'
}

func (s *rowStyle) apply(code string, kind byte) {
	switch kind {
	case 'r':
		s.codes, s.kinds = s.codes[:0], s.kinds[:0]
		return
	case 'f', 'b':
		for i := 0; i < len(s.kinds); i++ {
			if s.kinds[i] == kind {
				s.codes = slices.Delete(s.codes, i, i+1)
				s.kinds = slices.Delete(s.kinds, i, i+1)
				i--
			}
		}
	}
	s.codes = append(s.codes, code)
	s.kinds = append(s.kinds, kind)
}

// rowWalk reads line up to the byte offset limit and gives the column and the
// styling there. It fails when limit falls inside a rune or a code, or the
// row holds an escape that is not styling, or too much styling to replay.
func rowWalk(line string, limit int) (col int, style rowStyle, ok bool) {
	offset := 0
	for offset < limit {
		if line[offset] == '\x1b' {
			size := ansiPrefixLen(line[offset:])
			if size == 0 {
				return 0, style, false
			}
			code := line[offset : offset+size]
			kind := sgrKind(code)
			if kind == 0 {
				return 0, style, false
			}
			style.apply(code, kind)
			offset += size
			continue
		}
		if line[offset] < utf8.RuneSelf {
			col++
			offset++
			continue
		}
		r, size := utf8.DecodeRuneInString(line[offset:])
		col += terminalRuneWidth(r)
		offset += size
	}
	return col, style, offset == limit && len(style.codes) <= rowStyleLimit
}

// tuiRowDelta gives the changed part of a row: the column it starts at and
// the text to write there, styling included. toEnd says the text runs to the
// end of the row. ok is false when the row has to be sent whole.
func tuiRowDelta(prev, next string) (col int, text string, toEnd, ok bool) {
	prefix := 0
	for prefix < len(prev) && prefix < len(next) && prev[prefix] == next[prefix] {
		prefix++
	}
	cut := max(0, strings.LastIndexByte(next[:prefix], ' '))
	col, style, ok := rowWalk(next, cut)
	if !ok {
		return 0, "", false, false
	}
	suffix := 0
	for limit := min(len(prev), len(next)) - prefix; suffix < limit && prev[len(prev)-1-suffix] == next[len(next)-1-suffix]; {
		suffix++
	}
	end := len(next)
	for from, tries := len(next)-suffix, 0; tries < 3 && from < len(next); tries++ {
		space := strings.IndexByte(next[from:], ' ')
		if space < 0 {
			break
		}
		stop := from + space
		from = stop + 1
		if stop <= cut {
			continue
		}
		// The rows are the same from here on. The tail on screen stands only
		// if it also starts at the same column with the same styling.
		nextCol, nextStyle, nextOK := rowWalk(next, stop)
		prevCol, prevStyle, prevOK := rowWalk(prev, len(prev)-(len(next)-stop))
		if nextOK && prevOK && nextCol == prevCol && slices.Equal(nextStyle.codes, prevStyle.codes) {
			end = stop
			break
		}
	}
	var out strings.Builder
	out.WriteString("\x1b[0m")
	for _, code := range style.codes {
		out.WriteString(code)
	}
	out.WriteString(next[cut:end])
	if end < len(next) {
		out.WriteString("\x1b[0m") // the tail keeps its own styling; none leaks into the next write
	}
	return col, out.String(), end == len(next), true
}

// tuiPaintDelta rewrites what changed since prev: within a changed row, only
// the changed part when that is shorter than the row. Without a previous
// frame of the same size it paints the whole frame.
func tuiPaintDelta(prev, lines []string, width int) string {
	if len(prev) != len(lines) {
		return tuiPaint(lines, width)
	}
	var out strings.Builder
	for i, line := range lines {
		if prev[i] == line {
			continue
		}
		short := visibleWidth(line) < width
		if col, text, toEnd, ok := tuiRowDelta(prev[i], line); ok && len(text) < len(line) {
			fmt.Fprintf(&out, "\x1b[%d;%dH", i+1, col+1)
			out.WriteString(text)
			if toEnd && short {
				out.WriteString("\x1b[K")
			}
			continue
		}
		fmt.Fprintf(&out, "\x1b[%d;1H", i+1)
		out.WriteString(line)
		if short {
			out.WriteString("\x1b[K")
		}
	}
	return out.String()
}
