package app

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

// pointerRig drives tuiPointer the way the TUI loop does and records the keys
// a click presses.
type pointerRig struct {
	a       *Application
	state   *tuiState
	ptr     *tuiPointerState
	pressed []string
	clock   time.Time
}

func newPointerRig(a *Application, state *tuiState) *pointerRig {
	r := &pointerRig{a: a, state: state, clock: time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)}
	var dragging bool
	var thumb string
	var grab, thumbTop, lastCol, lastRow int
	var lastTime time.Time
	r.ptr = &tuiPointerState{
		dragging: &dragging, thumb: &thumb, grab: &grab, thumbTop: &thumbTop,
		lastTime: &lastTime, lastCol: &lastCol, lastRow: &lastRow,
		press:         func(key string) { r.pressed = append(r.pressed, key) },
		confirmSwitch: func() { r.pressed = append(r.pressed, "switch") },
		now:           func() time.Time { return r.clock },
	}
	r.draw()
	return r
}

func (r *pointerRig) draw() []string {
	return r.a.tuiLines(r.state, r.state.width-2, r.state.height)
}

// at sends a pointer event at a frame line and column.
func (r *pointerRig) at(action string, line, x int) bool {
	_, left := tuiFrameBox(r.state.width)
	return r.a.tuiPointer(r.state, fmt.Sprintf("mouse:%s:%d:%d", action, x+2+left, line+1), r.ptr)
}

// find is the first target of a kind (and key, when given).
func (r *pointerRig) find(t *testing.T, kind tuiHitKind, key string) tuiHit {
	t.Helper()
	for _, h := range r.state.hits {
		if h.kind == kind && (key == "" || h.key == key) {
			return h
		}
	}
	t.Fatalf("no target of kind %d key %q among %+v", kind, key, r.state.hits)
	return tuiHit{}
}

func wideRig() *pointerRig {
	a, state := listApp()
	a.p.KeyBg, a.p.BarBg, a.p.Track, a.p.Backdrop = "\x1b[48;5;238m", "\x1b[48;5;234m", "\x1b[38;5;238m", "\x1b[38;5;240m"
	state.motionEnabled = false
	state.width, state.height = 140, 40
	return newPointerRig(a, state)
}

func TestTheMoveReportDecodesAsAPointerMove(t *testing.T) {
	if got := decodeCSI([]byte("<35;10;5M")); got != "mouse:move:10:5" {
		t.Fatalf("motion with no button = %q", got)
	}
	if got := decodeCSI([]byte("<32;10;5M")); got != "mouse:drag:10:5" {
		t.Fatalf("motion with the left button = %q", got)
	}
}

func TestClickingAnAccountSelectsItAndASecondClickSwitches(t *testing.T) {
	r := wideRig()
	hit := r.find(t, hitAccount, "c@example.invalid")
	r.at("down", hit.top, hit.left+3)
	if r.state.selectedEmail != "c@example.invalid" || len(r.pressed) != 0 {
		t.Fatalf("selected %q, pressed %v", r.state.selectedEmail, r.pressed)
	}
	r.clock = r.clock.Add(200 * time.Millisecond)
	r.at("down", hit.top, hit.left+3)
	if strings.Join(r.pressed, ",") != "switch" {
		t.Fatalf("a second click pressed %v, want a switch", r.pressed)
	}
}

func TestHoverTintsTheAccountWithoutSelectingIt(t *testing.T) {
	r := wideRig()
	hit := r.find(t, hitAccount, "c@example.invalid")
	if !r.at("move", hit.top, hit.left+3) {
		t.Fatal("moving onto an account must redraw")
	}
	if r.at("move", hit.top, hit.left+4) {
		t.Fatal("moving within the same target must not redraw")
	}
	if r.state.selectedEmail != "b@example.invalid" || r.state.hover.key != "c@example.invalid" {
		t.Fatalf("hover changed the selection or missed the account: %q %+v", r.state.selectedEmail, r.state.hover)
	}
	lines := r.draw()
	if !strings.Contains(lines[hit.top], r.a.p.BarBg) {
		t.Fatalf("the hovered row has no tint: %q", lines[hit.top])
	}
	r.at("move", 0, 0)
	if r.state.hover.kind != 0 {
		t.Fatalf("leaving the account kept the hover: %+v", r.state.hover)
	}
}

func TestClickingAFooterKeyPressesIt(t *testing.T) {
	r := wideRig()
	chip := r.find(t, hitChip, "enter")
	r.at("down", chip.top, chip.left)
	if strings.Join(r.pressed, ",") != "enter" {
		t.Fatalf("pressed %v, want enter", r.pressed)
	}
	r.at("move", chip.top, chip.left)
	if lines := r.draw(); !strings.Contains(lines[chip.top], tuiAsBackground(r.a.p.Backdrop)) {
		t.Fatalf("the hovered key is not lit: %q", lines[chip.top])
	}
}

