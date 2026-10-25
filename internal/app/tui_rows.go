package app

import (
	"fmt"
	"strings"
	"time"
)

type meterLabels uint8

const (
	labelsAbbrev meterLabels = iota // Gem, C/G
	labelsShort                     // G, C
	labelsFull                      // Gemini, Claude/GPT
)

func (a *Application) tuiStatusColor(status tuiAccountStatus) string {
	switch status {
	case statusReady:
		return a.p.Green
	case statusLimited:
		return a.p.Red
	case statusPending:
		return a.p.Gray
	}
	return a.p.Yellow
}

// tuiPoolMeters draws one meter per pool, Gemini first: a label, a fractional
// bar and the percentage. A pool the account does not have shows an empty track.
func (a *Application) tuiPoolMeters(pools []poolView, labels meterLabels, barWidth int) string {
	slots := []struct{ id, long, short, full string }{
		{"gemini", "Gem", "G", "Gemini"},
		{"third_party", "C/G", "C", "Claude/GPT"},
	}
	parts := make([]string, 0, len(slots))
	for _, slot := range slots {
		label := slot.long
		switch labels {
		case labelsShort:
			label = slot.short
		case labelsFull:
			label = slot.full
		}
		var found *poolView
		for i := range pools {
			if pools[i].id == slot.id {
				found = &pools[i]
			}
		}
		if found == nil {
			parts = append(parts, a.p.Gray+label+" "+a.p.Track+strings.Repeat("▂", barWidth)+a.p.Gray+fmt.Sprintf(" %4s", "—")+a.p.Reset)
			continue
		}
		color := poolColor(a.p, found.state)
		bar := renderMeter(tuiMeterStyle, found.limiting.fraction, barWidth, color, a.p.Track, a.p.Reset)
		parts = append(parts, a.p.Gray+label+a.p.Reset+" "+bar+" "+color+fmt.Sprintf("%4s", poolPercent(found.limiting.fraction))+a.p.Reset)
	}
	return strings.Join(parts, "  ")
}

