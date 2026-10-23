package app

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func selectionState() (*Application, *tuiState) {
	a := rowsApp()
	state := rowsState(map[string]Account{
		"alpha@example.invalid": namedPoolAccount("Alpha User", poolGroup("gemini", "Gemini models", poolBucket("weekly", 0.9, 100*time.Hour))),
		"beta@example.invalid":  namedPoolAccount("Beta User", poolGroup("gemini", "Gemini models", poolBucket("weekly", 0.5, 100*time.Hour))),
	}, "alpha@example.invalid", "beta@example.invalid")
	state.selectedEmail = "beta@example.invalid"
	return a, state
}

func TestTheSelectedBlockCarriesAnAccentBarOnEveryLine(t *testing.T) {
	a, state := selectionState()
	selected := a.tuiAccountBlock(a.tuiAccountView(state, "beta@example.invalid", poolNow), state, 60, 2)
	for i, line := range selected {
		if !strings.HasPrefix(line, tuiSelectionBar) {
			t.Fatalf("selected line %d does not start with the accent bar: %q", i, line)
		}
	}
	other := a.tuiAccountBlock(a.tuiAccountView(state, "alpha@example.invalid", poolNow), state, 60, 2)
	for i, line := range other {
		if strings.Contains(line, tuiSelectionBar) {
			t.Fatalf("unselected line %d has the accent bar: %q", i, line)
		}
	}
	one := a.tuiAccountBlock(a.tuiAccountView(state, "beta@example.invalid", poolNow), state, 60, 1)
	if !strings.HasPrefix(one[0], tuiSelectionBar) {
		t.Fatalf("one-line row = %q", one[0])
	}
	overview := a.tuiOverviewRows(state, 100, 10)
	if !strings.HasPrefix(overview[2], tuiSelectionBar) || strings.Contains(overview[1], tuiSelectionBar) {
		t.Fatalf("overview rows = %q", overview)
	}
}

// The surfaces are one ramp of cool grays from the black body up: bars and
// cards, then the selection tint, then key chips and quiet lines, each a
// clear step lighter than the one below it.
func TestTheGraysAreOneRampAboveABlackBody(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	t.Setenv("COLORTERM", "truecolor")
	p := makePalette(true)
	rgb := func(code string) [3]int {
		var c [3]int
		if _, err := fmt.Sscanf(code[strings.Index(code, ";2;")+3:], "%d;%d;%dm", &c[0], &c[1], &c[2]); err != nil {
			t.Fatalf("%q is not a truecolor code: %v", code, err)
		}
		return c
	}
	body, bar, panel, sel, key, track := rgb(p.BodyBg), rgb(p.BarBg), rgb(p.PanelBg), rgb(p.SelectBg), rgb(p.KeyBg), rgb(p.Track)
	if body[0]+body[1]+body[2] > 3*16 {
		t.Fatalf("the body is not black: %v", body)
	}
	if bar != panel {
		t.Fatalf("bars %v and cards %v are one surface", bar, panel)
	}
	for _, step := range [][2][3]int{{body, bar}, {bar, sel}, {sel, key}} {
		if lo, hi := step[0], step[1]; hi[0]+hi[1]+hi[2] < lo[0]+lo[1]+lo[2]+24 {
			t.Fatalf("%v is not a clear step above %v", hi, lo)
		}
	}
	for _, c := range [][3]int{bar, sel, key, track} {
		if c[2] < c[1] || c[1] < c[0] {
			t.Fatalf("%v is not a cool gray (blue ≥ green ≥ red)", c)
		}
	}
	t.Setenv("COLORTERM", "")
	t.Setenv("TERM", "xterm-256color")
	p = makePalette(true)
	if p.BodyBg == "" || p.BarBg == p.SelectBg || p.SelectBg == p.KeyBg {
		t.Fatalf("256-colour ramp collapses: %q %q %q %q", p.BodyBg, p.BarBg, p.SelectBg, p.KeyBg)
	}
}

