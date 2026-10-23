package app

import (
	"strings"
	"testing"
	"time"
)

func overlayTestState() (*Application, *tuiState) {
	accounts := NewAccounts()
	account := quotaAccount("user@example.com", 0.85, 0.45, time.Now().Add(time.Hour))
	account["name"] = "Test User"
	accounts.Set("user@example.com", account)
	a := &Application{Version: "2.8.7", p: makePalette(true), color: true}
	return a, newTUIState(accounts, "user@example.com")
}

// assertOverlayKeepsMargins checks that an overlay sits inside the screen's
// two-column margins: the screen has no frame, so its edge columns stay blank.
func assertOverlayKeepsMargins(t *testing.T, lines []string, marker string) {
	t.Helper()
	found := false
	for i, line := range lines {
		plain := ansiPattern.ReplaceAllString(line, "")
		if !strings.Contains(plain, marker) {
			continue
		}
		found = true
		runes := []rune(plain)
		if first, last := runes[0], runes[len(runes)-1]; first != ' ' || last != ' ' {
			t.Fatalf("line %d runs into the screen edge: %q", i, plain)
		}
	}
	if !found {
		t.Fatalf("overlay marker %q missing", marker)
	}
}

func TestTUIToastKeepsFrameBorders(t *testing.T) {
	for _, width := range []int{118, 78, 46} {
		a, state := overlayTestState()
		state.showToast("Switched to user@example.com", "success")
		lines := a.tuiLines(state, width, 22)
		assertOverlayKeepsMargins(t, lines, "Switched to user@example.com")
		assertOverlayKeepsMargins(t, lines, "╰")
		for i, line := range lines {
			if visibleWidth(line) != width+2 {
				t.Fatalf("width=%d line %d width=%d", width, i, visibleWidth(line))
			}
		}
	}
}

func TestTUIDialogKeepsFrameBorders(t *testing.T) {
	a, state := overlayTestState()
	state.openHelp(a.tuiHelpLines(118))
	lines := a.tuiLines(state, 118, 24)
	assertOverlayKeepsMargins(t, lines, "╭")
	assertOverlayKeepsMargins(t, lines, "╰")
}

func TestOverlayVisibleKeepsSurroundingColumns(t *testing.T) {
	p := makePalette(true)
	base := p.Gray + "│" + p.Reset + " ab" + p.Green + "界cd" + p.Reset + " │"
	got := overlayVisible(base, "XY", 3, p)
	if plain := ansiPattern.ReplaceAllString(got, ""); plain != "│ aXY cd │" {
		t.Fatalf("plain=%q", plain)
	}
	if visibleWidth(got) != visibleWidth(base) {
		t.Fatalf("width=%d want %d", visibleWidth(got), visibleWidth(base))
	}
	if !strings.HasSuffix(got, "│") && !strings.HasSuffix(got, p.Reset) {
		t.Fatalf("suffix lost: %q", got)
	}
}

func TestTUIQuotaViewShowsSelectedEmailOnce(t *testing.T) {
	a, state := overlayTestState()
	a.p = makePalette(false)
	state.view = tuiViewQuota
	state.selectedEmail = "user@example.com"
	for _, width := range []int{118, 78} {
		joined := strings.Join(a.tuiLines(state, width, 24), "\n")
		if got := strings.Count(joined, "user@example.com"); got != 1 {
			// The overview rows name accounts only; the detail names the email once.
			t.Fatalf("width=%d: email appears %d times:\n%s", width, got, joined)
		}
		if strings.Contains(joined, "Selected account") || strings.Contains(joined, "SELECTED ACCOUNT") {
			t.Fatalf("width=%d: the old selected-account label is back:\n%s", width, joined)
		}
	}
}

func TestTheHeaderIsABarWithoutCorners(t *testing.T) {
	app := &Application{p: makePalette(false)}
	lines := app.tuiHeaderLines("AGY SWAP", "v2.9.1", 80)
	if strings.ContainsAny(lines[0], "╭╮") || visibleWidth(lines[0]) != 82 || !strings.Contains(lines[1], "agy") {
		t.Errorf("header = %q", lines)
	}
}
