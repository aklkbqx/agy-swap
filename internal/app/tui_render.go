package app

import (
	"fmt"
	"math"
	"os"
	"strings"
	"time"

	"golang.org/x/term"
)

type tuiLayout uint8

const (
	tuiLayoutWide tuiLayout = iota
	tuiLayoutStacked
	tuiLayoutCompact
)

func tuiLayoutFor(width, height int) tuiLayout {
	if width >= 92 && height >= 18 {
		return tuiLayoutWide
	}
	if width < 64 || height < 16 {
		return tuiLayoutCompact
	}
	return tuiLayoutStacked
}

func (a *Application) renderTUI(state *tuiState, out *os.File) {
	width, height, err := termSize(out)
	if err != nil {
		width, height = 80, 24
	}
	frameWidth := maxInt(28, width)
	state.width, state.height = frameWidth, height
	inner := frameWidth - 2
	lines := a.tuiLines(state, inner, height)
	var output strings.Builder
	output.WriteString("\x1b[H")
	for i := 0; i < height; i++ {
		line := ""
		if i < len(lines) {
			line = lines[i]
		}
		output.WriteString(truncateVisible(line, frameWidth, a.p))
		output.WriteString("\x1b[K\r\n")
	}
	output.WriteString("\x1b[J")
	_, _ = ioWriteString(a.Out, output.String())
}

func termSize(file *os.File) (int, int, error) {
	width, height, err := termGetSize(file)
	if err != nil {
		return 0, 0, err
	}
	return maxInt(width, 28), maxInt(height, 12), nil
}

func (a *Application) tuiLines(state *tuiState, width, height int) []string {
	layout := tuiLayoutFor(width+2, height)
	lines := a.tuiTopLines(state, width)
	lines = append(lines, a.tuiActiveLine(state, width))
	contentHeight := maxInt(3, height-7)
	switch layout {
	case tuiLayoutWide:
		lines = append(lines, a.tuiWideBody(state, width, contentHeight)...)
	case tuiLayoutStacked:
		lines = append(lines, a.tuiStackedBody(state, width, contentHeight)...)
	default:
		lines = append(lines, a.tuiCompactBody(state, width, contentHeight)...)
	}
	lines = append(lines, a.tuiStatusLines(state, width)...)
	lines = append(lines, a.tuiFooterLines(state, width)...)
	if state.mode == tuiHelp {
		lines = a.tuiOverlay(lines, a.tuiHelpLines(width), width, height)
	} else if state.mode == tuiConfirmDelete {
		lines = a.tuiOverlay(lines, a.tuiDeleteLines(state, width), width, height)
	}
	return lines
}

func (a *Application) tuiTopLines(state *tuiState, width int) []string {
	left := a.p.Bold + a.p.Orange + "AGY SWAP" + a.p.Reset + "  v" + stateVersion(a.Version)
	right := a.p.Gray + "Operator Deck" + a.p.Reset
	if state.refreshing {
		frames := []string{"◐", "◓", "◑", "◒"}
		right = a.p.Cyan + frames[state.animationPhase()] + " Syncing usage" + a.p.Reset
	} else if state.animation.kind == "success" && state.animation.active {
		right = a.p.Green + "✓ Synced just now" + a.p.Reset
	} else if state.animation.kind == "error" && state.animation.active {
		right = a.p.Yellow + "⚠ Sync warnings" + a.p.Reset
	}
	inner := maxInt(1, width-2)
	left = truncateVisible(left, minInt(inner, visibleWidth(left)), a.p)
	if visibleWidth(left)+visibleWidth(right)+1 > inner {
		right = truncateVisible(right, maxInt(1, inner-visibleWidth(left)-1), a.p)
	}
	content := left + strings.Repeat(" ", maxInt(1, inner-visibleWidth(left)-visibleWidth(right))) + right
	return []string{
		a.p.Gray + "┌" + strings.Repeat("─", maxInt(0, width)) + "┐" + a.p.Reset,
		"│ " + padVisible(content, inner) + " │",
	}
}

