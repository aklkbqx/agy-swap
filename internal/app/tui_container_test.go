package app

import (
	"strings"
	"testing"
)

func TestAVeryWideTerminalCentresAFrameOfAtMostTheContainerWidth(t *testing.T) {
	for _, tc := range []struct{ term, frame, left int }{
		{80, 80, 0},
		{tuiMaxFrameWidth, tuiMaxFrameWidth, 0},
		{tuiMaxFrameWidth + 1, tuiMaxFrameWidth, 0},
		{480, tuiMaxFrameWidth, (480 - tuiMaxFrameWidth) / 2},
	} {
		if frame, left := tuiFrameBox(tc.term); frame != tc.frame || left != tc.left {
			t.Errorf("terminal %d: frame %d at %d, want %d at %d", tc.term, frame, left, tc.frame, tc.left)
		}
	}
	a, state := selectionState()
	lines := a.tuiScreen(state, 480, 40)
	if len(lines) != 40 {
		t.Fatalf("rows = %d", len(lines))
	}
	_, left := tuiFrameBox(480)
	if !strings.HasPrefix(lines[1], strings.Repeat(" ", left)+"   agy") {
		t.Fatalf("the logo row is not indented by %d: %q", left, lines[1])
	}
	for i, line := range lines {
		if visibleWidth(strings.TrimLeft(line, " ")) > tuiMaxFrameWidth {
			t.Fatalf("row %d is wider than the container: %d", i, visibleWidth(line))
		}
	}
}

func TestMouseColumnsAreReadInsideTheCentredFrame(t *testing.T) {
	_, state := selectionState()
	state.width, state.height = 480, 40
	_, left := tuiFrameBox(480)
	// Columns are counted inside the frame's side rail.
	if got := state.frameColumn(left + 10); got != 9 {
		t.Fatalf("frame column = %d, want 9", got)
	}
	_, _, _, divider, ok := state.wideSplitBounds()
	if !ok || divider >= tuiMaxFrameWidth {
		t.Fatalf("divider = %d ok=%v, want a column inside the %d-wide frame", divider, ok, tuiMaxFrameWidth)
	}
}

func TestATallWindowAddsAtMostOneRowOfAirInsideAPanel(t *testing.T) {
	a, state := selectionState()
	for _, height := range []int{40, 60, 90} {
		plan := a.tuiFitPlan(state, 110, height, tuiLayoutPlan{arrangement: arrWide, listLines: 2, panels: panelsSide, digits: digitsFive})
		if plan.gap > 1 {
			t.Fatalf("height %d: gap = %d, want at most 1", height, plan.gap)
		}
	}
}

// A row that fills the terminal leaves the cursor on its last column, so an
// erase-to-end-of-line after it would wipe that column. Only a row shorter
// than the terminal is followed by one.
func TestPaintingNeverErasesTheLastColumnOfAFullRow(t *testing.T) {
	full := strings.Repeat("x", 80)
	short := strings.Repeat("y", 60)
	out := tuiPaint([]string{full, short, full}, 80)
	if strings.Contains(out, full+"\x1b[K") {
		t.Fatalf("a full-width row is followed by an erase: %q", out)
	}
	if !strings.Contains(out, short+"\x1b[K") {
		t.Fatalf("a short row must clear what is left of the line: %q", out)
	}
	if !strings.HasPrefix(out, "\x1b[H") || strings.Count(out, "\r\n") != 2 {
		t.Fatalf("frame = %q", out)
	}
	// The last row is full too: an erase-below after it would wipe the
	// bottom-right corner, so the frame ends on the row itself.
	if !strings.HasSuffix(out, full) {
		t.Fatalf("the bottom-right cell is erased after the last row: %q", out[len(out)-12:])
	}
	if out := tuiPaint([]string{full, short}, 80); !strings.HasSuffix(out, short+"\x1b[K\x1b[J") {
		t.Fatalf("a short last row still clears the rest of the screen: %q", out)
	}
}
