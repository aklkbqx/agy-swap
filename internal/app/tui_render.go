package app

import (
	"fmt"
	"os"
	"strings"
	"time"

	"golang.org/x/term"
)

const tuiCredit = "credit: aklkbqx"

type tuiHealthTone uint8

const (
	tuiHealthReady tuiHealthTone = iota
	tuiHealthWarning
	tuiHealthCritical
	tuiHealthLimited
	tuiHealthCooldown
	tuiHealthPending
)

type tuiAccountView struct {
	email    string
	name     string
	avatar   string
	selected bool
	active   bool
	health   string
	tone     tuiHealthTone
}

type tuiLayout uint8

const (
	tuiLayoutWide tuiLayout = iota
	tuiLayoutStacked
	tuiLayoutCompact
)

// tuiGeometry is the single source of truth for a frame. Renderers receive
// terminal-inner width for backwards compatibility, but all calculations are
// made from this structure so borders, panels, overlays and status rows share
// exactly the same width contract.
type tuiGeometry struct {
	frameWidth   int
	frameHeight  int
	innerWidth   int
	contentWidth int
	bodyRows     int
	layout       tuiLayout
	leftWidth    int
	rightWidth   int
}

func tuiLayoutFor(width, height int) tuiLayout {
	if width >= 92 && height >= 18 {
		return tuiLayoutWide
	}
	if width < 64 || height < 16 {
		return tuiLayoutCompact
	}
	return tuiLayoutStacked
}

func newTUIGeometry(innerWidth, height int) tuiGeometry {
	frameWidth := maxInt(28, innerWidth+2)
	frameHeight := maxInt(12, height)
	innerWidth = frameWidth - 2
	g := tuiGeometry{
		frameWidth:   frameWidth,
		frameHeight:  frameHeight,
		innerWidth:   innerWidth,
		contentWidth: maxInt(1, frameWidth-4),
		bodyRows:     maxInt(1, frameHeight-8),
		layout:       tuiLayoutFor(frameWidth, frameHeight),
	}
	if g.layout == tuiLayoutWide {
		// Give the account table enough room for identity and health columns,
		// while keeping the selected account details dominant on large screens.
		g.leftWidth = maxInt(42, minInt(64, (frameWidth-7)*36/100))
		g.rightWidth = maxInt(1, frameWidth-7-g.leftWidth)
	}
	return g
}

