package app

import (
	"strings"
	"time"
)

func poolBorderColor(p palette, pool poolView) string {
	switch pool.state {
	case poolLow:
		return p.Yellow
	case poolNearlyOut, poolUsedUp:
		return p.Red
	}
	return p.DarkGray
}

// joinBoxes puts boxes next to each other with one column between them.
func joinBoxes(left, right []string, leftWidth, rightWidth int, p palette) []string {
	rows := maxInt(len(left), len(right))
	joined := make([]string, 0, rows)
	for i := 0; i < rows; i++ {
		l, r := strings.Repeat(" ", leftWidth), strings.Repeat(" ", rightWidth)
		if i < len(left) {
			l = left[i]
		}
		if i < len(right) {
			r = right[i]
		}
		joined = append(joined, l+" "+r)
	}
	return joined
}

// tuiGaugeDetail is the selected account: who it is, what it can be used for,
// and one gauge panel per pool. Panels sit side by side or stacked and carry
// 5-row, 3-row or no big digits, as the layout plan says.
func (a *Application) tuiGaugeDetail(state *tuiState, width, maxRows int, plan tuiLayoutPlan) []string {
	width = maxInt(1, width)
	email, account, ok := state.selectedAccount()
	if !ok {
		return []string{fitVisible(a.tuiSecondary("Select an account to inspect usage."), width, a.p)}
	}
	now := a.renderTime()
	view := a.tuiAccountView(state, email, now)
	var rows []string
	nameLine := a.p.Bold + a.p.White + view.name + a.p.Reset + " " + a.tuiSecondary(email)
	if tier := tuiText(getString(getMap(getMap(account["quota_snapshot"])["tier"]), "name")); tier != "" {
		nameLine += a.p.Gray + " · " + tier + a.p.Reset
	}
	chip := tuiChip(a.p, a.tuiStatusColor(view.status), view.status.String())
	rows = append(rows, rowSpread(nameLine, chip, width, a.p))
	var facts []string
	if reset, ok := sessionTokenReset(account); ok {
		facts = append(facts, "Session token "+reset)
	}
	if age, ok := quotaAge(account, now); ok {
		if age < time.Minute {
			facts = append(facts, "updated just now")
		} else {
			facts = append(facts, "updated "+formatDuration(age.Seconds())+" ago")
		}
	}
	if len(facts) > 0 {
		rows = append(rows, a.tuiSecondary(strings.Join(facts, " · ")))
	}
	rows = append(rows, "")
	switch {
	case view.needsAttention:
		rows = append(rows, a.p.Yellow+"Needs re-auth"+a.p.Reset+" · "+tuiText(view.attentionReason))
		rows = append(rows, a.p.White+"Run 'agy-swap add' to re-authenticate or 'd' to remove"+a.p.Reset)
	case len(view.pools) == 0:
		shown := false
		for _, limit := range activeLimits(account, now) {
			if bar := formatCooldownBar(limit.Limit, a.p, now, tuiQuotaBarWidth(maxInt(8, width-14))); bar != "" {
				rows = append(rows, tuiDetailKV(tuiText(getString(limit.Limit, "model")), bar, width, 12, a.p))
				shown = true
			}
		}
		if !shown {
			rows = append(rows, a.tuiSecondary("Waiting for the first refresh. Press r to refresh."))
		}
	default:
		rows = append(rows, a.tuiPoolPanels(view.pools, now, width, plan)...)
	}
	if reason := tuiText(state.quotaErrors[email]); reason != "" {
		rows = append(rows, a.p.Yellow+"Usage unavailable"+a.p.Reset+" · "+reason)
	}
	if maxRows > 0 && len(rows) > maxRows {
		rows = rows[:maxRows]
	}
	for i := range rows {
		rows[i] = fitVisible(rows[i], width, a.p)
	}
	return rows
}

func (a *Application) tuiPoolPanels(pools []poolView, now time.Time, width int, plan tuiLayoutPlan) []string {
	if plan.digits == digitsNone {
		compact := func(pool poolView, boxWidth int) []string { return tuiPoolCompactLines(a.p, pool, now, boxWidth) }
		if plan.panels == panelsSide {
			half := (width - 1) / 2
			left := compact(pools[0], half)
			if len(pools) == 1 {
				return left
			}
			return joinBoxes(left, compact(pools[1], width-1-half), half, width-1-half, a.p)
		}
		var rows []string
		for i, pool := range pools {
			if i > 0 {
				rows = append(rows, "")
			}
			rows = append(rows, compact(pool, width)...)
		}
		return rows
	}
	box := func(pool poolView, boxWidth int) []string {
		return tuiBox(a.p, tuiPoolPanelLines(a.p, pool, now, maxInt(8, boxWidth-4), plan.digits), boxWidth, poolBorderColor(a.p, pool))
	}
	if plan.panels == panelsSide {
		half := (width - 1) / 2
		left := box(pools[0], half)
		if len(pools) == 1 {
			return left
		}
		return joinBoxes(left, box(pools[1], width-1-half), half, width-1-half, a.p)
	}
	var rows []string
	for _, pool := range pools {
		rows = append(rows, box(pool, width)...)
	}
	return rows
}
