package app

import (
	"fmt"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

func motionState() *tuiState {
	state := newTUIState(NewAccounts(), "")
	state.motionEnabled = true
	return state
}

func TestFollowSettlesAtOnceThenEasesToEachNewTarget(t *testing.T) {
	s := motionState()
	t0 := poolNow
	if got := s.follow("k", 10, 200*time.Millisecond, t0); got != 10 {
		t.Fatalf("first value = %v, want the target at once", got)
	}
	if got := s.follow("k", 20, 200*time.Millisecond, t0); got != 10 {
		t.Fatalf("a new target starts from the old value, got %v", got)
	}
	mid := s.follow("k", 20, 200*time.Millisecond, t0.Add(100*time.Millisecond))
	if mid <= 15 || mid >= 20 {
		t.Fatalf("eased midpoint = %v, want past halfway (ease-out) and short of 20", mid)
	}
	if !s.moving(t0.Add(100 * time.Millisecond)) {
		t.Fatal("a running tween is motion")
	}
	// Retargeting mid-flight starts from where it is, not from the old start.
	back := s.follow("k", 0, 200*time.Millisecond, t0.Add(100*time.Millisecond))
	if back != mid {
		t.Fatalf("retarget jumped from %v to %v", mid, back)
	}
	if got := s.follow("k", 0, 200*time.Millisecond, t0.Add(400*time.Millisecond)); got != 0 || s.moving(t0.Add(400*time.Millisecond)) {
		t.Fatalf("settled value = %v, moving = %v", got, s.moving(t0.Add(400*time.Millisecond)))
	}
}

func TestReducedMotionDrawsEveryEndStateAtOnce(t *testing.T) {
	s := motionState()
	s.motionEnabled = false
	s.follow("k", 10, time.Second, poolNow)
	if got := s.follow("k", 20, time.Second, poolNow); got != 20 {
		t.Fatalf("reduced motion value = %v", got)
	}
	s.startedAt = poolNow
	if p := s.fxProgress(s.startedAt, time.Second, poolNow); p != 1 || s.moving(poolNow) {
		t.Fatalf("reduced motion effect progress = %v", p)
	}
}

func TestEffectsRunFromTheirStartAndNotBefore(t *testing.T) {
	s := motionState()
	if p := s.fxProgress(time.Time{}, time.Second, poolNow); p != 1 {
		t.Fatalf("an effect that never began is done, got %v", p)
	}
	if p := s.fxProgress(poolNow.Add(time.Second), time.Second, poolNow); p != 1 {
		t.Fatalf("an effect from the future is done (clock skew), got %v", p)
	}
	if p := s.fxProgress(poolNow, time.Second, poolNow.Add(250*time.Millisecond)); p != 0.25 {
		t.Fatalf("progress = %v", p)
	}
}

func TestOnlyChangedRowsArePainted(t *testing.T) {
	prev := []string{strings.Repeat("a", 10), strings.Repeat("b", 10), "short"}
	next := []string{strings.Repeat("a", 10), strings.Repeat("B", 10), "shirt"}
	out := tuiPaintDiff(prev, next, 10)
	if strings.Contains(out, "aaaa") || !strings.Contains(out, "\x1b[2;1H"+strings.Repeat("B", 10)) || !strings.Contains(out, "\x1b[3;1Hshirt\x1b[K") {
		t.Fatalf("diff paint = %q", out)
	}
	if strings.Contains(out, strings.Repeat("B", 10)+"\x1b[K") {
		t.Fatalf("a full row is followed by an erase: %q", out)
	}
	if full := tuiPaintDiff(nil, next, 10); full != tuiPaint(next, 10) {
		t.Fatalf("without a previous frame the whole frame is painted: %q", full)
	}
	if full := tuiPaintDiff(prev[:2], next, 10); full != tuiPaint(next, 10) {
		t.Fatal("a frame of another size is painted whole")
	}
	if same := tuiPaintDiff(next, next, 10); same != "" {
		t.Fatalf("an unchanged frame paints %q", same)
	}
}

// weeklyPercent reads the weekly percentage a gauge row shows.
func weeklyPercent(t *testing.T, rows []string) int {
	t.Helper()
	for _, row := range rows {
		plain := stripEscapes(row)
		if strings.Contains(plain, "Weekly") {
			fields := strings.Fields(plain)
			for _, f := range fields {
				var n int
				if _, err := fmt.Sscanf(f, "%d%%", &n); err == nil && strings.HasSuffix(f, "%") {
					return n
				}
			}
		}
	}
	t.Fatalf("no weekly percentage in:\n%s", stripEscapes(strings.Join(rows, "\n")))
	return -1
}

func TestMetersEaseToANewValue(t *testing.T) {
	a, state := listApp()
	a.renderClock = func() time.Time { return poolNow }
	before := a.tuiAccountView(state, "a@example.invalid", poolNow).pools[0].limiting.fraction
	getMap(getSlice(getMap(getMap(state.accounts.ByEmail["a@example.invalid"]["quota_snapshot"]))["groups"])[0])["buckets"] = []any{poolBucket("weekly", 0.1, 100*time.Hour)}
	at := func(d time.Duration) float64 {
		return a.tuiAccountView(state, "a@example.invalid", poolNow.Add(d)).pools[0].limiting.fraction
	}
	start, mid, end := at(0), at(tuiMotionMeter/3), at(tuiMotionMeter*2)
	if before != 0.9 || start != 0.9 || mid >= 0.9 || mid <= 0.1 || end != 0.1 {
		t.Fatalf("meter: before %v, start %v, mid %v, end %v", before, start, mid, end)
	}
	// The status word follows the real value (10% is low), not the eased one.
	if view := a.tuiAccountView(state, "a@example.invalid", poolNow.Add(tuiMotionMeter/3)); view.status != statusLow {
		t.Fatalf("status = %v", view.status)
	}
}

func TestTheDetailEasesFromTheLastAccountsNumbers(t *testing.T) {
	a, state := listApp()
	plan := tuiLayoutPlan{arrangement: arrWide, panels: panelsStack, digits: digitsNone}
	render := func(d time.Duration) int {
		a.renderClock = func() time.Time { return poolNow.Add(d) }
		return weeklyPercent(t, a.tuiGaugeDetail(state, 60, 0, plan))
	}
	state.selectedEmail = "a@example.invalid"
	if got := render(0); got != 90 {
		t.Fatalf("settled detail = %d%%", got)
	}
	state.selectedEmail = "c@example.invalid"
	start, mid, end := render(0), render(tuiMotionMeter/3), render(tuiMotionMeter*2)
	if start != 90 || mid >= 90 || mid <= 20 || end != 20 {
		t.Fatalf("detail percent: start %d, mid %d, end %d", start, mid, end)
	}
}

func truecolorApp() (*Application, *tuiState) {
	a, state := listApp()
	a.p.Gray = "\x1b[38;2;139;148;158m"
	a.p.Backdrop = "\x1b[38;2;84;93;104m"
	a.p.Blue = "\x1b[38;2;56;189;248m"
	return a, state
}

func TestADialogOpensFromItsCentreOverADimmingBackdrop(t *testing.T) {
	a, state := truecolorApp()
	state.openDialog(confirmDialog("switch", "b@example.invalid", "Beta User"))
	state.modalOpenedAt = poolNow
	frame := func(d time.Duration) []string {
		a.renderClock = func() time.Time { return poolNow.Add(d) }
		return a.tuiLines(state, 98, 30)
	}
	boxRows := func(lines []string) int {
		n := 0
		for _, line := range lines {
			if strings.Contains(line, a.p.Blue) {
				n++
			}
		}
		return n
	}
	early, settled := frame(tuiMotionModal/4), frame(tuiMotionModal*2)
	if boxRows(early) >= boxRows(settled) || boxRows(early) < 2 {
		t.Fatalf("dialog rows: early %d, settled %d", boxRows(early), boxRows(settled))
	}
	if !strings.Contains(strings.Join(settled, ""), a.p.Backdrop) {
		t.Fatal("the settled backdrop uses the backdrop colour")
	}
	if strings.Contains(strings.Join(early, ""), a.p.Backdrop) {
		t.Fatal("early on the backdrop is still on its way from the text colour")
	}
	if got := lerpColor(a.p.Gray, a.p.Backdrop, 0.5); got != "\x1b[38;2;111;120;131m" {
		t.Fatalf("lerpColor = %q", got)
	}
	if got := lerpColor("\x1b[38;5;244m", a.p.Backdrop, 0.5); got != a.p.Backdrop {
		t.Fatalf("a 256-colour code does not blend: %q", got)
	}
}

func TestAToastSlidesUpAndItsEdgeCountsDown(t *testing.T) {
	a, state := truecolorApp()
	a.p.Green = "\x1b[32m"
	a.p.Track = "\x1b[38;5;238m"
	state.showToast("Switched to b@example.invalid", "success")
	state.toastShownAt = poolNow
	state.toastUntil = poolNow.Add(tuiToastDuration)
	frame := func(d time.Duration) []string {
		a.renderClock = func() time.Time { return poolNow.Add(d) }
		return a.tuiLines(state, 98, 30)
	}
	row := func(lines []string) int {
		for i, line := range lines {
			if strings.Contains(line, "Switched to") {
				return i
			}
		}
		t.Fatalf("no toast in frame")
		return -1
	}
	if early, settled := row(frame(tuiMotionToast/4)), row(frame(tuiMotionToast*2)); early <= settled {
		t.Fatalf("the toast rises: early row %d, settled row %d", early, settled)
	}
	edge := func(lines []string) string {
		return lines[row(lines)+1]
	}
	full, half := edge(frame(0)), edge(frame(tuiToastDuration/2))
	if strings.Contains(full, a.p.Track+"─") {
		t.Fatalf("a fresh toast's edge is lit end to end: %q", full)
	}
	if !strings.Contains(half, a.p.Track+"─") || !strings.Contains(half, a.p.Green+"╰─") {
		t.Fatalf("half way, part of the edge has run out: %q", half)
	}
}

func motionColorApp() (*Application, *tuiState) {
	a, state := listApp()
	a.p.KeyBg = "\x1b[48;5;238m"
	a.p.Track = "\x1b[38;5;238m"
	a.p.BodyBg = "\x1b[48;5;232m"
	return a, state
}

func TestASwitchPulsesTheActiveDotAndSweepsTheWordIn(t *testing.T) {
	a, state := motionColorApp()
	view := func(d time.Duration) tuiAccountView {
		a.renderClock = func() time.Time { return poolNow.Add(d) }
		a.tuiLines(state, 98, 30) // a frame notices the change
		return a.tuiAccountView(state, state.active, poolNow.Add(d))
	}
	view(0)
	state.active = "b@example.invalid"
	early := view(tuiMotionSweep / 5)
	_, word := a.tuiActiveName(early)
	if stripEscapes(word) == "ACTIVE" || visibleWidth(word) != len("ACTIVE") {
		t.Fatalf("early word = %q, want part of ACTIVE at full width", stripEscapes(word))
	}
	marks := map[string]bool{}
	for d := time.Duration(0); d < tuiMotionPulse; d += tuiMotionPulse / 12 {
		marks[stripEscapes(a.tuiActiveMarker(view(d)))] = true
	}
	if !marks["◉"] || !marks["●"] {
		t.Fatalf("the dot does not pulse: %v", marks)
	}
	settled := view(tuiMotionPulse * 2)
	if _, word := a.tuiActiveName(settled); stripEscapes(word) != "ACTIVE" || stripEscapes(a.tuiActiveMarker(settled)) != "●" {
		t.Fatal("after the pulse the mark rests as ● ACTIVE")
	}
	if got := asciiLine("◉"); got != "o" {
		t.Fatalf("ascii stand-in = %q", got)
	}
}

func TestThePressedKeyLightsItsChip(t *testing.T) {
	a, state := motionColorApp()
	a.renderClock = func() time.Time { return poolNow.Add(tuiMotionFlash / 2) }
	state.keyFlash, state.keyFlashAt = "enter", poolNow
	lit := a.tuiFooterLines(state, 98)[1]
	// Orange in the test palette is \x1b[31m, so the lit chip uses \x1b[41m.
	if !strings.Contains(lit, "\x1b[41m") || strings.Contains(lit, a.p.KeyBg+"\x1b[1m\x1b[37m Enter ") {
		t.Fatalf("the Enter chip is not lit: %q", lit)
	}
	a.renderClock = func() time.Time { return poolNow.Add(tuiMotionFlash * 2) }
	if row := a.tuiFooterLines(state, 98)[1]; strings.Contains(row, "\x1b[41m") {
		t.Fatalf("the chip stays lit: %q", row)
	}
}

func TestTheDividerLightsWhileDragged(t *testing.T) {
	a, state := motionColorApp()
	state.dragging = true
	lines := a.tuiLines(state, 118, 30)
	if !strings.Contains(strings.Join(lines, ""), a.p.Orange+"│") {
		t.Fatal("the divider is not lit while dragged")
	}
	state.dragging = false
	if strings.Contains(strings.Join(a.tuiLines(state, 118, 30), ""), a.p.Orange+"│") {
		t.Fatal("the divider stays lit")
	}
}

func TestTheSearchCaretBlinks(t *testing.T) {
	a, state := motionColorApp()
	state.beginSearch()
	seen := map[bool]bool{}
	for d := time.Duration(0); d < 2*tuiCaretBlink; d += tuiCaretBlink / 2 {
		a.renderClock = func() time.Time { return poolNow.Add(d) }
		seen[strings.Contains(a.tuiStatusLines(state, 98)[1], "▌")] = true
	}
	if !seen[true] || !seen[false] {
		t.Fatalf("caret states seen: %v", seen)
	}
}

func TestAChangedStatusWordFlashesOnce(t *testing.T) {
	a, state := motionColorApp()
	a.tuiAccountView(state, "a@example.invalid", poolNow)
	getMap(getSlice(getMap(getMap(state.accounts.ByEmail["a@example.invalid"]["quota_snapshot"]))["groups"])[0])["buckets"] = []any{poolBucket("weekly", 0.05, 100*time.Hour)}
	if v := a.tuiAccountView(state, "a@example.invalid", poolNow.Add(tuiMotionStatus/2)); !v.statusFlash {
		t.Fatal("the new status word does not flash")
	}
	if v := a.tuiAccountView(state, "a@example.invalid", poolNow.Add(tuiMotionStatus*2)); v.statusFlash {
		t.Fatal("the flash does not end")
	}
	if v := a.tuiAccountView(state, "b@example.invalid", poolNow); v.statusFlash {
		t.Fatal("a status seen for the first time flashes")
	}
}

func TestStartUpAndViewChangesWipeIn(t *testing.T) {
	a, state := motionColorApp()
	blank := func(lines []string) int {
		n := 0
		for _, line := range lines {
			if strings.TrimSpace(stripEscapes(line)) == "" {
				n++
			}
		}
		return n
	}
	a.renderClock = func() time.Time { return poolNow.Add(tuiMotionStart * 2) }
	settled := blank(a.tuiLines(state, 98, 30))
	state.startedAt = poolNow
	a.renderClock = func() time.Time { return poolNow.Add(tuiMotionStart / 4) }
	if early := blank(a.tuiLines(state, 98, 30)); early <= settled {
		t.Fatalf("start-up: %d blank rows early, %d settled", early, settled)
	}
	state.startedAt = time.Time{}
	state.viewChangedAt = poolNow
	a.renderClock = func() time.Time { return poolNow.Add(tuiMotionView / 4) }
	if early := blank(a.tuiLines(state, 98, 30)); early <= settled {
		t.Fatalf("view change: %d blank rows early, %d settled", early, settled)
	}
}

func TestAManualRefreshSweepsALightAcrossTheScreen(t *testing.T) {
	a, state := truecolorApp()
	a.p.BodyBg = "\x1b[48;2;10;12;16m"
	a.p.KeyBg = "\x1b[48;2;55;62;71m"
	// band is the left-most column drawn on the brightest shade in rows 8–11.
	band := func(lines []string) int {
		first := -1
		for _, line := range lines[8:12] {
			for col, bg := range cellBackgrounds(line) {
				if bg == "\x1b[48;2;55;62;71m" && (first < 0 || col < first) {
					first = col
				}
			}
		}
		return first
	}
	frame := func(d time.Duration) []string {
		a.renderClock = func() time.Time { return poolNow.Add(d) }
		return a.tuiLines(state, 98, 30)
	}
	if band(frame(0)) != -1 {
		t.Fatal("a sweep without a refresh")
	}
	state.refreshing, state.refreshStartedAt = true, poolNow
	early, later := band(frame(tuiRefreshSweep/4)), band(frame(tuiRefreshSweep/2))
	if early < 0 || later <= early {
		t.Fatalf("the light should move right: %d then %d", early, later)
	}
	if !state.moving(poolNow.Add(tuiRefreshSweep * 3)) {
		t.Fatal("the sweep keeps going while the refresh runs")
	}
	state.refreshing = false
	if band(frame(tuiRefreshSweep/2)) != -1 {
		t.Fatal("the sweep stops with the refresh")
	}
}

func TestTheActiveDotBreathesWhileIdle(t *testing.T) {
	a, state := truecolorApp()
	colours := map[string]bool{}
	for d := time.Duration(0); d < tuiBreathPeriod; d += tuiBreathPeriod / 6 {
		a.renderClock = func() time.Time { return poolNow.Add(d) }
		a.tuiLines(state, 98, 30)
		view := a.tuiAccountView(state, state.active, poolNow.Add(d))
		mark := a.tuiActiveMarker(view)
		colours[strings.TrimSuffix(mark, "●"+a.p.Reset)] = true
	}
	if len(colours) < 3 {
		t.Fatalf("the dot does not breathe: %v", colours)
	}
	if !state.ambient {
		t.Fatal("breathing needs frames")
	}
	state.motionEnabled = false
	a.tuiLines(state, 98, 30)
	if state.ambient {
		t.Fatal("reduced motion keeps the dot still")
	}
}

func TestTheDialogButtonSlidesBetweenChoices(t *testing.T) {
	a, state := truecolorApp()
	a.p.SelectBg = testSelectBg
	state.openDialog(confirmDialog("switch", "b@example.invalid", "Beta User"))
	state.modalOpenedAt = time.Time{}
	buttons := func(d time.Duration) string {
		a.renderClock = func() time.Time { return poolNow.Add(d) }
		for _, line := range a.tuiLines(state, 98, 30) {
			if strings.Contains(stripEscapes(line), "Cancel") && strings.Contains(stripEscapes(line), "Switch") {
				return line
			}
		}
		t.Fatal("no button row")
		return ""
	}
	buttons(0)
	state.dialogKey("right")
	mid := stripEscapes(buttons(tuiMotionSlide / 4))
	if strings.Contains(mid, "[ Switch ]") || strings.Contains(mid, "[ Cancel ]") {
		t.Fatalf("mid-slide a chip already sits on a button: %q", mid)
	}
	if end := stripEscapes(buttons(tuiMotionSlide * 2)); !strings.Contains(end, "[ Cancel ]") {
		t.Fatalf("settled row = %q", end)
	}
}

func TestFrameIntervalRunsAtSixtyFPS(t *testing.T) {
	if tuiFrameInterval > 16*time.Millisecond {
		t.Fatalf("tuiFrameInterval = %v, want <= 16ms (~60 FPS)", tuiFrameInterval)
	}
	if frames := int(tuiMotionSlide / tuiFrameInterval); frames < 9 {
		t.Fatalf("tuiMotionSlide (%v) at %v yields only %d frames, want >= 9", tuiMotionSlide, tuiFrameInterval, frames)
	}
}

func TestTruecolorSelectionBandAntiAliasesFractionalRows(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	t.Setenv("TERM", "xterm-256color")
	t.Setenv("COLORTERM", "truecolor")
	a, state := listApp()
	a.p = makePalette(true)
	render := func(d time.Duration) []string {
		a.renderClock = func() time.Time { return poolNow.Add(d) }
		return a.tuiAccountRowsSpaced(state, 60, 30, 2, true)
	}
	render(0)
	state.selectedEmail = "c@example.invalid"
	render(0)
	mid := render(tuiMotionSlide / 7)
	hasBlendedRow := false
	for _, row := range mid {
		if strings.HasPrefix(row, "\x1b[48;2;") && !strings.HasPrefix(row, a.p.SelectBg) && !strings.HasPrefix(row, a.p.BodyBg) {
			hasBlendedRow = true
			break
		}
	}
	if !hasBlendedRow {
		t.Fatalf("expected fractional edge row to blend between BodyBg and SelectBg during slide:\n%q", mid)
	}
}

func TestTruecolorDialogSlidingButtonsAntiAliasFractionalColumns(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	t.Setenv("TERM", "xterm-256color")
	t.Setenv("COLORTERM", "truecolor")
	a, state := listApp()
	a.p = makePalette(true)
	state.openDialog(confirmDialog("switch", "b@example.invalid", "Beta User"))
	state.modalOpenedAt = time.Time{}
	a.renderClock = func() time.Time { return poolNow }
	a.tuiLines(state, 98, 30)
	state.dialogKey("right")
	hasBlendedCell := false
	for step := 1; step <= 7; step++ {
		d := time.Duration(step) * tuiMotionSlide / 8
		a.renderClock = func() time.Time { return poolNow.Add(d) }
		for _, line := range a.tuiLines(state, 98, 30) {
			if strings.Contains(stripEscapes(line), "Cancel") && strings.Contains(stripEscapes(line), "Switch") {
				for _, bg := range cellBackgrounds(line) {
					if strings.HasPrefix(bg, "\x1b[48;2;") && bg != a.p.SelectBg && bg != a.p.BodyBg && bg != a.p.PanelBg {
						hasBlendedCell = true
					}
				}
			}
		}
	}
	if !hasBlendedCell {
		t.Fatal("expected sliding dialog button pill to anti-alias fractional boundary columns in truecolor")
	}
}

func TestTruecolorMeterBlendsFractionalLeadingCell(t *testing.T) {
	fill := "\x1b[38;2;52;211;153m"
	track := "\x1b[38;2;55;62;71m"
	bar := renderMeter(meterHalf, 0.33, 10, fill, track, "\x1b[0m")
	// 0.33 of 10 cells is 3.3 cells: 3 full cells in fill, 1 fractional cell in a blended truecolor, 6 rail cells in track.
	if got := visibleWidth(bar); got != 10 {
		t.Fatalf("visibleWidth = %d, want 10", got)
	}
	stripped := strings.ReplaceAll(strings.ReplaceAll(bar, fill, ""), track, "")
	if !strings.Contains(stripped, "\x1b[38;2;") {
		t.Fatalf("expected fractional leading cell to use a blended truecolor code between track and fill: %q", bar)
	}
}

// cellBackgrounds is the background code each column is drawn on.
func cellBackgrounds(line string) []string {
	var cells []string
	bg := ""
	for len(line) > 0 {
		if size := ansiPrefixLen(line); size > 0 {
			code := line[:size]
			switch {
			case code == "\x1b[0m" || code == "\x1b[49m":
				bg = ""
			case strings.HasPrefix(code, "\x1b[48;"):
				bg = code
			}
			line = line[size:]
			continue
		}
		r, size := utf8.DecodeRuneInString(line)
		for i := 0; i < terminalRuneWidth(r); i++ {
			cells = append(cells, bg)
		}
		line = line[size:]
	}
	return cells
}
