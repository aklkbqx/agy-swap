package app

import (
	"strings"
	"testing"
	"time"
)

func framelessApp() (*Application, *tuiState) {
	a, state := barApp()
	a.p.Track = "\x1b[38;5;240m"
	a.p.PanelBg = "\x1b[48;5;235m"
	return a, state
}

// The screen has no outer frame and no full-width rules; the one line left is
// the divider between the list and the gauges.
func TestTheScreenHasNoFrameOnlyTheMiddleDivider(t *testing.T) {
	a, state := framelessApp()
	for _, size := range [][2]int{{100, 40}, {80, 24}, {48, 24}} {
		lines := a.tuiLines(state, size[0]-2, size[1])
		for i, line := range lines {
			plain := stripEscapes(line)
			if visibleWidth(line) != size[0] {
				t.Fatalf("%v row %d is %d wide", size, i, visibleWidth(line))
			}
			for _, glyph := range []string{"╭", "╮", "╰", "╯", "├", "┤", "┬", "┴"} {
				if strings.Contains(plain, glyph) && !strings.Contains(plain, "GEMINI") && !strings.Contains(plain, "CLAUDE") {
					t.Fatalf("%v row %d still draws %q: %q", size, i, glyph, plain)
				}
			}
			if strings.HasPrefix(plain, "│") || strings.HasSuffix(plain, "│") {
				t.Fatalf("%v row %d has a side border: %q", size, i, plain)
			}
		}
	}
	wide := a.tuiLines(state, 98, 40)
	if !strings.Contains(wide[10], a.p.Track+"│"+a.p.Reset) {
		t.Fatalf("the list/gauge divider is drawn quietly: %q", wide[10])
	}
	if !strings.HasPrefix(wide[0], testBarBg) || !strings.HasPrefix(wide[len(wide)-1], testBarBg) {
		t.Fatalf("the bars run from the first to the last row:\n%q\n%q", wide[0], wide[len(wide)-1])
	}
}

func TestQuotaPanelsAreSolidCardsWithAStateEdge(t *testing.T) {
	a, _ := framelessApp()
	pool := panelPool(t, poolGroup("gemini", "Gemini models", poolBucket("weekly", 0.05, 54*time.Hour)))
	card := tuiBox(a.p, tuiPoolCompactLines(a.p, pool, poolNow, 30), 34, poolBorderColor(a.p, pool))
	for i, row := range card {
		if visibleWidth(row) != 34 {
			t.Fatalf("card row %d is %d wide", i, visibleWidth(row))
		}
		plain := stripEscapes(row)
		if strings.ContainsAny(plain, "╭╮╰╯│─") {
			t.Fatalf("card row %d draws a border: %q", i, plain)
		}
		if !strings.HasPrefix(plain, "▌") || !strings.Contains(row, a.p.PanelBg) {
			t.Fatalf("card row %d is not a solid card with a state edge: %q", i, row)
		}
	}
	if boxed := tuiBox(palette{}, []string{"x"}, 10, ""); !strings.HasPrefix(boxed[0], "╭") {
		t.Fatalf("without colour a card keeps its outline: %q", boxed)
	}
}

func TestStackedCardsHaveAGapSoTheyDoNotMergeIntoOneBlock(t *testing.T) {
	a, _ := framelessApp()
	pools := poolViews(argonPoolAccount("Pro User", 0.25), poolNow)
	for _, tc := range []struct {
		plan tuiLayoutPlan
		gaps int
	}{
		{tuiLayoutPlan{panels: panelsStack, digits: digitsInline}, 2}, // three cards, two gaps
		{tuiLayoutPlan{panels: panelsStack, digits: digitsNone}, 2},
		{tuiLayoutPlan{panels: panelsStack, digits: digitsThree}, 2},
		{tuiLayoutPlan{panels: panelsSide, digits: digitsNone}, 1}, // a pair, then the third
	} {
		rows := a.tuiPoolPanels(pools, poolNow, 80, tc.plan)
		gaps := 0
		for _, row := range rows {
			if !strings.Contains(row, a.p.PanelBg) && strings.TrimSpace(stripEscapes(row)) == "" {
				gaps++
			}
		}
		if gaps != tc.gaps {
			t.Fatalf("plan %+v: %d blank rows between cards, want %d:\n%s", tc.plan, gaps, tc.gaps, strings.Join(rows, "\n"))
		}
	}
}

// The divider has the same two columns of air on each side as the screen
// edges, so neither column looks pushed against it.
func TestTheDividerHasTheSameAirAsTheScreenEdges(t *testing.T) {
	a, state := overviewState()
	lines := a.tuiLines(state, 118, 40)
	col := dividerColumn(t, lines)
	for i, line := range lines {
		runes := []rune(line)
		if runes[col] != '│' {
			continue
		}
		if string(runes[col-2:col]) != "  " || string(runes[col+1:col+3]) != "  " {
			t.Fatalf("row %d crowds the divider: %q", i, line)
		}
	}
	state.width, state.height = 120, 40
	_, _, _, hit, _ := state.wideSplitBounds()
	if drawn := state.frameColumn(col + 1); hit != drawn {
		t.Fatalf("the mouse hit zone is frame column %d, the divider is drawn at %d", hit, drawn)
	}
}

func TestSideBySideCardsAreTheSameWidth(t *testing.T) {
	a, _ := framelessApp()
	pools := poolViews(argonPoolAccount("Pro User", 0.25), poolNow)
	rows := a.tuiPoolPanels(pools, poolNow, 80, tuiLayoutPlan{panels: panelsSide, digits: digitsNone})
	plain := []rune(stripEscapes(rows[1]))
	second := -1
	for i, r := range plain {
		if r == '▌' && i > 0 {
			second = i
			break
		}
	}
	// 80 columns: two 39-column cards, and the odd column widens the gutter
	// so the right card ends flush with everything else on the right.
	if second != 41 || visibleWidth(rows[1]) != 80 {
		t.Fatalf("second card starts at %d, row is %d wide: %q", second, visibleWidth(rows[1]), string(plain))
	}
}