// tuiAccountBlock is one account in the list: an identity line with the status
// word on the right, then (in two-line mode) the pool meters.
func (a *Application) tuiAccountBlock(view tuiAccountView, state *tuiState, width, lines int) []string {
	// The last columns stay empty: air on the right inside the selection and hover tints.
	full := maxInt(1, width)
	padRight := 1
	if full >= 36 {
		padRight = 2
	}
	width = maxInt(1, full-padRight)
	tag := "" // the name says who it is; initials would only repeat it
	name, activeWord := a.tuiActiveName(view)
	if activeWord != "" {
		name += " " + activeWord
	}
	left := a.tuiSelectionMarker(view, state) + " " + a.tuiActiveMarker(view) + tag + " " + name
	if width >= 64 && lines == 2 {
		left += " " + a.tuiSecondary("<"+view.email+">")
	}
	word := a.tuiStatusColor(view.status) + a.p.Bold + view.status.String() + a.p.Reset
	if view.statusFlash && a.p.BodyBg != "" {
		// A status that just changed shows once on its own colour.
		word = tuiAsBackground(a.tuiStatusColor(view.status)) + tuiAsForeground(a.p.BodyBg) + a.p.Bold + view.status.String() + a.p.Reset
	}
	if view.status == statusNoToken {
		word = a.p.Yellow + a.p.Bold + "⚠ NO TOKEN" + a.p.Reset
	}
	highlight := func(rows []string) []string {
		for i, row := range rows {
			rows[i] = fitVisible(row, width, a.p) + strings.Repeat(" ", full-width)
		}
		if !view.selected || a.p.SelectBg == "" || view.banded {
			return rows
		}
		for i, row := range rows {
			rows[i] = a.p.SelectBg + strings.ReplaceAll(row, "\x1b[0m", "\x1b[0m"+a.p.SelectBg) + a.p.Reset
		}
		return rows
	}
	if lines == 1 {
		right := word
		if len(view.pools) > 0 && view.status != statusVerify && view.status != statusNoToken && view.status != statusAttention && width >= 46 {
			barWidth := 4
			right = word + "  " + a.tuiPoolMeters(view.pools, labelsShort, barWidth)
		}
		room := maxInt(1, width-visibleWidth(right)-1)
		if visibleWidth(left) > room && tag != "" {
			// The tag is the first thing to give way to a full name.
			left = a.tuiSelectionMarker(view, state) + " " + a.tuiActiveMarker(view) + " " + name
		}
		if visibleWidth(left) > room && activeWord != "" {
			// Then the word ACTIVE; the green name and dot still say it.
			plain, _ := a.tuiActiveName(view)
			left = a.tuiSelectionMarker(view, state) + " " + a.tuiActiveMarker(view) + " " + plain
		}
		return highlight([]string{fitVisible(fitVisible(left, room, a.p)+" "+right, width, a.p)})
	}
	first := rowSpread(left, word, width, a.p)
	second := a.tuiSelectionGutter(view) + "   "
	switch {
	case view.status == statusVerify:
		hint := " · run agy for link"
		if view.verify != nil && view.verify.URL != "" {
			hint = " · press w"
		}
		second += a.p.Yellow + "needs Google verification" + a.p.Reset + a.p.Gray + hint + a.p.Reset
	case len(view.pools) > 0 && view.status != statusNoToken && view.status != statusAttention:
		switch {
		case width < 40:
			second += a.tuiPoolMeters(view.pools, labelsShort, 6)
		case width >= 60:
			// Bars take whatever the names and percentages leave over.
			second += a.tuiPoolMeters(view.pools, labelsFull, minInt(40, maxInt(8, (width-34)/2)))
		default:
			second += a.tuiPoolMeters(view.pools, labelsAbbrev, maxInt(8, minInt(14, (width-24)/2)))
		}
		for _, pool := range view.pools {
			if pool.stale {
				second += "  " + a.p.Gray + "refresh due" + a.p.Reset
				break
			}
		}
	case view.needsAttention:
		second += a.p.Yellow + strings.TrimPrefix(tuiText(view.health), "⚠ ") + a.p.Reset
		if view.status == statusNoToken {
			second += a.p.Gray + " · agy-swap add" + a.p.Reset
		}
	default:
		second += a.p.Gray + tuiText(view.health) + a.p.Reset
	}
	return highlight([]string{fitVisible(first, width, a.p), fitVisible(second, width, a.p)})
}

func (a *Application) tuiAccountRows(state *tuiState, width, maxRows int) []string {
	return a.tuiAccountRowsFor(state, width, maxRows, 2)
}

// tuiAccountRowsFor lists the accounts in blocks of listLines lines, grouped
// under READY and ATTENTION REQUIRED dividers when some account needs
// attention, windowed around the selection when they do not all fit.
func (a *Application) tuiAccountRowsFor(state *tuiState, width, maxRows, listLines int) []string {
	return a.tuiAccountRowsSpaced(state, width, maxRows, listLines, true)
}

// tuiAccountRowsSpaced is tuiAccountRowsFor with the blank lines between
// accounts optional, so a caller can first measure the plain list.
func (a *Application) tuiAccountRowsSpaced(state *tuiState, width, maxRows, listLines int, allowSpacing bool) []string {
	rows, _ := a.tuiListRows(state, width, maxRows, listLines, allowSpacing, true)
	return rows
}

// tuiAccountRowsMeasure is the list as it would be drawn, without moving its
// scroll position or the selection band: for working out a layout.
func (a *Application) tuiAccountRowsMeasure(state *tuiState, width, maxRows, listLines int, allowSpacing bool) []string {
	rows, _ := a.tuiListRows(state, width, maxRows, listLines, allowSpacing, false)
	return rows
}

// tuiAccountRowsHits also returns, for every row, the account it belongs to
// ("" for dividers and spacing), which is what a mouse click needs.
func (a *Application) tuiAccountRowsHits(state *tuiState, width, maxRows, listLines int, allowSpacing bool) ([]string, []string) {
	return a.tuiListRows(state, width, maxRows, listLines, allowSpacing, true)
}

