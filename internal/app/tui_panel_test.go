package app

import (
	"strings"
	"testing"
	"time"
)

func panelPool(t *testing.T, group map[string]any) poolView {
	t.Helper()
	pools := poolViews(poolAccount(group), poolNow)
	if len(pools) != 1 {
		t.Fatalf("pools = %#v", pools)
	}
	return pools[0]
}

func TestPoolPanelWithFiveRowDigitsMatchesTheDesign(t *testing.T) {
	pool := panelPool(t, poolGroup("gemini", "Gemini models",
		poolBucket("weekly", 0.36, 125*time.Hour+45*time.Minute), poolBucket("5h", 0.93, 4*time.Hour+41*time.Minute)))
	got := tuiPoolPanelLines(palette{}, pool, poolNow, 34, digitsFive)
	spread := func(left, right string) string { return left + strings.Repeat(" ", 34-len(left)-len(right)) + right }
	want := []string{
		spread("GEMINI", "[ PLENTY ]"),
		"",
		"██████  ██████  ██  ██",
		"    ██  ██          ██",
		"██████  ██████    ██  ",
		"    ██  ██  ██  ██    ",
		"██████  ██████  ██  ██",
		"weekly is the tighter limit",
		"",
		spread("Weekly", "36%"),
		"████████████▎░░░░░░░░░░░░░░░░░░░░░",
		"resets in 5d 5h 45m",
		"",
		spread("5-hour", "93%"),
		"███████████████████████████████▋░░",
		"resets in 4h 41m",
		"",
		"usable right now",
	}
	if len(got) != len(want) {
		t.Fatalf("rows = %d, want %d:\n%s", len(got), len(want), strings.Join(got, "\n"))
	}
	for i := range want {
		if strings.TrimRight(got[i], " ") != strings.TrimRight(want[i], " ") {
			t.Fatalf("row %d = %q, want %q", i, got[i], want[i])
		}
		if visibleWidth(got[i]) != 34 {
			t.Fatalf("row %d is %d columns wide, want 34: %q", i, visibleWidth(got[i]), got[i])
		}
	}
}

func TestPoolPanelOfAUsedUpPoolShowsPausedAndBackIn(t *testing.T) {
	pool := panelPool(t, poolGroup("third_party", "Claude and GPT models", poolBucket("weekly", 0, 22*time.Hour+43*time.Minute)))
	got := strings.Join(tuiPoolPanelLines(palette{}, pool, poolNow, 34, digitsThree), "\n")
	for _, want := range []string{"CLAUDE/GPT", "[ USED UP ]", "█▀█ ▀ █", "weekly limit used up", "resets in 22h 43m", "paused until weekly resets", "back in 22h 43m"} {
		if !strings.Contains(got, want) {
			t.Fatalf("panel is missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "5-hour  ") && strings.Contains(got, "█") && strings.Count(got, "█") > 12 {
		t.Fatalf("a paused 5-hour limit must not draw a bar:\n%s", got)
	}
}

func TestPoolPanelWithoutDigitsHasNoBigNumber(t *testing.T) {
	pool := panelPool(t, poolGroup("gemini", "Gemini models", poolBucket("5h", 0.6, 90*time.Minute)))
	got := tuiPoolPanelLines(palette{}, pool, poolNow, 34, digitsNone)
	text := strings.Join(got, "\n")
	if strings.Contains(text, "▀") || strings.Contains(text, "tighter limit") {
		t.Fatalf("digitsNone drew digits:\n%s", text)
	}
	if !strings.Contains(text, "5-hour") || strings.Contains(text, "Weekly") {
		t.Fatalf("only the 5-hour limit exists here:\n%s", text)
	}
}

func TestPoolPanelFlagsStaleNumbers(t *testing.T) {
	pool := panelPool(t, poolGroup("gemini", "Gemini models", poolBucket("weekly", 0.5, -time.Hour)))
	text := strings.Join(tuiPoolPanelLines(palette{}, pool, poolNow, 34, digitsNone), "\n")
	if !strings.Contains(text, "refresh due") {
		t.Fatalf("stale reset not flagged:\n%s", text)
	}
}

func TestPoolPanelBoxKeepsEveryRowTheSameWidth(t *testing.T) {
	pool := panelPool(t, poolGroup("gemini", "Gemini models", poolBucket("weekly", 0.5, 90*time.Hour)))
	box := tuiBox(palette{}, tuiPoolPanelLines(palette{}, pool, poolNow, 34, digitsThree), 38, "")
	for i, line := range box {
		if visibleWidth(line) != 38 {
			t.Fatalf("row %d is %d wide: %q", i, visibleWidth(line), line)
		}
	}
	if !strings.HasPrefix(box[0], "╭") || !strings.HasSuffix(box[len(box)-1], "╯") {
		t.Fatalf("box corners: %q ... %q", box[0], box[len(box)-1])
	}
}

func TestCompactPoolLinesFitOneLinePerLimitWhenThereIsRoom(t *testing.T) {
	pool := panelPool(t, poolGroup("gemini", "Gemini models",
		poolBucket("weekly", 0.36, 100*time.Hour), poolBucket("5h", 0.93, 4*time.Hour+41*time.Minute)))
	lines := tuiPoolCompactLines(palette{}, pool, poolNow, 70)
	if len(lines) != 3 {
		t.Fatalf("lines = %q", lines)
	}
	for _, line := range lines {
		if visibleWidth(line) != 70 {
			t.Fatalf("line is %d wide: %q", visibleWidth(line), line)
		}
	}
	if !strings.HasPrefix(lines[0], "GEMINI") || !strings.HasSuffix(strings.TrimRight(lines[0], " "), "[ PLENTY ]") {
		t.Fatalf("header = %q", lines[0])
	}
	if !strings.HasPrefix(lines[1], "Weekly") || !strings.Contains(lines[1], "36%") || !strings.Contains(lines[1], "resets in 4d 4h 0m") {
		t.Fatalf("weekly line = %q", lines[1])
	}
	if !strings.HasPrefix(lines[2], "5-hour") || !strings.Contains(lines[2], "93%") || !strings.Contains(lines[2], "resets in 4h 41m") {
		t.Fatalf("5-hour line = %q", lines[2])
	}
}

func TestCompactPoolLinesWrapTheResetWhenTheBarWouldGetTooShort(t *testing.T) {
	pool := panelPool(t, poolGroup("gemini", "Gemini models", poolBucket("weekly", 0.36, 100*time.Hour)))
	lines := tuiPoolCompactLines(palette{}, pool, poolNow, 34)
	if len(lines) != 3 || !strings.Contains(lines[2], "resets in 4d 4h 0m") || strings.Contains(lines[1], "resets") {
		t.Fatalf("lines = %q", lines)
	}
	for _, line := range lines {
		if visibleWidth(line) != 34 {
			t.Fatalf("line is %d wide: %q", visibleWidth(line), line)
		}
	}
}

func TestCompactPoolLinesForAUsedUpPoolSayBackInAndPaused(t *testing.T) {
	pool := panelPool(t, poolGroup("third_party", "Claude and GPT models", poolBucket("weekly", 0, 22*time.Hour+43*time.Minute)))
	text := strings.Join(tuiPoolCompactLines(palette{}, pool, poolNow, 70), "\n")
	for _, want := range []string{"CLAUDE/GPT", "[ USED UP ]", "back in 22h 43m", "5-hour  paused until weekly resets"} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q:\n%s", want, text)
		}
	}
}
