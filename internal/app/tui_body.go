package app

import (
	"fmt"
	"strings"
)

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
	plan := tuiLayoutPlanFor(g.frameWidth, g.frameHeight)
	if plan.arrangement == arrWide {
		return a.tuiPoolWideBody(state, g, plan)
	}
	return a.tuiPoolColumnBody(state, g, plan)
}

// tuiFitPlan keeps the richest digit mode whose detail fits the rows on offer,
// so a short terminal shows plain limit lines instead of a cut-off panel.
func (a *Application) tuiFitPlan(state *tuiState, width, budget int, plan tuiLayoutPlan) tuiLayoutPlan {
	modes := []tuiDigits{digitsNone}
	switch plan.digits {
	case digitsFive:
		modes = []tuiDigits{digitsFive, digitsThree, digitsNone}
	case digitsThree:
		modes = []tuiDigits{digitsThree, digitsNone}
	}
	for _, digits := range modes {
		candidate := plan
		candidate.digits = digits
		if len(a.tuiGaugeDetail(state, width, 0, candidate)) <= budget {
			return candidate
		}
	}
	plan.digits = digitsNone
	return plan
}

func (a *Application) tuiPoolWideBody(state *tuiState, g tuiGeometry, plan tuiLayoutPlan) []string {
	totalAvail := maxInt(2, g.frameWidth-7)
	offset := 0
	if state != nil {
		offset = state.splitOffset
	}
	minLeft := minInt(34, totalAvail-1)
	maxLeft := maxInt(minLeft, totalAvail-44)
	g.leftWidth = maxInt(minLeft, minInt(maxLeft, 42+offset))
	g.rightWidth = maxInt(1, totalAvail-g.leftWidth)
	lines := []string{panelDivider(g, a.p)}
	available := maxInt(0, g.bodyRows-len(lines))
	list := []string{a.tuiListTitle(state), ""}
	rows := a.tuiAccountRowsFor(state, g.leftWidth, maxInt(0, available-len(list)), plan.listLines)
	if len(rows) == 0 {
		rows = a.tuiEmptyListRows(state, g.leftWidth, available-len(list))
	}
	list = append(list, rows...)
	plan = a.tuiFitPlan(state, g.rightWidth, available, plan)
	details := a.tuiGaugeDetail(state, g.rightWidth, available, plan)
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
	list := a.tuiAccountRowsFor(state, g.contentWidth, 1000, plan.listLines)
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
