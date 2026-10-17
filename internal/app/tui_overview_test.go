package app

import (
	"fmt"
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

func TestOverviewRowsShowStatusMetersAndResetsInExactColumns(t *testing.T) {
	a, state := overviewState()
	rows := a.tuiOverviewRows(state, 100, 10)
	if len(rows) != 4 { // header and three accounts
		t.Fatalf("rows = %q", rows)
	}
	for _, row := range rows {
		if visibleWidth(row) != 100 {
			t.Fatalf("row is %d wide: %q", visibleWidth(row), row)
		}
	}
	header := rows[0]
	for _, want := range []string{"ACCOUNT", "STATUS", "GEMINI", "CLAUDE/GPT"} {
		if !strings.Contains(header, want) {
			t.Fatalf("header is missing %q: %q", want, header)
		}
	}
	beta := rows[2]
	for _, want := range []string{"Beta User", "GEMINI ONLY", " 43%", "16h 0m", "  0%", "back 22h 43m"} {
		if !strings.Contains(beta, want) {
			t.Fatalf("Beta row is missing %q: %q", want, beta)
		}
	}
	if !strings.Contains(rows[3], "—") {
		t.Fatalf("a missing Gemini pool should show a dash: %q", rows[3])
	}
}

func TestOverviewRowsDropResetsThenFitNarrowWidths(t *testing.T) {
	a, state := overviewState()
	for _, width := range []int{40, 56, 62, 80, 100, 140, 235} {
		for _, row := range a.tuiOverviewRows(state, width, 8) {
			if visibleWidth(row) != width {
				t.Fatalf("width %d: row is %d wide: %q", width, visibleWidth(row), row)
			}
		}
	}
	narrow := strings.Join(a.tuiOverviewRows(state, 62, 8), "\n")
	if strings.Contains(narrow, "back 22h") {
		t.Fatalf("narrow rows keep the reset text:\n%s", narrow)
	}
}

func TestOverviewRowsWindowWithMarkersAndKeepTheSelection(t *testing.T) {
	a := rowsApp()
	state := newTUIState(sizesAccounts(30), "")
	state.selectedEmail = "user17@example.invalid"
	rows := a.tuiOverviewRows(state, 100, 10)
	if len(rows) > 10 {
		t.Fatalf("rows = %d, want at most 10", len(rows))
	}
	text := strings.Join(rows, "\n")
	for _, want := range []string{"User 17", "▲", "▼"} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q:\n%s", want, text)
		}
	}
	if !strings.Contains(text, fmt.Sprintf("%d more", countAbove(text))) {
		t.Fatalf("marker count is off:\n%s", text)
	}
}

func countAbove(text string) int {
	var n int
	for _, line := range strings.Split(text, "\n") {
		if strings.Contains(line, "▲") {
			_, _ = fmt.Sscanf(strings.TrimSpace(strings.Trim(strings.TrimSpace(line), "│ ")), "▲ %d more", &n)
		}
	}
	return n
}

