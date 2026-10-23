package app

import (
	"context"
	"strings"
)

// beginSwitchConfirm opens the switch confirmation for the selected account.
// The account Antigravity already uses gets a message instead of a question.
func (s *tuiState) beginSwitchConfirm() bool {
	email, account, ok := s.selectedAccount()
	if !ok {
		return false
	}
	if strings.EqualFold(email, s.active) {
		s.message, s.messageType = "Already using "+email, "info"
		return false
	}
	s.openDialog(confirmDialog("switch", email, tuiText(firstString(account["name"], email))))
	return true
}

// tuiSwitchSelected points the shared Antigravity session at the selected
// account and returns the toast to show. It reads nothing from the terminal,
// so the dashboard stays on screen while it runs.
func (a *Application) tuiSwitchSelected(ctx context.Context, state *tuiState) (message, kind string) {
	email, account, ok := state.selectedAccount()
	if !ok {
		return "", ""
	}
	token, err := a.accountToken(ctx, account)
	if err != nil {
		return "Switch failed: " + err.Error(), "error"
	}
	alreadyUsing := a.credentials.Current(ctx) == token
	if !a.applyAccount(ctx, token, email) {
		return "Could not switch to " + email, "error"
	}
	if ineligible, verify := accountEligibility(account); verify {
		hint := ""
		if ineligible.URL != "" {
			hint = " · press w"
		}
		return "Switched to " + email + " · needs Google verification" + hint, "warning"
	}
	if alreadyUsing {
		return "Already using " + email, "info"
	}
	return "Switched to " + email, "success"
}
