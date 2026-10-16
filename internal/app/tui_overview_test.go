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

func TestDashboardAddsAnOverviewOnlyWhenTheWindowIsTallEnough(t *testing.T) {
	a, state := overviewState()
	if strings.Contains(strings.Join(a.tuiLines(state, 118, 34), "\n"), "ALL ACCOUNTS") {
		t.Fatal("a 34-row window has no spare rows for the overview")
	}
	tall := strings.Join(a.tuiLines(state, 118, 56), "\n")
	if !strings.Contains(tall, "ALL ACCOUNTS") || !strings.Contains(tall, "Only Claude") {
		t.Fatalf("a tall window should show the overview:\n%s", tall)
	}
}
