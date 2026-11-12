package app

import (
	"bytes"
	"context"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

// screenModel is a terminal reduced to what a frame can change: a grid of
// cells, each with its text and the styling it was written with. It follows
// the cursor moves, erases and colour codes the screen sends, so two ways of
// painting the same frame can be compared cell by cell.
type screenModel struct {
	width, height int
	cells         [][]screenCell
	row, col      int
	style         screenStyle
}

type screenStyle struct {
	fg, bg string
	attrs  uint16
}

type screenCell struct {
	text  string // the rune and any marks joined to it; empty for the second half of a wide rune
	style screenStyle
}

func newScreenModel(width, height int) *screenModel {
	s := &screenModel{width: width, height: height, cells: make([][]screenCell, height)}
	for i := range s.cells {
		s.cells[i] = make([]screenCell, width)
		for j := range s.cells[i] {
			s.cells[i][j] = screenCell{text: " "}
		}
	}
	return s
}

func (s *screenModel) erase(row, from int) {
	if row < 0 || row >= s.height {
		return
	}
	for col := max(0, from); col < s.width; col++ {
		s.cells[row][col] = screenCell{text: " ", style: screenStyle{bg: s.style.bg}}
	}
}

func (s *screenModel) sgr(params string) {
	parts := strings.Split(params, ";")
	for i := 0; i < len(parts); i++ {
		n, _ := strconv.Atoi(parts[i])
		switch {
		case n == 0:
			s.style = screenStyle{}
		case n >= 1 && n <= 9:
			s.style.attrs |= 1 << uint(n)
		case n == 22:
			s.style.attrs &^= 1<<1 | 1<<2
		case n >= 23 && n <= 29:
			s.style.attrs &^= 1 << uint(n-20)
		case n == 38 || n == 48:
			take := 0
			if i+1 < len(parts) && parts[i+1] == "2" {
				take = 4
			} else if i+1 < len(parts) && parts[i+1] == "5" {
				take = 2
			}
			colour := strings.Join(parts[i:min(len(parts), i+take+1)], ";")
			if n == 38 {
				s.style.fg = colour
			} else {
				s.style.bg = colour
			}
			i += take
		case n == 39:
			s.style.fg = ""
		case n == 49:
			s.style.bg = ""
		case n >= 30 && n <= 37 || n >= 90 && n <= 97:
			s.style.fg = parts[i]
		case n >= 40 && n <= 47 || n >= 100 && n <= 107:
			s.style.bg = parts[i]
		}
	}
}

func (s *screenModel) write(t *testing.T, data string) {
	t.Helper()
	for len(data) > 0 {
		if data[0] == '\x1b' {
			size := ansiPrefixLen(data)
			if size == 0 {
				t.Fatalf("the screen was sent an escape it cannot read: %q", data[:min(len(data), 12)])
			}
			body, final := data[2:size-1], data[size-1]
			data = data[size:]
			switch final {
			case 'm':
				s.sgr(body)
			case 'H':
				row, col := 1, 1
				if fields := strings.Split(body, ";"); body != "" {
					row, _ = strconv.Atoi(fields[0])
					if len(fields) > 1 {
						col, _ = strconv.Atoi(fields[1])
					}
				}
				s.row, s.col = row-1, col-1
			case 'K':
				s.erase(s.row, s.col)
			case 'J':
				s.erase(s.row, s.col)
				for row := s.row + 1; row < s.height; row++ {
					s.erase(row, 0)
				}
			case 'h', 'l': // private modes (synchronized output) change no cell
			default:
				t.Fatalf("the screen was sent an unexpected sequence ending in %q", final)
			}
			continue
		}
		r, size := utf8.DecodeRuneInString(data)
		data = data[size:]
		switch r {
		case '\r':
			s.col = 0
			continue
		case '\n':
			s.row++
			continue
		}
		width := terminalRuneWidth(r)
		if s.row < 0 || s.row >= s.height {
			continue
		}
		if width == 0 {
			if s.col > 0 && s.col <= s.width {
				s.cells[s.row][s.col-1].text += string(r)
			}
			continue
		}
		if s.col+width <= s.width {
			if s.cells[s.row][s.col].text == "" && s.col > 0 {
				s.cells[s.row][s.col-1].text = " " // half of a wide rune was overwritten
			}
			if next := s.col + width; next < s.width && s.cells[s.row][next].text == "" {
				s.cells[s.row][next].text = " "
			}
			s.cells[s.row][s.col] = screenCell{text: string(r), style: s.style}
			if width == 2 {
				s.cells[s.row][s.col+1] = screenCell{style: s.style}
			}
		}
		s.col += width
	}
}

func (s *screenModel) firstDifference(other *screenModel) string {
	for row := range s.cells {
		for col := range s.cells[row] {
			if s.cells[row][col] != other.cells[row][col] {
				return fmt.Sprintf("row %d column %d: got %+v, want %+v", row+1, col+1, s.cells[row][col], other.cells[row][col])
			}
		}
	}
	return ""
}

// deltaFrames is a spread of real screens: every selection, each view, the
// modals, a toast and the moving refresh light.
func deltaFrames(t *testing.T, width, height int) [][]string {
	t.Helper()
	t.Setenv("NO_COLOR", "")
	t.Setenv("TERM", "xterm-256color")
	t.Setenv("COLORTERM", "truecolor")
	clock := time.Date(2026, time.September, 29, 12, 0, 0, 0, time.UTC)
	application, err := NewDemo("2.0.0", &bytes.Buffer{}, &bytes.Buffer{}, &bytes.Buffer{}, DemoOptions{Home: t.TempDir(), Now: clock})
	if err != nil {
		t.Fatal(err)
	}
	application.color = true
	application.p = makePalette(true)
	application.renderClock = func() time.Time { return clock }
	accounts, err := application.store.Load(false)
	if err != nil {
		t.Fatal(err)
	}
	fresh := func() *tuiState {
		state := newTUIState(accounts, application.credentials.Current(context.Background()))
		state.motionEnabled = false
		return state
	}
	var frames [][]string
	shoot := func(state *tuiState) {
		frames = append(frames, application.tuiScreen(state, width, height))
	}
	for _, email := range accounts.Order {
		state := fresh()
		state.selectedEmail = email
		shoot(state)
	}
	for _, view := range []tuiView{tuiViewProfiles, tuiViewHistory, tuiViewSettings, tuiViewDoctor, tuiViewBackup} {
		state := fresh()
		application.beginTUIView(state, view)
		shoot(state)
	}
	palette := fresh()
	palette.beginPalette()
	shoot(palette)
	search := fresh()
	search.beginSearch()
	search.search = "be"
	shoot(search)
	toast := fresh()
	toast.showToast("Switched to beta@example.invalid", "success")
	toast.toastUntil = time.Time{}
	shoot(toast)
	refreshing := fresh()
	refreshing.motionEnabled = true
	refreshing.refreshing = true
	refreshing.refreshStartedAt = clock
	for _, offset := range []time.Duration{0, 16 * time.Millisecond, 32 * time.Millisecond, 400 * time.Millisecond} {
		application.renderClock = func() time.Time { return clock.Add(offset) }
		shoot(refreshing)
	}
	application.renderClock = func() time.Time { return clock }
	return frames
}

func TestRowDeltaPaintsTheSameCellsAsAWholeRepaint(t *testing.T) {
	for _, size := range []struct{ width, height int }{{180, 50}, {120, 34}, {80, 24}, {48, 24}, {220, 60}} {
		frames := deltaFrames(t, size.width, size.height)
		for i, from := range frames {
			for j, to := range frames {
				if i == j {
					continue
				}
				painted := newScreenModel(size.width, size.height)
				painted.write(t, tuiPaint(from, size.width))
				painted.write(t, tuiPaintDelta(from, to, size.width))
				want := newScreenModel(size.width, size.height)
				want.write(t, tuiPaint(to, size.width))
				if diff := painted.firstDifference(want); diff != "" {
					t.Fatalf("%dx%d frame %d to %d: %s", size.width, size.height, i, j, diff)
				}
			}
		}
	}
}

func TestRowDeltaSendsLessThanWholeRows(t *testing.T) {
	frames := deltaFrames(t, 180, 50)
	selection := []int{0, 1} // two neighbouring selections
	rows := len(tuiPaintDiff(frames[selection[0]], frames[selection[1]], 180))
	delta := len(tuiPaintDelta(frames[selection[0]], frames[selection[1]], 180))
	if delta*10 > rows*7 {
		t.Fatalf("a selection move sends %d bytes as a delta and %d as whole rows; want at least 30%% less", delta, rows)
	}
	last := len(frames)
	light := len(tuiPaintDiff(frames[last-3], frames[last-2], 180)) // one 16 ms step of the refresh light
	lightDelta := len(tuiPaintDelta(frames[last-3], frames[last-2], 180))
	if lightDelta*10 > light*7 {
		t.Fatalf("a refresh frame sends %d bytes as a delta and %d as whole rows; want at least 30%% less", lightDelta, light)
	}
}

func TestRowDeltaKeepsStylingAndFallsBackToTheWholeRow(t *testing.T) {
	red, blue, reset := "\x1b[38;2;200;0;0m", "\x1b[38;2;0;0;200m", "\x1b[0m"
	prev := []string{red + "name one  " + blue + "idle   " + reset + " tail here"}
	next := []string{red + "name one  " + blue + "ready  " + reset + " tail here"}
	out := tuiPaintDelta(prev, next, 40)
	want := "\x1b[1;10H" + reset + red + " " + blue + "ready" + reset
	if out != want {
		t.Fatalf("delta = %q, want the changed word with its colours restored %q", out, want)
	}
	// Nothing in common before a space: the row goes out whole, as before.
	if got, whole := tuiPaintDelta([]string{"abcdef"}, []string{"abcxyz"}, 6), tuiPaintDiff([]string{"abcdef"}, []string{"abcxyz"}, 6); got != whole {
		t.Fatalf("delta = %q, want the whole row %q", got, whole)
	}
	if full := tuiPaintDelta(nil, next, 40); full != tuiPaint(next, 40) {
		t.Fatal("without a previous frame the whole frame is painted")
	}
	if same := tuiPaintDelta(next, next, 40); same != "" {
		t.Fatalf("an unchanged frame paints %q", same)
	}
	// A mark joined to the last unchanged letter must be repainted with it.
	marked := tuiPaintDelta([]string{"ab cdé x"}, []string{"ab cde x"}, 20)
	check := newScreenModel(20, 1)
	check.write(t, tuiPaint([]string{"ab cdé x"}, 20))
	check.write(t, marked)
	plain := newScreenModel(20, 1)
	plain.write(t, tuiPaint([]string{"ab cde x"}, 20))
	if diff := check.firstDifference(plain); diff != "" {
		t.Fatalf("a removed combining mark stayed on screen: %s", diff)
	}
	// The same tail one column further right is not the tail on screen.
	shorter, longer := []string{"left side ab  tail text here"}, []string{"left side abcd  tail text here"}
	shifted := newScreenModel(40, 1)
	shifted.write(t, tuiPaint(shorter, 40))
	shifted.write(t, tuiPaintDelta(shorter, longer, 40))
	whole := newScreenModel(40, 1)
	whole.write(t, tuiPaint(longer, 40))
	if diff := shifted.firstDifference(whole); diff != "" {
		t.Fatalf("a tail that moved was left where it was: %s", diff)
	}
}
