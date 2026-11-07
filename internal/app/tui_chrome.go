package app

import (
	"os"
	"strings"
	"unicode/utf8"

	"golang.org/x/term"
)

func (a *Application) tuiStatusLines(state *tuiState, width int) []string {
	g := newTUIGeometry(width, 12)
	content := ""
	switch {
	case state.mode == tuiSearch:
		caret := "▌"
		if state.motionEnabled && a.renderTime().UnixMilli()/tuiCaretBlink.Milliseconds()%2 == 1 {
			caret = " "
		}
		content = a.p.Cyan + "/" + cleanText(state.search) + caret + a.p.Reset
	case state.mode == tuiDialogOpen && state.dialog != nil && state.dialog.Kind != "help":
		content = a.p.Yellow + cleanText(state.dialog.Title) + "?" + a.p.Reset + a.p.Gray + "  ←→ choose · Enter select · Esc cancel" + a.p.Reset
	case state.mode == tuiPalette:
		content = a.p.Cyan + "Action palette · type to filter · Enter to run" + a.p.Reset
	case state.mode == tuiForm:
		content = a.p.Cyan + "Editing · choose a field, then Enter to save" + a.p.Reset
	case state.job != nil && !state.job.Done:
		content = a.p.Cyan + "◐ " + cleanText(state.job.Label) + "…" + a.p.Reset
	case state.refreshing:
		content = a.p.Cyan + "◐ Syncing usage…" + a.p.Reset
	case state.resolvingToken != "":
		content = a.p.Cyan + "⌁ Resolving active session…" + a.p.Reset
	case state.message != "":
		prefix, color := "› ", a.p.Cyan
		switch state.messageType {
		case "success":
			prefix, color = "✓ ", a.p.Green
		case "error":
			prefix, color = "✕ ", a.p.Red
		case "warning":
			prefix, color = "⚠ ", a.p.Yellow
		}
		content = color + prefix + cleanText(state.message) + a.p.Reset
	case state.accounts == nil || state.accounts.Len() == 0:
		content = a.p.Yellow + "No accounts yet · press a to add your first account" + a.p.Reset
	case state.view != tuiViewDashboard:
		content = a.p.Gray + tuiViewHint(state.view) + a.p.Reset
	default:
		content = a.p.Gray + "Ready · select an account to inspect health" + a.p.Reset
	}
	return []string{
		a.barRow("", g),
		a.barRow(content, g),
	}
}

// tuiViewHint is the status line of a screen other than the dashboard.
func tuiViewHint(view tuiView) string {
	switch view {
	case tuiViewProfiles:
		return "A profile picks the account for a job · c creates one"
	case tuiViewHistory:
		return "Switches and refreshes stay on this machine"
	case tuiViewSettings:
		return "Policy, notifications and integrations · Enter edits"
	case tuiViewDoctor:
		return "A read-only check of this installation · Enter runs it again"
	case tuiViewBackup:
		return "Export or import your accounts · x, i and v"
	}
	return "Ready"
}

