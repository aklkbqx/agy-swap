package app

import (
	"fmt"
	"math"
	"os"
	"strings"
	"time"
	"unicode/utf8"

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
	email           string
	name            string
	avatar          string
	selected        bool
	active          bool
	health          string
	tone            tuiHealthTone
	needsAttention  bool
	attentionReason string
	pools           []poolView
	status          tuiAccountStatus
	verify          *eligibilityError
	// banded: the list draws the selection as a band over the rows, so the
	// block itself carries neither the tint nor the selection bar.
	banded bool
	// activeFx is how far the switch effect on the active account has run
	// (1 at rest); statusFlash marks a status word that just changed.
	activeFx    float64
	statusFlash bool
	// breath is how far the idle active dot has dimmed, 0 to 1.
	breath float64
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
	// The smallest terminal is 28 columns; the rails take two of them.
	frameWidth := maxInt(28-tuiRails, innerWidth+2)
	frameHeight := maxInt(12, height)
	innerWidth = frameWidth - 2
	g := tuiGeometry{
		frameWidth:   frameWidth,
		frameHeight:  frameHeight,
		innerWidth:   innerWidth,
		contentWidth: maxInt(1, frameWidth-4),
		bodyRows:     tuiBodyRowsFor(frameHeight),
		layout:       tuiLayoutFor(frameWidth+tuiRails, frameHeight),
	}
	if g.layout == tuiLayoutWide {
		// Give the account table enough room for identity and health columns,
		// while keeping the selected account details dominant on large screens.
		g.leftWidth = maxInt(48, minInt(68, (frameWidth-tuiWideChrome)*46/100))
		g.rightWidth = maxInt(1, frameWidth-tuiWideChrome-g.leftWidth)
	}
	return g
}

// tuiMaxFrameWidth is the widest the dashboard grows. On a wider terminal the
// frame keeps this width and sits in the middle, like a page container, instead
// of stretching the list and gauges across the screen.
const tuiMaxFrameWidth = 160

// tuiFrameBox is the frame width and its left margin on a terminal this wide.
func tuiFrameBox(terminalWidth int) (width, left int) {
	width = minInt(maxInt(28, terminalWidth), tuiMaxFrameWidth)
	return width, maxInt(0, terminalWidth-width) / 2
}

// frameColumn turns a terminal column into a column of the centred frame.
func (s *tuiState) frameColumn(col int) int {
	_, left := tuiFrameBox(s.width)
	return col - left - tuiRails/2
}

// tuiScreen is every terminal row: the frame, indented by the margin that
// centres it.
func (a *Application) tuiScreen(state *tuiState, width, height int) []string {
	frameWidth, left := tuiFrameBox(width)
	lines := a.tuiLines(state, frameWidth-2, height)
	margin := strings.Repeat(" ", left)
	screen := make([]string, height)
	for i := range screen {
		line := ""
		if i < len(lines) {
			line = lines[i]
		}
		// tuiLines already guarantees this width. Keep the final clamp as a
		// terminal-safety net for unexpected data, never as layout logic.
		screen[i] = margin + fitVisible(line, frameWidth, a.p)
	}
	return screen
}

func (a *Application) renderTUI(state *tuiState, out *os.File) {
	width, height, err := termSize(out)
	if err != nil {
		width, height = 80, 24
	}
	state.width, state.height = maxInt(28, width), height
	lines := a.tuiScreen(state, state.width, height)
	if tuiUseASCII(os.Getenv) {
		lines = asciiLines(lines)
	}
	if state.lastFrameWidth != state.width {
		state.lastFrame = nil // a new size is painted whole
	}
	_, _ = ioWriteString(a.Out, tuiPaintDiff(state.lastFrame, lines, state.width))
	state.lastFrame, state.lastFrameWidth = lines, state.width
}

