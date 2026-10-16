package app

import (
	"fmt"
	"strings"
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
func (a *Application) tuiPoolMeters(pools []poolView, short bool, barWidth int) string {
	slots := []struct{ id, long, short string }{
		{"gemini", "Gem", "G"},
		{"third_party", "C/G", "C"},
	}
	parts := make([]string, 0, len(slots))
	for _, slot := range slots {
		label := pick(short, slot.short, slot.long)
		var found *poolView
		for i := range pools {
			if pools[i].id == slot.id {
				found = &pools[i]
			}
		}
		if found == nil {
			parts = append(parts, a.p.Gray+label+" "+a.p.Track+strings.Repeat("░", barWidth)+a.p.Gray+fmt.Sprintf(" %4s", "—")+a.p.Reset)
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
	width = maxInt(1, width)
	tag := ""
	if width >= 40 {
		// Tags are user labels, so they stay quiet unless the row is selected.
		label := avatar(view.name, view.email, false)
		tag = " " + a.p.Gray + label + a.p.Reset
		if view.selected {
			tag = " " + view.avatar
		}
	}
	name := a.tuiIdentity(view.name)
	if view.selected {
		name = a.p.Bold + a.p.White + view.name + a.p.Reset
	}
	left := a.tuiSelectionMarker(view, state) + " " + a.tuiActiveMarker(view) + tag + " " + name
	if width >= 64 && lines == 2 {
		left += " " + a.tuiSecondary("<"+view.email+">")
	}
	word := a.tuiStatusColor(view.status) + a.p.Bold + view.status.String() + a.p.Reset
	if view.status == statusNoToken {
		word = a.p.Yellow + a.p.Bold + "⚠ NO TOKEN" + a.p.Reset
	}
	highlight := func(rows []string) []string {
		if !view.selected || a.p.SelectBg == "" {
			return rows
		}
		for i, row := range rows {
			rows[i] = a.p.SelectBg + strings.ReplaceAll(row, "\x1b[0m", "\x1b[0m"+a.p.SelectBg) + a.p.Reset
		}
		return rows
	}
	if lines == 1 {
		right := word
		if len(view.pools) > 0 && width >= 46 {
			barWidth := 4
			right = word + "  " + a.tuiPoolMeters(view.pools, true, barWidth)
		}
		room := maxInt(1, width-visibleWidth(right)-1)
		if visibleWidth(left) > room && tag != "" {
			// The tag is the first thing to give way to a full name.
			left = a.tuiSelectionMarker(view, state) + " " + a.tuiActiveMarker(view) + " " + name
		}
		return highlight([]string{fitVisible(fitVisible(left, room, a.p)+" "+right, width, a.p)})
	}
	first := rowSpread(left, word, width, a.p)
	second := "    "
	switch {
	case len(view.pools) > 0 && view.status != statusNoToken && view.status != statusAttention:
		if width < 40 {
			second += a.tuiPoolMeters(view.pools, true, 6)
		} else {
			second += a.tuiPoolMeters(view.pools, false, 8)
		}
		for _, pool := range view.pools {
			if pool.stale {
				second += "  " + a.p.Gray + "refresh due" + a.p.Reset
				break
			}
		}
	case view.needsAttention:
		second += a.p.Yellow + strings.TrimPrefix(tuiText(view.health), "⚠ ") + a.p.Reset
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
	width = maxInt(1, width)
	listLines = pick2(listLines == 1, 1, 2)
	if maxRows <= 0 {
		return nil
	}
	emails := state.visibleEmails()
	if len(emails) == 0 {
		return nil
	}
	ready, attention := state.partitionedEmails()
	now := a.renderTime()
	type block struct {
		isHeader bool
		email    string
		lines    []string
	}
	var blocks []block
	add := func(group []string) {
		for _, email := range group {
			view := a.tuiAccountView(state, email, now)
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
			spaced := make([]block, 0, len(blocks)*2)
			for i, b := range blocks {
				spaced = append(spaced, b)
				if !b.isHeader && i+1 < len(blocks) && !blocks[i+1].isHeader {
					spaced = append(spaced, block{isHeader: true, lines: []string{strings.Repeat(" ", width)}})
				}
			}
			blocks = spaced
		}
	}
	selected := 0
	for index, b := range blocks {
		if !b.isHeader && strings.EqualFold(b.email, state.selectedEmail) {
			selected = index
			break
		}
	}
	// Grow a window of whole blocks around the selection into capacity rows.
	window := func(capacity int) (int, int) {
		start, end := selected, selected+1
		used := len(blocks[selected].lines)
		for {
			grown := false
			if start > 0 && used+len(blocks[start-1].lines) <= capacity {
				start--
				used += len(blocks[start].lines)
				grown = true
			}
			if end < len(blocks) && used+len(blocks[end].lines) <= capacity {
				used += len(blocks[end].lines)
				end++
				grown = true
			}
			if !grown {
				return start, end
			}
		}
	}
	countAccounts := func(list []block) int {
		n := 0
		for _, b := range list {
			if !b.isHeader {
				n++
			}
		}
		return n
	}
	start, end := window(maxRows)
	// A marker row on each side that hides accounts takes room from the window.
	for pass := 0; pass < 3; pass++ {
		reserve := pick2(countAccounts(blocks[:start]) > 0, 1, 0) + pick2(countAccounts(blocks[end:]) > 0, 1, 0)
		if reserve == 0 {
			break
		}
		nextStart, nextEnd := window(maxRows - reserve)
		if nextStart == start && nextEnd == end {
			break
		}
		start, end = nextStart, nextEnd
	}
	hiddenAbove, hiddenBelow := countAccounts(blocks[:start]), countAccounts(blocks[end:])
	var rows []string
	if hiddenAbove > 0 {
		rows = append(rows, a.tuiMoreMarker("▲", hiddenAbove, width))
	}
	for _, b := range blocks[start:end] {
		rows = append(rows, b.lines...)
	}
	if hiddenBelow > 0 {
		rows = append(rows, a.tuiMoreMarker("▼", hiddenBelow, width))
	}
	if len(rows) > maxRows {
		rows = rows[:maxRows]
	}
	return rows
}

func pick2(condition bool, yes, no int) int {
	if condition {
		return yes
	}
	return no
}