func (a *Application) tuiToastOverlay(base []string, state *tuiState, width, height int) []string {
	g := newTUIGeometry(width, height)
	now := a.renderTime()
	if !state.toastActive(now) {
		return fitFrameLines(base, g, a.p)
	}

	color, icon := a.p.Cyan, "›"
	switch state.toastType {
	case "success":
		color, icon = a.p.Green, "✓"
	case "error":
		color, icon = a.p.Red, "✕"
	}
	plain := icon + " " + tuiText(state.toast)
	boxWidth := minInt(maxInt(24, visibleWidth(plain)+4), maxInt(18, g.frameWidth-4))
	innerWidth := maxInt(1, boxWidth-2)
	top := color + "╭" + strings.Repeat("─", innerWidth) + "╮" + a.p.Reset
	body := color + "│" + a.p.Reset + fitVisible(" "+color+plain+a.p.Reset, innerWidth, a.p) + color + "│" + a.p.Reset
	// The bottom edge counts down: the part still lit is the time left.
	lit := innerWidth
	if !state.toastUntil.IsZero() {
		left := float64(state.toastUntil.Sub(now)) / float64(tuiToastDuration)
		lit = int(float64(innerWidth)*max(0, min(1, left)) + 0.5)
	}
	bottom := color + "╰" + strings.Repeat("─", lit)
	if lit < innerWidth {
		bottom += a.p.Track + strings.Repeat("─", innerWidth-lit) + color
	}
	bottom += "╯" + a.p.Reset
	box := []string{top, body, bottom}
	base = fitFrameLines(base, g, a.p)
	// Sit above the status divider so the box never merges with a frame rule;
	// a new toast rises two rows into place.
	rise := 1 - easeOutCubic(state.fxProgress(state.toastShownAt, tuiMotionToast, now))
	rowStart := maxInt(1, g.frameHeight-len(box)-5) + int(2*rise+0.5)
	left := maxInt(1, g.frameWidth-boxWidth-2)
	for index, line := range box {
		row := rowStart + index
		if row >= 0 && row < len(base) {
			base[row] = overlayVisible(base[row], line, left, a.p)
		}
	}
	return base
}

// formatKeycaps draws key hints. With colour a key is a chip, " Enter " on
// the key colour, the same width as the "[Enter]" drawn without colour.
// tuiFlashKey lights the chip of the key just pressed, for a moment, as the
// key's feedback.
func (a *Application) tuiFlashKey(state *tuiState, row string) string {
	if a.p.KeyBg == "" || state.fxProgress(state.keyFlashAt, tuiMotionFlash, a.renderTime()) >= 1 {
		return row
	}
	label := map[string]string{
		"enter": "Enter", "up": "↑↓", "down": "↑↓", "k": "↑↓", "j": "↑↓", "ctrl-k": "^K", ":": "^K",
		"?": "?", "q": "q", "w": "w", "b": "b", "c": "c", "x": "x", "i": "i", "v": "v", "e": "e",
		"a": "a", "t": "t", "d": "d", "[": "[ ]", "]": "[ ]",
	}[state.keyFlash]
	if label == "" {
		return row
	}
	chip := a.p.KeyBg + a.p.Bold + a.p.White + " " + label + " " + a.p.Reset
	lit := tuiAsBackground(a.p.Orange) + a.p.Bold + tuiAsForeground(a.p.BodyBg) + " " + label + " " + a.p.Reset
	return strings.Replace(row, chip, lit, 1)
}

// tuiAsBackground turns a foreground colour code into the same colour as a
// background, and tuiAsForeground the other way; black when there is none.
func tuiAsBackground(fg string) string {
	switch {
	case strings.HasPrefix(fg, "\x1b[38;"):
		return "\x1b[48;" + strings.TrimPrefix(fg, "\x1b[38;")
	case len(fg) == 5 && strings.HasPrefix(fg, "\x1b[3"):
		return "\x1b[4" + fg[3:]
	}
	return fg
}

func tuiAsForeground(bg string) string {
	if strings.HasPrefix(bg, "\x1b[48;") {
		return "\x1b[38;" + strings.TrimPrefix(bg, "\x1b[48;")
	}
	return "\x1b[30m"
}

func formatKeycaps(p palette, pairs ...string) string {
	parts := make([]string, 0, len(pairs)/2)
	for i := 0; i+1 < len(pairs); i += 2 {
		key := p.White + pairs[i] + p.Reset
		if p.KeyBg != "" {
			inner := strings.TrimSuffix(strings.TrimPrefix(pairs[i], "["), "]")
			if inner == "[]/[]" {
				inner = "[ ]"
			}
			key = p.KeyBg + p.Bold + p.White + " " + inner + " " + p.Reset
		}
		parts = append(parts, key+" "+p.Gray+pairs[i+1]+p.Reset)
	}
	return strings.Join(parts, "   ")
}