// tuiPaint is one full redraw: home, then every row. A row that fills the
// terminal leaves the cursor on its last column waiting to wrap, and any erase
// sent there (to the end of the line, or of the screen) wipes that column, so
// only a shorter row (a centred frame on a wide terminal) is followed by one.
// The rows cover the whole screen, so nothing below them needs clearing.
func tuiPaint(lines []string, width int) string {
	var output strings.Builder
	output.Grow((width + 8) * len(lines))
	output.WriteString("\x1b[H")
	for i, line := range lines {
		output.WriteString(line)
		if visibleWidth(line) < width {
			output.WriteString("\x1b[K")
		}
		if i < len(lines)-1 {
			output.WriteString("\r\n")
		}
	}
	if n := len(lines); n == 0 || visibleWidth(lines[n-1]) < width {
		output.WriteString("\x1b[J")
	}
	return output.String()
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
// tuiRails is the two columns, one at each screen edge, that the solid side
// rails take outside the frame. The layout inside the rails keeps its own
// two-column margins, so the body has as much air at its sides as the one row
// between it and each bar; layout breakpoints still read the terminal width.
const tuiRails = 2

// tuiLines is the whole screen for a terminal width+2 columns wide: the frame
// inside, with a one-column rail at each edge in the bar colour.
func (a *Application) tuiLines(state *tuiState, width, height int) []string {
	now := a.renderTime()
	// The active dot breathes when the terminal can blend colours smoothly.
	state.ambient = state.motionEnabled && state.active != "" && strings.HasPrefix(a.p.Blue, "\x1b[38;2;")
	if state.active != state.prevActive {
		// A switch from one account to another starts the switch effect.
		if state.prevActive != "" && state.active != "" {
			state.activeChangedAt = now
		}
		state.prevActive = state.active
	}
	lines, footerStart := a.tuiFrameLines(state, maxInt(1, width-tuiRails), height)
	modal := oneOfMode(state.mode, tuiDialogOpen, tuiPalette, tuiForm)
	for i, line := range lines {
		bar := i < state.bodyTop || i >= footerStart
		if a.p.BodyBg != "" && i < footerStart && (!bar || modal) {
			// The body is black edge to edge: every reset hands back to it.
			line = a.p.BodyBg + strings.ReplaceAll(line, a.p.Reset, a.p.Reset+a.p.BodyBg) + a.p.Reset
		}
		switch {
		case a.p.BarBg == "" || (modal && i < footerStart):
			// No colour, or behind a modal: the frame dims with everything else.
			lines[i] = " " + line + " "
		case bar:
			// A bar runs on into its rails as one band.
			lines[i] = a.p.BarBg + " " + line + a.p.BarBg + " " + a.p.Reset
		default:
			rail := a.p.BarBg + " " + a.p.Reset
			lines[i] = rail + line + rail
		}
	}
	lines = a.tuiRefreshSweep(state, lines, width+2, now)
	return a.tuiWipe(state, lines, footerStart, width+2, now)
}

// tuiWipe reveals the screen from the top: all of it once at start-up, the
// body after a change of view. Rows not reached yet are blank.
func (a *Application) tuiWipe(state *tuiState, lines []string, footerStart, width int, now time.Time) []string {
	from, to, reach := 0, len(lines), 1.0
	if p := state.fxProgress(state.startedAt, tuiMotionStart, now); p < 1 {
		reach = easeOutCubic(p)
	} else if p := state.fxProgress(state.viewChangedAt, tuiMotionView, now); p < 1 {
		from, to, reach = state.bodyTop, footerStart, easeOutCubic(p)
	}
	if reach >= 1 {
		return lines
	}
	blank := strings.Repeat(" ", width)
	if a.p.BodyBg != "" {
		blank = a.p.BodyBg + blank + a.p.Reset
	}
	for i := from + int(float64(to-from)*reach+0.5); i < to; i++ {
		lines[i] = blank
	}
	return lines
}

// tuiFrameLines draws the frame inside the rails and reports the first footer row.
func (a *Application) tuiFrameLines(state *tuiState, width, height int) ([]string, int) {
	g := newTUIGeometry(width, height)
	lines := a.tuiTopLines(state, g.innerWidth)
	if tuiHeaderAir(g.frameHeight) {
		lines = append(lines, a.barRow("", g))
	}
	lines = append(lines, a.tuiActiveLine(state, g.innerWidth), a.barRow("", g))
	state.bodyTop = len(lines)
	if state.view != tuiViewDashboard {
		lines = append(lines, a.tuiViewBody(state, g.innerWidth, g.bodyRows)...)
	} else {
		lines = append(lines, a.tuiPoolBody(state, g.innerWidth, g.bodyRows)...)
	}
	status := a.tuiStatusLines(state, g.innerWidth)
	footer := a.tuiFooterLines(state, g.innerWidth)
	lines = append(lines, status...)
	lines = append(lines, footer...)
	lines = fitFrameLines(lines, g, a.p)
	footerStart := len(lines) - len(status) - len(footer)
	// A modal opens over 160 ms: the backdrop dims from the text colour and
	// the box opens from its centre line.
	open := easeOutCubic(state.fxProgress(state.modalOpenedAt, tuiMotionModal, a.renderTime()))
	if oneOfMode(state.mode, tuiDialogOpen, tuiPalette, tuiForm) {
		// A modal sits on a backdrop; its status line and key hints stay lit.
		lines = a.tuiBackdrop(lines, len(lines)-len(status)-len(footer), lerpColor(a.p.Gray, a.p.Backdrop, open))
	}
	switch state.mode {
	case tuiDialogOpen:
		chosen := 0.0
		if state.dialog != nil {
			// The choice slides from one button to the next.
			chosen = state.follow("dialog:"+state.dialog.Kind, float64(state.dialog.Index), tuiMotionSlide, a.renderTime())
		}
		lines = a.tuiOverlayOpening(lines, a.tuiDialogLinesAt(state.dialog, chosen), g.innerWidth, g.frameHeight, open)
	case tuiPalette:
		lines = a.tuiOverlayOpening(lines, a.tuiPaletteLines(state, g.innerWidth), g.innerWidth, g.frameHeight, open)
	case tuiForm:
		lines = a.tuiOverlayOpening(lines, a.tuiFormLines(state, g.innerWidth), g.innerWidth, g.frameHeight, open)
	}
	if state.toastActive(a.renderTime()) {
		lines = a.tuiToastOverlay(lines, state, g.innerWidth, g.frameHeight)
	}
	return fitFrameLines(lines, g, a.p), footerStart
}

func (a *Application) tuiHeaderLinesWithVersion(version string, state *tuiState, width int) []string {
	g := newTUIGeometry(width, 12)
	left := tuiWordmark(a.p) + "  " + a.p.Gray + "v" + tuiText(stateVersion(version)) + a.p.Reset
	sync := a.p.Gray + "LOCAL" + a.p.Reset
	if state != nil {
		switch {
		case state.refreshing:
			frames := []string{"◐", "◓", "◑", "◒"}
			sync = a.p.Cyan + frames[state.animationPhase()] + " SYNCING" + a.p.Reset
		case state.animation.kind == "success" && state.animation.active:
			sync = a.p.Green + "✓ SYNCED" + a.p.Reset
		case state.animation.kind == "error" && state.animation.active:
			sync = a.p.Yellow + "⚠ SYNC WARNINGS" + a.p.Reset
		}
	}
	// The sync state always stays; the credit goes first when the row is short.
	right := sync
	if full := a.p.Gray + tuiCredit + "  ·  " + a.p.Reset + sync; visibleWidth(left)+2+visibleWidth(full) <= g.contentWidth {
		right = full
	}
	if visibleWidth(left) > g.contentWidth {
		left = truncateVisible(left, g.contentWidth, a.p)
	}
	if visibleWidth(left)+1+visibleWidth(right) > g.contentWidth {
		right = truncateVisible(right, maxInt(1, g.contentWidth-visibleWidth(left)-1), a.p)
	}
	gap := maxInt(1, g.contentWidth-visibleWidth(left)-visibleWidth(right))
	content := left + strings.Repeat(" ", gap) + right
	return []string{
		a.barRow("", g),
		a.barRow(content, g),
	}
}

func (a *Application) displayVersion() string {
	v := stateVersion(a.Version)
	build := strings.TrimSpace(a.BuildID)
	if build != "" && build != "unknown" && build != "release" && build != "release-v"+v && build != "demo" {
		if !strings.HasSuffix(v, "-"+build) {
			return v + "-" + build
		}
	}
	return v
}

func (a *Application) tuiTopLines(state *tuiState, width int) []string {
	return a.tuiHeaderLinesWithVersion(a.displayVersion(), state, width)
}

func (a *Application) tuiHeaderLines(title, version string, width int) []string {
	return a.tuiHeaderLinesWithVersion(version, nil, width)
}

func (a *Application) tuiActiveLine(state *tuiState, width int) string {
	g := newTUIGeometry(width, 12)
	if state.active == "" {
		return a.barRow(a.p.Orange+"⬢ "+a.p.Bold+a.p.White+"ACTIVE"+a.p.Reset+"  "+a.p.Gray+"— no saved session"+a.p.Reset, g)
	}
	name := "Google User"
	status := a.p.Yellow + "UNSAVED" + a.p.Reset
	if state.accounts != nil {
		if account, ok := state.accounts.Get(state.active); ok {
			name = firstString(tuiText(getString(account, "name")), "Google User")
			status = a.p.Green + "SAVED" + a.p.Reset
		}
	}
	content := a.p.Orange + "⬢ " + a.p.Bold + a.p.White + "ACTIVE" + a.p.Reset + "  " + a.p.Blue + "●" + a.p.Reset + " " + avatar(name, state.active, a.color) + " " + a.p.White + a.p.Bold + tuiText(name) + a.p.Reset + " " + a.p.Gray + "<" + tuiText(state.active) + ">" + a.p.Reset + "  " + "[" + status + "]"
	return a.barRow(content, g)
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

func (a *Application) tuiAccountView(state *tuiState, email string, now time.Time) tuiAccountView {
	account := state.accounts.ByEmail[email]
	name := firstString(tuiText(getString(account, "name")), "Google User")
	groups := quotaGroupHealths(account)
	view := tuiAccountView{
		email:    email,
		name:     name,
		avatar:   avatar(name, email, a.color),
		selected: strings.EqualFold(email, state.selectedEmail),
		active:   strings.EqualFold(email, state.active),
		health:   accountHealthCompactForGroups(account, groups, now),
		tone:     tuiHealthToneForGroups(groups, account, now),
		pools:    poolViews(account, now),
	}
	view.status = accountStatusFor(view.pools, false)
	if state != nil {
		view.pools = state.tweenPools("meter:"+email, view.pools, now)
		if needs, reason := state.accountNeedsAttention(email); needs {
			view.needsAttention = true
			view.attentionReason = reason
			view.tone = tuiHealthWarning
			if strings.Contains(strings.ToLower(reason), "vault") || strings.Contains(strings.ToLower(reason), "token") {
				view.health = "⚠ Token unavailable"
				view.status = statusNoToken
			} else {
				view.health = "⚠ Needs attention"
				view.status = statusAttention
			}
		}
	}
	if ineligible, ok := accountEligibility(account); ok && view.status != statusNoToken {
		view.verify = ineligible
		view.status = statusVerify
		view.needsAttention = false
		view.health = "Needs Google verification"
		view.tone = tuiHealthWarning
	}
	view.activeFx = 1
	if state != nil {
		if view.active {
			view.activeFx = state.fxProgress(state.activeChangedAt, tuiMotionPulse, now)
			if state.ambient {
				phase := float64(now.UnixNano()%int64(tuiBreathPeriod)) / float64(tuiBreathPeriod)
				view.breath = 0.5 - 0.5*math.Cos(2*math.Pi*phase)
			}
		}
		// A status that changes flashes once; the first one seen does not.
		if state.statusSeen == nil {
			state.statusSeen, state.statusChangedAt = map[string]tuiAccountStatus{}, map[string]time.Time{}
		}
		if previous, seen := state.statusSeen[email]; seen && previous != view.status {
			state.statusChangedAt[email] = now
		}
		state.statusSeen[email] = view.status
		view.statusFlash = state.fxProgress(state.statusChangedAt[email], tuiMotionStatus, now) < 1
	}
	return view
}

// tuiSelectionBar marks the selected account down the left edge of every line
// of its block, so the selection reads as a band rather than a single glyph.
const tuiSelectionBar = "┃"

func (a *Application) tuiSelectionMarker(view tuiAccountView, state *tuiState) string {
	if !view.selected || view.banded {
		return " "
	}
	marker := tuiSelectionBar
	if state != nil && state.animation.kind == "focus" && state.animation.active && state.animationPhase()%2 == 1 {
		marker = "*"
	}
	return a.p.Orange + marker + a.p.Reset
}

// tuiSelectionGutter is the left edge of a block's continuation lines.
func (a *Application) tuiSelectionGutter(view tuiAccountView) string {
	if !view.selected || view.banded {
		return " "
	}
	return a.p.Orange + tuiSelectionBar + a.p.Reset
}

func (a *Application) tuiActiveMarker(view tuiAccountView) string {
	if view.active && view.activeFx < 1 && int(view.activeFx*6)%2 == 1 {
		// Just switched to: the dot pulses a few times.
		return a.p.Blue + "◉" + a.p.Reset
	}
	if view.active {
		if view.needsAttention {
			return a.p.Yellow + "●" + a.p.Reset
		}
		if view.breath > 0 {
			// While nothing else moves, the live account's dot breathes.
			return lerpColor(a.p.Blue, "\x1b[38;2;10;12;16m", 0.55*view.breath) + "●" + a.p.Reset
		}
		return a.p.Blue + "●" + a.p.Reset
	}
	if view.needsAttention {
		return a.p.Yellow + "⚠" + a.p.Reset
	}
	return " " // only the live account carries a dot
}

// tuiActiveName draws the account name and, for the account Antigravity is
// using, the word ACTIVE beside it. The word is blue, like the active dot:
// green, yellow and red are kept for how much quota is left. A live account
// that needs attention stays yellow.
func (a *Application) tuiActiveName(view tuiAccountView) (name, word string) {
	name = a.tuiIdentity(view.name)
	if view.selected {
		name = a.p.Bold + a.p.White + view.name + a.p.Reset
	}
	if !view.active {
		return name, ""
	}
	color := a.p.Blue
	if view.needsAttention {
		color = a.p.Yellow
	}
	word = "ACTIVE"
	if sweep := view.activeFx * float64(tuiMotionPulse) / float64(tuiMotionSweep); sweep < 1 {
		// Just switched to: the word sweeps in, keeping its width.
		shown := int(float64(len(word)) * easeOutCubic(sweep))
		word = word[:shown] + strings.Repeat(" ", len(word)-shown)
	}
	return name, color + a.p.Bold + word + a.p.Reset
}

func (a *Application) tuiAccountIdentityLine(view tuiAccountView, state *tuiState) string {
	return fmt.Sprintf("%s %s %s %s %s", a.tuiSelectionMarker(view, state), a.tuiActiveMarker(view), view.avatar, a.tuiIdentity(view.name), a.tuiSecondary("<"+view.email+">"))
}

func (a *Application) renderAccountRow(view tuiAccountView, width int) string {
	line := a.tuiAccountIdentityLine(view, nil)
	return fitVisible(line, width, a.p)
}

func (a *Application) tuiSectionDividerRow(title string, count int, isAttention bool, width int) string {
	width = maxInt(1, width)
	label := fmt.Sprintf("%s (%d)", title, count)
	color := a.p.Bold + a.p.White
	if isAttention {
		color = a.p.Bold + a.p.Yellow
	}
	prefix := a.p.DarkGray + "── " + color + label + a.p.Reset + " " + a.p.DarkGray
	prefixLen := visibleWidth(prefix)
	dashCount := maxInt(0, width-prefixLen)
	content := prefix + strings.Repeat("─", dashCount) + a.p.Reset
	return fitVisible(content, width, a.p)
}

func (a *Application) tuiWelcomeRows(width, maxRows int) []string {
	rows := []string{a.p.Bold + a.p.White + "Welcome to agy-swap" + a.p.Reset, "No saved accounts yet."}
	if width >= 50 {
		rows = append(rows, "Press "+a.p.Orange+"a"+a.p.Reset+" to sign in, or "+a.p.Cyan+"Ctrl-K"+a.p.Reset+" for every action.")
	} else {
		rows = append(rows, "Press "+a.p.Orange+"a"+a.p.Reset+" to sign in,", "or "+a.p.Cyan+"Ctrl-K"+a.p.Reset+" for every action.")
	}
	rows = append(rows, wrapWords("Your tokens go to a private local vault after sign-in.", width)...)
	if maxRows < len(rows) {
		rows = rows[:maxRows]
	}
	for i := range rows {
		rows[i] = fitVisible(rows[i], width, a.p)
	}
	return rows
}

// wrapWords breaks plain text into lines of at most width columns.
func wrapWords(text string, width int) []string {
	var lines []string
	line := ""
	for _, word := range strings.Fields(text) {
		switch {
		case line == "":
			line = word
		case len(line)+1+len(word) <= width:
			line += " " + word
		default:
			lines = append(lines, line)
			line = word
		}
	}
	if line != "" {
		lines = append(lines, line)
	}
	return lines
}

func sessionTokenReset(account Account) (string, bool) {
	if reset, ok := tokenResetInfo(tuiText(getString(account, "token_data"))); ok {
		return reset, true
	}
	return tokenResetFromExpiry(tuiText(getString(account, "access_expires_at")))
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
	now := a.renderTime()
	name := firstString(tuiText(getString(account, "name")), "Google User")
	labelWidth := maxInt(12, minInt(22, width/3))
	statusStr := accountStatus(account, a.p, now)
	var actionStr string
	ineligible, needsVerification := accountEligibility(account)
	if needs, reason := state.accountNeedsAttention(email); needs && !needsVerification {
		statusStr = a.p.Yellow + "[Needs Re-auth]" + a.p.Reset + " · " + reason
		actionStr = "Run 'agy-swap add' to re-authenticate or 'd' to remove"
	}
	if needsVerification {
		statusStr = a.p.Yellow + "Verification required" + a.p.Reset
		if ineligible.Message != "" {
			statusStr += " · " + tuiText(ineligible.Message)
		}
		actionStr = "Run 'agy' to see Google's link, then press r"
		if ineligible.URL != "" {
			actionStr = "[w] open Google's verification page in the browser, then press r"
		}
	}
	rows := []string{
		a.tuiSectionTitle("SELECTED ACCOUNT"),
		a.tuiIdentity(name),
		a.tuiSecondary(email),
		a.p.DarkGray + strings.Repeat("─", width) + a.p.Reset,
		tuiDetailKV("STATUS", statusStr, width, labelWidth, a.p),
	}
	if actionStr != "" {
		rows = append(rows, tuiDetailKV("ACTION", a.p.White+actionStr+a.p.Reset, width, labelWidth, a.p))
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
	if reset, ok := sessionTokenReset(account); ok {
		rows = append(rows, tuiDetailKV("SESSION TOKEN", reset, width, labelWidth, a.p))
	}
	if reason := tuiText(state.quotaErrors[email]); reason != "" && !strings.HasPrefix(reason, eligibilityErrorPrefix) {
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
	case tuiViewQuota:
		return "Limits for every saved account · r refreshes · b returns"
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
		case tuiViewQuota:
			if canVerify {
				footer = formatKeycaps(a.p, "[↑↓]", "Navigate", "[Enter]", "Switch", "[w]", "Verify", "[b]", "Dashboard", "[^K]", "Actions", "[?]", "Help")
			}
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
		"u           Download and install the latest release",
		"b           Return to dashboard",
		"Esc / any   Close help",
	}
	for i := range lines {
		lines[i] = truncateVisible(lines[i], maxInt(18, width-8), a.p)
	}
	return lines
}

// tuiOverlayOpening draws a dialog box over base. While it opens (open < 1)
// only its frame and the middle of its content show, centred where the whole
// box will sit, so it grows from its centre line.
func (a *Application) tuiOverlayOpening(base, overlay []string, width, height int, open float64) []string {
	g := newTUIGeometry(width, height)
	base = fitFrameLines(base, g, a.p)
	if len(overlay) == 0 {
		return base
	}
	contentWidth := minInt(64, maxInt(18, g.frameWidth-10))
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
