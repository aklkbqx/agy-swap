package app

import (
	"strings"
	"testing"
	"time"
)

func argonPoolAccount(name string, argon float64) Account {
	return namedPoolAccount(name,
		poolGroup("gemini", "Gemini models", poolBucket("weekly", 0.9, 100*time.Hour), poolBucket("5h", 0.8, 3*time.Hour)),
		poolGroup("third_party", "Claude and GPT models", poolBucket("weekly", 0.7, 100*time.Hour)),
		poolGroup("argon", "Gemini 4 Argon", poolBucket("weekly", argon, 120*time.Hour)))
}

func TestPoolPanelsSideBySideKeepEveryPool(t *testing.T) {
	a := rowsApp()
	pools := poolViews(argonPoolAccount("Pro User", 0.25), poolNow)
	if len(pools) != 3 {
		t.Fatalf("pools = %d", len(pools))
	}
	for _, plan := range []tuiLayoutPlan{
		{panels: panelsSide, digits: digitsNone},
		{panels: panelsSide, digits: digitsNone, tight: true},
		{panels: panelsSide, digits: digitsThree},
		{panels: panelsSide, digits: digitsFive, scale: 1},
	} {
		for _, width := range []int{64, 90, 120, 160} {
			rows := a.tuiPoolPanels(pools, poolNow, width, plan)
			text := strings.Join(rows, "\n")
			for _, label := range []string{"GEMINI", "CLAUDE/GPT", "ARGON"} {
				if !strings.Contains(text, label) {
					t.Fatalf("plan %+v width %d drops %s:\n%s", plan, width, label, text)
				}
			}
			for _, row := range rows {
				if visibleWidth(row) > width {
					t.Fatalf("plan %+v width %d: row is %d wide: %q", plan, width, visibleWidth(row), row)
				}
			}
		}
	}
}

func TestAUsedUpArgonPoolIsNotShownAsAPausedFiveHourLimit(t *testing.T) {
	pool := panelPool(t, poolGroup("argon", "Gemini 4 Argon", poolBucket("weekly", 0, 54*time.Hour)))
	if pool.paused {
		t.Fatal("Argon has no 5-hour limit to pause")
	}
	text := strings.Join(tuiPoolCompactLines(palette{}, pool, poolNow, 40), "\n")
	if strings.Contains(text, "paused") || strings.Contains(text, "5-hour") {
		t.Fatalf("Argon lines = %q", text)
	}
}
