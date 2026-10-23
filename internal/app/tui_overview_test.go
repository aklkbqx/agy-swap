package app

import (
	"strings"
	"testing"
	"time"
)

func overviewState() (*Application, *tuiState) {
	a := rowsApp()
	state := rowsState(map[string]Account{
		"alpha@example.invalid": namedPoolAccount("Alpha User",
			poolGroup("gemini", "Gemini models", poolBucket("weekly", 0.86, 123*time.Hour)),
			poolGroup("third_party", "Claude and GPT models", poolBucket("weekly", 1, 123*time.Hour))),
		"beta@example.invalid": namedPoolAccount("Beta User",
			poolGroup("gemini", "Gemini models", poolBucket("weekly", 0.43, 16*time.Hour), poolBucket("5h", 0.93, 4*time.Hour+41*time.Minute)),
			poolGroup("third_party", "Claude and GPT models", poolBucket("weekly", 0, 22*time.Hour+43*time.Minute))),
		"c@example.invalid": namedPoolAccount("Only Claude",
			poolGroup("third_party", "Claude and GPT models", poolBucket("weekly", 0.5, 30*time.Hour))),
	}, "alpha@example.invalid", "beta@example.invalid", "c@example.invalid")
	state.selectedEmail = "beta@example.invalid"
	return a, state
}

func TestDashboardNeverRepeatsTheAccountList(t *testing.T) {
	a, state := overviewState()
	for _, size := range [][2]int{{118, 34}, {118, 56}, {235, 66}} {
		text := strings.Join(a.tuiLines(state, size[0], size[1]), "\n")
		if strings.Contains(text, "ALL ACCOUNTS") {
			t.Fatalf("%dx%d: the list is already beside the gauges, a second table repeats it:\n%s", size[0], size[1], text)
		}
		if n := strings.Count(text, "Only Claude"); n != 1 {
			t.Fatalf("%dx%d: Only Claude appears %d times, want once:\n%s", size[0], size[1], n, text)
		}
	}
}

func TestTheDividerRunsDownToTheStatusLineAtAnyHeight(t *testing.T) {
	a, state := overviewState()
	for _, size := range [][2]int{{100, 30}, {118, 56}, {235, 66}} {
		lines := a.tuiLines(state, size[0], size[1])
		col := dividerColumn(t, lines)
		first := -1
		// The status bar's blank lead-in row ends the body.
		last := len(lines) - 6
		for i := 0; i <= last; i++ {
			runes := []rune(lines[i])
			if first < 0 {
				if runes[col] == '│' {
					first = i
				}
				continue
			}
			if runes[col] != '│' {
				t.Fatalf("%dx%d: row %d breaks the divider at column %d: %q", size[0], size[1], i, col, lines[i])
			}
		}
		if first < 0 || last-first < 3 {
			t.Fatalf("%dx%d: the divider runs from row %d to %d", size[0], size[1], first, last)
		}
		if []rune(lines[last+1])[col] == '│' {
			t.Fatalf("%dx%d: the divider runs into the status line", size[0], size[1])
		}
	}
}

// dividerColumn is the 0-based column of the list/detail divider: the first
// vertical line on screen (the screen has no frame).
func dividerColumn(t *testing.T, lines []string) int {
	t.Helper()
	for _, line := range lines {
		for j, r := range []rune(line) {
			if r == '│' {
				return j
			}
		}
	}
	t.Fatalf("no divider in:\n%s", strings.Join(lines, "\n"))
	return -1
}

func TestTheDragTargetMatchesWhereTheDividerIsDrawn(t *testing.T) {
	a, state := overviewState()
	for _, width := range []int{100, 120, 235} {
		for _, offset := range []int{-100, -6, 0, 9, 100} {
			state.width, state.height = width, 40
			state.splitOffset = 0
			state.setSplit(offset)
			lines := a.tuiScreen(state, width, 40)
			_, _, _, dividerCol, ok := state.wideSplitBounds()
			if !ok {
				t.Fatalf("%d wide is not a wide layout", width)
			}
			// Mouse columns start at 1, string columns at 0; the hit zone is
			// counted inside the centred frame.
			if got := state.frameColumn(dividerColumn(t, lines) + 1); got != dividerCol {
				t.Fatalf("width %d offset %d: drawn at %d, hit zone at %d", width, offset, got+1, dividerCol)
			}
		}
	}
}

func TestDraggingFromTheHitZoneRoundTrips(t *testing.T) {
	a, state := overviewState()
	state.width, state.height = 235, 66
	base, _, _, dividerCol, _ := state.wideSplitBounds()
	for _, target := range []int{50, 70, 90} {
		state.setSplit(target - 5 - base)
		lines := a.tuiScreen(state, 235, 66)
		if got := state.frameColumn(dividerColumn(t, lines) + 1); got != target {
			t.Fatalf("dragged to column %d but the divider is at %d (was %d)", target, got, dividerCol)
		}
	}
}