func (a *Application) renderTUI(state *tuiState, out *os.File) {
	width, height, err := termSize(out)
	if err != nil {
		width, height = 80, 24
	}
	frameWidth := maxInt(28, width)
	state.width, state.height = frameWidth, height
	lines := a.tuiLines(state, frameWidth-2, height)
	var output strings.Builder
	output.Grow((frameWidth + 8) * height)
	output.WriteString("\x1b[H")
	for i := 0; i < height; i++ {
		line := ""
		if i < len(lines) {
			line = lines[i]
		}
		// tuiLines already guarantees this width. Keep the final clamp as a
		// terminal-safety net for unexpected data, never as layout logic.
		line = fitVisible(line, frameWidth, a.p)
		output.WriteString(line)
		output.WriteString("\x1b[K")
		if i < height-1 {
			output.WriteString("\r\n")
		}
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

// tuiLines keeps the historical inner-width API used by tests while routing
// every row through one geometry contract.
func (a *Application) tuiLines(state *tuiState, width, height int) []string {
	g := newTUIGeometry(width, height)
	lines := a.tuiTopLines(state, g.innerWidth)
	lines = append(lines, a.tuiActiveLine(state, g.innerWidth))
	if state.view != tuiViewDashboard {
		lines = append(lines, a.tuiViewBody(state, g.innerWidth, g.bodyRows)...)
	} else {
		switch g.layout {
		case tuiLayoutWide:
			lines = append(lines, a.tuiWideBody(state, g.innerWidth, g.bodyRows)...)
		case tuiLayoutStacked:
			lines = append(lines, a.tuiStackedBody(state, g.innerWidth, g.bodyRows)...)
		default:
			lines = append(lines, a.tuiCompactBody(state, g.innerWidth, g.bodyRows)...)
		}
	}
	lines = append(lines, a.tuiStatusLines(state, g.innerWidth)...)
	lines = append(lines, a.tuiFooterLines(state, g.innerWidth)...)
	lines = fitFrameLines(lines, g, a.p)
	if state.mode == tuiHelp {
		lines = a.tuiOverlay(lines, a.tuiHelpLines(g.innerWidth), g.innerWidth, g.frameHeight)
	} else if state.mode == tuiConfirmDelete {
		lines = a.tuiOverlay(lines, a.tuiDeleteLines(state, g.innerWidth), g.innerWidth, g.frameHeight)
	} else if state.mode == tuiConfirmAction {
		lines = a.tuiOverlay(lines, a.tuiConfirmActionLines(state, g.innerWidth), g.innerWidth, g.frameHeight)
	} else if state.mode == tuiPalette {
		lines = a.tuiOverlay(lines, a.tuiPaletteLines(state, g.innerWidth), g.innerWidth, g.frameHeight)
	} else if state.mode == tuiForm {
		lines = a.tuiOverlay(lines, a.tuiFormLines(state, g.innerWidth), g.innerWidth, g.frameHeight)
	}
	if state.toastActive(time.Now()) {
		lines = a.tuiToastOverlay(lines, state, g.innerWidth, g.frameHeight)
	}
	return fitFrameLines(lines, g, a.p)
}

func (a *Application) tuiTopLines(state *tuiState, width int) []string {
	g := newTUIGeometry(width, 12)
	left := a.p.Bold + a.p.Orange + "AGY SWAP" + a.p.Reset + "  v" + stateVersion(a.Version)
	right := a.p.Gray + tuiCredit + "  ·  LOCAL" + a.p.Reset
	if state.refreshing {
		frames := []string{"◐", "◓", "◑", "◒"}
		right = a.p.Gray + tuiCredit + "  ·  " + a.p.Cyan + frames[state.animationPhase()] + " SYNCING" + a.p.Reset
	} else if state.animation.kind == "success" && state.animation.active {
		right = a.p.Gray + tuiCredit + "  ·  " + a.p.Green + "✓ SYNCED" + a.p.Reset
	} else if state.animation.kind == "error" && state.animation.active {
		right = a.p.Gray + tuiCredit + "  ·  " + a.p.Yellow + "⚠ SYNC WARNINGS" + a.p.Reset
	}
	if visibleWidth(left) > g.contentWidth {
		left = truncateVisible(left, g.contentWidth, a.p)
	}
	if visibleWidth(right) > g.contentWidth {
		right = truncateVisible(right, g.contentWidth, a.p)
	}
	gap := maxInt(1, g.contentWidth-visibleWidth(left)-visibleWidth(right))
	if visibleWidth(left)+visibleWidth(right)+gap > g.contentWidth {
		right = truncateVisible(right, maxInt(1, g.contentWidth-visibleWidth(left)-1), a.p)
		gap = maxInt(1, g.contentWidth-visibleWidth(left)-visibleWidth(right))
	}
	content := left + strings.Repeat(" ", gap) + right
	return []string{
		a.p.Gray + "┌" + strings.Repeat("─", g.frameWidth-2) + "┐" + a.p.Reset,
		frameRow(content, g, a.p),
	}
}

func (a *Application) tuiActiveLine(state *tuiState, width int) string {
	g := newTUIGeometry(width, 12)
	if state.active == "" {
		return frameRow(a.p.Yellow+"ACTIVE"+a.p.Reset+"  — no saved session", g, a.p)
	}
	name := "Google User"
	status := a.p.Yellow + "UNSAVED" + a.p.Reset
	if state.accounts != nil {
		if account, ok := state.accounts.Get(state.active); ok {
			name = firstString(tuiText(getString(account, "name")), "Google User")
			status = a.p.Green + "SAVED" + a.p.Reset
		}
	}
	content := a.p.Bold + "ACTIVE" + a.p.Reset + "  " + a.p.Green + "●" + a.p.Reset + " " + avatar(name, state.active, a.color) + " " + a.p.White + tuiText(name) + a.p.Reset + " " + a.p.Gray + "<" + tuiText(state.active) + ">" + a.p.Reset + "  " + status
	return frameRow(content, g, a.p)
}

func (a *Application) tuiSectionTitle(value string) string {
	return a.p.Bold + a.p.White + tuiText(value) + a.p.Reset
}

func (a *Application) tuiIdentity(value string) string {
	return a.p.Bold + a.p.White + tuiText(value) + a.p.Reset
}

func (a *Application) tuiSecondary(value string) string {
	return a.p.Gray + tuiText(value) + a.p.Reset
}

func tuiHealthToneForGroups(groups []quotaGroupHealth, account Account, now time.Time) tuiHealthTone {
	if best, ok := bestAvailableQuotaGroup(groups); ok {
		if best.fraction <= .1 {
			return tuiHealthCritical
		}
		if best.fraction <= .3 {
			return tuiHealthWarning
		}
		return tuiHealthReady
	}
	if len(groups) > 0 {
		return tuiHealthLimited
	}
	if len(activeLimits(account, now)) > 0 {
		return tuiHealthCooldown
	}
	return tuiHealthPending
}

func (a *Application) tuiHealthColor(tone tuiHealthTone) string {
	switch tone {
	case tuiHealthReady:
		return a.p.Green
	case tuiHealthWarning:
		return a.p.Yellow
	case tuiHealthCritical:
		return a.p.Red
	case tuiHealthLimited:
		return a.p.Red
	case tuiHealthCooldown:
		return a.p.Yellow
	default:
		return a.p.Gray
	}
}

func (a *Application) tuiAccountView(state *tuiState, email string, now time.Time) tuiAccountView {
	account := state.accounts.ByEmail[email]
	name := firstString(tuiText(getString(account, "name")), "Google User")
	groups := quotaGroupHealths(account)
	return tuiAccountView{
		email:    email,
		name:     name,
		avatar:   avatar(name, email, a.color),
		selected: strings.EqualFold(email, state.selectedEmail),
		active:   strings.EqualFold(email, state.active),
		health:   accountHealthCompactForGroups(account, groups, now),
		tone:     tuiHealthToneForGroups(groups, account, now),
	}
}

func (a *Application) tuiSelectionMarker(view tuiAccountView, state *tuiState) string {
	if !view.selected {
		return " "
	}
	marker := ">"
	if state.animation.kind == "focus" && state.animation.active && state.animationPhase()%2 == 1 {
		marker = "*"
	}
	return a.p.Orange + marker + a.p.Reset
}

func (a *Application) tuiActiveMarker(view tuiAccountView) string {
	if view.active {
		return a.p.Green + "●" + a.p.Reset
	}
	return a.p.DarkGray + "·" + a.p.Reset
}

func (a *Application) tuiAccountIdentityLine(view tuiAccountView, state *tuiState) string {
	return fmt.Sprintf("%s %s %s %s %s", a.tuiSelectionMarker(view, state), a.tuiActiveMarker(view), view.avatar, a.tuiIdentity(view.name), a.tuiSecondary("<"+view.email+">"))
}

func (a *Application) tuiWideBody(state *tuiState, width, height int) []string {
	g := newTUIGeometry(width, height+8)
	g.bodyRows = maxInt(1, height)
	if g.leftWidth == 0 {
		g.leftWidth = maxInt(42, minInt(64, (g.frameWidth-7)*36/100))
		g.rightWidth = maxInt(1, g.frameWidth-7-g.leftWidth)
	}
	lines := []string{panelDivider(g, a.p)}
	if g.bodyRows > 1 {
		leftTitle := a.tuiSectionTitle("ACCOUNTS") + "  " + a.p.Gray + fmt.Sprintf("%d", len(state.visibleEmails())) + "  > selected · ● active" + a.p.Reset
		rightTitle := a.tuiSectionTitle("ACCOUNT HEALTH")
		lines = append(lines, panelRow(leftTitle, rightTitle, g, a.p))
	}
	available := maxInt(0, g.bodyRows-len(lines))
	list := a.tuiAccountTableRows(state, g.leftWidth, available)
	details := a.tuiDetailTableLines(state, g.rightWidth, available)
	rows := maxInt(len(list), len(details))
	for i := 0; i < rows && len(lines) < g.bodyRows; i++ {
		left, right := "", ""
		if i < len(list) {
			left = list[i]
		}
		if i < len(details) {
			right = details[i]
		}
		lines = append(lines, panelRow(left, right, g, a.p))
	}
	return fitBodyLines(lines, g, a.p)
}

func (a *Application) tuiStackedBody(state *tuiState, width, height int) []string {
	g := newTUIGeometry(width, height+8)
	g.bodyRows = maxInt(1, height)
	lines := []string{a.p.Gray + "├" + strings.Repeat("─", g.frameWidth-2) + "┤" + a.p.Reset}
	if len(lines) < g.bodyRows {
		title := a.tuiSectionTitle("ACCOUNTS") + "  " + a.p.Gray + fmt.Sprintf("%d", len(state.visibleEmails())) + "  > selected · ● active" + a.p.Reset
		lines = append(lines, frameRow(title, g, a.p))
	}
	if len(lines) >= g.bodyRows {
		return fitBodyLines(lines, g, a.p)
	}
	detailBudget := 0
	if _, _, ok := state.selectedAccount(); ok {
		available := maxInt(0, g.bodyRows-len(lines))
		detailBudget = minInt(8, maxInt(3, available/2+1))
		if detailBudget+3 > available {
			detailBudget = maxInt(3, available-3)
		}
	}
	listBudget := maxInt(1, g.bodyRows-len(lines)-detailBudget-2)
	rows := a.tuiAccountRows(state, g.contentWidth, listBudget)
	if len(rows) == 0 {
		if state.accounts == nil || state.accounts.Len() == 0 {
			rows = a.tuiWelcomeRows(g.contentWidth, maxInt(1, g.bodyRows-1))
		} else {
			rows = []string{a.p.Gray + "No matching accounts" + a.p.Reset + " · press / to clear search"}
		}
	}
	for _, row := range rows {
		if len(lines) >= g.bodyRows-detailBudget-1 {
			break
		}
		lines = append(lines, frameRow(row, g, a.p))
	}
	if detailBudget > 0 && len(lines) < g.bodyRows {
		lines = append(lines, a.p.Gray+"├"+strings.Repeat("─", g.frameWidth-2)+"┤"+a.p.Reset)
		if len(lines) < g.bodyRows {
			lines = append(lines, frameRow(a.tuiSectionTitle("ACCOUNT HEALTH"), g, a.p))
		}
		for _, row := range a.tuiDetailLines(state, g.contentWidth, detailBudget) {
			if len(lines) >= g.bodyRows {
				break
			}
			lines = append(lines, frameRow(row, g, a.p))
		}
	}
	return fitBodyLines(lines, g, a.p)
}

func (a *Application) tuiCompactBody(state *tuiState, width, height int) []string {
	if height >= 16 {
		// A narrow terminal still needs selected-account details. Reuse the
		// stacked renderer once there is enough vertical room instead of
		// silently reducing the UI to an account list.
		return a.tuiStackedBody(state, width, height)
	}
	g := newTUIGeometry(width, height+8)
	g.bodyRows = maxInt(1, height)
	lines := []string{a.p.Gray + "├" + strings.Repeat("─", g.frameWidth-2) + "┤" + a.p.Reset}
	rows := a.tuiAccountRows(state, g.contentWidth, maxInt(0, g.bodyRows-1))
	if len(rows) == 0 {
		if state.accounts == nil || state.accounts.Len() == 0 {
			rows = a.tuiWelcomeRows(g.contentWidth, maxInt(1, g.bodyRows-1))
		} else {
			rows = []string{a.p.Gray + "No matching accounts" + a.p.Reset + " · press / to clear search"}
		}
	}
	for _, row := range rows {
		if len(lines) >= g.bodyRows {
			break
		}
		lines = append(lines, frameRow(row, g, a.p))
	}
	return fitBodyLines(lines, g, a.p)
}

func (a *Application) tuiAccountRows(state *tuiState, width, maxRows int) []string {
	width = maxInt(1, width)
	if maxRows <= 0 {
		return nil
	}
	// An account is a two-line unit (identity + health summary). Never leave
	// a dangling identity row at the bottom of a compact viewport.
	if maxRows > 1 {
		maxRows -= maxRows % 2
	}
	emails := state.visibleEmails()
	if len(emails) == 0 {
		return nil
	}
	accountBudget := maxInt(1, maxRows/2)
	start := 0
	for i, email := range emails {
		if strings.EqualFold(email, state.selectedEmail) {
			start = maxInt(0, i-accountBudget/2)
			break
		}
	}
	end := minInt(len(emails), start+accountBudget)
	rows := make([]string, 0, end-start)
	now := time.Now()
	for i := start; i < end; i++ {
		email := emails[i]
		view := a.tuiAccountView(state, email, now)
		label := a.tuiAccountIdentityLine(view, state)
		if maxRows == 1 {
			rows = append(rows, fitVisible(label, width, a.p))
			break
		}
		summary := "  " + a.tuiHealthColor(view.tone) + tuiText(view.health) + a.p.Reset
		rows = append(rows, fitVisible(label, width, a.p), fitVisible(summary, width, a.p))
	}
	return rows
}

// tuiAccountTableRows renders the wide-layout account list as a real table:
// selection, active state, identity, and health each occupy a stable column.
// Keeping each account to one row makes the list scannable and prevents a
// dangling second line from looking like a missing record.
func (a *Application) tuiAccountTableRows(state *tuiState, width, maxRows int) []string {
	width = maxInt(1, width)
	if maxRows <= 0 {
		return nil
	}
	markerWidth, identityWidth, healthWidth := tuiAccountColumnWidths(width)
	columns := []string{
		fitVisible("S", 1, a.p),
		fitVisible("A", 1, a.p),
		fitVisible("TAG", markerWidth, a.p),
		fitVisible("ACCOUNT", identityWidth, a.p),
		fitVisible("HEALTH", healthWidth, a.p),
	}
	header := a.p.Bold + a.p.Gray + strings.Join(columns, " ") + a.p.Reset
	rows := []string{fitVisible(header, width, a.p)}
	if maxRows == 1 {
		return rows
	}
	rows = append(rows, fitVisible(a.p.DarkGray+strings.Repeat("─", width)+a.p.Reset, width, a.p))

	emails := state.visibleEmails()
	if len(emails) == 0 {
		remaining := maxInt(0, maxRows-len(rows))
		for _, welcome := range a.tuiWelcomeRows(width, remaining) {
			rows = append(rows, fitVisible(welcome, width, a.p))
		}
		return rows
	}
	rowBudget := maxRows - len(rows)
	start := 0
	selectedIndex := state.selectedIndex()
	if len(emails) > rowBudget {
		start = maxInt(0, minInt(selectedIndex-rowBudget/2, len(emails)-rowBudget))
	}
	end := minInt(len(emails), start+rowBudget)
	now := time.Now()
	for _, email := range emails[start:end] {
		view := a.tuiAccountView(state, email, now)
		// Keep the list column about recognition. The selected account's full
		// email is shown in the health pane, so long addresses do not turn every
		// row into an ellipsis-heavy line.
		identity := a.tuiIdentity(view.name)
		health := a.tuiHealthColor(view.tone) + tuiText(view.health) + a.p.Reset
		cells := []string{
			fitVisible(a.tuiSelectionMarker(view, state), 1, a.p),
			fitVisible(a.tuiActiveMarker(view), 1, a.p),
			fitVisible(view.avatar, markerWidth, a.p),
			fitVisible(identity, identityWidth, a.p),
			fitVisible(health, healthWidth, a.p),
		}
		rows = append(rows, fitVisible(strings.Join(cells, " "), width, a.p))
	}
	return rows
}

func (a *Application) tuiWelcomeRows(width, maxRows int) []string {
	rows := []string{
		a.p.Bold + a.p.White + "Welcome to AGY SWAP" + a.p.Reset,
		"No saved accounts yet.",
		"Press " + a.p.Orange + "a" + a.p.Reset + " to sign in, or " + a.p.Cyan + "Ctrl-K" + a.p.Reset + " for every action.",
		"Your tokens are stored in the OS vault after sign-in.",
	}
	if maxRows < len(rows) {
		rows = rows[:maxRows]
	}
	for i := range rows {
		rows[i] = fitVisible(rows[i], width, a.p)
	}
	return rows
}

func tuiAccountColumnWidths(width int) (marker, identity, health int) {
	width = maxInt(16, width)
	marker = 4 // [AA]
	health = maxInt(14, minInt(24, width/3))
	identity = width - marker - health - 6 // two marker columns plus four spaces
	if identity < 12 {
		health = maxInt(10, health-(12-identity))
		identity = width - marker - health - 6
	}
	return marker, maxInt(1, identity), maxInt(1, health)
}

// tuiDetailTableLines mirrors the account table with key/value columns for
// status, quota buckets, and token state. The old free-form detail renderer
// made the right pane look like a paragraph instead of a data table.
func (a *Application) tuiDetailTableLines(state *tuiState, width, maxRows int) []string {
	width = maxInt(1, width)
	if maxRows <= 0 {
		return nil
	}
	email, account, ok := state.selectedAccount()
	if !ok {
		return []string{fitVisible(a.tuiSecondary("Select an account to inspect usage."), width, a.p)}
	}
	now := time.Now()
	name := firstString(tuiText(getString(account, "name")), "Google User")
	labelWidth := maxInt(12, minInt(22, width/3))
	rows := []string{
		a.tuiSectionTitle("SELECTED ACCOUNT"),
		a.tuiIdentity(name),
		a.tuiSecondary(email),
		a.p.DarkGray + strings.Repeat("─", width) + a.p.Reset,
		tuiDetailKV("STATUS", accountStatus(account, a.p, now), width, labelWidth, a.p),
	}
	for _, rawGroup := range quotaGroups(account) {
		group := getMap(rawGroup)
		groupName := firstString(tuiText(getString(group, "name")), "Model group")
		rows = append(rows, a.p.Bold+a.p.Blue+tuiText(groupName)+a.p.Reset)
		for _, rawBucket := range getSlice(group["buckets"]) {
			bucket := getMap(rawBucket)
			valueWidth := maxInt(8, width-labelWidth-3)
			barWidth := tuiQuotaBarWidth(valueWidth)
			rows = append(rows, tuiDetailKV(tuiText(getString(bucket, "name")), formatQuotaBarResponsive(bucket, a.p, now, barWidth, valueWidth), width, labelWidth, a.p))
		}
	}
	if len(quotaGroups(account)) == 0 {
		for _, limit := range activeLimits(account, now) {
			valueWidth := maxInt(8, width-labelWidth-3)
			bar := formatCooldownBar(limit.Limit, a.p, now, tuiQuotaBarWidth(valueWidth))
			if bar != "" {
				rows = append(rows, tuiDetailKV(tuiText(getString(limit.Limit, "model")), bar, width, labelWidth, a.p))
			}
		}
	}
	if getString(account, "secret_ref") != "" && getString(account, "token_data") == "" {
		rows = append(rows, tuiDetailKV("SESSION TOKEN", "Stored in OS vault", width, labelWidth, a.p))
	} else if reset, ok := tokenResetInfo(tuiText(getString(account, "token_data"))); ok {
		rows = append(rows, tuiDetailKV("SESSION TOKEN", reset, width, labelWidth, a.p))
	}
	if reason := tuiText(state.quotaErrors[email]); reason != "" {
		rows = append(rows, tuiDetailKV("USAGE", a.p.Yellow+"Unavailable"+a.p.Reset+" · "+reason, width, labelWidth, a.p))
	}
	if len(rows) > maxRows {
		rows = rows[:maxRows]
	}
	for i := range rows {
		rows[i] = fitVisible(rows[i], width, a.p)
	}
	return rows
}

func tuiQuotaBarWidth(valueWidth int) int {
	return minInt(18, maxInt(4, (maxInt(8, valueWidth)-8)/2))
}

func tuiDetailKV(label, value string, width, labelWidth int, p palette) string {
	valueWidth := maxInt(1, width-labelWidth-3)
	left := p.Cyan + fitVisible(strings.ToUpper(tuiText(label)), labelWidth, p) + p.Reset
	rule := p.DarkGray + "│" + p.Reset
	// value may already contain ANSI color sequences (quota bars/status text),
	// so preserve them and let fitVisible measure the visible width.
	right := fitVisible(value, valueWidth, p)
	return fitVisible(left+" "+rule+" "+right, width, p)
}

func tuiText(value string) string {
	// Invalid UTF-8 and literal replacement characters otherwise become the
	// confusing � glyph seen in some terminal captures. Keep table cells ASCII
	// safe while preserving normal Unicode names and email addresses.
	value = strings.ToValidUTF8(value, "?")
	value = strings.ReplaceAll(value, "\uFFFD", "?")
	return cleanText(value)
}

func accountHealthCompact(account Account, now time.Time) string {
	return accountHealthCompactForGroups(account, quotaGroupHealths(account), now)
}

func accountHealthCompactForGroups(account Account, groups []quotaGroupHealth, now time.Time) string {
	if best, ok := bestAvailableQuotaGroup(groups); ok {
		return fmt.Sprintf("%s %.0f%% ready", best.label, best.fraction*100)
	}
	if len(groups) > 0 {
		return "limited"
	}
	if len(activeLimits(account, now)) > 0 {
		return "cooldown"
	}
	return "pending"
}

func (a *Application) tuiDetailLines(state *tuiState, width, maxRows int) []string {
	if maxRows <= 0 {
		return nil
	}
	// The stacked view already has an ACCOUNT HEALTH section title. Remove
	// the repeated table heading and rule at narrow widths so the same detail
	// budget can retain both model families and their reset windows.
	rows := a.tuiDetailTableLines(state, width, maxRows+2)
	if width <= 78 {
		compact := make([]string, 0, len(rows))
		for _, row := range rows {
			plain := ansiPattern.ReplaceAllString(row, "")
			if strings.TrimSpace(plain) == "SELECTED ACCOUNT" || strings.Trim(plain, "─ ") == "" {
				continue
			}
			compact = append(compact, row)
		}
		rows = compact
	}
	if len(rows) > maxRows {
		rows = rows[:maxRows]
	}
	return rows
}

func (a *Application) tuiStatusLines(state *tuiState, width int) []string {
	g := newTUIGeometry(width, 12)
	content := ""
	if state.mode == tuiSearch {
		content = a.p.Cyan + "/" + cleanText(state.search) + "▌" + a.p.Reset
	} else if state.mode == tuiConfirmDelete {
		content = a.p.Yellow + "Delete " + cleanText(state.confirmEmail) + "?  [y] confirm  [n] cancel" + a.p.Reset
	} else if state.mode == tuiConfirmAction {
		content = a.p.Yellow + firstString(state.confirmTitle, "Confirm action") + "?  [y] confirm  [n] cancel" + a.p.Reset
	} else if state.mode == tuiPalette {
		content = a.p.Cyan + "Action palette · type to filter · Enter to run" + a.p.Reset
	} else if state.mode == tuiForm {
		content = a.p.Cyan + "Editing · choose a field, then Enter to save" + a.p.Reset
	} else if state.job != nil && !state.job.Done {
		content = a.p.Cyan + "◐ " + cleanText(state.job.Label) + "…" + a.p.Reset
	} else if state.refreshing {
		content = a.p.Cyan + "◐ Syncing usage…" + a.p.Reset
	} else if state.resolvingToken != "" {
		content = a.p.Cyan + "⌁ Resolving active session…" + a.p.Reset
	} else if state.message != "" {
		prefix, color := "› ", a.p.Cyan
		if state.messageType == "success" {
			prefix, color = "✓ ", a.p.Green
		} else if state.messageType == "error" {
			prefix, color = "✕ ", a.p.Red
		}
		content = color + prefix + cleanText(state.message) + a.p.Reset
	} else if state.accounts == nil || state.accounts.Len() == 0 {
		content = a.p.Yellow + "No accounts yet · press a to add your first account" + a.p.Reset
	} else {
		content = a.p.Gray + "Ready · select an account to inspect health" + a.p.Reset
	}
	return []string{
		a.p.Gray + "├" + strings.Repeat("─", g.frameWidth-2) + "┤" + a.p.Reset,
		frameRow(content, g, a.p),
	}
}

func (a *Application) tuiToastOverlay(base []string, state *tuiState, width, height int) []string {
	g := newTUIGeometry(width, height)
	if !state.toastActive(time.Now()) {
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
	bottom := color + "╰" + strings.Repeat("─", innerWidth) + "╯" + a.p.Reset
	box := []string{top, body, bottom}
	base = fitFrameLines(base, g, a.p)
	rowStart := maxInt(1, g.frameHeight-len(box)-4)
	left := maxInt(1, g.frameWidth-boxWidth-2)
	for index, line := range box {
		row := rowStart + index
		if row >= 0 && row < len(base) {
			base[row] = fitVisible(strings.Repeat(" ", left)+line, g.frameWidth, a.p)
		}
	}
	return base
}

func (a *Application) tuiFooterLines(state *tuiState, width int) []string {
	g := newTUIGeometry(width, 12)
	footer := "↑↓/jk Navigate   Enter Switch   Ctrl-K/: Actions   ? Help   q Quit"
	if state.view != tuiViewDashboard {
		footer = "↑↓ Navigate   Enter Edit   b Dashboard   Ctrl-K/: Actions   ? Help"
		switch state.view {
		case tuiViewBackup:
			footer = "x Export   i Import   v Verify   b Dashboard   Ctrl-K/: Actions"
		case tuiViewHistory:
			footer = "c Clear   x Export   b Dashboard   Ctrl-K/: Actions"
		case tuiViewSettings:
			footer = "e Edit   a Alias   b Binding   t Target   Ctrl-K/: Actions"
		case tuiViewDoctor:
			footer = "Enter/r Run again   b Dashboard   Ctrl-K/: Actions"
		case tuiViewProfiles:
			footer = "c Create   Enter/e Edit   d Delete   b Dashboard   Ctrl-K/: Actions"
		}
	}
	if state.mode == tuiSearch {
		footer = "Type to filter   Enter Apply   Esc Cancel   Backspace Erase"
	} else if state.mode == tuiHelp {
		footer = "Esc or any key · Close help"
	} else if state.mode == tuiPalette {
		footer = "↑↓ Move   Enter Run   Type Filter   Esc Close"
	} else if state.mode == tuiForm {
		footer = "↑↓ Field   ←→ Choice   Enter Next/Save   Esc Cancel"
	} else if state.mode == tuiConfirmAction {
		footer = "y Confirm   n / Esc Cancel"
	}
	return []string{
		a.p.Gray + "├" + strings.Repeat("─", g.frameWidth-2) + "┤" + a.p.Reset,
		frameRow(a.p.Gray+footer+a.p.Reset, g, a.p),
		a.p.Gray + "└" + strings.Repeat("─", g.frameWidth-2) + "┘" + a.p.Reset,
	}
}

func (a *Application) tuiHelpLines(width int) []string {
	lines := []string{
		"KEYBOARD GUIDE",
		"",
		"↑ ↓ / j k   Move through accounts",
		"Enter       Switch selected account",
		"/           Search by name or email",
		"Ctrl-K / :  Open action palette",
		"p h s o b   Profiles, history, settings, doctor, backup",
		"r           Refresh quota",
		"a           Add account",
		"d           Delete selected account",
		"n           Choose next available account",
		"t           Toggle manual tier",
		"m           Migrate secrets to OS vault",
		"l           Log out",
		"e           Edit tags / selected item",
		"x / i / v   Export, import, or verify in managers",
		"c / x       Create/clear or export in managers",
		"u           Download and install the latest release",
		"b           Return to dashboard",
		"Esc / any   Close help",
		"",
		a.p.Gray + "Actions keep the active session safe." + a.p.Reset,
	}
	for i := range lines {
		lines[i] = truncateVisible(lines[i], maxInt(18, width-8), a.p)
	}
	return lines
}

func (a *Application) tuiDeleteLines(state *tuiState, width int) []string {
	return []string{
		"DELETE ACCOUNT",
		"",
		"This removes the saved account and its local quota data.",
		"",
		a.p.Bold + cleanText(state.confirmEmail) + a.p.Reset,
		"",
		a.p.Yellow + "[y] Confirm    [n] Cancel    [Esc] Back" + a.p.Reset,
	}
}

func (a *Application) tuiOverlay(base, overlay []string, width, height int) []string {
	g := newTUIGeometry(width, height)
	base = fitFrameLines(base, g, a.p)
	if len(overlay) == 0 {
		return base
	}
	contentWidth := minInt(64, maxInt(18, g.frameWidth-10))
	maxContent := maxInt(1, g.frameHeight-4)
	if len(overlay) > maxContent {
		overlay = append([]string(nil), overlay[:maxContent]...)
		overlay[len(overlay)-1] = a.p.Gray + "… more in ? help" + a.p.Reset
	}
	dialog := make([]string, 0, len(overlay)+2)
	dialog = append(dialog, a.p.Blue+"╭"+strings.Repeat("─", contentWidth)+"╮"+a.p.Reset)
	for _, line := range overlay {
		content := fitVisible(" "+line, contentWidth, a.p)
		dialog = append(dialog, a.p.Blue+"│"+a.p.Reset+content+a.p.Blue+"│"+a.p.Reset)
	}
	dialog = append(dialog, a.p.Blue+"╰"+strings.Repeat("─", contentWidth)+"╯"+a.p.Reset)
	start := maxInt(1, (g.frameHeight-len(dialog))/2)
	for i, line := range dialog {
		row := start + i
		if row >= 0 && row < len(base) {
			left := maxInt(0, (g.frameWidth-visibleWidth(line))/2)
			base[row] = fitVisible(strings.Repeat(" ", left)+line, g.frameWidth, a.p)
		}
	}
	return fitFrameLines(base, g, a.p)
}

func frameRow(content string, g tuiGeometry, p palette) string {
	return "│ " + fitVisible(content, g.contentWidth, p) + " │"
}

func panelRow(left, right string, g tuiGeometry, p palette) string {
	return "│ " + fitVisible(left, g.leftWidth, p) + " │ " + fitVisible(right, g.rightWidth, p) + " │"
}

func panelDivider(g tuiGeometry, p palette) string {
	return p.Gray + "├" + strings.Repeat("─", g.leftWidth+2) + "┬" + strings.Repeat("─", g.rightWidth+2) + "┤" + p.Reset
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

func prefixRows(rows []string, left, right string, contentWidth int) []string {
	result := make([]string, 0, len(rows))
	for _, row := range rows {
		result = append(result, left+padVisible(row, contentWidth)+right)
	}
	return result
}

func padVisible(value string, width int) string {
	return fitVisible(value, width, palette{})
}

func fitVisible(value string, width int, p palette) string {
	width = maxInt(0, width)
	if width == 0 {
		return ""
	}
	if visibleWidth(value) > width {
		return truncateVisible(value, width, p)
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
