package app

import (
	"strings"
	"testing"
)

const (
	testBarBg = "\x1b[48;5;236m"
	testKeyBg = "\x1b[48;5;239m"
)

func barApp() (*Application, *tuiState) {
	a, state := selectionState()
	a.p = palette{Green: "\x1b[32m", Yellow: "\x1b[33m", Orange: "\x1b[31m", Blue: "\x1b[34m", Gray: "\x1b[90m", White: "\x1b[37m", Bold: "\x1b[1m", Reset: "\x1b[0m", BarBg: testBarBg, KeyBg: testKeyBg}
	state.active = "alpha@example.invalid"
	return a, state
}

// assertSolidBar checks that a row's inside is one unbroken bar: every reset
// inside it hands straight back to the bar colour.
func assertSolidBar(t *testing.T, name, row string) {
	t.Helper()
	if !strings.HasPrefix(row, testBarBg) || !strings.HasSuffix(row, "\x1b[0m") {
		t.Fatalf("%s is not a full-width bar: %q", name, row)
	}
	inner := strings.TrimSuffix(strings.TrimPrefix(row, testBarBg), "\x1b[0m")
	for i := strings.Index(inner, "\x1b[0m"); i >= 0; {
		if !strings.HasPrefix(inner[i+len("\x1b[0m"):], testBarBg) {
			t.Fatalf("%s: the bar breaks after a reset: %q", name, row)
		}
		next := strings.Index(inner[i+1:], "\x1b[0m")
		if next < 0 {
			break
		}
		i += 1 + next
	}
}

func TestTheHeaderAndFooterAreSolidBars(t *testing.T) {
	a, state := barApp()
	lines := a.tuiLines(state, 98, 40)
	assertSolidBar(t, "top padding", lines[0])
	assertSolidBar(t, "logo row", lines[1])
	assertSolidBar(t, "air row", lines[2])
	assertSolidBar(t, "active row", lines[3])
	assertSolidBar(t, "status row", lines[len(lines)-4])
	assertSolidBar(t, "key row", lines[len(lines)-2])
	assertSolidBar(t, "bottom padding", lines[len(lines)-1])
	rail := testBarBg + " \x1b[0m"
	for i := 5; i < len(lines)-5; i++ {
		inner := strings.TrimSuffix(strings.TrimPrefix(lines[i], rail), rail)
		if strings.Contains(inner, testBarBg) {
			t.Fatalf("body row %d has the bar colour inside its rails: %q", i, lines[i])
		}
	}
	for i, line := range lines {
		if visibleWidth(line) != 100 {
			t.Fatalf("row %d is %d wide", i, visibleWidth(line))
		}
	}
}

func TestKeysAreChipsWithColourAndBracketsWithout(t *testing.T) {
	a, state := barApp()
	row := a.tuiFooterLines(state, 98)[1]
	if !strings.Contains(row, testKeyBg+"\x1b[1m\x1b[37m Enter \x1b[0m") || strings.Contains(row, "[Enter]") {
		t.Fatalf("coloured key row = %q", row)
	}
	if got := formatKeycaps(palette{}, "[Enter]", "Switch"); got != "[Enter] Switch" {
		t.Fatalf("plain key row = %q", got)
	}
	if got := formatKeycaps(a.p, "[[]/[]]", "Resize"); !strings.Contains(got, " [ ] ") {
		t.Fatalf("resize chip = %q", got)
	}
	for _, width := range []int{26, 36, 50, 118} {
		if got := visibleWidth(a.tuiFooterLines(state, width)[1]); got != width+2 {
			t.Fatalf("width %d: key row is %d wide", width, got)
		}
	}
}

// A bar has one row of padding above and below its content and two columns
// on either side, the same air a quota card has.
func TestBarsHaveEqualPaddingOnEverySide(t *testing.T) {
	a, state := barApp()
	// A bar row is coloured across; a body row only at its one-column rails.
	isBar := func(line string) bool {
		return strings.HasPrefix(line, testBarBg) && !strings.HasPrefix(line, testBarBg+" \x1b[0m")
	}
	blank := func(line string) bool { return isBar(line) && strings.TrimSpace(stripEscapes(line)) == "" }
	for _, tc := range []struct {
		height int
		header []string // what each header row holds
	}{
		{40, []string{"pad", "logo", "pad", "active", "pad"}},
		{24, []string{"pad", "logo", "active", "pad"}},
	} {
		lines := a.tuiLines(state, 98, tc.height)
		for i, kind := range tc.header {
			switch {
			case kind == "pad" && !blank(lines[i]):
				t.Fatalf("height %d: header row %d is not padding: %q", tc.height, i, lines[i])
			case kind == "logo" && !strings.Contains(lines[i], "agy"):
				t.Fatalf("height %d: header row %d is not the logo: %q", tc.height, i, lines[i])
			case kind == "active" && !strings.Contains(lines[i], "ACTIVE"):
				t.Fatalf("height %d: header row %d is not the active line: %q", tc.height, i, lines[i])
			}
		}
		if isBar(lines[len(tc.header)]) {
			t.Fatalf("height %d: the header bar runs into the body", tc.height)
		}
		n := len(lines)
		footer := []string{lines[n-5], lines[n-4], lines[n-3], lines[n-2], lines[n-1]}
		if !blank(footer[0]) || blank(footer[1]) || !blank(footer[2]) || blank(footer[3]) || !blank(footer[4]) || isBar(lines[n-6]) {
			t.Fatalf("height %d: footer is not pad, status, pad, keys, pad:\n%q", tc.height, footer)
		}
		// Bar text lines up with the body: the rail column, then two of air.
		if plain := stripEscapes(lines[1]); !strings.HasPrefix(plain, "   agy") || strings.HasPrefix(plain, "    ") || !strings.HasSuffix(plain, "LOCAL   ") {
			t.Fatalf("height %d: the logo row is not inset by three columns: %q", tc.height, plain)
		}
		if !strings.HasPrefix(stripEscapes(footer[3]), "   ") || !strings.Contains(footer[3], testKeyBg) {
			t.Fatalf("height %d: the key row is not inset by three columns: %q", tc.height, footer[3])
		}
	}
}

// The bars wrap the body: every body row carries a one-column rail of the bar
// colour at each screen edge, then two columns of air before the content, the
// same air as the one row between a bar and the body.
func TestTheBodySitsInsideASolidFrame(t *testing.T) {
	a, state := barApp()
	lines := a.tuiLines(state, 98, 40)
	rail := testBarBg + " " + "\x1b[0m"
	body := 0
	for i := 5; i < len(lines)-5; i++ {
		line := lines[i]
		if !strings.HasPrefix(line, rail) || !strings.HasSuffix(line, rail) {
			t.Fatalf("body row %d has no solid rail at both edges: %q", i, line)
		}
		inner := strings.TrimSuffix(strings.TrimPrefix(line, rail), rail)
		if plain := stripEscapes(inner); !strings.HasPrefix(plain, "  ") || !strings.HasSuffix(plain, "  ") || strings.HasPrefix(inner, testBarBg) {
			t.Fatalf("body row %d has no two columns of air inside the rail: %q", i, line)
		}
		if visibleWidth(line) != 100 {
			t.Fatalf("body row %d is %d wide", i, visibleWidth(line))
		}
		body++
	}
	if body < 20 {
		t.Fatalf("checked %d body rows", body)
	}
	plain, _ := selectionState()
	for _, line := range plain.tuiLines(state, 98, 40) {
		if strings.Contains(line, "\x1b[") {
			t.Fatalf("without colour there is no rail: %q", line)
		}
	}
}
