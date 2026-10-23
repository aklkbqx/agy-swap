package app

import (
	"os"
	"regexp"
	"strings"
	"unicode/utf8"
)

// Every frame records what the pointer can act on, so clicks, the wheel and
// hover all ask one question: what is under the pointer? Hits are kept in
// frame lines and frame columns (0-based, the rails excluded); the screen
// helpers below turn terminal positions into those.

type tuiHitKind int

const (
	hitAccount   tuiHitKind = iota + 1 // a list row; key is the account email
	hitDivider                         // the side-by-side divider
	hitRule                            // the stacked rule
	hitScroll                          // a scroll view, for the wheel; key names it
	hitScrollbar                       // a scroll view's bar column; key names it
	hitChip                            // a footer key hint; key is the key it presses
	hitButton                          // a dialog button; index is the button
	hitPalette                         // a palette row; index is the action
)

type tuiHit struct {
	kind                     tuiHitKind
	key                      string
	index                    int
	top, bottom, left, right int // ends excluded
}

func (s *tuiState) addHit(h tuiHit) {
	if h.bottom > h.top && h.right > h.left {
		s.hits = append(s.hits, h)
	}
}

// hitAt is the target of one of kinds (any but a scroll area when none are
// given) at a frame line and column. Later hits sit on top (a dialog over the
// body), so the search runs from the last.
func (s *tuiState) hitAt(line, x int, kinds ...tuiHitKind) (tuiHit, bool) {
	for i := len(s.hits) - 1; i >= 0; i-- {
		h := s.hits[i]
		if line < h.top || line >= h.bottom || x < h.left || x >= h.right {
			continue
		}
		// A scroll view's area is for the wheel only; it never hides the
		// targets drawn inside it.
		if (len(kinds) == 0 && h.kind != hitScroll) || oneOfKind(h.kind, kinds...) {
			return h, true
		}
	}
	return tuiHit{}, false
}

func oneOfKind(kind tuiHitKind, kinds ...tuiHitKind) bool {
	for _, k := range kinds {
		if kind == k {
			return true
		}
	}
	return false
}

// pointerAt turns a terminal position (from 1) into a frame line and column.
func (s *tuiState) pointerAt(col, row int) (line, x int) {
	return row - 1, s.frameColumn(col) - 1
}

// hovering says the pointer rests on this target.
func (s *tuiState) hovering(kind tuiHitKind, key string) bool {
	return s.hover.kind == kind && s.hover.key == key
}

// tuiKeyChip is a key hint drawn in the footer: what it shows and where.
type tuiKeyChip struct {
	label      string
	start, end int // visible columns of the line, end excluded
}

var tuiPlainChip = regexp.MustCompile(`\[[^\]]+\]`)

// tuiChips finds the key hints in a drawn row: runs on the key-chip colour,
// or "[Key]" without colour.
func (a *Application) tuiChips(row string) []tuiKeyChip {
	if a.p.KeyBg == "" {
		plain := stripEscapes(row)
		var chips []tuiKeyChip
		for _, m := range tuiPlainChip.FindAllStringIndex(plain, -1) {
			start := visibleWidth(plain[:m[0]])
			text := plain[m[0]:m[1]]
			chips = append(chips, tuiKeyChip{label: strings.Trim(text, "[]"), start: start, end: start + visibleWidth(text)})
		}
		return chips
	}
	var chips []tuiKeyChip
	col, bg, text := 0, "", ""
	open := -1
	flush := func() {
		if open >= 0 {
			chips = append(chips, tuiKeyChip{label: strings.TrimSpace(text), start: open, end: col})
		}
		open, text = -1, ""
	}
	for len(row) > 0 {
		if size := ansiPrefixLen(row); size > 0 {
			code := row[:size]
			switch {
			case code == "\x1b[0m" || code == "\x1b[m" || code == "\x1b[49m":
				bg = ""
			case strings.HasPrefix(code, "\x1b[48;") || (len(code) == 5 && strings.HasPrefix(code, "\x1b[4")):
				bg = code
			}
			row = row[size:]
			continue
		}
		r, size := utf8.DecodeRuneInString(row)
		if bg == a.p.KeyBg {
			if open < 0 {
				open = col
			}
			text += string(r)
		} else {
			flush()
		}
		col += terminalRuneWidth(r)
		row = row[size:]
	}
	flush()
	return chips
}

// chipKey is the key a click on a hint presses; "" for hints that name a
// group of keys (↑↓, ←→, Type) rather than one.
func chipKey(label string) string {
	switch label {
	case "Enter":
		return "enter"
	case "Esc", "Any key":
		return "esc"
	case "^K":
		return "ctrl-k"
	case "Bksp":
		return "backspace"
	}
	if utf8.RuneCountInString(label) == 1 && label != "↑" && label != "↓" {
		return strings.ToLower(label)
	}
	return ""
}

// tuiPointerShapes says the terminal changes the mouse pointer on request
// (OSC 22). Others would ignore the request, but there is no need to send it.
func tuiPointerShapes() bool {
	term, program := os.Getenv("TERM"), strings.ToLower(os.Getenv("TERM_PROGRAM"))
	return strings.Contains(term, "kitty") || strings.HasPrefix(term, "foot") ||
		program == "ghostty" || program == "wezterm"
}

// pointerShape is the OSC 22 shape for a target.
func pointerShape(kind tuiHitKind) string {
	switch kind {
	case hitDivider:
		return "ew-resize"
	case hitRule:
		return "ns-resize"
	case hitAccount, hitChip, hitButton, hitPalette, hitScrollbar:
		return "pointer"
	}
	return "default"
}

// tuiMouseEnabled is false when AGY_SWAP_MOUSE=0 asks for no mouse reports,
// for terminals and multiplexers that mishandle them.
func tuiMouseEnabled() bool {
	return os.Getenv("AGY_SWAP_MOUSE") != "0"
}
