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
