package app

import (
	"context"
	"fmt"
	"strings"
)

// cmdVerify opens Google's verification page for an account. Without a stored
// record it asks Google first, so the link is current.
func (a *Application) cmdVerify(ctx context.Context, args cliArgs) int {
	if args.account == "" {
		fmt.Fprintf(a.Err, "%sUsage: agy-swap verify <account>%s\n", a.p.Red, a.p.Reset)
		return 2
	}
	accounts, err := a.store.Load(true)
	if err != nil {
		return a.storeError(err)
	}
	settings, _ := a.loadSettings()
	email, err := resolveConfiguredTarget(args.account, accounts, settings)
	if err != nil {
		fmt.Fprintf(a.Err, "%s%s%s\n", a.p.Red, strings.TrimPrefix(err.Error(), errAmbiguous.Error()+": "), a.p.Reset)
		return 1
	}
	if email == "" {
		fmt.Fprintf(a.Err, "%sAccount '%s' not found.%s\n", a.p.Red, args.account, a.p.Reset)
		return 1
	}
	if a.quota != nil {
		stop := a.spinner("Checking Antigravity access...")
		failures := a.quota.refreshSelected(ctx, accounts, true, nil, email)
		stop()
		if _, ok := accountEligibility(accounts.ByEmail[email]); !ok {
			if failure := failures[email]; failure != "" {
				fmt.Fprintf(a.Err, "%sCould not check %s: %s%s\n", a.p.Red, email, failure, a.p.Reset)
				return 1
			}
			fmt.Fprintf(a.Out, "%s✓ %s can use Antigravity; no verification needed.%s\n", a.p.Green, email, a.p.Reset)
			return 0
		}
	}
	account := accounts.ByEmail[email]
	ineligible, ok := accountEligibility(account)
	if !ok {
		fmt.Fprintf(a.Out, "%s has no verification request saved. Run 'agy-swap limits --refresh' to check again.\n", email)
		return 1
	}
	fmt.Fprintf(a.Out, "%s⚠ %s needs Google verification before Antigravity can use it.%s\n", a.p.Yellow, email, a.p.Reset)
	a.printVerification(ineligible)
	if ineligible.URL == "" {
		return 1
	}
	message, opened := a.openVerification(ctx, account)
	fmt.Fprintln(a.Out, message)
	if !opened {
		return 1
	}
	fmt.Fprintf(a.Out, "%sWhen you are done, run 'agy-swap limits --refresh' to check again.%s\n", a.p.Gray, a.p.Reset)
	return 0
}

// checkNewAccount asks Google once whether a just-saved account can use
// Antigravity, so a verification request shows up now and not at first use.
func (a *Application) checkNewAccount(ctx context.Context, email string) (*eligibilityError, bool) {
	if a.quota == nil {
		return nil, false
	}
	accounts, err := a.store.Load(true)
	if err != nil {
		return nil, false
	}
	if _, ok := accounts.Get(email); !ok {
		return nil, false
	}
	stop := a.spinner("Checking Antigravity access...")
	a.quota.refreshSelected(ctx, accounts, true, nil, email)
	stop()
	account, _ := accounts.Get(email)
	ineligible, ok := accountEligibility(account)
	if !ok {
		return nil, false
	}
	if a.inTUISuspend {
		return ineligible, true
	}
	fmt.Fprintf(a.Out, "\n%s⚠ %s needs Google verification before Antigravity can use it.%s\n", a.p.Yellow, email, a.p.Reset)
	a.printVerification(ineligible)
	if ineligible.URL == "" || !a.stdinTTY {
		fmt.Fprintf(a.Out, "%sRun 'agy-swap verify %s' to open the page.%s\n", a.p.Gray, email, a.p.Reset)
		return ineligible, true
	}
	for {
		open, valid := parseYesNo(a.readLine(a.p.Cyan+"Open it in your browser now? [Y/n] "+a.p.Reset), true)
		if !valid {
			fmt.Fprintln(a.Out, "Please answer y or n.")
			continue
		}
		if open {
			message, opened := a.openVerification(ctx, account)
			fmt.Fprintln(a.Out, message)
			if opened {
				fmt.Fprintf(a.Out, "%sWhen you are done, refresh quota to check again.%s\n", a.p.Gray, a.p.Reset)
			}
		}
		return ineligible, true
	}
}

// printVerification writes Google's message and, on a line of its own with no
// color codes, the link, so a terminal copy gets the whole link.
func (a *Application) printVerification(ineligible *eligibilityError) {
	if ineligible.Message != "" {
		fmt.Fprintf(a.Out, "%s%s%s\n", a.p.Gray, ineligible.Message, a.p.Reset)
	}
	if ineligible.URL == "" {
		fmt.Fprintf(a.Out, "%sGoogle sent no link here. Run 'agy' to see it.%s\n", a.p.Gray, a.p.Reset)
		return
	}
	fmt.Fprintln(a.Out, ineligible.URL)
}