func TestTheBodyIsPaintedBlackEdgeToEdge(t *testing.T) {
	a, state := barApp()
	a.p.BodyBg = "\x1b[48;5;232m"
	lines := a.tuiLines(state, 98, 40)
	for i := 6; i < len(lines)-6; i++ {
		inner := lines[i][len(testBarBg+" \x1b[0m") : len(lines[i])-len(testBarBg+" \x1b[0m")]
		if !strings.HasPrefix(inner, a.p.BodyBg) {
			t.Fatalf("body row %d does not start black: %q", i, inner)
		}
		rest := strings.TrimSuffix(inner, "\x1b[0m")
		for j := strings.Index(rest, "\x1b[0m"); j >= 0; j = strings.Index(rest, "\x1b[0m") {
			rest = rest[j+len("\x1b[0m"):]
			if !strings.HasPrefix(rest, a.p.BodyBg) {
				t.Fatalf("body row %d drops back to the terminal colour after a reset: %q", i, inner)
			}
		}
	}
}

func TestTheAccentBarHasAnASCIIStandIn(t *testing.T) {
	if got := asciiLine(tuiSelectionBar + " Beta"); got != "> Beta" {
		t.Fatalf("ascii = %q", got)
	}
}

func activeColorApp() *Application {
	a := rowsApp()
	// Real escape codes, so the row widths are measured as on a terminal.
	a.p = palette{Green: "\x1b[32m", Yellow: "\x1b[33m", Orange: "\x1b[31m", Blue: "\x1b[34m", White: "\x1b[37m", Bold: "\x1b[1m", Reset: "\x1b[0m"}
	return a
}

// Green, yellow and red say how much quota is left; the active account is
// marked in blue so it never reads as a quota state.
func TestTheActiveAccountIsMarkedInBlueNotTheQuotaGreen(t *testing.T) {
	a, state := selectionState()
	a.p = activeColorApp().p
	state.active = "alpha@example.invalid"
	block := a.tuiAccountBlock(a.tuiAccountView(state, "alpha@example.invalid", poolNow), state, 70, 2)
	ready := "\x1b[32m\x1b[1mREADY" // the status word may be green; the rest may not
	identity, _, found := strings.Cut(block[0], ready)
	if !found {
		t.Fatalf("no READY on the active row: %q", block[0])
	}
	for _, want := range []string{"\x1b[34m●\x1b[0m", "\x1b[1m\x1b[37mAlpha User\x1b[0m", "\x1b[34m\x1b[1mACTIVE\x1b[0m"} {
		if !strings.Contains(identity, want) {
			t.Fatalf("active identity %q is missing %q", identity, want)
		}
	}
	if strings.Contains(identity, "\x1b[32m") {
		t.Fatalf("the active marker uses the quota green: %q", identity)
	}
	other := a.tuiAccountBlock(a.tuiAccountView(state, "beta@example.invalid", poolNow), state, 70, 2)
	if strings.Contains(other[0], "ACTIVE") || strings.Contains(other[0], "\x1b[34m") {
		t.Fatalf("inactive identity line = %q", other[0])
	}
	overview := a.tuiOverviewRows(state, 100, 10)
	overviewIdentity, _, _ := strings.Cut(overview[1], ready)
	if !strings.Contains(overview[1], "\x1b[34m●\x1b[0m") || strings.Contains(overviewIdentity, "\x1b[32m") {
		t.Fatalf("overview active row = %q", overview[1])
	}
	if header := a.tuiActiveLine(state, 98); !strings.Contains(header, "\x1b[34m●\x1b[0m") || strings.Contains(header, "\x1b[32m●") {
		t.Fatalf("header = %q", header)
	}
}

func TestTheActiveWordGivesWayBeforeTheName(t *testing.T) {
	a, state := selectionState()
	state.active = "alpha@example.invalid"
	wide := a.tuiAccountBlock(a.tuiAccountView(state, "alpha@example.invalid", poolNow), state, 80, 1)[0]
	if !strings.Contains(wide, "Alpha User ACTIVE") {
		t.Fatalf("wide one-line row = %q", wide)
	}
	narrow := a.tuiAccountBlock(a.tuiAccountView(state, "alpha@example.invalid", poolNow), state, 26, 1)[0]
	if strings.Contains(narrow, "ACTIVE") || !strings.Contains(narrow, "Alpha") || visibleWidth(narrow) != 26 {
		t.Fatalf("narrow one-line row = %q", narrow)
	}
}