func TestTheSplitCanUseTheWholeRangeOfALargeWindow(t *testing.T) {
	_, state := overviewState()
	state.width, state.height = 235, 66
	state.setSplit(1000)
	_, _, max, _, _ := state.wideSplitBounds()
	frameWidth, _ := tuiFrameBox(235)
	g := newTUIGeometry(frameWidth-2-tuiRails, 66)
	_, _, _, left := tuiWideSplit(g.frameWidth, state.splitOffset)
	if left != max || state.splitOffset <= 20 {
		t.Fatalf("left=%d max=%d offset=%d: the %d-column frame should allow more than +20", left, max, state.splitOffset, frameWidth)
	}
	state.adjustSplit(-2)
	_, _, _, left2 := tuiWideSplit(g.frameWidth, state.splitOffset)
	if left2 != max-2 {
		t.Fatalf("after one step back the list should be %d wide, got %d", max-2, left2)
	}
	state.setSplit(-1000)
	_, min, _, _, _ := state.wideSplitBounds()
	_, _, _, left3 := tuiWideSplit(g.frameWidth, state.splitOffset)
	if left3 != min {
		t.Fatalf("narrowest list = %d, want %d", left3, min)
	}
}

func TestEveryListRowRemembersWhichAccountItBelongsTo(t *testing.T) {
	a, state := overviewState()
	state.width, state.height = 120, 40
	lines := a.tuiLines(state, 118, 40)
	if len(state.listHits) == 0 {
		t.Fatal("the wide dashboard recorded no click targets")
	}
	for _, email := range state.visibleEmails() {
		view := a.tuiAccountView(state, email, a.renderTime())
		row := -1
		for i, hit := range state.listHits {
			if hit == email {
				row = state.bodyTop + state.listFirstRow + i
				if strings.Contains(lines[row], view.name) {
					break
				}
			}
		}
		if row < 0 {
			t.Fatalf("%s has no click target", email)
		}
		found := false
		for i, hit := range state.listHits {
			if hit == email && strings.Contains(lines[state.bodyTop+state.listFirstRow+i], view.name) {
				found = true
			}
		}
		if !found {
			t.Fatalf("no row of %s carries its name at the recorded position:\n%s", email, strings.Join(lines, "\n"))
		}
	}
}

func TestTallWindowsKeepOneBlankRowBetweenAccounts(t *testing.T) {
	a, state := overviewState()
	tall := a.tuiAccountRowsFor(state, 60, 40, 2)
	if strings.TrimSpace(tall[2]) != "" || strings.TrimSpace(tall[3]) == "" {
		t.Fatalf("expected exactly one blank row between the first two accounts:\n%s", strings.Join(tall, "\n"))
	}
}

func TestTheListBarsGrowWithTheListWidth(t *testing.T) {
	a, state := overviewState()
	view := a.tuiAccountView(state, "beta@example.invalid", poolNow)
	narrow := a.tuiAccountBlock(view, state, 62, 2)[1]
	wide := a.tuiAccountBlock(view, state, 96, 2)[1]
	if strings.Count(wide, "▄")+strings.Count(wide, "▂") <= strings.Count(narrow, "▄")+strings.Count(narrow, "▂") {
		t.Fatalf("a wider row should draw longer bars:\n%s\n%s", narrow, wide)
	}
	if visibleWidth(wide) != 96 {
		t.Fatalf("wide row = %d columns", visibleWidth(wide))
	}
}

func TestBigDigitsScaleSquare(t *testing.T) {
	for scale := 1; scale <= 5; scale++ {
		rows := bigNumberScaled("100", scale)
		if len(rows) != 5*scale {
			t.Fatalf("scale %d: %d rows", scale, len(rows))
		}
		for _, row := range rows {
			if visibleWidth(row) != bigNumberWidth(3, scale) {
				t.Fatalf("scale %d: row is %d wide, want %d", scale, visibleWidth(row), bigNumberWidth(3, scale))
			}
		}
	}
	if got, want := strings.Join(bigNumberScaled("36%", 1), "\n"), strings.Join(bigNumber("36%", 5), "\n"); got != want {
		t.Fatalf("scale 1 must equal the 5-row digits:\n%s\n%s", got, want)
	}
}

func TestBigWindowsGetBiggerDigitsButNotPosterSized(t *testing.T) {
	a, state := overviewState()
	state.width, state.height = 235, 66
	text := strings.Join(a.tuiLines(state, 233, 66), "\n")
	// Scale 2 draws 4-cell-wide pixels: a run of 6 blocks is only possible when scaled.
	if !strings.Contains(text, strings.Repeat("█", 6)) {
		t.Fatalf("a 235x66 window should show scaled digits:\n%s", text)
	}
	for _, row := range strings.Split(text, "\n") {
		if visibleWidth(row) != 235 {
			t.Fatalf("row is %d wide: %q", visibleWidth(row), row)
		}
	}
	// Scale 3 would draw 6 row-tall digit strokes; the digits stop at scale 2.
	if strings.Contains(text, "\n"+strings.Repeat("█", 1)) && strings.Count(text, "█") > 235*20 {
		t.Fatalf("a 235x66 window should not be mostly solid blocks")
	}
}
