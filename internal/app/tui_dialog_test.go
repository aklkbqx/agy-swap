package app

import (
	"strings"
	"testing"
)

func TestEveryQuestionUsesTheSameDialogWithChoosableButtons(t *testing.T) {
	for _, tc := range []struct {
		kind, confirm string
		destructive   bool
	}{
		{"switch", "Switch", false},
		{"update", "Update", false},
		{"delete", "Delete", true},
		{"profile-remove", "Remove", true},
		{"history-clear", "Clear", true},
		{"settings-reset", "Reset", true},
	} {
		d := confirmDialog(tc.kind, "user@example.invalid", "User")
		if len(d.Buttons) != 2 || d.Buttons[0].Label != tc.confirm || d.Buttons[1].Label != "Cancel" {
			t.Fatalf("%s buttons = %+v", tc.kind, d.Buttons)
		}
		want := 0
		if tc.destructive {
			want = 1 // a slip of Enter must not destroy anything
		}
		if d.Index != want {
			t.Fatalf("%s starts on button %d, want %d", tc.kind, d.Index, want)
		}
		if d.Title == "" || len(d.Body) == 0 {
			t.Fatalf("%s has no title or body: %+v", tc.kind, d)
		}
	}
}

func TestArrowsChooseAButtonAndEnterPressesIt(t *testing.T) {
	_, state := selectionState()
	state.openDialog(confirmDialog("switch", "beta@example.invalid", "Beta User"))
	for _, key := range []string{"left", "right", "right", "left", "down", "up", "tab", "shift-tab"} {
		if _, _, done := state.dialogKey(key); done {
			t.Fatalf("%s closed the dialog", key)
		}
	}
	if state.dialog.Index != 0 {
		t.Fatalf("index = %d after moving back and forth", state.dialog.Index)
	}
	state.dialogKey("right")
	kind, action, done := state.dialogKey("enter")
	if !done || kind != "switch" || action != "cancel" || state.mode != tuiBrowse || state.dialog != nil {
		t.Fatalf("enter on Cancel = %q %q %v, mode %v", kind, action, done, state.mode)
	}
	state.openDialog(confirmDialog("switch", "beta@example.invalid", "Beta User"))
	if _, action, done := state.dialogKey("enter"); !done || action != "confirm" {
		t.Fatalf("enter on Switch = %q %v", action, done)
	}
	state.openDialog(confirmDialog("delete", "beta@example.invalid", "Beta User"))
	if _, action, done := state.dialogKey("y"); !done || action != "confirm" {
		t.Fatalf("y = %q %v", action, done)
	}
	state.openDialog(confirmDialog("delete", "beta@example.invalid", "Beta User"))
	if _, action, done := state.dialogKey("esc"); !done || action != "cancel" {
		t.Fatalf("esc = %q %v", action, done)
	}
}

func TestTheChosenButtonIsMarkedWithoutRelyingOnColour(t *testing.T) {
	a, state := selectionState()
	state.openDialog(confirmDialog("delete", "beta@example.invalid", "Beta User"))
	row := a.tuiDialogLines(state.dialog)
	text := strings.Join(row, "\n")
	if !strings.Contains(text, "[ Cancel ]") || strings.Contains(text, "[ Delete ]") {
		t.Fatalf("delete dialog starts on Cancel:\n%s", text)
	}
	state.dialogKey("left")
	text = strings.Join(a.tuiDialogLines(state.dialog), "\n")
	if !strings.Contains(text, "[ Delete ]") || strings.Contains(text, "[ Cancel ]") {
		t.Fatalf("after ← the Delete button is chosen:\n%s", text)
	}
	footer := strings.Join(a.tuiFooterLines(state, 80), "\n")
	if !strings.Contains(footer, "←→") || !strings.Contains(footer, "Enter") {
		t.Fatalf("footer = %q", footer)
	}
}

func TestHelpUsesTheDialogToo(t *testing.T) {
	a, state := selectionState()
	state.openHelp(a.tuiHelpLines(80))
	if state.mode != tuiDialogOpen || state.dialog.Kind != "help" || len(state.dialog.Buttons) != 1 {
		t.Fatalf("help dialog = %+v", state.dialog)
	}
	if _, _, done := state.dialogKey("x"); !done || state.mode != tuiBrowse {
		t.Fatal("any key closes help")
	}
}
