package app

import (
	"fmt"
	"strings"
)

func (a *Application) tuiListTitle(state *tuiState) string {
	return a.tuiSectionTitle("ACCOUNTS") + "  " + a.p.Gray + fmt.Sprintf("%d", len(state.visibleEmails())) + "  " + tuiSelectionBar + " selected · ● active" + a.p.Reset
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
	state.stackRule, state.stackRuleBase = -1, -1
	state.listHits = nil
	if state.accounts == nil || state.accounts.Len() == 0 {
		return a.tuiEmptyBody(g)
	}
	plan := tuiLayoutPlanFor(g.frameWidth+tuiRails, g.frameHeight)
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
		// Grow the digits into the room, but only so far: the biggest
		// scale (at most 2) whose digits still fit a panel and whose panels
		// still fit the rows, with at most a little air between the sections.
		// Beyond that the figures read as a poster, not a gauge.
		inner := width - 4
		if plan.panels == panelsSide {
			inner = (width-1)/2 - 4
		}
		for scale := 2; scale >= 2; scale-- {
			if bigNumberWidth(3, scale) > inner {
				continue
			}
			// The bar stays one full-block row: a second row reads as two bars in
			// terminals that leave a gap between lines. The extra rows become air.
			for thick := 1; thick >= 1; thick-- {
				for gap := 1; gap >= 0; gap-- {
					candidate := plan
					candidate.scale, candidate.gap, candidate.thick = scale, gap, thick
					candidates = append(candidates, candidate)
				}
			}
		}
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
	lines := []string{frameRow("", g, a.p)}
	for _, row := range a.tuiWelcomeRows(g.contentWidth, maxInt(1, g.bodyRows-2)) {
		lines = append(lines, frameRow(row, g, a.p))
	}
	return fitBodyLines(lines, g, a.p)
}

// tuiWideSplit is the one place that decides how wide the account list is in
// the two-column dashboard. The list grows a little on very wide screens; the
// detail pane keeps at least 44 columns and the list at least 34. The mouse
// drag, the [ ] keys and the renderer all read it, so they cannot disagree.
// tuiWideChrome is the columns a wide body spends around its two panes: two
// at each screen edge and two either side of the one-column divider.
const tuiWideChrome = 9

func tuiWideSplit(frameWidth, offset int) (base, minLeft, maxLeft, left int) {
	totalAvail := maxInt(2, frameWidth-tuiWideChrome)
	minLeft = minInt(34, totalAvail-1)
	maxLeft = maxInt(minLeft, totalAvail-44)
	base = maxInt(42, minInt(96, totalAvail*30/100))
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
	g.rightWidth = maxInt(1, maxInt(2, g.frameWidth-tuiWideChrome)-left)
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
	plan = a.tuiFitPlan(state, g.rightWidth, available, plan)
	details := a.tuiScrollDetail(state, a.tuiGaugeDetail(state, g.rightWidth, available, plan), available, g.rightWidth, func(width int) []string {
		return a.tuiGaugeDetail(state, width, available, plan)
	})
	state.bodySplit = g.leftWidth + 4
	// Pointer targets, in frame columns: the list runs from column 2 up to
	// the divider at leftWidth+4, the gauges from leftWidth+7.
	top := state.bodyTop
	state.addAccountHits(hits, top+state.listFirstRow, 2, 2+g.leftWidth)
	state.addScrollHits("list", top+state.listFirstRow, top+state.listFirstRow+len(rows), 0, g.leftWidth+3, 1+g.leftWidth)
	state.addHit(tuiHit{kind: hitDivider, top: top, bottom: top + g.bodyRows, left: g.leftWidth + 3, right: g.leftWidth + 6})
	state.addScrollHits("detail", top+1, top+g.bodyRows, g.leftWidth+6, g.frameWidth, g.leftWidth+6+g.rightWidth)
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
	if lit := a.tuiGripColor(state, hitDivider); lit != "" {
		// Under the pointer the divider lights, and while dragged it takes the
		// action colour.
		for i := range lines {
			lines[i] = strings.Replace(lines[i], a.p.Track+"│", lit+"│", 1)
		}
	}
	return fitBodyLines(lines, g, a.p)
}

// tuiStackAirRows is the body height from which the stacked dashboard keeps
// a blank row under the list title and either side of the rule.
const tuiStackAirRows = 14