func (a *Application) tuiFooterLines(state *tuiState, width int) []string {
	g := newTUIGeometry(width, 12)
	canVerify := false
	if state != nil && state.accounts != nil {
		if _, account, ok := state.selectedAccount(); ok {
			if ineligible, ok := accountEligibility(account); ok && ineligible.URL != "" {
				canVerify = true
			}
		}
	}
	hints := []keyHint{{"[↑↓]", "Move", 4}, {"[Enter]", "Switch", 5}, {"[^K]", "Actions", 2}, {"[?]", "Help", 3}, {"[q]", "Quit", 4}}
	if canVerify {
		hints = []keyHint{{"[↑↓]", "Move", 4}, {"[Enter]", "Switch", 5}, {"[w]", "Verify", 5}, {"[^K]", "Actions", 2}, {"[?]", "Help", 3}, {"[q]", "Quit", 4}}
	}
	if state.view == tuiViewDashboard && g.layout == tuiLayoutWide {
		hints = []keyHint{{"[↑↓]", "Move", 4}, {"[Enter]", "Switch", 5}, {"[[]/[]]", "Resize", 1}, {"[^K]", "Actions", 2}, {"[?]", "Help", 3}, {"[q]", "Quit", 4}}
		if canVerify {
			hints = []keyHint{{"[↑↓]", "Move", 4}, {"[Enter]", "Switch", 5}, {"[w]", "Verify", 5}, {"[[]/[]]", "Resize", 1}, {"[^K]", "Actions", 2}, {"[?]", "Help", 3}, {"[q]", "Quit", 4}}
		}
	}
	footer := a.fitKeycaps(hints, g.contentWidth)
	if state.view != tuiViewDashboard {
		footer = formatKeycaps(a.p, "[↑↓]", "Navigate", "[Enter]", "Edit", "[b]", "Dashboard", "[^K]", "Actions", "[?]", "Help")
		switch state.view {
		case tuiViewBackup:
			footer = formatKeycaps(a.p, "[x]", "Export", "[i]", "Import", "[v]", "Verify", "[b]", "Dashboard", "[^K]", "Actions")
		case tuiViewHistory:
			footer = formatKeycaps(a.p, "[c]", "Clear", "[x]", "Export", "[b]", "Dashboard", "[^K]", "Actions")
		case tuiViewSettings:
			footer = formatKeycaps(a.p, "[e]", "Edit", "[a]", "Alias", "[b]", "Binding", "[t]", "Target", "[^K]", "Actions")
		case tuiViewDoctor:
			footer = formatKeycaps(a.p, "[Enter]", "Run again", "[b]", "Dashboard", "[^K]", "Actions")
		case tuiViewProfiles:
			footer = formatKeycaps(a.p, "[c]", "Create", "[Enter]", "Edit", "[d]", "Delete", "[b]", "Dashboard", "[^K]", "Actions")
		}
	}
	switch state.mode {
	case tuiSearch:
		footer = formatKeycaps(a.p, "[Type]", "Filter", "[Enter]", "Apply", "[Esc]", "Cancel", "[Bksp]", "Erase")
	case tuiDialogOpen:
		if state.dialog != nil && state.dialog.Kind == "help" {
			footer = a.p.White + "[Any key]" + a.p.Reset + " " + a.p.Gray + "Close help" + a.p.Reset
		} else {
			footer = formatKeycaps(a.p, "[←→]", "Choose", "[Enter]", "Select", "[y/n]", "Shortcut", "[Esc]", "Cancel")
		}
	case tuiPalette:
		footer = formatKeycaps(a.p, "[↑↓]", "Move", "[Enter]", "Run", "[Type]", "Filter", "[Esc]", "Close")
	case tuiForm:
		footer = formatKeycaps(a.p, "[↑↓]", "Field", "[←→]", "Choice", "[Enter]", "Next/Save", "[Esc]", "Cancel")
	}
	return []string{
		a.barRow("", g),
		a.barRow(a.tuiFlashKey(state, footer), g),
		a.barRow("", g),
	}
}

// keyHint is one footer key. A higher keep value survives narrower screens.
type keyHint struct {
	key, label string
	keep       int
}

