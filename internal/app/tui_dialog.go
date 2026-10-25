package app

import (
	"strings"
	"time"
)

// A dialog is every question the TUI asks, and its help screen: a title, a
// few lines of text and a row of buttons. ←/→, ↑/↓ and Tab move between the
// buttons, Enter presses the chosen one, a button's letter presses it directly
// and Esc cancels. The chosen button is drawn as [ Label ], so the choice
// reads without colour.

type tuiDialogButton struct {
	Label  string
	Key    string // shortcut letter, empty for none
	Action string // "confirm" or "cancel"
	Danger bool
}

type tuiDialog struct {
	Kind    string // what confirming does: "switch", "delete", "update", …
	Subject string // the account email or profile name it acts on
	Title   string
	Body    []string
	Buttons []tuiDialogButton
	Index   int
	// anyKeyCloses is for help, which only has a Close button.
	anyKeyCloses bool
}

// confirmDialog builds the question for an action. Actions that delete or
// reset something start on Cancel, so a stray Enter cannot destroy anything.
func confirmDialog(kind, subject, name string) tuiDialog {
	confirm := func(label string, danger bool) tuiDialogButton {
		return tuiDialogButton{Label: label, Key: "y", Action: "confirm", Danger: danger}
	}
	cancel := tuiDialogButton{Label: "Cancel", Key: "n", Action: "cancel"}
	d := tuiDialog{Kind: kind, Subject: subject}
	switch kind {
	case "switch":
		d.Title = "Switch to " + name
		d.Body = []string{"Antigravity will use " + subject + ".", "Running processes may need to reload credentials."}
		d.Buttons = []tuiDialogButton{confirm("Switch", false), cancel}
	case "update":
		d.Title = "Update agy-swap"
		d.Body = []string{"Download and install the latest release."}
		d.Buttons = []tuiDialogButton{confirm("Update", false), cancel}
	case "delete":
		d.Title = "Delete " + name
		d.Body = []string{"Remove " + subject + " from agy-swap?", "Its saved token and local quota data are deleted."}
		d.Buttons, d.Index = []tuiDialogButton{confirm("Delete", true), cancel}, 1
	case "profile-remove":
		d.Title = "Remove profile " + name
		d.Body = []string{"The profile is removed from local settings."}
		d.Buttons, d.Index = []tuiDialogButton{confirm("Remove", true), cancel}, 1
	case "history-clear":
		d.Title = "Clear local history"
		d.Body = []string{"Every saved switch and quota event is deleted."}
		d.Buttons, d.Index = []tuiDialogButton{confirm("Clear", true), cancel}, 1
	case "settings-reset":
		d.Title = "Reset settings"
		d.Body = []string{"Every setting goes back to its default.", "Aliases, tags, profiles, bindings and targets are removed."}
		d.Buttons, d.Index = []tuiDialogButton{confirm("Reset", true), cancel}, 1
	}
	return d
}

func (s *tuiState) openDialog(d tuiDialog) {
	s.dialog = &d
	s.mode = tuiDialogOpen
	s.modalOpenedAt = time.Now()
}

// openHelp shows the keyboard guide in the same dialog, closed by any key.
func (s *tuiState) openHelp(lines []string) {
	s.openDialog(tuiDialog{Kind: "help", Body: lines, Buttons: []tuiDialogButton{{Label: "Close", Action: "cancel"}}, anyKeyCloses: true})
}

