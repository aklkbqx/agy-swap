package app

import (
	"fmt"
	"math"
	"os"
	"strings"
	"time"
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
			line = strings.ReplaceAll(line, "\x1b[49m", a.p.BodyBg)
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
	state.hits = state.hits[:0]
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
	a.tuiFooterHits(state, lines, footerStart+len(status)+1)
	switch state.mode {
	case tuiDialogOpen:
		chosen := 0.0
		if state.dialog != nil {
			// The choice slides from one button to the next.
			chosen = state.follow("dialog:"+state.dialog.Kind, float64(state.dialog.Index), tuiMotionSlide, a.renderTime())
		}
		content := a.tuiDialogLinesAt(state.dialog, chosen)
		lines = a.tuiOverlayOpening(lines, content, g.innerWidth, g.frameHeight, open)
		if open >= 1 {
			a.tuiDialogHits(state, g, len(content))
		}
	case tuiPalette:
		content := a.tuiPaletteLines(state, g.innerWidth)
		lines = a.tuiOverlayOpening(lines, content, g.innerWidth, g.frameHeight, open)
		if open >= 1 {
			top, left, contentWidth := tuiOverlayBox(g, len(content))
			for k, index := range state.paletteRows {
				if index >= 0 && k < g.frameHeight-4 {
					state.addHit(tuiHit{kind: hitPalette, index: index, top: top + 1 + k, bottom: top + 2 + k, left: left + 1, right: left + 1 + contentWidth})
				}
			}
		}
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
	// confusing  glyph seen in some terminal captures. Keep table cells ASCII
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