func (a *Application) tuiActiveLine(state *tuiState, width int) string {
	if state.active == "" {
		content := a.p.Gray + "ACTIVE  — Not logged in" + a.p.Reset
		return "│ " + padVisible(content, width-2) + " │"
	}
	name := "Google User"
	status := "Saved"
	if account, ok := state.accounts.Get(state.active); ok {
		name = getString(account, "name")
	} else {
		status = "Unsaved · press a to save"
	}
	content := a.p.Bold + "ACTIVE" + a.p.Reset + "  " + a.p.Green + "●" + a.p.Reset + " " + avatar(name, state.active, a.color) + " " + a.p.Bold + cleanText(name) + a.p.Reset + " " + a.p.Gray + "<" + state.active + ">" + a.p.Reset + "  " + a.p.Green + status + a.p.Reset
	return "│ " + padVisible(content, width-2) + " │"
}

func (a *Application) tuiWideBody(state *tuiState, width, height int) []string {
	left := maxInt(31, minInt(39, width/3))
	right := maxInt(1, width-left-5)
	lines := []string{"├" + strings.Repeat("─", left+2) + "┬" + strings.Repeat("─", right+2) + "┤"}
	leftTitle := a.p.Bold + "ACCOUNTS " + a.p.Gray + fmt.Sprintf("%d", len(state.visibleEmails())) + a.p.Reset
	rightTitle := a.p.Bold + "ACCOUNT HEALTH" + a.p.Reset
	lines = append(lines, "│ "+padVisible(leftTitle, left)+" │ "+padVisible(rightTitle, right)+" │")
	list := a.tuiAccountRows(state, left, maxInt(1, height-2))
	details := a.tuiDetailLines(state, right, maxInt(1, height-2))
	rows := maxInt(len(list), len(details))
	for i := 0; i < rows; i++ {
		l, r := "", ""
		if i < len(list) {
			l = list[i]
		}
		if i < len(details) {
			r = details[i]
		}
		lines = append(lines, "│ "+padVisible(l, left)+" │ "+padVisible(r, right)+" │")
	}
	return lines
}

func (a *Application) tuiStackedBody(state *tuiState, width, height int) []string {
	title := a.p.Bold + "ACCOUNTS " + a.p.Gray + fmt.Sprintf("%d", len(state.visibleEmails())) + a.p.Reset
	lines := []string{"├" + strings.Repeat("─", width) + "┤", "│ " + padVisible(title, width-2) + " │"}
	rows := a.tuiAccountRows(state, width-2, maxInt(1, height-5))
	lines = append(lines, prefixRows(rows, "│ ", " │", width-2)...)
	if state.accounts == nil || state.accounts.Len() == 0 {
		lines = append(lines, "│ "+padVisible(a.p.Gray+"No accounts. Press a to add one."+a.p.Reset, width-2)+" │")
	}
	if _, _, ok := state.selectedAccount(); ok {
		lines = append(lines, "│ "+a.p.Gray+strings.Repeat("·", maxInt(1, width-2))+a.p.Reset+" │")
		lines = append(lines, prefixRows(a.tuiDetailLines(state, width-2, maxInt(1, height-len(lines)-2)), "│ ", " │", width-2)...)
	}
	return lines
}

func (a *Application) tuiCompactBody(state *tuiState, width, height int) []string {
	lines := []string{"├" + strings.Repeat("─", width) + "┤"}
	rows := a.tuiAccountRows(state, width-2, maxInt(1, height-3))
	lines = append(lines, prefixRows(rows, "│ ", " │", width-2)...)
	if state.accounts == nil || state.accounts.Len() == 0 {
		lines = append(lines, "│ "+padVisible(a.p.Gray+"No accounts. Press a to add one."+a.p.Reset, width-2)+" │")
	}
	return lines
}

