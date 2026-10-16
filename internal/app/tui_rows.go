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
			parts = append(parts, a.p.Gray+label+" "+a.p.DarkGray+strings.Repeat("░", barWidth)+a.p.Gray+fmt.Sprintf(" %4s", "—")+a.p.Reset)
			continue
		}
		color := poolColor(a.p, found.state)
		bar := renderFractionalBar(found.limiting.fraction, barWidth, color, a.p.DarkGray, a.p.Reset)
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
		tag = " " + view.avatar
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
	if lines == 1 {
		right := word
		if len(view.pools) > 0 && width >= 46 {
			barWidth := 4
			right = word + "  " + a.tuiPoolMeters(view.pools, true, barWidth)
		}
		room := maxInt(1, width-visibleWidth(right)-1)
		return []string{fitVisible(fitVisible(left, room, a.p)+" "+right, width, a.p)}
	}
	first := rowSpread(left, word, width, a.p)
	second := "    "
	switch {
	case len(view.pools) > 0 && view.status != statusNoToken && view.status != statusAttention:
		second += a.tuiPoolMeters(view.pools, false, 8)
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
	return []string{fitVisible(first, width, a.p), fitVisible(second, width, a.p)}
}

func (a *Application) tuiAccountRows(state *tuiState, width, maxRows int) []string {
	return a.tuiAccountRowsFor(state, width, maxRows, 2)
}

// tuiAccountRowsFor lists the accounts in blocks of listLines lines, grouped
// under READY and ATTENTION REQUIRED dividers when some account needs
// attention, windowed around the selection when they do not all fit.
func (a *Application) tuiAccountRowsFor(state *tuiState, width, maxRows, listLines int) []string {
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
	selected := 0
	for index, b := range blocks {
		if !b.isHeader && strings.EqualFold(b.email, state.selectedEmail) {
			selected = index
			break
		}
	}
	start, end := selected, selected+1
	used := len(blocks[selected].lines)
	for {
		grown := false
		if start > 0 && used+len(blocks[start-1].lines) <= maxRows {
			start--
			used += len(blocks[start].lines)
			grown = true
		}
		if end < len(blocks) && used+len(blocks[end].lines) <= maxRows {
			used += len(blocks[end].lines)
			end++
			grown = true
		}
		if !grown {
			break
		}
	}
	var rows []string
	for _, b := range blocks[start:end] {
		rows = append(rows, b.lines...)
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
