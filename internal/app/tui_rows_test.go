package app

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func rowsApp() *Application {
	return &Application{Version: "2.13.0", p: palette{}, color: false, renderClock: func() time.Time { return poolNow }}
}

func rowsState(accounts map[string]Account, order ...string) *tuiState {
	set := NewAccounts()
	for _, email := range order {
		set.Set(email, accounts[email])
	}
	return newTUIState(set, "")
}

func namedPoolAccount(name string, groups ...map[string]any) Account {
	account := poolAccount(groups...)
	account["name"] = name
	return account
}

func TestAccountBlockTwoLinesShowsStatusWordAndBothPoolMeters(t *testing.T) {
	a := rowsApp()
	state := rowsState(map[string]Account{
		"beta@example.invalid": namedPoolAccount("Beta User",
			poolGroup("gemini", "Gemini models", poolBucket("weekly", 0.36, 100*time.Hour)),
			poolGroup("third_party", "Claude and GPT models", poolBucket("weekly", 0, 22*time.Hour))),
	}, "beta@example.invalid")
	view := a.tuiAccountView(state, "beta@example.invalid", poolNow)
	lines := a.tuiAccountBlock(view, state, 50, 2)
	if len(lines) != 2 {
		t.Fatalf("lines = %q", lines)
	}
	if !strings.Contains(lines[0], "❯") || !strings.Contains(lines[0], "Beta User") || !strings.HasSuffix(strings.TrimRight(lines[0], " "), "GEMINI ONLY") {
		t.Fatalf("identity line = %q", lines[0])
	}
	gem := "Gem " + renderFractionalBar(0.36, 8, "", "", "") + "  36%"
	cg := "C/G " + renderFractionalBar(0, 8, "", "", "") + "   0%"
	if !strings.Contains(lines[1], gem) || !strings.Contains(lines[1], cg) {
		t.Fatalf("meter line = %q, want %q and %q", lines[1], gem, cg)
	}
	for _, line := range lines {
		if visibleWidth(line) != 50 {
			t.Fatalf("line is %d wide: %q", visibleWidth(line), line)
		}
	}
}

func TestAccountBlockOneLineKeepsStatusAndShortMeters(t *testing.T) {
	a := rowsApp()
	state := rowsState(map[string]Account{
		"alpha@example.invalid": namedPoolAccount("Alpha User",
			poolGroup("gemini", "Gemini models", poolBucket("weekly", 0.86, 100*time.Hour)),
			poolGroup("third_party", "Claude and GPT models", poolBucket("weekly", 1, 100*time.Hour))),
	}, "alpha@example.invalid")
	view := a.tuiAccountView(state, "alpha@example.invalid", poolNow)
	lines := a.tuiAccountBlock(view, state, 61, 1)
	if len(lines) != 1 || visibleWidth(lines[0]) != 61 {
		t.Fatalf("lines = %q", lines)
	}
	for _, want := range []string{"Alpha User", "READY", "G ", "C ", " 86%", "100%"} {
		if !strings.Contains(lines[0], want) {
			t.Fatalf("one-line row is missing %q: %q", want, lines[0])
		}
	}
}

func TestAccountBlockShowsADashForAPoolTheAccountDoesNotHave(t *testing.T) {
	a := rowsApp()
	state := rowsState(map[string]Account{
		"c@example.invalid": namedPoolAccount("C User", poolGroup("third_party", "Claude and GPT models", poolBucket("weekly", 0.5, 90*time.Hour))),
	}, "c@example.invalid")
	view := a.tuiAccountView(state, "c@example.invalid", poolNow)
	lines := a.tuiAccountBlock(view, state, 50, 2)
	if !strings.Contains(lines[1], "Gem ░░░░░░░░    —") {
		t.Fatalf("meter line = %q", lines[1])
	}
}

func TestAccountBlockForAnAccountWithoutQuotaYetSaysPending(t *testing.T) {
	a := rowsApp()
	state := rowsState(map[string]Account{"n@example.invalid": {"name": "New User"}}, "n@example.invalid")
	view := a.tuiAccountView(state, "n@example.invalid", poolNow)
	lines := a.tuiAccountBlock(view, state, 42, 2)
	if !strings.Contains(lines[0], "New User") || !strings.HasSuffix(strings.TrimRight(lines[0], " "), "…") || !strings.Contains(lines[1], "pending") {
		t.Fatalf("lines = %q", lines)
	}
}

func TestAccountRowsWindowAroundTheSelectionAndKeepWholeBlocks(t *testing.T) {
	a := rowsApp()
	accounts := map[string]Account{}
	var order []string
	for i := 0; i < 10; i++ {
		email := fmt.Sprintf("user%d@example.invalid", i)
		order = append(order, email)
		accounts[email] = namedPoolAccount(fmt.Sprintf("User %d", i),
			poolGroup("gemini", "Gemini models", poolBucket("weekly", 0.9, 100*time.Hour)))
	}
	state := rowsState(accounts, order...)
	state.selectedEmail = order[7]
	rows := a.tuiAccountRowsFor(state, 42, 6, 2)
	if len(rows) != 6 {
		t.Fatalf("rows = %d, want 6", len(rows))
	}
	text := strings.Join(rows, "\n")
	if !strings.Contains(text, "User 7") || strings.Contains(text, "User 0") {
		t.Fatalf("window does not follow the selection:\n%s", text)
	}
	one := a.tuiAccountRowsFor(state, 42, 6, 1)
	if len(one) != 6 {
		t.Fatalf("one-line rows = %d, want 6", len(one))
	}
}

func TestAccountBlockUsesShortLabelsAndBarsOnNarrowLists(t *testing.T) {
	a := rowsApp()
	state := rowsState(map[string]Account{
		"beta@example.invalid": namedPoolAccount("Beta User",
			poolGroup("gemini", "Gemini models", poolBucket("weekly", 0.36, 100*time.Hour)),
			poolGroup("third_party", "Claude and GPT models", poolBucket("weekly", 0, 22*time.Hour))),
	}, "beta@example.invalid")
	view := a.tuiAccountView(state, "beta@example.invalid", poolNow)
	lines := a.tuiAccountBlock(view, state, 38, 2)
	if !strings.Contains(lines[1], "G "+renderFractionalBar(0.36, 6, "", "", "")+"  36%") || !strings.Contains(lines[1], "C ") || strings.Contains(lines[1], "…") {
		t.Fatalf("narrow meter line = %q", lines[1])
	}
	if visibleWidth(lines[1]) != 38 {
		t.Fatalf("line is %d wide: %q", visibleWidth(lines[1]), lines[1])
	}
}