func (a *Application) tuiAccountRows(state *tuiState, width, maxRows int) []string {
	emails := state.visibleEmails()
	if len(emails) == 0 {
		if state.accounts == nil || state.accounts.Len() == 0 {
			return nil
		}
		return []string{a.p.Gray + "No matching accounts · press / to search or a to add" + a.p.Reset}
	}
	selected := state.selectedEmail
	start := 0
	for i, email := range emails {
		if strings.EqualFold(email, selected) {
			start = maxInt(0, i-maxRows/2)
			break
		}
	}
	end := minInt(len(emails), start+maxRows)
	rows := make([]string, 0, end-start)
	now := time.Now()
	for i := start; i < end; i++ {
		email := emails[i]
		account := state.accounts.ByEmail[email]
		selectedMark := " "
		if strings.EqualFold(email, selected) {
			marker := "▶"
			if state.animation.kind == "focus" && state.animation.active && state.animationPhase()%2 == 1 {
				marker = "◆"
			}
			selectedMark = a.p.Orange + marker + a.p.Reset
		}
		activeMark := a.p.DarkGray + "○" + a.p.Reset
		if strings.EqualFold(email, state.active) {
			activeMark = a.p.Green + "●" + a.p.Reset
		}
		name := cleanText(getString(account, "name"))
		label := fmt.Sprintf("%s %s %s %s %s", selectedMark, activeMark, avatar(name, email, a.color), a.p.Bold+name+a.p.Reset, a.p.Gray+"<"+email+">"+a.p.Reset)
		summary := "  " + a.p.Gray + accountHealthSummary(account, now) + a.p.Reset
		rows = append(rows, truncateVisible(label, width, a.p), truncateVisible(summary, width, a.p))
	}
	return rows
}

func (a *Application) tuiDetailLines(state *tuiState, width, maxRows int) []string {
	email, account, ok := state.selectedAccount()
	if !ok {
		return []string{a.p.Gray + "Select an account to inspect usage." + a.p.Reset}
	}
	now := time.Now()
	lines := []string{a.p.Bold + cleanText(getString(account, "name")) + a.p.Reset, a.p.Gray + email + a.p.Reset, "", accountStatus(account, a.p, now)}
	for _, rawGroup := range quotaGroups(account) {
		group := getMap(rawGroup)
		lines = append(lines, a.p.Bold+a.p.Blue+getString(group, "name")+a.p.Reset)
		for _, rawBucket := range getSlice(group["buckets"]) {
			bucket := getMap(rawBucket)
			barWidth := minInt(18, maxInt(8, width/3))
			lines = append(lines, "  "+truncateVisible(getString(bucket, "name"), maxInt(12, width-barWidth-21), a.p)+"  "+formatQuotaBar(bucket, a.p, now, barWidth))
		}
	}
	if len(quotaGroups(account)) == 0 {
		for _, limit := range activeLimits(account, now) {
			if bar := formatCooldownBar(limit.Limit, a.p, now, minInt(18, maxInt(8, width/3))); bar != "" {
				lines = append(lines, "  "+getString(limit.Limit, "model")+"  "+bar)
			}
		}
	}
	if reset, ok := tokenResetInfo(getString(account, "token_data")); ok {
		lines = append(lines, "", a.p.Cyan+"Session token  "+reset+a.p.Reset)
	}
	if reason := state.quotaErrors[email]; reason != "" {
		lines = append(lines, "", a.p.Yellow+"Usage unavailable"+a.p.Reset+" · "+cleanText(reason))
	}
	if len(lines) > maxRows {
		lines = lines[:maxRows]
	}
	return lines
}

func accountHealthSummary(account Account, now time.Time) string {
	minFraction := 1.0
	found := false
	for _, rawGroup := range quotaGroups(account) {
		for _, rawBucket := range getSlice(getMap(rawGroup)["buckets"]) {
			fraction, ok := getFloat(getMap(rawBucket)["remaining_fraction"])
			if ok {
				minFraction = math.Min(minFraction, maxFloat(0, minFloat(1, fraction)))
				found = true
			}
		}
	}
	if found {
		return fmt.Sprintf("%s %.0f%% available", tierSummary(account), minFraction*100)
	}
	if limits := activeLimits(account, now); len(limits) > 0 {
		return tierSummary(account) + " cooldown"
	}
	return tierSummary(account) + " usage pending"
}

func tierSummary(account Account) string {
	snapshot := getMap(account["quota_snapshot"])
	tier := getMap(snapshot["tier"])
	if name := cleanText(getString(tier, "name")); name != "" {
		return name
	}
	return firstString(getString(account, "plan"), "Tier unknown")
}

