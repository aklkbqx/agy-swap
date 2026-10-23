package app

import (
	"context"
	"os/exec"
	"runtime"
)

// openInBrowser hands the link to the system opener as one argument, so a
// long link reaches the browser whole: no shell, clipboard or line wrap.
func openInBrowser(_ context.Context, link string) error {
	switch runtime.GOOS {
	case "darwin":
		return startDetached(exec.Command("open", link))
	case "windows":
		return startDetached(exec.Command("rundll32", "url.dll,FileProtocolHandler", link))
	}
	return startDetached(exec.Command("xdg-open", link))
}

// openVerification opens Google's page for an account that needs it and
// returns what to tell the user.
func (a *Application) openVerification(ctx context.Context, account Account) (string, bool) {
	ineligible, ok := accountEligibility(account)
	if !ok {
		return "This account does not need verification.", false
	}
	if ineligible.URL == "" {
		return "Google sent no verification link. Run 'agy' to see it.", false
	}
	open := a.openURL
	if open == nil {
		open = openInBrowser
	}
	if err := open(ctx, ineligible.URL); err != nil {
		return "Could not open a browser: " + err.Error(), false
	}
	return "Opened Google's verification page in your browser.", true
}

// tuiVerifySelected opens the verification page of the selected account. The
// w key and the palette both use it.
func (a *Application) tuiVerifySelected(ctx context.Context, state *tuiState) {
	_, account, ok := state.selectedAccount()
	if !ok {
		state.message, state.messageType = "Select an account first.", "warning"
		return
	}
	message, opened := a.openVerification(ctx, account)
	state.message, state.messageType = message+" Press r to check again.", "warning"
	if opened {
		state.message, state.messageType = message+" Press r when you are done.", "success"
	}
}

// startDetached starts the opener and reaps it in the background: xdg-open can
// stay until the browser exits, and the page is open long before that.
func startDetached(command *exec.Cmd) error {
	if err := command.Start(); err != nil {
		return err
	}
	go func() { _ = command.Wait() }()
	return nil
}
