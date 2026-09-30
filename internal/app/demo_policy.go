package app

import "errors"

var errDemoUnavailable = errors.New("Unavailable in demo") //nolint:staticcheck // shown verbatim in the TUI status line

// allowDemoAction is deny-by-default for the demo binary. Non-demo CLI use is unchanged.
func (a *Application) allowDemoAction(action string) error {
	if !a.demo {
		return nil
	}
	switch action {
	case "switch", "refresh", "profile-create", "profile-edit", "profile-remove", "tags", "alias", "settings", "settings-reset", "history-clear", "account-delete", "toggle-tier":
		return nil
	default:
		return errDemoUnavailable
	}
}