func (a *Application) tuiStatusLines(state *tuiState, width int) []string {
	content := ""
	if state.mode == tuiSearch {
		content = a.p.Cyan + "/" + state.search + "▌" + a.p.Reset
	} else if state.mode == tuiConfirmDelete {
		content = a.p.Yellow + "Delete " + state.confirmEmail + "? [y] Confirm  [n] Cancel" + a.p.Reset
	} else if state.message != "" {
		prefix, color := "› ", a.p.Cyan
		if state.messageType == "success" {
			prefix, color = "✓ ", a.p.Green
		} else if state.messageType == "error" {
			prefix, color = "✕ ", a.p.Red
		}
		content = color + prefix + cleanText(state.message) + a.p.Reset
	} else {
		content = a.p.Gray + "Ready · select an account to inspect health" + a.p.Reset
	}
	return []string{
		"├" + strings.Repeat("─", width) + "┤",
		"│ " + padVisible(content, width-2) + " │",
	}
}

func (a *Application) tuiFooterLines(state *tuiState, width int) []string {
	footer := "↑↓/jk Navigate  Enter Switch  / Search  r Refresh  a Add  d Delete  n Next  ? Help  q Quit"
	if state.mode == tuiSearch {
		footer = "Type to filter  Enter Apply  Esc Cancel  Backspace Erase"
	} else if state.mode == tuiHelp {
		footer = "Esc or any key Close help"
	}
	return []string{
		"└" + strings.Repeat("─", width) + "┘",
		a.p.Gray + "  " + truncateVisible(footer, width-2, a.p) + a.p.Reset,
	}
}

func (a *Application) tuiHelpLines(width int) []string {
	lines := []string{"KEYBOARD GUIDE", "", "↑ ↓ / j k   Move through accounts", "Enter       Switch selected account", "/           Search by name or email", "r           Refresh quota", "a           Add account", "d           Delete selected account", "n           Choose next available account", "t           Toggle manual tier", "Esc / any   Close help", "", a.p.Gray + "All actions keep the active session safe." + a.p.Reset}
	for i := range lines {
		lines[i] = truncateVisible(lines[i], maxInt(24, width-8), a.p)
	}
	return lines
}

func (a *Application) tuiDeleteLines(state *tuiState, width int) []string {
	return []string{"DELETE ACCOUNT", "", "This removes the saved account and its local quota data.", "", a.p.Bold + state.confirmEmail + a.p.Reset, "", a.p.Yellow + "[y] Confirm    [n] Cancel" + a.p.Reset}
}

func (a *Application) tuiOverlay(base, overlay []string, width, height int) []string {
	if len(overlay) == 0 {
		return base
	}
	contentWidth := minInt(64, maxInt(24, width-8))
	maxContent := maxInt(1, height-4)
	if len(overlay) > maxContent {
		overlay = append([]string(nil), overlay[:maxContent]...)
		if len(overlay) > 0 {
			overlay[len(overlay)-1] = a.p.Gray + "… more in ? help" + a.p.Reset
		}
	}
	boxWidth := contentWidth + 2
	start := maxInt(1, (height-(len(overlay)+2))/2)
	left := maxInt(0, (width+2-boxWidth)/2)
	result := append([]string(nil), base...)
	for len(result) < height {
		result = append(result, "")
	}
	dialog := make([]string, 0, len(overlay)+2)
	dialog = append(dialog, a.p.Blue+"╭"+strings.Repeat("─", contentWidth)+"╮"+a.p.Reset)
	for _, line := range overlay {
		content := padVisible(truncateVisible(" "+line, contentWidth, a.p), contentWidth)
		dialog = append(dialog, a.p.Blue+"│"+a.p.Reset+content+a.p.Blue+"│"+a.p.Reset)
	}
	dialog = append(dialog, a.p.Blue+"╰"+strings.Repeat("─", contentWidth)+"╯"+a.p.Reset)
	for i, line := range dialog {
		if start+i >= len(result) {
			break
		}
		result[start+i] = strings.Repeat(" ", left) + line
	}
	return result
}

func prefixRows(rows []string, left, right string, contentWidth int) []string {
	result := make([]string, 0, len(rows))
	for _, row := range rows {
		result = append(result, left+padVisible(row, contentWidth)+right)
	}
	return result
}

func padVisible(value string, width int) string {
	if visibleWidth(value) >= width {
		return truncateVisible(value, width, palette{})
	}
	return value + strings.Repeat(" ", width-visibleWidth(value))
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

func maxFloat(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

func minFloat(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
