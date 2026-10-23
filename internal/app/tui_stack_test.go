package app

import (
	"strings"
	"testing"
)

// stackedApp is a dashboard narrow enough that the list sits above the
// detail, with more accounts than the list has room for.
func stackedApp() (*Application, *tuiState) {
	a, state := listApp()
	state.accounts = sizesAccounts(12)
	state.active, state.selectedEmail = "user00@example.invalid", "user01@example.invalid"
	state.motionEnabled = false
	state.width, state.height = 70, 44
	return a, state
}

// stackedRule is the frame line index of the rule between list and detail.
func stackedRule(t *testing.T, lines []string) int {
	t.Helper()
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(stripEscapes(line)), "────") {
			return i
		}
	}
	t.Fatalf("no rule:\n%s", stripEscapes(strings.Join(lines, "\n")))
	return -1
}

func blankLine(line string) bool { return strings.TrimSpace(stripEscapes(line)) == "" }

func TestTheStackedDashboardKeepsAirAroundTheTitleAndTheRule(t *testing.T) {
	a, state := stackedApp()
	lines := a.tuiLines(state, state.width-2, state.height)
	title := -1
	for i, line := range lines {
		if strings.Contains(stripEscapes(line), "ACCOUNTS") {
			title = i
			break
		}
	}
	rule := stackedRule(t, lines)
	text := stripEscapes(strings.Join(lines, "\n"))
	if title < 1 || !blankLine(lines[title-1]) || !blankLine(lines[title+1]) {
		t.Fatalf("the list title needs a blank row above and below:\n%s", text)
	}
	if !blankLine(lines[rule-1]) || !blankLine(lines[rule+1]) {
		t.Fatalf("the rule needs a blank row above and below:\n%s", text)
	}
}

func TestTheStackedRuleMovesWithTheOffsetAndStopsAtTheEnds(t *testing.T) {
	a, state := stackedApp()
	base := stackedRule(t, a.tuiLines(state, state.width-2, state.height))
	state.stackOffset = 3
	if got := stackedRule(t, a.tuiLines(state, state.width-2, state.height)); got != base+3 {
		t.Fatalf("offset +3 put the rule at %d, want %d", got, base+3)
	}
	state.stackOffset = -100
	lines := a.tuiLines(state, state.width-2, state.height)
	top := stackedRule(t, lines)
	if !strings.Contains(strings.Join(lines[:top], "\n"), tuiSelectionBar) {
		t.Fatalf("the list must keep at least one account:\n%s", stripEscapes(strings.Join(lines, "\n")))
	}
	if state.stackOffset != top-base {
		t.Fatalf("the offset was not clamped to where the rule stopped: %d, want %d", state.stackOffset, top-base)
	}
	state.stackOffset = 100
	lines = a.tuiLines(state, state.width-2, state.height)
	if bottom := stackedRule(t, lines); bottom > state.bodyTop+tuiBodyRowsFor(state.height)-5 {
		t.Fatalf("the detail must keep a few rows, rule at %d:\n%s", bottom, stripEscapes(strings.Join(lines, "\n")))
	}
}

func TestTheStackedRuleCanBeDraggedToARow(t *testing.T) {
	a, state := stackedApp()
	lines := a.tuiLines(state, state.width-2, state.height)
	rule := stackedRule(t, lines)
	row := rule + 1 // terminal rows start at 1
	onRule := func(row int) bool { _, ok := state.hitAt(row-1, 10, hitRule); return ok }
	if !onRule(row) || onRule(row-3) {
		t.Fatalf("hit test wrong around terminal row %d", row)
	}
	state.dragStackRule(row + 4)
	if got := stackedRule(t, a.tuiLines(state, state.width-2, state.height)); got != rule+4 {
		t.Fatalf("dragging to row %d put the rule at line %d, want %d", row+4, got, rule+4)
	}
}

func TestTheSplitKeysResizeTheStackedList(t *testing.T) {
	a, state := stackedApp()
	a.tuiLines(state, state.width-2, state.height)
	state.adjustSplit(2)
	if state.stackOffset != 1 || state.splitOffset != 0 {
		t.Fatalf("] on a stacked dashboard: stack %d split %d, want 1 and 0", state.stackOffset, state.splitOffset)
	}
	state.adjustSplit(-6)
	if state.stackOffset != -2 {
		t.Fatalf("{ on a stacked dashboard: stack %d, want -2", state.stackOffset)
	}
	state.resetSplit()
	if state.stackOffset != 0 {
		t.Fatalf("= left the stack offset at %d", state.stackOffset)
	}
}

func TestThereIsNoQuotaOverviewPage(t *testing.T) {
	_, state := listApp()
	for _, action := range tuiActions(state) {
		if action.ID == "quota" || strings.Contains(strings.ToLower(action.Label), "overview") {
			t.Fatalf("the palette still offers %q", action.Label)
		}
	}
}

// The rule moves only as far as it changes what shows: with every account
// and every gauge in view there is nothing to resize, so it stays put.
func TestTheStackedRuleStaysWhenEverythingFits(t *testing.T) {
	a, state := listApp()
	state.motionEnabled = false
	state.width, state.height = 70, 44
	base := stackedRule(t, a.tuiLines(state, state.width-2, state.height))
	for _, offset := range []int{-5, 5} {
		state.stackOffset = offset
		if got := stackedRule(t, a.tuiLines(state, state.width-2, state.height)); got != base || state.stackOffset != 0 {
			t.Fatalf("offset %d moved the rule to %d (from %d), offset now %d", offset, got, base, state.stackOffset)
		}
	}
}