// fitKeycaps draws the hints in order. When the row is too wide it gives up
// the least useful things first: the labels and then the keys of the
// lowest-priority hints, then the labels of the more important ones.
func (a *Application) fitKeycaps(hints []keyHint, width int) string {
	list := append([]keyHint(nil), hints...)
	render := func() string {
		pairs := make([]string, 0, len(list)*2)
		for _, hint := range list {
			pairs = append(pairs, hint.key, hint.label)
		}
		return strings.TrimRight(formatKeycaps(a.p, pairs...), " ")
	}
	fits := func() bool { return visibleWidth(render()) <= width }
	for level := 1; level <= 5 && !fits(); level++ {
		if level <= 2 {
			// A bare key such as [^K] says nothing, so these go whole.
			kept := list[:0]
			for _, hint := range list {
				if hint.keep > level {
					kept = append(kept, hint)
				}
			}
			list = kept
			continue
		}
		for i := range list {
			if list[i].keep == level {
				list[i].label = ""
			}
		}
	}
	for !fits() && len(list) > 1 {
		lowest := 0
		for i, hint := range list {
			if hint.keep < list[lowest].keep {
				lowest = i
			}
		}
		list = append(list[:lowest], list[lowest+1:]...)
	}
	return render()
}

func (a *Application) tuiHelpLines(width int) []string {
	lines := []string{
		tuiWordmark(a.p) + a.p.Gray + "  v" + tuiText(stateVersion(a.Version)) + "  ·  by " + strings.TrimPrefix(tuiCredit, "credit: ") + "  ·  MIT" + a.p.Reset,
		a.p.Gray + "github.com/aklkbqx/agy-swap" + a.p.Reset,
		a.p.Gray + "Independent, not affiliated with Google." + a.p.Reset,
		"",
		a.p.Bold + "KEYBOARD GUIDE" + a.p.Reset,
		"↑ ↓ / j k   Move through accounts",
		"1 – 9       Select an account by number",
		"Enter       Switch selected account",
		"/           Search by name or email",
		"Ctrl-K / :  Open action palette",
		"p h s o b   Profiles, history, settings, doctor, backup",
		"r           Refresh quota",
		"w           Open Google's verification page (VERIFY)",
		"n           Refresh, then pick the next account",
		"a           Add account        d  Delete account",
		"t           Toggle manual tier  l  Log out",
		"m           Move tokens into the vault",
		"e           Edit tags / selected item",
		"x i v c     In managers: export, import, verify, create/clear",
		"[ ] / + -   Resize the accounts split (drag, = resets)",
		"Mouse       Click, drag and scroll; hover shows what a click does",
		"u           Download and install the latest release",
		"b           Return to dashboard",
		"Esc / any   Close help",
		"",
		a.p.Bold + "SUPPORT" + a.p.Reset,
		"Sponsor     github.com/sponsors/aklkbqx",
	}
	for i := range lines {
		lines[i] = truncateVisible(lines[i], maxInt(18, width-8), a.p)
	}
	return lines
}

// tuiOverlayOpening draws a dialog box over base. While it opens (open < 1)
// only its frame and the middle of its content show, centred where the whole
// box will sit, so it grows from its centre line.
// tuiOverlayBox is where a dialog of n content lines sits once open: its top
// border's frame line, its left border's column and its content width. Line
// k of the content is at frame line top+1+k, from column left+2.
func tuiOverlayBox(g tuiGeometry, n int) (top, left, contentWidth int) {
	contentWidth = minInt(64, maxInt(18, g.frameWidth-10))
	n = minInt(n, maxInt(1, g.frameHeight-4))
	top = maxInt(1, (g.frameHeight-(n+2))/2)
	left = maxInt(0, (g.frameWidth-(contentWidth+2))/2)
	return top, left, contentWidth
}