func (a *Application) tuiPoolColumnBody(state *tuiState, g tuiGeometry, plan tuiLayoutPlan) []string {
	lines := []string{frameRow("", g, a.p)}
	if g.bodyRows > 1 {
		lines = append(lines, frameRow(a.tuiListTitle(state), g, a.p))
	}
	avail := g.bodyRows - len(lines)
	if avail <= 0 {
		return fitBodyLines(lines, g, a.p)
	}
	list := a.tuiAccountRowsMeasure(state, g.contentWidth, 1000, plan.listLines, false)
	_, _, hasSelected := state.selectedAccount()
	// The three rows of air (under the title, either side of the rule) come
	// before big digits, but never at the cost of a pool: they are taken only
	// when the gauges, in whatever form fits, still show every pool whole.
	air := g.bodyRows >= tuiStackAirRows
	if air && hasSelected {
		minList := minInt(len(list), plan.listLines*2)
		airy := a.tuiFitPlan(state, g.contentWidth, maxInt(1, avail-4-minList), plan)
		air = len(a.tuiGaugeDetail(state, g.contentWidth, 0, airy)) <= avail-4-minInt(len(list), plan.listLines*3)
	}
	if air {
		lines = append(lines, frameRow("", g, a.p))
		avail--
	}
	ruleRows := 1
	if air {
		ruleRows = 3 // a blank row, the rule, a blank row
	}
	if len(list) == 0 {
		list = a.tuiEmptyListRows(state, g.contentWidth, avail)
	}
	var detail []string
	if hasSelected {
		minList := minInt(len(list), plan.listLines*2)
		fitted := a.tuiFitPlan(state, g.contentWidth, maxInt(1, avail-ruleRows-minList), plan)
		detail = a.tuiGaugeDetail(state, g.contentWidth, 0, fitted)
	}
	listBudget, detailBudget := len(list), 0
	if len(detail) > 0 {
		// A rule separates the list from the detail.
		room := avail - ruleRows
		minList := minInt(len(list), plan.listLines*3)
		detailBudget = minInt(len(detail), maxInt(0, room-minList))
		listBudget = maxInt(1, room-detailBudget)
		if detailBudget == 0 {
			listBudget = avail
		} else {
			// The rule sits under the list as it would be drawn, and the
			// user's offset moves it from there, as far as it changes what is
			// shown: up until the gauges show whole (but never under one
			// account), down until the whole list shows (but never over the
			// last three gauge rows). What does not fit scrolls.
			natural := listBudget
			if shown := a.tuiAccountRowsMeasure(state, g.contentWidth, listBudget, plan.listLines, true); len(shown) > 0 {
				natural = minInt(listBudget, len(shown))
			}
			whole := len(a.tuiAccountRowsMeasure(state, g.contentWidth, 1000, plan.listLines, true))
			lo := minInt(natural, maxInt(maxInt(1, plan.listLines), room-len(detail)))
			hi := maxInt(natural, minInt(whole, room-minInt(len(detail), 3)))
			listBudget = maxInt(lo, minInt(hi, natural+state.stackOffset))
			state.stackOffset = listBudget - natural
			detailBudget = room - listBudget
			state.stackRuleBase = len(lines) + natural + ruleRows/2
			// A smaller detail picks the richest form that still fits, and
			// scrolls when even that is too tall.
			fitted := a.tuiFitPlan(state, g.contentWidth, detailBudget, plan)
			if detailBudget < len(detail) {
				detail = a.tuiGaugeDetail(state, g.contentWidth, 0, fitted)
			}
			detail = a.tuiScrollDetail(state, detail, detailBudget, g.contentWidth, func(width int) []string {
				return a.tuiGaugeDetail(state, width, 0, fitted)
			})
		}
	}
	if shown, hits := a.tuiAccountRowsHits(state, g.contentWidth, listBudget, plan.listLines, true); len(shown) > 0 {
		list = shown
		state.listHits, state.listFirstRow = hits, len(lines)
		state.addAccountHits(hits, state.bodyTop+len(lines), 2, 2+g.contentWidth)
	}
	listEnd := len(lines) + listBudget
	state.addScrollHits("list", state.bodyTop+len(lines), state.bodyTop+minInt(listEnd, len(lines)+len(list)), 0, g.frameWidth, 1+g.contentWidth)
	for i, row := range list {
		if i >= listBudget || len(lines) >= g.bodyRows {
			break
		}
		lines = append(lines, frameRow(row, g, a.p))
	}
	if detailBudget > 0 && len(lines) < g.bodyRows {
		for len(lines) < listEnd {
			lines = append(lines, frameRow("", g, a.p)) // the list ends above a lowered rule
		}
		if air {
			lines = append(lines, frameRow("", g, a.p))
		}
		state.stackRule = len(lines)
		state.addHit(tuiHit{kind: hitRule, top: state.bodyTop + len(lines), bottom: state.bodyTop + len(lines) + 1, left: 0, right: g.frameWidth})
		rule := a.tuiSeparator(g)
		if lit := a.tuiGripColor(state, hitRule); lit != "" {
			rule = strings.Replace(rule, a.p.Track, lit, 1)
		}
		lines = append(lines, rule)
		if air {
			lines = append(lines, frameRow("", g, a.p))
		}
		state.addScrollHits("detail", state.bodyTop+len(lines), state.bodyTop+len(lines)+detailBudget, 0, g.frameWidth, 1+g.contentWidth)
		for i, row := range detail {
			if i >= detailBudget || len(lines) >= g.bodyRows {
				break
			}
			lines = append(lines, frameRow(row, g, a.p))
		}
	}
	return fitBodyLines(lines, g, a.p)
}

// tuiGripColor is the colour of the divider or rule when it is not quiet:
// the action colour while dragged, the text gray under the pointer.
func (a *Application) tuiGripColor(state *tuiState, kind tuiHitKind) string {
	switch {
	case a.p.Track == "" || state == nil:
		return ""
	case state.dragging:
		return a.p.Orange
	case state.hover.kind == kind:
		return a.p.Gray
	}
	return ""
}
