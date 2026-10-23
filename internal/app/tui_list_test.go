package app

import (
	"strings"
	"testing"
	"time"
)

const testSelectBg = "\x1b[48;5;237m"

func listApp() (*Application, *tuiState) {
	a := rowsApp()
	a.p = palette{Green: "\x1b[32m", Yellow: "\x1b[33m", Orange: "\x1b[31m", Blue: "\x1b[34m", Gray: "\x1b[90m", DarkGray: "\x1b[90m", White: "\x1b[37m", Bold: "\x1b[1m", Reset: "\x1b[0m", SelectBg: testSelectBg}
	group := func(f float64) map[string]any {
		return poolGroup("gemini", "Gemini models", poolBucket("weekly", f, 100*time.Hour))
	}
	state := rowsState(map[string]Account{
		"a@example.invalid": namedPoolAccount("Alpha User", group(0.9)),
		"b@example.invalid": namedPoolAccount("Beta User", group(0.5)),
		"c@example.invalid": namedPoolAccount("Gamma User", group(0.2)),
	}, "a@example.invalid", "b@example.invalid", "c@example.invalid")
	state.active = "a@example.invalid"
	state.selectedEmail = "b@example.invalid"
	state.motionEnabled = true // TestMain turns motion off for everything else
	return a, state
}

func TestTheListShowsNamesWithoutInitialsOrIdleDots(t *testing.T) {
	a, state := listApp()
	rows := a.tuiAccountRowsSpaced(state, 60, 30, 2, true)
	text := stripEscapes(strings.Join(rows, "\n"))
	for _, gone := range []string{"[AU]", "[BU]", "[GU]", "·"} {
		if strings.Contains(text, gone) {
			t.Fatalf("the list still shows %q:\n%s", gone, text)
		}
	}
	if !strings.Contains(text, "● Alpha User ACTIVE") {
		t.Fatalf("the active account lost its dot:\n%s", text)
	}
}

func TestTheSelectedBlockHasPaddingInsideItsTint(t *testing.T) {
	a, state := listApp()
	rows := a.tuiAccountRowsSpaced(state, 60, 30, 2, true)
	var tinted []int
	for i, row := range rows {
		if strings.HasPrefix(row, testSelectBg) {
			tinted = append(tinted, i)
		}
	}
	// A padding row, the two lines of the account, a padding row.
	if len(tinted) != 4 || tinted[3]-tinted[0] != 3 {
		t.Fatalf("tinted rows = %v:\n%s", tinted, stripEscapes(strings.Join(rows, "\n")))
	}
	for _, i := range []int{tinted[0], tinted[3]} {
		if plain := stripEscapes(rows[i]); strings.TrimSpace(plain) != tuiSelectionBar {
			t.Fatalf("padding row %d holds more than the selection bar: %q", i, plain)
		}
	}
	for _, i := range tinted {
		plain := []rune(stripEscapes(rows[i]))
		if len(plain) != 60 || plain[len(plain)-1] != ' ' {
			t.Fatalf("row %d has no column of air on the right inside the tint: %q", i, string(plain))
		}
	}
	if !strings.HasSuffix(strings.TrimRight(stripEscapes(rows[tinted[1]]), " "), "READY") && !strings.HasSuffix(strings.TrimRight(stripEscapes(rows[tinted[1]]), " "), "LOW") {
		t.Fatalf("status word moved: %q", stripEscapes(rows[tinted[1]]))
	}
	// Outside the tint a blank row still parts it from its neighbours: three
	// two-line accounts, two parting rows and the two padding rows.
	if len(rows) != 3*2+2+2 {
		t.Fatalf("rows = %d, want 10", len(rows))
	}
	for _, i := range []int{tinted[0] - 1, tinted[3] + 1} {
		if strings.HasPrefix(rows[i], testSelectBg) || strings.TrimSpace(stripEscapes(rows[i])) != "" {
			t.Fatalf("row %d should be a blank margin outside the tint: %q", i, rows[i])
		}
	}
}

func TestClickingThePaddingSelectsItsAccountAndGapsSelectNothing(t *testing.T) {
	a, state := listApp()
	rows, hits := a.tuiAccountRowsHits(state, 60, 30, 2, true)
	for i, row := range rows {
		switch {
		case strings.HasPrefix(row, testSelectBg) && hits[i] != "b@example.invalid":
			t.Fatalf("tinted row %d hits %q", i, hits[i])
		case strings.TrimSpace(stripEscapes(row)) == "" && hits[i] != "":
			t.Fatalf("blank row %d hits %q", i, hits[i])
		}
	}
}

// tintedRows lists the rows the selection band covers.
func tintedRows(rows []string) []int {
	var tinted []int
	for i, row := range rows {
		if strings.HasPrefix(row, testSelectBg) {
			tinted = append(tinted, i)
		}
	}
	return tinted
}

func TestTheSelectionBandSlidesToTheNewAccount(t *testing.T) {
	a, state := listApp()
	at := func(d time.Duration) []int {
		a.renderClock = func() time.Time { return poolNow.Add(d) }
		return tintedRows(a.tuiAccountRowsSpaced(state, 60, 30, 2, true))
	}
	from := at(0)
	state.selectedEmail = "c@example.invalid"
	if start := at(0); start[0] != from[0] {
		t.Fatalf("the band jumped at once: %v -> %v", from, start)
	}
	// Ease-out covers most of a short slide early, so look near the start.
	mid := at(tuiMotionSlide / 5)
	end := at(tuiMotionSlide * 2)
	if mid[0] <= from[0] || mid[0] >= end[0] {
		t.Fatalf("band tops: from %d, mid %d, end %d", from[0], mid[0], end[0])
	}
	if len(mid) != len(end) {
		t.Fatalf("the band changes height on the way: %v vs %v", mid, end)
	}
	// Once settled, the band covers exactly the selected account.
	rows, hits := a.tuiAccountRowsHits(state, 60, 30, 2, true)
	for _, i := range tintedRows(rows) {
		if hits[i] != "c@example.invalid" {
			t.Fatalf("settled band covers row %d of %q", i, hits[i])
		}
	}
}

func TestReducedMotionMovesTheBandAtOnce(t *testing.T) {
	a, state := listApp()
	state.motionEnabled = false
	a.tuiAccountRowsSpaced(state, 60, 30, 2, true)
	state.selectedEmail = "c@example.invalid"
	rows, hits := a.tuiAccountRowsHits(state, 60, 30, 2, true)
	for _, i := range tintedRows(rows) {
		if hits[i] != "c@example.invalid" {
			t.Fatalf("reduced motion band covers row %d of %q", i, hits[i])
		}
	}
}