// tuiListRows draws the list into maxRows rows. When it is taller it scrolls,
// with a bar at the right edge, and keeps the selected account in view. Only
// a commit moves the scroll position and the selection band.
func (a *Application) tuiListRows(state *tuiState, width, maxRows, listLines int, allowSpacing, commit bool) ([]string, []string) {
	width = maxInt(1, width)
	listLines = pick2(listLines == 1, 1, 2)
	if maxRows <= 0 {
		return nil, nil
	}
	if len(state.visibleEmails()) == 0 {
		return nil, nil
	}
	now := a.renderTime()
	banded := a.p.SelectBg != ""
	rows, hits := a.tuiListAllRows(state, width, maxRows, listLines, allowSpacing, now)
	if len(rows) > maxRows && width > 12 {
		rows, hits = a.tuiListAllRows(state, width-2, maxRows, listLines, allowSpacing, now) // room for the bar
	}
	top, end := -1, -1
	for i, email := range hits {
		if email != "" && strings.EqualFold(email, state.selectedEmail) {
			if top < 0 {
				top = i
			}
			end = i + 1
		}
	}
	offset := state.scrollTo("list", state.selectedEmail, top, end, len(rows), maxRows, commit)
	if banded && commit {
		a.tuiSelectionBand(state, rows, hits, width, now)
	}
	if commit && a.p.BarBg != "" && state.hover.kind == hitAccount && !strings.EqualFold(state.hover.key, state.selectedEmail) {
		// The account under the pointer gets a faint tint: a click selects it.
		hTop, hEnd := -1, -1
		for i, email := range hits {
			if strings.EqualFold(email, state.hover.key) {
				if hTop < 0 {
					hTop = i
				}
				hEnd = i + 1
			}
		}
		if hTop >= 0 {
			if listLines == 2 && hTop > 0 && hits[hTop-1] == "" && strings.TrimSpace(stripEscapes(rows[hTop-1])) == "" {
				hTop--
			}
			if listLines == 2 && hEnd < len(rows) && hits[hEnd] == "" && strings.TrimSpace(stripEscapes(rows[hEnd])) == "" {
				hEnd++
			}
			for i := hTop; i < hEnd; i++ {
				rows[i] = a.tuiHoverRow(rows[i], width)
			}
		}
	}
	if len(rows) <= maxRows {
		return rows, hits
	}
	hits = hits[offset:minInt(len(hits), offset+maxRows)]
	if width > 12 {
		return a.tuiScrollWindow(rows, offset, maxRows, width), hits
	}
	return rows[offset:minInt(len(rows), offset+maxRows)], hits
}

// tuiListAllRows is every row of the list, before scrolling: account blocks,
// grouped under READY and ATTENTION REQUIRED dividers when some account needs
// attention, with a blank row between accounts when they all fit that way.
func (a *Application) tuiListAllRows(state *tuiState, width, maxRows, listLines int, allowSpacing bool, now time.Time) ([]string, []string) {
	ready, attention := state.partitionedEmails()
	type block struct {
		isHeader bool
		email    string
		lines    []string
	}
	var blocks []block
	// With a tint to draw, the selection is a band laid over the rows after
	// they are placed, so it can slide from one account to the next.
	banded := a.p.SelectBg != ""
	add := func(group []string) {
		for _, email := range group {
			view := a.tuiAccountView(state, email, now)
			view.banded = banded
			blocks = append(blocks, block{email: email, lines: a.tuiAccountBlock(view, state, width, listLines)})
		}
	}
	if len(attention) == 0 {
		add(ready)
	} else {
		if len(ready) > 0 {
			blocks = append(blocks, block{isHeader: true, lines: []string{a.tuiSectionDividerRow("READY", len(ready), false, width)}})
			add(ready)
		}
		blocks = append(blocks, block{isHeader: true, lines: []string{a.tuiSectionDividerRow("ATTENTION REQUIRED", len(attention), true, width)}})
		add(attention)
	}
	if listLines == 2 && allowSpacing {
		// With rows to spare, a blank line between accounts lets each read as one.
		total, accountsShown := 0, 0
		for _, b := range blocks {
			total += len(b.lines)
			if !b.isHeader {
				accountsShown++
			}
		}
		if total+accountsShown-1 <= maxRows {
			// One blank row is enough air; more makes the list read as scattered.
			// With two rows more to spare, the selected account gets a row of
			// padding above and below inside its tint; the blank rows that part
			// it from its neighbours stay outside as margin.
			padded := banded && total+accountsShown-1+2 <= maxRows
			spaced := make([]block, 0, len(blocks)*2)
			for i, b := range blocks {
				if padded && !b.isHeader && strings.EqualFold(b.email, state.selectedEmail) {
					pad := strings.Repeat(" ", width) // the band tints it
					b.lines = append(append([]string{pad}, b.lines...), pad)
				}
				spaced = append(spaced, b)
				if !b.isHeader && i+1 < len(blocks) {
					spaced = append(spaced, block{isHeader: true, lines: []string{strings.Repeat(" ", width)}})
				}
			}
			blocks = spaced
		}
	}
	var rows, hits []string
	for _, b := range blocks {
		for _, line := range b.lines {
			rows = append(rows, line)
			hits = append(hits, b.email)
		}
	}
	return rows, hits
}

