package app

import (
	"fmt"
	"strings"
	"time"
)

// shortDuration is a duration in at most two units: 5d 3h, 16h 0m, 41m.
func shortDuration(d time.Duration) string {
	minutes := int(d.Minutes())
	if minutes < 0 {
		minutes = 0
	}
	switch {
	case minutes >= 24*60:
		return fmt.Sprintf("%dd %dh", minutes/(24*60), minutes%(24*60)/60)
	case minutes >= 60:
		return fmt.Sprintf("%dh %dm", minutes/60, minutes%60)
	}
	return fmt.Sprintf("%dm", minutes)
}

// windowAround picks which of n items to show when only capacity rows fit and
// a marker row is needed on each side that hides something.
func windowAround(n, selected, capacity int) (start, end int) {
	if capacity >= n {
		return 0, n
	}
	visible := maxInt(1, capacity-2)
	start = maxInt(0, minInt(selected-visible/2, n-visible))
	end = start + visible
	if start == 0 {
		end = minInt(n, maxInt(1, capacity-1))
	} else if end == n {
		start = maxInt(0, n-maxInt(1, capacity-1))
	}
	return start, end
}

func (a *Application) tuiMoreMarker(arrow string, count, width int) string {
	return fitVisible(a.p.Gray+"  "+arrow+fmt.Sprintf(" %d more", count)+a.p.Reset, width, a.p)
}

// tuiOverviewCell is one pool of one account: a meter, the percentage and,
// when there is room, the next time that matters (the reset, or when a used-up
// pool is back).
func (a *Application) tuiOverviewCell(pools []poolView, id string, barWidth int, withReset bool, now time.Time) string {
	var pool *poolView
	for i := range pools {
		if pools[i].id == id {
			pool = &pools[i]
		}
	}
	reset := ""
	var cell string
	if pool == nil {
		cell = a.p.Track + strings.Repeat("▄", barWidth) + a.p.Reset + a.p.Gray + fmt.Sprintf(" %4s", "—") + a.p.Reset
	} else {
		color := poolColor(a.p, pool.state)
		cell = renderMeter(tuiMeterStyle, pool.limiting.fraction, barWidth, color, a.p.Track, a.p.Reset) + " " + color + fmt.Sprintf("%4s", poolPercent(pool.limiting.fraction)) + a.p.Reset
		switch {
		case pool.state == poolUsedUp && pool.backIn > 0:
			reset = a.p.Red + "back " + shortDuration(pool.backIn) + a.p.Reset
		case !pool.limiting.resetAt.IsZero() && pool.limiting.resetAt.After(now):
			reset = a.p.Gray + shortDuration(pool.limiting.resetAt.Sub(now)) + a.p.Reset
		case pool.stale:
			reset = a.p.Gray + "due" + a.p.Reset
		}
	}
	if withReset {
		return cell + " " + fitVisible(reset, 12, a.p)
	}
	return cell
}

// tuiOverviewRows is the whole-fleet table: one line per account with its
// status word and a meter for each pool. It is the quota overview screen and
// the extra section of a tall dashboard. The first row is the column titles.
func (a *Application) tuiOverviewRows(state *tuiState, width, maxRows int) []string {
	width = maxInt(1, width)
	if maxRows < 2 {
		return nil
	}
	emails := state.visibleEmails()
	if len(emails) == 0 {
		return nil
	}
	now := a.renderTime()
	bar, withReset, statusWidth := 8, true, 15
	fixed := 2 + 1 + statusWidth + 1 + 26 + 1 + 26
	switch {
	case width >= 89:
	case width >= 61:
		withReset, fixed = false, 2+1+statusWidth+1+13+1+13
	case width >= 56:
		bar, withReset, fixed = 6, false, 2+1+statusWidth+1+11+1+11
	default:
		var rows []string
		start, end := windowAround(len(emails), indexOf(emails, state.selectedEmail), maxRows)
		if start > 0 {
			rows = append(rows, a.tuiMoreMarker("▲", start, width))
		}
		for _, email := range emails[start:end] {
			rows = append(rows, a.tuiAccountBlock(a.tuiAccountView(state, email, now), state, width, 1)...)
		}
		if end < len(emails) {
			rows = append(rows, a.tuiMoreMarker("▼", len(emails)-end, width))
		}
		return rows
	}
	nameWidth := maxInt(12, minInt(30, width-fixed))
	cellWidth := pick2(withReset, 26, bar+1+4)
	header := a.p.Bold + a.p.Gray + "   " + fitVisible("ACCOUNT", nameWidth-1, a.p) + " " + fitVisible("STATUS", statusWidth, a.p) + " " + fitVisible("GEMINI", cellWidth, a.p) + " " + fitVisible("CLAUDE/GPT", cellWidth, a.p) + a.p.Reset
	rows := []string{fitVisible(header, width, a.p)}
	capacity := maxRows - 1
	start, end := windowAround(len(emails), indexOf(emails, state.selectedEmail), capacity)
	if start > 0 {
		rows = append(rows, a.tuiMoreMarker("▲", start, width))
	}
	for _, email := range emails[start:end] {
		view := a.tuiAccountView(state, email, now)
		name := a.tuiIdentity(view.name)
		if view.selected {
			name = a.p.Bold + a.p.White + view.name + a.p.Reset
		}
		status := a.tuiStatusColor(view.status) + a.p.Bold + view.status.String() + a.p.Reset
		line := a.tuiSelectionMarker(view, state) + a.tuiActiveMarker(view) + " " +
			fitVisible(name, nameWidth-1, a.p) + " " + fitVisible(status, statusWidth, a.p) + " " +
			fitVisible(a.tuiOverviewCell(view.pools, "gemini", bar, withReset, now), cellWidth, a.p) + " " +
			fitVisible(a.tuiOverviewCell(view.pools, "third_party", bar, withReset, now), cellWidth, a.p)
		line = fitVisible(line, width, a.p)
		if view.selected && a.p.SelectBg != "" {
			line = a.p.SelectBg + strings.ReplaceAll(line, "\x1b[0m", "\x1b[0m"+a.p.SelectBg) + a.p.Reset
		}
		rows = append(rows, line)
	}
	if end < len(emails) {
		rows = append(rows, a.tuiMoreMarker("▼", len(emails)-end, width))
	}
	return rows
}

func indexOf(list []string, target string) int {
	for i, item := range list {
		if strings.EqualFold(item, target) {
			return i
		}
	}
	return 0
}
