package app

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

const githubSponsorsURL = "https://github.com/sponsors/aklkbqx"

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

func (a *Application) openExternalURL(ctx context.Context, link string) error {
	open := a.openURL
	if open == nil {
		open = openInBrowser
	}
	return open(ctx, link)
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
	if err := a.openExternalURL(ctx, ineligible.URL); err != nil {
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

func (a *Application) cmdDonate(ctx context.Context, opts extendedOptions, positional []string) int {
	if len(positional) > 1 {
		return a.extendedError("donate", opts, fmt.Errorf("usage: agy-swap donate [github] [--open] [--json]"))
	}
	if len(positional) == 1 {
		switch strings.ToLower(strings.TrimSpace(positional[0])) {
		case "github", "sponsors", "sponsor":
		case "open":
			opts.Open = true
		default:
			return a.extendedError("donate", opts, fmt.Errorf("unknown channel %q (expected github)", positional[0]))
		}
	}

	openedURL := ""
	if opts.Open {
		if err := a.openExternalURL(ctx, githubSponsorsURL); err != nil {
			return a.extendedError("donate", opts, fmt.Errorf("could not open browser: %w", err))
		}
		openedURL = githubSponsorsURL
	}

	if opts.JSON {
		return a.extendedResult("donate", opts, map[string]any{
			"url":             githubSponsorsURL,
			"github_sponsors": githubSponsorsURL,
			"opened":          openedURL,
			"channels": []map[string]string{
				{"id": "github", "name": "GitHub Sponsors", "url": githubSponsorsURL},
			},
		}, nil)
	}

	fmt.Fprintf(a.Out, "%sSupport agy-swap%s\n", a.p.Bold+a.p.Cyan, a.p.Reset)
	fmt.Fprintf(a.Out, "  GitHub Sponsors:  %s%s%s\n", a.p.Green, githubSponsorsURL, a.p.Reset)
	if opts.Open {
		fmt.Fprintf(a.Out, "\n%s✓ Opened GitHub Sponsors in your browser.%s\n", a.p.Green, a.p.Reset)
	} else {
		fmt.Fprintf(a.Out, "\n%sRun 'agy-swap donate --open' to open in your browser.%s\n", a.p.Gray, a.p.Reset)
	}
	return 0
}

func (a *Application) tuiOpenDonate(ctx context.Context, state *tuiState) {
	if err := a.openExternalURL(ctx, githubSponsorsURL); err != nil {
		if state != nil {
			state.message, state.messageType = "Could not open browser: "+err.Error()+" (press ? for link).", "warning"
		}
		return
	}
	if state != nil {
		state.message, state.messageType = "Opened GitHub Sponsors in your browser.", "success"
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