// dialogKey applies a key to the open dialog. done reports that a button was
// pressed, which closes the dialog; kind and action say which one.
func (s *tuiState) dialogKey(key string) (kind, action string, done bool) {
	d := s.dialog
	if d == nil {
		s.mode = tuiBrowse
		return "", "cancel", true
	}
	press := func(button tuiDialogButton) (string, string, bool) {
		s.dialog, s.mode = nil, tuiBrowse
		return d.Kind, button.Action, true
	}
	if d.anyKeyCloses {
		return press(d.Buttons[d.Index])
	}
	switch key {
	case "left", "up", "shift-tab":
		d.Index = maxInt(0, d.Index-1)
	case "right", "down", "tab":
		d.Index = minInt(len(d.Buttons)-1, d.Index+1)
	case "enter":
		return press(d.Buttons[d.Index])
	case "esc":
		return press(tuiDialogButton{Action: "cancel"})
	default:
		for _, button := range d.Buttons {
			if button.Key != "" && button.Key == key {
				return press(button)
			}
		}
	}
	return "", "", false
}

// tuiDialogLines draws a dialog's content for tuiOverlay: the title, the body
// (its first line in full colour, the rest quieter) and the button row.
func (a *Application) tuiDialogLines(d *tuiDialog) []string {
	if d == nil {
		return nil
	}
	return a.tuiDialogLinesAt(d, float64(d.Index))
}

// tuiDialogLinesAt draws the dialog with its choice at chosen, a button index
// that is fractional while the choice slides from one button to the next.
func (a *Application) tuiDialogLinesAt(d *tuiDialog, chosen float64) []string {
	if d == nil {
		return nil
	}
	var lines []string
	if d.Title != "" {
		lines = append(lines, a.p.Bold+tuiText(d.Title)+a.p.Reset, "")
	}
	for i, line := range d.Body {
		if d.Kind != "help" && i > 0 {
			line = a.p.Gray + tuiText(line) + a.p.Reset
		}
		lines = append(lines, line)
	}
	if settled := float64(d.Index); chosen != settled && a.p.SelectBg != "" {
		return append(lines, "", a.tuiSlidingButtons(d, chosen), " ")
	}
	buttons := make([]string, 0, len(d.Buttons))
	for i, button := range d.Buttons {
		if i != d.Index {
			buttons = append(buttons, a.p.Gray+"  "+button.Label+"  "+a.p.Reset)
			continue
		}
		color := a.p.Orange
		if button.Danger {
			color = a.p.Red
		}
		buttons = append(buttons, a.p.SelectBg+color+a.p.Bold+"[ "+button.Label+" ]"+a.p.Reset)
	}
	return append(lines, "", strings.Join(buttons, "  "), " ")
}

// tuiSlidingButtons is the button row while the choice moves: every label
// plain, and the selection tint sliding between the two buttons it travels.
// A chosen chip and a plain label are the same width, so the labels stay put.
func (a *Application) tuiSlidingButtons(d *tuiDialog, chosen float64) string {
	starts, widths := make([]int, len(d.Buttons)), make([]int, len(d.Buttons))
	parts := make([]string, len(d.Buttons))
	col := 0
	for i, button := range d.Buttons {
		starts[i], widths[i] = col, len(button.Label)+4
		parts[i] = a.p.Gray + "  " + button.Label + "  " + a.p.Reset
		col += widths[i] + 2
	}
	lo := max(0, min(len(d.Buttons)-1, int(chosen)))
	hi := min(len(d.Buttons)-1, lo+1)
	frac := chosen - float64(lo)
	from := float64(starts[lo]) + float64(starts[hi]-starts[lo])*frac
	to := from + float64(widths[lo]) + float64(widths[hi]-widths[lo])*frac
	baseBg := a.p.BodyBg
	truecolor := baseBg != "" && lerpColor(baseBg, a.p.SelectBg, 0.5) != a.p.SelectBg
	return tintColumns(strings.Join(parts, "  "), func(c int) string {
		if truecolor {
			overlap := min(to, float64(c+1)) - max(from, float64(c))
			if overlap >= 0.99 {
				return a.p.SelectBg
			}
			if overlap > 0.01 {
				return lerpColor(baseBg, a.p.SelectBg, overlap)
			}
			return ""
		}
		if float64(c) >= from-0.5 && float64(c) < to-0.5 {
			return a.p.SelectBg
		}
		return ""
	})
}
