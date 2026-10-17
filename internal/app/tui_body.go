package app

import (
	"fmt"
	"strings"
)

// tuiDetailMaxWidth caps the gauge area so two panels never stretch across a huge window.
const tuiDetailMaxWidth = 108

func (a *Application) tuiListTitle(state *tuiState) string {
	return a.tuiSectionTitle("ACCOUNTS") + "  " + a.p.Gray + fmt.Sprintf("%d", len(state.visibleEmails())) + "  ❯ selected · ● active" + a.p.Reset
}

func (a *Application) tuiEmptyListRows(state *tuiState, width, height int) []string {
	if state.accounts == nil || state.accounts.Len() == 0 {
		return a.tuiWelcomeRows(width, maxInt(1, height))
	}
	return []string{a.p.Gray + "No matching accounts" + a.p.Reset + " · press / to clear search"}
}

// tuiPoolBody is the dashboard body for every terminal size: the layout plan
// decides the arrangement and how much detail the gauges carry.
func (a *Application) tuiPoolBody(state *tuiState, width, height int) []string {
	g := newTUIGeometry(width, height+8)
	g.bodyRows = maxInt(1, height)
	state.bodySplit = 0
	state.listHits = nil
	if state.accounts == nil || state.accounts.Len() == 0 {
		return a.tuiEmptyBody(g)
	}
	plan := tuiLayoutPlanFor(g.frameWidth, g.frameHeight)
	if plan.arrangement == arrWide {
		return a.tuiPoolWideBody(state, g, plan)
	}
	return a.tuiPoolColumnBody(state, g, plan)
}

// tuiFitPlan keeps the richest digit mode whose detail fits the rows on offer,
// so a short terminal shows plain limit lines instead of a cut-off panel.
func (a *Application) tuiFitPlan(state *tuiState, width, budget int, plan tuiLayoutPlan) tuiLayoutPlan {
	var candidates []tuiLayoutPlan
	with := func(digits tuiDigits, panels tuiPanels, tight bool) {
		candidate := plan
		candidate.digits, candidate.panels, candidate.tight = digits, panels, tight
		candidates = append(candidates, candidate)
	}
	switch plan.digits {
	case digitsFive:
		with(digitsFive, plan.panels, false)
		with(digitsThree, plan.panels, false)
	case digitsThree:
		with(digitsThree, plan.panels, false)
	}
	if width >= 44 {
		with(digitsInline, panelsStack, false)
	}
	with(digitsNone, plan.panels, false)
	with(digitsNone, plan.panels, true)
	for _, candidate := range candidates {
		if len(a.tuiGaugeDetail(state, width, 0, candidate)) <= budget {
			return candidate
		}
	}
	return candidates[len(candidates)-1]
}

// tuiEmptyBody is the first screen: no accounts, so one column of welcome text
// across the whole width instead of an empty list beside an empty detail.
func (a *Application) tuiEmptyBody(g tuiGeometry) []string {
	lines := []string{a.p.Gray + "├" + strings.Repeat("─", g.frameWidth-2) + "┤" + a.p.Reset}
	for _, row := range a.tuiWelcomeRows(g.contentWidth, maxInt(1, g.bodyRows-2)) {
		lines = append(lines, frameRow(row, g, a.p))
	}
	return fitBodyLines(lines, g, a.p)
}

// tuiWideSplit is the one place that decides how wide the account list is in
// the two-column dashboard. The list grows a little on very wide screens; the
// detail pane keeps at least 44 columns and the list at least 34. The mouse
// drag, the [ ] keys and the renderer all read it, so they cannot disagree.
func tuiWideSplit(frameWidth, offset int) (base, minLeft, maxLeft, left int) {
	totalAvail := maxInt(2, frameWidth-7)
	minLeft = minInt(34, totalAvail-1)
	maxLeft = maxInt(minLeft, totalAvail-44)
	base = maxInt(42, minInt(64, totalAvail*30/100))
	left = maxInt(minLeft, minInt(maxLeft, base+offset))
	return base, minLeft, maxLeft, left
}

