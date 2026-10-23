package app

import (
	"strings"
	"testing"
)

const testBackdrop = "\x1b[38;5;239m"

func backdropApp() (*Application, *tuiState) {
	a, state := selectionState()
	a.p = palette{Green: "\x1b[32m", Yellow: "\x1b[33m", Orange: "\x1b[31m", Blue: "\x1b[34m", Gray: "\x1b[90m", White: "\x1b[37m", Bold: "\x1b[1m", Reset: "\x1b[0m", Backdrop: testBackdrop}
	return a, state
}

func TestAModalDimsEverythingBehindItButItsOwnBoxAndTheKeyHints(t *testing.T) {
	a, state := backdropApp()
	state.active = "alpha@example.invalid"
	state.beginSwitchConfirm()
	lines := a.tuiLines(state, 98, 30)
	footer := lines[len(lines)-3:]
	if !strings.Contains(strings.Join(footer, ""), "Choose") || strings.Contains(strings.Join(footer, ""), testBackdrop) {
		t.Fatalf("the key hints must stay readable: %q", footer)
	}
	// Dialog rows carry the dialog's blue frame; every other row is backdrop.
	dimmed, boxed := 0, 0
	for _, line := range lines[1 : len(lines)-5] {
		// Behind a modal the side rails are plain blank columns.
		if !strings.HasPrefix(line, " ") || !strings.HasSuffix(line, " ") {
			t.Fatalf("a rail behind the modal keeps its colour: %q", line)
		}
		line = line[1 : len(line)-1]
		if strings.Contains(line, "\x1b[34m") {
			boxed++
			before, _, _ := strings.Cut(line, "\x1b[34m")
			if !strings.HasPrefix(before, testBackdrop) || strings.LastIndex(line, testBackdrop) < strings.LastIndex(line, "\x1b[34m") {
				t.Fatalf("the backdrop carries on either side of the dialog: %q", line)
			}
			continue
		}
		dimmed++
		rest := strings.ReplaceAll(strings.ReplaceAll(line, testBackdrop, ""), "\x1b[0m", "")
		if !strings.HasPrefix(line, testBackdrop) || strings.Contains(rest, "\x1b[") {
			t.Fatalf("a backdrop row keeps its own colours: %q", line)
		}
	}
	if boxed < 5 || dimmed < 10 {
		t.Fatalf("boxed=%d dimmed=%d", boxed, dimmed)
	}
	for _, line := range lines {
		if visibleWidth(line) != 100 {
			t.Fatalf("row is %d wide: %q", visibleWidth(line), line)
		}
	}
}

func TestNoModalNoBackdrop(t *testing.T) {
	a, state := backdropApp()
	for _, line := range a.tuiLines(state, 98, 30) {
		if strings.Contains(line, testBackdrop) {
			t.Fatalf("backdrop without a modal: %q", line)
		}
	}
}

func TestPaletteAndFormsShareTheBackdrop(t *testing.T) {
	a, state := backdropApp()
	state.beginPalette()
	if !strings.Contains(strings.Join(a.tuiLines(state, 98, 30), "\n"), testBackdrop) {
		t.Fatal("the palette has no backdrop")
	}
}