// tuiSelectionBand tints the selected account's rows and draws the selection
// bar down them. When the selection moves, the band slides from the rows it
// covered to the new ones instead of jumping.
func (a *Application) tuiSelectionBand(state *tuiState, rows, hits []string, width int, now time.Time) {
	top, height := -1, 0
	for i, email := range hits {
		if email != "" && strings.EqualFold(email, state.selectedEmail) {
			if top < 0 {
				top = i
			}
			height++
		}
	}
	if top < 0 {
		return
	}
	// The band slides only when the selection moves; when the same account's
	// rows move (the list reflows or scrolls), it goes with them at once.
	if tween := state.tweens["selection"]; tween != nil && tween.to != float64(top) && strings.EqualFold(state.bandEmail, state.selectedEmail) {
		state.settle("selection", float64(top))
	}
	state.bandEmail = state.selectedEmail
	pos := state.follow("selection", float64(top), tuiMotionSlide, now)
	if a.p.BodyBg != "" && lerpColor(a.p.BodyBg, a.p.SelectBg, 0.5) != a.p.SelectBg {
		for i := range rows {
			overlap := min(pos+float64(height), float64(i+1)) - max(pos, float64(i))
			if overlap >= 0.99 {
				rows[i] = a.tuiBandRow(rows[i], width)
			} else if overlap > 0.01 {
				rows[i] = a.tuiBandRowAlpha(rows[i], width, overlap)
			}
		}
		return
	}
	at := int(pos + 0.5)
	for i := max(0, at); i < min(len(rows), at+height); i++ {
		rows[i] = a.tuiBandRow(rows[i], width)
	}
}

// tuiBandRow is one row under the selection band: the tint behind it and the
// selection bar in its first column.
func (a *Application) tuiBandRow(row string, width int) string {
	return a.tuiBandRowAlpha(row, width, 1)
}

func (a *Application) tuiBandRowAlpha(row string, width int, alpha float64) string {
	bg := a.p.SelectBg
	barColor := a.p.Orange
	if alpha < 1 && a.p.BodyBg != "" {
		bg = lerpColor(a.p.BodyBg, a.p.SelectBg, alpha)
		barColor = lerpColor(tuiAsForeground(a.p.BodyBg), a.p.Orange, alpha)
	}
	rest := strings.ReplaceAll(visibleSpan(row, 1, width), a.p.Reset, a.p.Reset+bg)
	return bg + barColor + tuiSelectionBar + a.p.Reset + bg + rest + a.p.Reset
}

func (a *Application) tuiHoverRow(row string, width int) string {
	bg := a.p.BarBg
	padded := fitVisible(row, width, a.p)
	rest := strings.ReplaceAll(visibleSpan(padded, 0, width), a.p.Reset, a.p.Reset+bg)
	return bg + rest + a.p.Reset
}

func pick2(condition bool, yes, no int) int {
	if condition {
		return yes
	}
	return no
}