func (a *Application) tuiPoolWideBody(state *tuiState, g tuiGeometry, plan tuiLayoutPlan) []string {
	offset := 0
	if state != nil {
		offset = state.splitOffset
	}
	_, _, _, left := tuiWideSplit(g.frameWidth, offset)
	g.leftWidth = left
	g.rightWidth = maxInt(1, maxInt(2, g.frameWidth-7)-left)
	lines := []string{panelDivider(g, a.p)}
	available := maxInt(0, g.bodyRows-len(lines))
	list := []string{a.tuiListTitle(state), ""}
	rows, hits := a.tuiAccountRowsHits(state, g.leftWidth, maxInt(0, available-len(list)), plan.listLines, true)
	state.listHits = hits
	state.listFirstRow = 1 + len(list) // body index of the first account row
	if len(rows) == 0 {
		rows = a.tuiEmptyListRows(state, g.leftWidth, available-len(list))
	}
	list = append(list, rows...)
	plan = a.tuiFitPlan(state, minInt(g.rightWidth, tuiDetailMaxWidth), available, plan)
	details := a.tuiGaugeDetail(state, minInt(g.rightWidth, tuiDetailMaxWidth), available, plan)
	state.bodySplit = g.leftWidth + 3
	count := maxInt(len(list), len(details))
	for i := 0; i < count && len(lines) < g.bodyRows; i++ {
		left, right := "", ""
		if i < len(list) {
			left = list[i]
		}
		if i < len(details) {
			right = details[i]
		}
		lines = append(lines, panelRow(left, right, g, a.p))
	}
	// The divider runs down to the status rule however short the content is.
	for len(lines) < g.bodyRows {
		lines = append(lines, panelRow("", "", g, a.p))
	}
	return fitBodyLines(lines, g, a.p)
}

func (a *Application) tuiPoolColumnBody(state *tuiState, g tuiGeometry, plan tuiLayoutPlan) []string {
	rule := a.p.Gray + "├" + strings.Repeat("─", g.frameWidth-2) + "┤" + a.p.Reset
	lines := []string{rule}
	if g.bodyRows > 1 {
		lines = append(lines, frameRow(a.tuiListTitle(state), g, a.p))
	}
	avail := g.bodyRows - len(lines)
	if avail <= 0 {
		return fitBodyLines(lines, g, a.p)
	}
	list := a.tuiAccountRowsSpaced(state, g.contentWidth, 1000, plan.listLines, false)
	if len(list) == 0 {
		list = a.tuiEmptyListRows(state, g.contentWidth, avail)
	}
	_, _, hasSelected := state.selectedAccount()
	var detail []string
	if hasSelected {
		minList := minInt(len(list), plan.listLines*2)
		plan = a.tuiFitPlan(state, g.contentWidth, maxInt(1, avail-1-minList), plan)
		detail = a.tuiGaugeDetail(state, g.contentWidth, 0, plan)
	}
	listBudget, detailBudget := len(list), 0
	if len(detail) > 0 {
		// One rule separates the list from the detail.
		room := avail - 1
		minList := minInt(len(list), plan.listLines*3)
		detailBudget = minInt(len(detail), maxInt(0, room-minList))
		listBudget = maxInt(1, room-detailBudget)
		if detailBudget == 0 {
			listBudget = avail
		}
	}
	if shown := a.tuiAccountRowsFor(state, g.contentWidth, listBudget, plan.listLines); len(shown) > 0 {
		list = shown
	}
	for i, row := range list {
		if i >= listBudget || len(lines) >= g.bodyRows {
			break
		}
		lines = append(lines, frameRow(row, g, a.p))
	}
	if detailBudget > 0 && len(lines) < g.bodyRows {
		lines = append(lines, rule)
		for i, row := range detail {
			if i >= detailBudget || len(lines) >= g.bodyRows {
				break
			}
			lines = append(lines, frameRow(row, g, a.p))
		}
	}
	return fitBodyLines(lines, g, a.p)
}
