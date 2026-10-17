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
	// Five-row digits are for a pool that needs attention: 9% left.
	pool := panelPool(t, poolGroup("gemini", "Gemini models",
		poolBucket("weekly", 0.09, 54*time.Hour), poolBucket("5h", 0.4, 2*time.Hour+5*time.Minute)))
	got := tuiPoolPanelLines(palette{}, pool, poolNow, 34, digitsFive)
	spread := func(left, right string) string { return left + strings.Repeat(" ", 34-len(left)-len(right)) + right }
	want := []string{
		spread("GEMINI", "[ NEARLY OUT ]"),
		"",
		"██████",
		"██  ██",
		"██████",
		"    ██",
		"██████",
		"% of the weekly limit left",
		"",
		spread("Weekly", "9%"),
		renderMeter(meterHalf, 0.09, 34, "", "", ""),
		"resets in 2d 6h 0m",
		"",
		spread("5-hour", "40%"),
		renderMeter(meterHalf, 0.4, 34, "", "", ""),
		"resets in 2h 5m",
		"",
		"still usable",
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

func TestAPoolWithPlentyGetsTheSmallFigureAndNoReassurance(t *testing.T) {
	pool := panelPool(t, poolGroup("gemini", "Gemini models", poolBucket("weekly", 0.86, 100*time.Hour)))
	text := strings.Join(tuiPoolPanelLines(palette{}, pool, poolNow, 34, digitsFive), "\n")
	if strings.Contains(text, "██████") && strings.Count(text, "\n") > 14 {
		t.Fatalf("a plenty pool drew the big figure:\n%s", text)
	}
	if !strings.Contains(text, "█▀█") && !strings.Contains(text, "▀▀█") && !strings.Contains(text, "▄") {
		t.Fatalf("a plenty pool lost its figure:\n%s", text)
	}
	if strings.Contains(text, "usable right now") || strings.Contains(text, "still usable") {
		t.Fatalf("PLENTY already says it:\n%s", text)
	}
	if !strings.Contains(text, "% of the weekly limit left") {
		t.Fatalf("missing caption:\n%s", text)
	}
}

func TestInlinePoolLinesPutTheDigitsBesideTheBars(t *testing.T) {
	pool := panelPool(t, poolGroup("gemini", "Gemini models",
		poolBucket("weekly", 0.43, 16*time.Hour), poolBucket("5h", 0.93, 4*time.Hour+41*time.Minute)))
	lines := tuiPoolInlineLines(palette{}, pool, poolNow, 48)
	if len(lines) != 5 {
		t.Fatalf("lines = %q", lines)
	}
	for _, line := range lines {
		if visibleWidth(line) != 48 {
			t.Fatalf("line is %d wide: %q", visibleWidth(line), line)
		}
	}
	digits := bigNumber("43", 3)
	for i := 0; i < 3; i++ {
		if !strings.HasPrefix(lines[1+i], digits[i]) {
			t.Fatalf("row %d does not start with the digits %q: %q", i, digits[i], lines[1+i])
		}
	}
	for _, want := range []string{"Weekly", " 43%", "resets in 16h 0m", "5-hour", " 93%", "resets in 4h 41m"} {
		if !strings.Contains(strings.Join(lines, "\n"), want) {
			t.Fatalf("inline panel is missing %q:\n%s", want, strings.Join(lines, "\n"))
		}
	}
	used := panelPool(t, poolGroup("third_party", "Claude and GPT models", poolBucket("weekly", 0, 22*time.Hour+43*time.Minute)))
	text := strings.Join(tuiPoolInlineLines(palette{}, used, poolNow, 48), "\n")
	for _, want := range []string{"back in 22h 43m", "paused", "until weekly resets"} {
		if !strings.Contains(text, want) {
			t.Fatalf("used-up inline panel is missing %q:\n%s", want, text)
		}
	}
}

func TestTheInlineDigitsShowAtTheCommonWindowSizes(t *testing.T) {
	a, state := overviewState()
	digits := bigNumber("43", 3)[0]
	for _, size := range [][2]int{{100, 30}, {100, 28}, {120, 30}} {
		text := strings.Join(a.tuiLines(state, size[0]-2, size[1]), "\n")
		if !strings.Contains(text, digits) {
			t.Fatalf("%dx%d: no digits beside the bars:\n%s", size[0], size[1], text)
		}
	}
}

func TestPoolPanelOfAUsedUpPoolShowsPausedAndBackIn(t *testing.T) {
	pool := panelPool(t, poolGroup("third_party", "Claude and GPT models", poolBucket("weekly", 0, 22*time.Hour+43*time.Minute)))
	got := strings.Join(tuiPoolPanelLines(palette{}, pool, poolNow, 34, digitsThree), "\n")
	for _, want := range []string{"CLAUDE/GPT", "[ USED UP ]", "█▀█", "weekly limit used up", "resets in 22h 43m", "paused until weekly resets", "back in 22h 43m"} {
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
