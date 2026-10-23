package app

import (
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestAnAccountThatNeedsVerificationSaysVerify(t *testing.T) {
	a := rowsApp()
	account := Account{"name": "New User", "eligibility": (&eligibilityError{Reason: "VALIDATION_REQUIRED", Message: "Verify", URL: "https://accounts.google.com/v"}).record(poolNow)}
	state := rowsState(map[string]Account{"n@example.invalid": account}, "n@example.invalid")
	state.quotaErrors = map[string]string{"n@example.invalid": "Google needs this account verified: Verify"}
	view := a.tuiAccountView(state, "n@example.invalid", poolNow)
	if view.status != statusVerify || view.status.String() != "VERIFY" {
		t.Fatalf("status = %v", view.status)
	}
	lines := a.tuiAccountBlock(view, state, 60, 2)
	if !strings.Contains(lines[0], "VERIFY") || !strings.Contains(lines[1], "press w") {
		t.Fatalf("lines = %q", lines)
	}
	detail := strings.Join(a.tuiGaugeDetail(state, 70, 12, tuiLayoutPlanFor(120, 40)), "\n")
	if strings.Contains(detail, "re-auth") || strings.Contains(detail, "agy-swap add") || strings.Contains(detail, "https://") || strings.Contains(detail, "Usage unavailable") {
		t.Fatalf("detail misleads or prints the link:\n%s", detail)
	}
	if !strings.Contains(detail, "Verify") || !strings.Contains(detail, "[w]") {
		t.Fatalf("detail = %s", detail)
	}
}

func TestVerifyDetailWithoutALinkPointsAtAgy(t *testing.T) {
	a := rowsApp()
	account := Account{"name": "New User", "eligibility": (&eligibilityError{Message: "Verify"}).record(poolNow)}
	state := rowsState(map[string]Account{"n@example.invalid": account}, "n@example.invalid")
	detail := strings.Join(a.tuiGaugeDetail(state, 70, 12, tuiLayoutPlanFor(120, 40)), "\n")
	if strings.Contains(detail, "[w]") || !strings.Contains(detail, "Run 'agy'") {
		t.Fatalf("detail = %s", detail)
	}
}

func TestOpenVerificationPassesTheWholeLinkToTheOpener(t *testing.T) {
	var opened []string
	a := rowsApp()
	a.openURL = func(_ context.Context, link string) error { opened = append(opened, link); return nil }
	long := "https://accounts.google.com/v?" + strings.Repeat("x", 600)
	account := Account{"eligibility": (&eligibilityError{Message: "Verify", URL: long}).record(poolNow)}
	if _, ok := a.openVerification(context.Background(), account); !ok || len(opened) != 1 || opened[0] != long {
		t.Fatalf("opened = %d links", len(opened))
	}
	noLink := Account{"eligibility": (&eligibilityError{Message: "Verify"}).record(poolNow)}
	if message, ok := a.openVerification(context.Background(), noLink); ok || !strings.Contains(message, "agy") || len(opened) != 1 {
		t.Fatalf("message = %q", message)
	}
	if message, ok := a.openVerification(context.Background(), Account{}); ok || !strings.Contains(message, "does not need") {
		t.Fatalf("message = %q", message)
	}
}

func TestDetailTableForAnAccountThatNeedsVerificationDoesNotAskForReauth(t *testing.T) {
	a := rowsApp()
	account := Account{"name": "New User", "eligibility": (&eligibilityError{Message: "Verify", URL: "https://accounts.google.com/v"}).record(poolNow)}
	state := rowsState(map[string]Account{"n@example.invalid": account}, "n@example.invalid")
	state.quotaErrors = map[string]string{"n@example.invalid": "Google needs this account verified: Verify"}
	detail := strings.Join(a.tuiDetailTableLines(state, 70, 12), "\n")
	if strings.Contains(detail, "Re-auth") || strings.Contains(detail, "agy-swap add") || strings.Contains(detail, "https://") || !strings.Contains(detail, "[w]") {
		t.Fatalf("detail = %s", detail)
	}
}

func TestVerifyActionOpensThePageForTheSelectedAccount(t *testing.T) {
	a := rowsApp()
	var opened string
	a.openURL = func(_ context.Context, link string) error { opened = link; return nil }
	account := Account{"name": "New User", "eligibility": (&eligibilityError{Message: "Verify", URL: "https://accounts.google.com/v"}).record(poolNow)}
	state := rowsState(map[string]Account{"n@example.invalid": account}, "n@example.invalid")
	enabled := false
	for _, action := range tuiActions(state) {
		if action.ID == "verify" {
			enabled = action.Enabled && action.Shortcut == "w"
		}
	}
	if !enabled {
		t.Fatal("the palette has no enabled verify action for an account that needs it")
	}
	a.tuiVerifySelected(context.Background(), state)
	if opened != "https://accounts.google.com/v" || state.messageType != "success" {
		t.Fatalf("opened=%q message=%q (%s)", opened, state.message, state.messageType)
	}
	ready := rowsState(map[string]Account{"r@example.invalid": {"name": "Ready"}}, "r@example.invalid")
	for _, action := range tuiActions(ready) {
		if action.ID == "verify" && action.Enabled {
			t.Fatal("verify is enabled for an account that does not need it")
		}
	}
	a.tuiVerifySelected(context.Background(), ready)
	if ready.messageType != "warning" {
		t.Fatalf("message=%q (%s)", ready.message, ready.messageType)
	}
}

func TestHelpListsTheVerificationKey(t *testing.T) {
	if text := strings.Join(rowsApp().tuiHelpLines(80), "\n"); !strings.Contains(text, "w           Open Google's verification page") {
		t.Fatalf("help = %s", text)
	}
}

func TestOtherFailuresStayVisibleUnderVerify(t *testing.T) {
	a := rowsApp()
	account := Account{"name": "New User", "eligibility": (&eligibilityError{Message: "Verify", URL: "https://accounts.google.com/v"}).record(poolNow)}
	state := rowsState(map[string]Account{"n@example.invalid": account}, "n@example.invalid")
	state.quotaErrors = map[string]string{"n@example.invalid": "loadCodeAssist failed (HTTP 503)"}
	detail := strings.Join(a.tuiGaugeDetail(state, 70, 12, tuiLayoutPlanFor(120, 40)), "\n")
	if !strings.Contains(detail, "HTTP 503") {
		t.Fatalf("detail hides the current failure:\n%s", detail)
	}
	table := strings.Join(a.tuiDetailTableLines(state, 110, 12), "\n")
	if !strings.Contains(table, "HTTP 503") {
		t.Fatalf("quota view hides the current failure:\n%s", table)
	}
}

func TestStartDetachedDoesNotWaitForTheBrowser(t *testing.T) {
	start := time.Now()
	if err := startDetached(exec.Command("sleep", "3")); err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("waited %s for the opener", elapsed)
	}
	if err := startDetached(exec.Command("agy-swap-no-such-opener")); err == nil {
		t.Fatal("a missing opener must be reported")
	}
}