func TestQuotaOverviewScreenUsesTheNewStatusNotTheOldReadyText(t *testing.T) {
	a, state := overviewState()
	state.view = tuiViewQuota
	text := strings.Join(a.tuiViewRows(state, 100, 28), "\n")
	for _, want := range []string{"QUOTA OVERVIEW", "GEMINI ONLY", "Beta User", "back 22h 43m"} {
		if !strings.Contains(text, want) {
			t.Fatalf("quota overview is missing %q:\n%s", want, text)
		}
	}
	for _, old := range []string{"Gemini 43% ready", "Gemini Models", "SELECTED ACCOUNT"} {
		if strings.Contains(text, old) {
			t.Fatalf("quota overview still shows the old %q:\n%s", old, text)
		}
	}
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

func TestTheDividerRunsDownToTheStatusRuleAtAnyHeight(t *testing.T) {
	a, state := overviewState()
	for _, size := range [][2]int{{100, 30}, {118, 56}, {235, 66}} {
		lines := a.tuiLines(state, size[0], size[1])
		col, first := -1, -1
		for i, line := range lines {
			runes := []rune(line)
			if col < 0 && strings.ContainsRune(string(runes), '┬') {
				for j, r := range runes {
					if r == '┬' {
						col, first = j, i
					}
				}
			}
			if col < 0 {
				continue
			}
			if runes[col] == '┴' {
				if i < first+3 {
					t.Fatalf("%dx%d: the divider ends too early", size[0], size[1])
				}
				goto joined
			}
			if runes[col] != '│' && i != first {
				t.Fatalf("%dx%d: row %d breaks the divider at column %d: %q", size[0], size[1], i, col, string(runes))
			}
		}
		t.Fatalf("%dx%d: the divider never meets the status rule", size[0], size[1])
	joined:
	}
}

// dividerColumn is the 0-based column of the list/detail divider in a frame.
func dividerColumn(t *testing.T, lines []string) int {
	t.Helper()
	for _, line := range lines {
		for j, r := range []rune(line) {
			if r == '┬' {
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
			lines := a.tuiLines(state, width-2, 40)
			_, _, _, dividerCol, ok := state.wideSplitBounds()
			if !ok {
				t.Fatalf("%d wide is not a wide layout", width)
			}
			// Mouse columns start at 1, string columns at 0.
			if got := dividerColumn(t, lines); got+1 != dividerCol {
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
		state.setSplit(target - 4 - base)
		lines := a.tuiLines(state, 233, 66)
		if got := dividerColumn(t, lines) + 1; got != target {
			t.Fatalf("dragged to column %d but the divider is at %d (was %d)", target, got, dividerCol)
		}
	}
}

func TestTheSplitCanUseTheWholeRangeOfALargeWindow(t *testing.T) {
	_, state := overviewState()
	state.width, state.height = 235, 66
	state.setSplit(1000)
	_, _, max, _, _ := state.wideSplitBounds()
	g := newTUIGeometry(233, 66)
	_, _, _, left := tuiWideSplit(g.frameWidth, state.splitOffset)
	if left != max || state.splitOffset <= 40 {
		t.Fatalf("left=%d max=%d offset=%d: a 235-column window should allow more than +40", left, max, state.splitOffset)
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

func TestTallWindowsGiveTheListMoreAirBetweenAccounts(t *testing.T) {
	a, state := overviewState()
	short := a.tuiAccountRowsFor(state, 60, 8, 2)
	tall := a.tuiAccountRowsFor(state, 60, 24, 2)
	if len(tall) <= len(short) {
		t.Fatalf("a tall list should be roomier: %d rows vs %d", len(tall), len(short))
	}
	blanks := 0
	for _, row := range tall[2:5] {
		if strings.TrimSpace(row) == "" {
			blanks++
		}
	}
	if blanks != 3 {
		t.Fatalf("expected three blank rows between the first two accounts:\n%s", strings.Join(tall, "\n"))
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

func TestBigWindowsGetBiggerDigitsAndFillTheRows(t *testing.T) {
	a, state := overviewState()
	state.width, state.height = 235, 66
	text := strings.Join(a.tuiLines(state, 233, 66), "\n")
	// Scale 3 draws 6-cell-wide pixels: a run of 6 blocks is only possible when scaled.
	if !strings.Contains(text, strings.Repeat("█", 6)) {
		t.Fatalf("a 235x66 window should show scaled digits:\n%s", text)
	}
	rows := strings.Split(text, "\n")
	lastBox, bodyEnd := -1, -1
	for i, row := range rows {
		if strings.Contains(row, "╰") && strings.Contains(row, "│") && i < len(rows)-3 {
			lastBox = i
		}
		if strings.Contains(row, "┴") {
			bodyEnd = i
		}
	}
	if lastBox < 0 || bodyEnd-lastBox > 8 {
		t.Fatalf("the panels should reach the bottom of a 66-row window (last panel row %d, body ends %d):\n%s", lastBox, bodyEnd, text)
	}
	for _, row := range rows {
		if visibleWidth(row) != 235 {
			t.Fatalf("row is %d wide: %q", visibleWidth(row), row)
		}
	}
}
