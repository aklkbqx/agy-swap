package app

import (
	"strconv"
	"strings"
	"time"
)

// tuiPointerState is what a pointer event needs from the TUI loop: the drags
// in progress, the last click (for double clicks) and a way to press a key,
// switch accounts and set the pointer shape.
type tuiPointerState struct {
	dragging         *bool   // the divider or the rule
	thumb            *string // the scroll view whose thumb is dragged
	grab, thumbTop   *int
	lastTime         *time.Time
	lastCol, lastRow *int
	press            func(key string)
	confirmSwitch    func()
	shape            func(shape string)
	now              func() time.Time // the clock, for tests
}

// tuiPointer applies a "mouse:<action>:<col>:<row>" event. It reports whether
// the screen needs drawing again.
func (a *Application) tuiPointer(state *tuiState, key string, ptr *tuiPointerState) bool {
	parts := strings.Split(key, ":")
	if len(parts) != 4 {
		return false
	}
	action := parts[1]
	col, _ := strconv.Atoi(parts[2])
	row, _ := strconv.Atoi(parts[3])
	line, x := state.pointerAt(col, row)
	hit, onTarget := state.hitAt(line, x)
	if oneOfMode(state.mode, tuiDialogOpen, tuiPalette, tuiForm, tuiSearch) && !oneOfKind(hit.kind, hitButton, hitPalette, hitChip) {
		// Under a modal only its own targets and the key hints answer.
		hit, onTarget = tuiHit{}, false
	}
	now := time.Now()
	if ptr.now != nil {
		now = ptr.now()
	}
	switch action {
	case "move":
		if *ptr.dragging || *ptr.thumb != "" {
			return false
		}
		return a.tuiHover(state, hit, onTarget, ptr)
	case "drag":
		switch {
		case *ptr.dragging:
			if baseLeft, _, _, _, wide := state.wideSplitBounds(); wide {
				state.setSplit(x + 1 - 5 - baseLeft)
			} else {
				state.dragStackRule(row)
			}
			state.message, state.messageType = state.splitMessage(), "info"
			return true
		case *ptr.thumb != "":
			state.scrollToThumb(*ptr.thumb, line-*ptr.thumbTop-*ptr.grab)
			return true
		}
		return false
	case "up":
		if *ptr.dragging || *ptr.thumb != "" {
			*ptr.dragging, state.dragging, *ptr.thumb = false, false, ""
			return true
		}
		return false
	case "down":
	default:
		return false
	}
	double := now.Sub(*ptr.lastTime) < 350*time.Millisecond && absInt(col-*ptr.lastCol) <= 2 && absInt(row-*ptr.lastRow) <= 1
	*ptr.lastTime, *ptr.lastCol, *ptr.lastRow = now, col, row
	if !onTarget {
		// A click beside a dialog or the palette closes it, as Esc does.
		if state.mode == tuiDialogOpen || state.mode == tuiPalette {
			ptr.press("esc")
		}
		return false
	}
	switch hit.kind {
	case hitDivider, hitRule:
		if state.mode != tuiBrowse {
			return false
		}
		if double {
			state.resetSplit()
			state.message, state.messageType = "Split reset to default", "info"
			*ptr.dragging, state.dragging = false, false
		} else {
			*ptr.dragging, state.dragging = true, true
			state.message, state.messageType = state.splitMessage(), "info"
		}
	case hitAccount:
		if state.mode != tuiBrowse {
			return false
		}
		// A click selects an account; a second click on it switches.
		if strings.EqualFold(hit.key, state.selectedEmail) && double {
			ptr.confirmSwitch()
		} else {
			state.selectedEmail = hit.key
		}
	case hitScrollbar:
		sc := state.scrollState(hit.key)
		at, size := scrollThumb(sc.offset, sc.total, sc.view)
		switch r := line - hit.top; {
		case r < at:
			state.scrollBy(hit.key, -sc.view) // the track above the thumb pages up
		case r >= at+size:
			state.scrollBy(hit.key, sc.view)
		default:
			*ptr.thumb, *ptr.grab, *ptr.thumbTop = hit.key, r-at, hit.top
		}
	case hitChip:
		ptr.press(hit.key)
		return false
	case hitButton:
		if state.mode == tuiDialogOpen && state.dialog != nil {
			state.dialog.Index = hit.index
			ptr.press("enter")
		}
		return false
	case hitPalette:
		if state.mode == tuiPalette {
			state.paletteIndex = hit.index
			ptr.press("enter")
		}
		return false
	default:
		return false
	}
	return true
}

// tuiHover follows the pointer: it shows what a click would do without
// changing anything a key would see, except that over a dialog button or a
// palette row the choice follows the pointer, as in a menu.
func (a *Application) tuiHover(state *tuiState, hit tuiHit, onTarget bool, ptr *tuiPointerState) bool {
	if !onTarget {
		hit = tuiHit{}
	}
	changed := false
	switch {
	case hit.kind == hitButton && state.mode == tuiDialogOpen && state.dialog != nil && state.dialog.Index != hit.index:
		state.dialog.Index, changed = hit.index, true
	case hit.kind == hitPalette && state.mode == tuiPalette && state.paletteIndex != hit.index:
		state.paletteIndex, changed = hit.index, true
	}
	if hit.kind != state.hover.kind || hit.key != state.hover.key || hit.index != state.hover.index {
		state.hover, changed = hit, true
	}
	if shape := pointerShape(hit.kind); ptr.shape != nil && tuiPointerShapes() && shape != state.shape {
		state.shape = shape
		ptr.shape(shape)
	}
	return changed
}