func TestNoTokenWinsOverVerify(t *testing.T) {
	a := rowsApp()
	account := Account{"name": "New User", "eligibility": (&eligibilityError{Message: "Verify", URL: "https://accounts.google.com/v"}).record(poolNow)}
	state := rowsState(map[string]Account{"n@example.invalid": account}, "n@example.invalid")
	state.quotaErrors = map[string]string{"n@example.invalid": "refresh token is unavailable"}
	if view := a.tuiAccountView(state, "n@example.invalid", poolNow); view.status != statusNoToken {
		t.Fatalf("status = %v", view.status)
	}
}

func TestVerifyAccountStartsInAttentionPartitionBeforeRefresh(t *testing.T) {
	accounts := NewAccounts()
	accounts.Set("v@example.invalid", Account{
		"email":       "v@example.invalid",
		"name":        "Verify First",
		"eligibility": (&eligibilityError{Reason: "VALIDATION_REQUIRED", Message: "Verify", URL: "https://accounts.google.com/v"}).record(poolNow),
	})
	accounts.Set("r@example.invalid", Account{
		"email": "r@example.invalid",
		"name":  "Ready Second",
	})
	state := newTUIState(accounts, "")
	// Frame 0: quotaErrors is still empty before background refresh finishes.
	ready, attention := state.partitionedEmails()
	if len(ready) != 1 || ready[0] != "r@example.invalid" || len(attention) != 1 || attention[0] != "v@example.invalid" {
		t.Fatalf("expected verify account in attention from frame 0, got ready=%v attention=%v", ready, attention)
	}
}

func TestVerifyRowHidesPressWWithoutLinkAndSuppressesStaleCompactMeters(t *testing.T) {
	a := rowsApp()
	staleAccount := Account{
		"name": "Stale Verify",
		"quota_snapshot": map[string]any{
			"observed_at": isoTime(poolNow),
			"tier":        map[string]any{"id": "free-tier", "name": "Free"},
			"groups": []any{
				map[string]any{
					"id": "gemini", "name": "Gemini",
					"buckets": []any{map[string]any{"id": "gemini-weekly", "name": "Weekly", "window": "weekly", "remaining_fraction": 0.9, "reset_at": "2030-01-01T00:00:00Z"}},
				},
			},
		},
		"eligibility": (&eligibilityError{Message: "Verify"}).record(poolNow),
	}
	state := rowsState(map[string]Account{"n@example.invalid": staleAccount}, "n@example.invalid")
	view := a.tuiAccountView(state, "n@example.invalid", poolNow)

	twoLine := a.tuiAccountBlock(view, state, 60, 2)
	if strings.Contains(twoLine[1], "press w") || !strings.Contains(twoLine[1], "run agy") {
		t.Fatalf("row without URL must not tell user to press w: %q", twoLine)
	}
	oneLine := a.tuiAccountBlock(view, state, 60, 1)
	if strings.Contains(oneLine[0], "90%") {
		t.Fatalf("single-line VERIFY row must not draw stale pool meters: %q", oneLine[0])
	}
}

func TestDashboardFooterShowsVerifyKeycapWhenSelectedAccountHasLink(t *testing.T) {
	a := rowsApp()
	withLink := Account{"name": "New User", "eligibility": (&eligibilityError{Message: "Verify", URL: "https://accounts.google.com/v"}).record(poolNow)}
	state := rowsState(map[string]Account{"n@example.invalid": withLink}, "n@example.invalid")
	state.view = tuiViewDashboard
	footer := strings.Join(a.tuiFooterLines(state, 120), "\n")
	if !strings.Contains(footer, "[w]") || !strings.Contains(footer, "Verify") {
		t.Fatalf("expected [w] Verify in footer:\n%s", footer)
	}
}
