package app

import (
	"strings"
	"testing"
)

func headerApp() (*Application, *tuiState) {
	accounts := NewAccounts()
	accounts.Set("user@example.com", Account{"email": "user@example.com", "name": "User"})
	return &Application{Version: "2.13.0", p: palette{}, color: false}, newTUIState(accounts, "user@example.com")
}

func TestHeaderShowsTheWordmarkVersionAndCreditWhenThereIsRoom(t *testing.T) {
	a, state := headerApp()
	line := a.tuiHeaderLinesWithVersion("2.13.0", state, 118)[1]
	for _, want := range []string{"agy▔▔╲▁▁swap", "v2.13.0", tuiCredit, "LOCAL"} {
		if !strings.Contains(line, want) {
			t.Fatalf("header is missing %q: %q", want, line)
		}
	}
	if visibleWidth(line) != 120 {
		t.Fatalf("header row is %d wide: %q", visibleWidth(line), line)
	}
}

func TestHeaderDropsTheCreditBeforeTheSyncStateOnNarrowScreens(t *testing.T) {
	a, state := headerApp()
	for _, width := range []int{26, 30, 38, 46} {
		for _, refreshing := range []bool{false, true} {
			state.refreshing = refreshing
			line := a.tuiHeaderLinesWithVersion("2.13.0", state, width)[1]
			if visibleWidth(line) != width+2 {
				t.Fatalf("width %d: row is %d wide: %q", width, visibleWidth(line), line)
			}
			if width <= 38 && strings.Contains(line, tuiCredit) {
				t.Fatalf("width %d still shows the credit: %q", width, line)
			}
			if width >= 38 {
				state := pick(refreshing, "SYNCING", "LOCAL")
				if !strings.Contains(line, state) {
					t.Fatalf("width %d lost the sync state %q: %q", width, state, line)
				}
			}
		}
	}
}

func TestHelpOpensWithTheLogoAndCreditThenTheKeys(t *testing.T) {
	a, _ := headerApp()
	text := strings.Join(a.tuiHelpLines(80), "\n")
	for _, want := range []string{"agy▔▔╲▁▁swap", "aklkbqx", "MIT", "github.com/aklkbqx/agy-swap", "not affiliated with Google", "KEYBOARD GUIDE"} {
		if !strings.Contains(text, want) {
			t.Fatalf("help is missing %q:\n%s", want, text)
		}
	}
	if strings.Index(text, "agy▔▔╲▁▁swap") > strings.Index(text, "KEYBOARD GUIDE") {
		t.Fatalf("the logo and credit must come before the key guide:\n%s", text)
	}
}

func TestHelpOverlayShowsTheLogoAndCreditAtCommonWindowSizes(t *testing.T) {
	a, state := headerApp()
	for _, size := range [][2]int{{100, 30}, {80, 24}, {63, 24}, {120, 40}} {
		state.mode = tuiHelp
		text := strings.Join(a.tuiLines(state, size[0]-2, size[1]), "\n")
		for _, want := range []string{"agy▔▔╲▁▁swap", "aklkbqx", "KEYBOARD GUIDE", "Enter"} {
			if !strings.Contains(text, want) {
				t.Fatalf("%dx%d: help is missing %q:\n%s", size[0], size[1], want, text)
			}
		}
		if strings.Contains(text, "more in ? help") {
			t.Fatalf("%dx%d: the circular hint is back:\n%s", size[0], size[1], text)
		}
	}
}