func TestDialogButtonsFollowThePointerAndPressOnClick(t *testing.T) {
	r := wideRig()
	r.state.openDialog(confirmDialog("switch", "c@example.invalid", "Gamma User"))
	r.draw()
	cancel := r.find(t, hitButton, "Cancel")
	r.at("move", cancel.top, cancel.left+1)
	if r.state.dialog.Index != cancel.index {
		t.Fatalf("hover left the choice on %d, want %d", r.state.dialog.Index, cancel.index)
	}
	switchButton := r.find(t, hitButton, "Switch")
	r.at("down", switchButton.top, switchButton.left+1)
	if r.state.dialog.Index != switchButton.index || strings.Join(r.pressed, ",") != "enter" {
		t.Fatalf("click chose %d and pressed %v", r.state.dialog.Index, r.pressed)
	}
	r.pressed = nil
	behind := r.find(t, hitAccount, "a@example.invalid")
	r.at("move", behind.top, behind.left+2)
	if r.state.hover.kind != 0 {
		t.Fatalf("an account behind the dialog took the hover: %+v", r.state.hover)
	}
	r.at("down", behind.top, behind.left+2)
	if strings.Join(r.pressed, ",") != "esc" || r.state.selectedEmail != "b@example.invalid" {
		t.Fatalf("a click on the backdrop pressed %v and selected %q; want esc and no change", r.pressed, r.state.selectedEmail)
	}
	r.pressed = nil
	r.at("down", 0, 0)
	if strings.Join(r.pressed, ",") != "esc" {
		t.Fatalf("a click beside the dialog pressed %v, want esc", r.pressed)
	}
}

func TestPaletteRowsRunOnClick(t *testing.T) {
	r := wideRig()
	r.state.beginPalette()
	r.draw()
	row := r.find(t, hitPalette, "")
	for _, h := range r.state.hits {
		if h.kind == hitPalette && h.index > row.index {
			row = h
			break
		}
	}
	r.at("down", row.top, row.left+2)
	if r.state.paletteIndex != row.index || strings.Join(r.pressed, ",") != "enter" {
		t.Fatalf("palette index %d, pressed %v; want %d and enter", r.state.paletteIndex, r.pressed, row.index)
	}
}

func TestTheDividerLightsUnderThePointerAndDrags(t *testing.T) {
	r := wideRig()
	divider := r.find(t, hitDivider, "")
	line := divider.top + 3
	r.at("move", line, divider.left+1)
	if lines := r.draw(); !strings.Contains(lines[line], r.a.p.Gray+"│") {
		t.Fatalf("the hovered divider is not lit: %q", lines[line])
	}
	before := r.state.splitOffset
	r.at("down", line, divider.left+1)
	r.at("drag", line, divider.left+1+6)
	r.at("up", line, divider.left+1+6)
	if r.state.splitOffset != before+6 || r.state.dragging {
		t.Fatalf("drag moved the split %d -> %d (dragging %v), want +6", before, r.state.splitOffset, r.state.dragging)
	}
}

func TestTheScrollbarPagesAndItsThumbDrags(t *testing.T) {
	a, state := stackedApp()
	state.accounts = sizesAccounts(30)
	state.selectedEmail = "user01@example.invalid"
	r := newPointerRig(a, state)
	bar := r.find(t, hitScrollbar, "list")
	sc := state.scrollState("list")
	start := sc.offset
	r.at("down", bar.bottom-1, bar.left)
	if sc.offset <= start {
		t.Fatalf("a click on the track below the thumb did not page down: %d -> %d", start, sc.offset)
	}
	r.draw()
	at, _ := scrollThumb(sc.offset, sc.total, sc.view)
	r.at("down", bar.top+at, bar.left)
	r.at("drag", bar.top, bar.left)
	r.at("up", bar.top, bar.left)
	if sc.offset != 0 {
		t.Fatalf("dragging the thumb to the top left the view at %d", sc.offset)
	}
}

func TestChipKeysNameOneKeyOrNone(t *testing.T) {
	for label, want := range map[string]string{"Enter": "enter", "^K": "ctrl-k", "?": "?", "q": "q", "Esc": "esc", "Any key": "esc", "↑↓": "", "←→": "", "Type": "", "[ ]": ""} {
		if got := chipKey(label); got != want {
			t.Errorf("chipKey(%q) = %q, want %q", label, got, want)
		}
	}
}

func TestTheMouseCanBeTurnedOff(t *testing.T) {
	t.Setenv("AGY_SWAP_MOUSE", "0")
	if tuiMouseEnabled() {
		t.Fatal("AGY_SWAP_MOUSE=0 must turn mouse reports off")
	}
	t.Setenv("AGY_SWAP_MOUSE", "")
	if !tuiMouseEnabled() {
		t.Fatal("the mouse is on by default")
	}
}
