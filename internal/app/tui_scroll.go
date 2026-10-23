package app

import (
	"strconv"
	"strings"
)

// A scroll view shows part of a list or panel that is taller than its room,
// with a one-column bar at its right edge. The mouse wheel over a view scrolls
// it; moving the selection brings the selected rows back into view.

type tuiScroll struct {
	offset, total, view int
	// follow is what the view last brought into sight, the selected account:
	// when it changes, the view moves to show it again. loose says the wheel
	// has scrolled away from it since; until then the view keeps it in sight
	// however the rows reflow.
	follow string
	loose  bool
}

func (s *tuiState) scrollState(key string) *tuiScroll {
	if s.scrolls == nil {
		s.scrolls = map[string]*tuiScroll{}
	}
	sc, ok := s.scrolls[key]
	if !ok {
		sc = &tuiScroll{}
		s.scrolls[key] = sc
	}
	return sc
}

// scrollTo is the offset of a view of view rows over total rows. When follow
// changed since the last frame, rows [top, end) come into view first. Only a
// commit stores the result, so measuring a layout never moves a view.
func (s *tuiState) scrollTo(key, follow string, top, end, total, view int, commit bool) int {
	sc := s.scrollState(key)
	offset := sc.offset
	if (follow != sc.follow || !sc.loose) && top >= 0 {
		if top < offset {
			offset = top
		} else if end > offset+view {
			offset = end - view
		}
	}
	offset = maxInt(0, minInt(offset, total-view))
	if commit {
		if follow != sc.follow {
			sc.loose = false
		}
		sc.offset, sc.total, sc.view, sc.follow = offset, total, view, follow
	}
	return offset
}

// scrollBy scrolls a view by delta rows. It reports false when the view has
// nothing out of sight, so the wheel can do something else there.
func (s *tuiState) scrollBy(key string, delta int) bool {
	sc := s.scrollState(key)
	if sc.total <= sc.view {
		return false
	}
	sc.offset = maxInt(0, minInt(sc.offset+delta, sc.total-sc.view))
	sc.loose = true
	return true
}

// addScrollHits records a scroll view at frame lines [top, bottom) and
// columns [left, right) for the wheel, and its bar column when it scrolls.
func (s *tuiState) addScrollHits(key string, top, bottom, left, right, bar int) {
	s.addHit(tuiHit{kind: hitScroll, key: key, top: top, bottom: bottom, left: left, right: right})
	if sc := s.scrollState(key); sc.total > sc.view {
		s.addHit(tuiHit{kind: hitScrollbar, key: key, top: top, bottom: minInt(bottom, top+sc.view), left: bar, right: bar + 1})
	}
}

// addAccountHits records the list rows drawn at frame line first, one hit per
// row of an account.
func (s *tuiState) addAccountHits(hits []string, first, left, right int) {
	for i, email := range hits {
		if email != "" {
			s.addHit(tuiHit{kind: hitAccount, key: email, top: first + i, bottom: first + i + 1, left: left, right: right})
		}
	}
}

// scrollThumb is where a view's thumb sits among its rows and how tall it is.
func scrollThumb(offset, total, view int) (at, size int) {
	if total <= view || view <= 0 {
		return 0, view
	}
	size = maxInt(1, (view*view+total/2)/total)
	at = (offset*(view-size) + (total-view)/2) / (total - view)
	return at, size
}

// scrollToThumb scrolls a view so its thumb's top sits at row at of the view.
func (s *tuiState) scrollToThumb(key string, at int) {
	sc := s.scrollState(key)
	if sc.total <= sc.view {
		return
	}
	_, size := scrollThumb(sc.offset, sc.total, sc.view)
	span := maxInt(1, sc.view-size)
	sc.offset = maxInt(0, minInt(sc.total-sc.view, (at*(sc.total-sc.view)+span/2)/span))
	sc.loose = true
}

// tuiScrollWindow cuts rows, drawn width-2 wide, to view rows from offset and
// puts the scroll bar in the last column.
func (a *Application) tuiScrollWindow(rows []string, offset, view, width int) []string {
	total := len(rows)
	end := minInt(total, offset+view)
	at, thumb := scrollThumb(offset, total, view)
	out := make([]string, 0, view)
	for i, row := range rows[offset:end] {
		cell := a.p.Track + "│" + a.p.Reset
		if i >= at && i < at+thumb {
			cell = a.p.Gray + "▐" + a.p.Reset
		}
		out = append(out, fitVisible(row, width-2, a.p)+" "+cell)
	}
	return out
}

// tuiScrollDetail fits the selected account's gauges into view rows. When
// even their smallest form is taller, they scroll, starting from the top for
// each account; render draws them again narrower, to leave room for the bar.
func (a *Application) tuiScrollDetail(state *tuiState, detail []string, view, width int, render func(width int) []string) []string {
	if view <= 0 {
		return nil
	}
	follow := state.selectedEmail
	if len(detail) <= view || width <= 12 {
		state.scrollTo("detail", follow, 0, 0, len(detail), view, true)
		if len(detail) > view {
			detail = detail[:view]
		}
		return detail
	}
	detail = render(width - 2)
	offset := state.scrollTo("detail", follow, 0, 0, len(detail), view, true)
	return a.tuiScrollWindow(detail, offset, view, width)
}

// wheelEvent reads a wheel key, "wheel-up:col:row" or "wheel-down:col:row":
// the direction (-1 up, 1 down, 0 for another key) and where the pointer was.
func wheelEvent(key string) (dir, col, row int) {
	switch {
	case strings.HasPrefix(key, "wheel-up"):
		dir = -1
	case strings.HasPrefix(key, "wheel-down"):
		dir = 1
	default:
		return 0, 0, 0
	}
	parts := strings.Split(key, ":")
	if len(parts) == 3 {
		col, _ = strconv.Atoi(parts[1])
		row, _ = strconv.Atoi(parts[2])
	}
	return dir, col, row
}
