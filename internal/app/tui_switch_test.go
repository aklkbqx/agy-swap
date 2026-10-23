package app

import (
	"context"
	"strings"
	"testing"
)

func TestEnterAsksBeforeSwitchingAndNamesTheAccount(t *testing.T) {
	a, state := selectionState()
	state.active = "alpha@example.invalid"
	if !state.beginSwitchConfirm() {
		t.Fatal("no confirmation for a different account")
	}
	if state.mode != tuiDialogOpen || state.dialog.Kind != "switch" || !strings.Contains(state.dialog.Title, "Beta User") {
		t.Fatalf("mode=%v dialog=%+v", state.mode, state.dialog)
	}
	text := strings.Join(a.tuiDialogLines(state.dialog), "\n")
	for _, want := range []string{"beta@example.invalid", "[ Switch ]", "Cancel"} {
		if !strings.Contains(text, want) {
			t.Fatalf("dialog is missing %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "may change local configuration") {
		t.Fatalf("switch dialog uses the generic text:\n%s", text)
	}
}

func TestEnterOnTheActiveAccountSaysSoInsteadOfAsking(t *testing.T) {
	_, state := selectionState()
	state.active = "beta@example.invalid"
	if state.beginSwitchConfirm() {
		t.Fatal("asked to switch to the account already in use")
	}
	if state.mode != tuiBrowse || !strings.Contains(state.message, "Already using") {
		t.Fatalf("mode=%v message=%q", state.mode, state.message)
	}
}

func TestSwitchingFromTheTUIDoesNotLeaveTheScreen(t *testing.T) {
	f := newCLIFixture(t, "alpha@example.com", "beta@example.com")
	accounts, err := f.a.store.Load(false)
	if err != nil {
		t.Fatal(err)
	}
	state := newTUIState(accounts, "")
	state.selectedEmail = "beta@example.com"
	message, kind := f.a.tuiSwitchSelected(context.Background(), state)
	if kind != "success" || !strings.Contains(message, "Switched to beta@example.com") {
		t.Fatalf("toast = %q (%s)", message, kind)
	}
	if f.backend.token == "" || !tokenMatchesEmail(f.backend.token, "beta@example.com") {
		t.Fatal("the session was not switched")
	}
	if f.out.Len() != 0 || f.errOut.Len() != 0 {
		t.Fatalf("switch wrote to the terminal: %q %q", f.out.String(), f.errOut.String())
	}
	if message, kind := f.a.tuiSwitchSelected(context.Background(), state); kind != "info" || !strings.Contains(message, "Already using") {
		t.Fatalf("second switch toast = %q (%s)", message, kind)
	}
}