func (a *Application) tuiOverlayOpening(base, overlay []string, width, height int, open float64) []string {
	g := newTUIGeometry(width, height)
	base = fitFrameLines(base, g, a.p)
	if len(overlay) == 0 {
		return base
	}
	_, _, contentWidth := tuiOverlayBox(g, len(overlay))
	maxContent := maxInt(1, g.frameHeight-4)
	if len(overlay) > maxContent {
		overlay = append([]string(nil), overlay[:maxContent]...)
		overlay[len(overlay)-1] = a.p.Gray + "… the window is too short for every key" + a.p.Reset
	}
	dialog := make([]string, 0, len(overlay)+2)
	dialog = append(dialog, a.p.Blue+"╭"+strings.Repeat("─", contentWidth)+"╮"+a.p.Reset)
	for _, line := range overlay {
		content := fitVisible(" "+line, contentWidth, a.p)
		dialog = append(dialog, a.p.Blue+"│"+a.p.Reset+content+a.p.Blue+"│"+a.p.Reset)
	}
	dialog = append(dialog, a.p.Blue+"╰"+strings.Repeat("─", contentWidth)+"╯"+a.p.Reset)
	start := maxInt(1, (g.frameHeight-len(dialog))/2)
	if open < 1 && len(dialog) > 2 {
		inner := dialog[1 : len(dialog)-1]
		show := int(float64(len(inner))*open + 0.5)
		from := (len(inner) - show) / 2
		start += from
		dialog = append(append([]string{dialog[0]}, inner[from:from+show]...), dialog[len(dialog)-1])
	}
	for i, line := range dialog {
		row := start + i
		if row >= 0 && row < len(base) {
			// The backdrop carries on either side of the dialog.
			left := maxInt(0, (g.frameWidth-visibleWidth(line))/2)
			base[row] = overlayVisible(base[row], line, left, a.p)
		}
	}
	return fitFrameLines(base, g, a.p)
}

// tuiBackdrop redraws rows [0, end) in the backdrop colour, dropping their own
// colours, so the modal drawn over them is the only thing in full colour.
// Without colour there is nothing to dim and the rows are kept.
func (a *Application) tuiBackdrop(lines []string, end int, color string) []string {
	if a.p.Backdrop == "" {
		return lines
	}
	for i := 0; i < end && i < len(lines); i++ {
		lines[i] = color + stripEscapes(lines[i]) + a.p.Reset
	}
	return lines
}

func oneOfMode(mode tuiMode, modes ...tuiMode) bool {
	for _, m := range modes {
		if mode == m {
			return true
		}
	}
	return false
}

// barRow is a frame row drawn as part of a solid bar: the header and the
// footer. Every reset inside it hands back to the bar colour, so colours in
// the content do not punch holes in the bar.
func (a *Application) barRow(content string, g tuiGeometry) string {
	inner := "  " + fitVisible(content, g.contentWidth, a.p) + "  "
	if a.p.BarBg == "" {
		return inner
	}
	return a.p.BarBg + strings.ReplaceAll(inner, a.p.Reset, a.p.Reset+a.p.BarBg) + a.p.Reset
}

// tuiSeparator is the quiet line between the list and the gauges when they
// are stacked, the horizontal form of the middle divider.
func (a *Application) tuiSeparator(g tuiGeometry) string {
	return "  " + a.p.Track + strings.Repeat("─", g.contentWidth) + a.p.Reset + "  "
}

func frameRow(content string, g tuiGeometry, p palette) string {
	return "  " + fitVisible(content, g.contentWidth, p) + "  "
}

func panelRow(left, right string, g tuiGeometry, p palette) string {
	// The one line left on screen: a quiet divider between list and gauges.
	return "  " + fitVisible(left, g.leftWidth, p) + "  " + p.Track + "│" + p.Reset + "  " + fitVisible(right, g.rightWidth, p) + "  "
}

func panelDivider(g tuiGeometry, p palette) string {
	return panelRow("", "", g, p)
}

func fitBodyLines(lines []string, g tuiGeometry, p palette) []string {
	result := make([]string, 0, g.bodyRows)
	for _, line := range lines {
		if len(result) >= g.bodyRows {
			break
		}
		result = append(result, fitVisible(line, g.frameWidth, p))
	}
	for len(result) < g.bodyRows {
		result = append(result, frameRow("", g, p))
	}
	return result
}

