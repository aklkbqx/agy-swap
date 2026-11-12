package app

import (
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

// frameTick says whether a timer frame should be drawn now. A frame that falls
// due while the terminal still holds the last one is skipped and owed: motion
// follows the clock, so nothing is lost, and the screen is drawn again as soon
// as the terminal has caught up instead of stopping on a stale frame.
func (s *tuiState) frameTick() bool {
	if s.frames.busy() {
		s.frameOwed = true
		return false
	}
	return true
}

// repaintWhole makes the next frame a whole paint, after the screen was left
// to another program.
func (s *tuiState) repaintWhole() {
	s.lastFrame = nil
	s.frames.repaintNext()
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
	state.frameOwed = false
	if state.frames != nil {
		state.frames.submit(lines, state.width)
		return
	}
	if state.lastFrameWidth != state.width {
		state.lastFrame = nil // a new size is painted whole
	}
	if diff := tuiPaintDiff(state.lastFrame, lines, state.width); diff != "" {
		if out != nil && term.IsTerminal(int(out.Fd())) {
			// DEC private mode 2026 batches the entire diff into one atomic vblank
			// update on supporting terminals; older terminals ignore it cleanly.
			_, _ = ioWriteString(a.Out, "\x1b[?2026h"+diff+"\x1b[?2026l")
		} else {
			_, _ = ioWriteString(a.Out, diff)
		}
	}
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
	if state != nil {
		capHint := 8
		if state.accounts != nil && state.accounts.Len() > capHint {
			capHint = state.accounts.Len()
		}
		state.frameViews = make(map[string]tuiAccountView, capHint)
		defer func() { state.frameViews = nil }()
	}
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
