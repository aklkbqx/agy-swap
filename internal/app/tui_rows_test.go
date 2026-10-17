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
	gem := "Gem " + renderMeter(meterHalf, 0.36, 13, "", "", "") + "  36%"
	cg := "C/G " + renderMeter(meterHalf, 0, 13, "", "", "") + "   0%"
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
	if !strings.Contains(lines[1], "Gem "+strings.Repeat("▂", 13)+"    —") {
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
	if !strings.Contains(lines[1], "G "+renderMeter(meterHalf, 0.36, 6, "", "", "")+"  36%") || !strings.Contains(lines[1], "C ") || strings.Contains(lines[1], "…") {
		t.Fatalf("narrow meter line = %q", lines[1])
	}
	if visibleWidth(lines[1]) != 38 {
		t.Fatalf("line is %d wide: %q", visibleWidth(lines[1]), lines[1])
	}
}

func TestAccountRowsMarkHiddenAccountsAboveAndBelow(t *testing.T) {
	a := rowsApp()
	state := newTUIState(sizesAccounts(30), "")
	for _, listLines := range []int{1, 2} {
		state.selectedEmail = "user15@example.invalid"
		rows := a.tuiAccountRowsFor(state, 50, 12, listLines)
		if len(rows) > 12 {
			t.Fatalf("lines=%d: %d rows, want at most 12", listLines, len(rows))
		}
		text := strings.Join(rows, "\n")
		if !strings.Contains(text, "User 15") || !strings.Contains(text, "▲") || !strings.Contains(text, "▼") {
			t.Fatalf("lines=%d: window lost the selection or a marker:\n%s", listLines, text)
		}
		var above, below, shown int
		_, _ = fmt.Sscanf(strings.TrimSpace(rows[0]), "▲ %d more", &above)
		_, _ = fmt.Sscanf(strings.TrimSpace(rows[len(rows)-1]), "▼ %d more", &below)
		for _, row := range rows {
			if strings.Contains(row, "User ") {
				shown++
			}
		}
		if above+shown+below != 30 {
			t.Fatalf("lines=%d: %d above + %d shown + %d below != 30:\n%s", listLines, above, shown, below, text)
		}
	}
	state.selectedEmail = "user00@example.invalid"
	top := strings.Join(a.tuiAccountRowsFor(state, 50, 12, 2), "\n")
	if strings.Contains(top, "▲") || !strings.Contains(top, "▼") {
		t.Fatalf("at the top only the lower marker belongs:\n%s", top)
	}
}

func attentionState() (*Application, *tuiState) {
	a := rowsApp()
	state := rowsState(map[string]Account{
		"ok@example.invalid": namedPoolAccount("Fine User",
			poolGroup("gemini", "Gemini models", poolBucket("weekly", 0.9, 100*time.Hour)),
			poolGroup("third_party", "Claude and GPT models", poolBucket("weekly", 0.8, 100*time.Hour))),
		"bad@example.invalid": namedPoolAccount("Broken User"),
	}, "ok@example.invalid", "bad@example.invalid")
	state.quotaErrors["bad@example.invalid"] = "secret vault entry is unavailable"
	return a, state
}

func TestAttentionDividerIsPreceededByABlankRow(t *testing.T) {
	a, state := attentionState()
	rows := a.tuiAccountRowsFor(state, 60, 12, 2)
	for i, row := range rows {
		if strings.Contains(row, "ATTENTION REQUIRED") {
			if i == 0 || strings.TrimSpace(rows[i-1]) != "" {
				t.Fatalf("no blank row before the divider:\n%s", strings.Join(rows, "\n"))
			}
			return
		}
	}
	t.Fatalf("no ATTENTION REQUIRED divider:\n%s", strings.Join(rows, "\n"))
}

func TestNoTokenRowsSayHowToFixIt(t *testing.T) {
	a, state := attentionState()
	text := strings.Join(a.tuiAccountRowsFor(state, 60, 12, 2), "\n")
	if !strings.Contains(text, "NO TOKEN") || !strings.Contains(text, "agy-swap add") {
		t.Fatalf("a NO TOKEN row should name the fix:\n%s", text)
	}
}

func TestWideListsNameThePoolsInFull(t *testing.T) {
	a, state := overviewState()
	view := a.tuiAccountView(state, "beta@example.invalid", poolNow)
	wide := a.tuiAccountBlock(view, state, 62, 2)[1]
	if !strings.Contains(wide, "Gemini ") || !strings.Contains(wide, "Claude/GPT ") {
		t.Fatalf("a 62-column row should use full pool names: %q", wide)
	}
	mid := a.tuiAccountBlock(view, state, 50, 2)[1]
	if !strings.Contains(mid, "Gem ") || !strings.Contains(mid, "C/G ") {
		t.Fatalf("a 50-column row keeps the short names: %q", mid)
	}
}

func TestWarningsAreYellowWarningsNotRedErrors(t *testing.T) {
	a, state := overviewState()
	state.message, state.messageType = "Usage refresh completed with 2 warning(s)", "warning"
	text := strings.Join(a.tuiStatusLines(state, 100), "\n")
	if !strings.Contains(text, "⚠ Usage refresh completed with 2 warning(s)") || strings.Contains(text, "✕") {
		t.Fatalf("status line = %q", text)
	}
}