func fitFrameLines(lines []string, g tuiGeometry, p palette) []string {
	result := make([]string, 0, g.frameHeight)
	for _, line := range lines {
		if len(result) >= g.frameHeight {
			break
		}
		result = append(result, fitVisible(line, g.frameWidth, p))
	}
	for len(result) < g.frameHeight {
		result = append(result, frameRow("", g, p))
	}
	return result
}

func fitVisible(value string, width int, p palette) string {
	width = maxInt(0, width)
	if width == 0 {
		return ""
	}
	valueWidth := visibleWidth(value)
	if valueWidth > width {
		return truncateVisible(value, width, p)
	}
	return value + strings.Repeat(" ", width-valueWidth)
}

// overlayVisible draws overlay over base from visible column left. The base
// columns on both sides stay in place, so a toast or dialog never erases the
// frame border beside it.
func overlayVisible(base, overlay string, left int, p palette) string {
	width := visibleWidth(base)
	left = maxInt(0, left)
	if left >= width {
		return base
	}
	overlay = fitVisible(overlay, minInt(visibleWidth(overlay), width-left), p)
	right := left + visibleWidth(overlay)
	return visibleSpan(base, 0, left) + p.Reset + overlay + p.Reset + visibleSpan(base, right, width)
}

// visibleSpan returns columns [from, to) of s. Every escape sequence is kept,
// so the span starts and ends with the colors of the original row. A wide rune
// cut by either edge becomes spaces to preserve the column count.
func visibleSpan(s string, from, to int) string {
	var b strings.Builder
	col := 0
	for len(s) > 0 {
		if s[0] == '\x1b' {
			if size := ansiPrefixLen(s); size > 0 {
				b.WriteString(s[:size])
				s = s[size:]
				continue
			}
		}
		r, size := utf8.DecodeRuneInString(s)
		runeWidth := terminalRuneWidth(r)
		if col >= from && col+runeWidth <= to {
			b.WriteString(s[:size])
		} else if col < to && col+runeWidth > from {
			b.WriteString(strings.Repeat(" ", minInt(col+runeWidth, to)-maxInt(col, from)))
		}
		col += runeWidth
		s = s[size:]
	}
	return b.String()
}

func stateVersion(version string) string {
	return strings.TrimPrefix(version, "v")
}

func ioWriteString(writer interface{ Write([]byte) (int, error) }, value string) (int, error) {
	return writer.Write([]byte(value))
}

func termGetSize(file *os.File) (int, int, error) {
	return term.GetSize(int(file.Fd()))
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// tuiFooterHits records the key hints on the footer's middle line, each
// pressing its key when clicked, and lights the one under the pointer.
func (a *Application) tuiFooterHits(state *tuiState, lines []string, line int) {
	if line < 0 || line >= len(lines) {
		return
	}
	for _, chip := range a.tuiChips(lines[line]) {
		key := chipKey(chip.label)
		if key == "" {
			continue
		}
		state.addHit(tuiHit{kind: hitChip, key: key, top: line, bottom: line + 1, left: chip.start, right: chip.end})
		if state.hovering(hitChip, key) && a.p.Backdrop != "" {
			lit := tuiAsBackground(a.p.Backdrop)
			lines[line] = tintColumns(lines[line], func(col int) string {
				if col >= chip.start && col < chip.end {
					return lit
				}
				return ""
			})
		}
	}
}

// tuiDialogHits records the open dialog's buttons. Each is as wide as its
// label and four columns, two apart, on the second-last content line.
func (a *Application) tuiDialogHits(state *tuiState, g tuiGeometry, n int) {
	d := state.dialog
	if d == nil || len(d.Buttons) == 0 || n < 2 || n-2 >= g.frameHeight-4 {
		return
	}
	top, left, _ := tuiOverlayBox(g, n)
	line := top + 1 + n - 2
	col := left + 2
	for i, button := range d.Buttons {
		width := len(button.Label) + 4
		state.addHit(tuiHit{kind: hitButton, key: button.Label, index: i, top: line, bottom: line + 1, left: col, right: col + width})
		col += width + 2
	}
}
