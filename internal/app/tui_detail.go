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
	// Quota refreshes in the background every minute, so its age is news only
	// when refreshes have stopped landing.
	if age, ok := quotaAge(account, now); ok && age > tuiStaleQuotaAge && !plan.tight {
		rows = append(rows, a.p.Yellow+"updated "+formatDuration(age.Seconds())+" ago"+a.p.Reset)
	}
	if !plan.tight {
		rows = append(rows, "")
	}
	switch {
	case view.verify != nil:
		message := firstString(tuiText(view.verify.Message), "Google needs this account verified before Antigravity can use it.")
		rows = append(rows, a.p.Yellow+"Verification required"+a.p.Reset+" · "+message)
		if view.verify.URL != "" {
			rows = append(rows, a.p.White+"[w] open Google's verification page in the browser, then [r] to check again"+a.p.Reset)
		} else {
			rows = append(rows, a.p.White+"Run 'agy' to see Google's link, then [r] to check again"+a.p.Reset)
		}
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
		// One set of keys for whichever account is shown, so a new selection
		// eases from the last account's numbers.
		rows = append(rows, a.tuiPoolPanels(state.tweenPools("detail", view.pools, now), now, width, plan)...)
	}
	if reason := tuiText(state.quotaErrors[email]); reason != "" && !strings.HasPrefix(reason, eligibilityErrorPrefix) {
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
	// Solid cards that touch read as one block, so a blank row parts them.
	// Outlined boxes (no colour) are parted by their own borders.
	stack := func(rows, card []string) []string {
		if len(rows) > 0 && a.p.PanelBg != "" {
			rows = append(rows, "")
		}
		return append(rows, card...)
	}
	if plan.digits == digitsInline {
		var rows []string
		for _, pool := range pools {
			rows = stack(rows, tuiBox(a.p, tuiPoolInlineLines(a.p, pool, now, maxInt(8, width-4)), width, poolBorderColor(a.p, pool)))
		}
		return rows
	}
	// side lays the pools out two to a row; an odd last pool keeps the left half.
	// cards says the rendered pools are cards, which are parted by a blank row.
	side := func(gutter int, cards bool, render func(poolView, int) []string, join func(left, right []string, leftWidth, rightWidth int) []string) []string {
		add := func(rows, more []string) []string {
			if cards {
				return stack(rows, more)
			}
			return append(rows, more...)
		}
		half := (width - gutter) / 2
		var rows []string
		for i := 0; i < len(pools); i += 2 {
			left := render(pools[i], half)
			if i+1 == len(pools) {
				rows = add(rows, left)
				continue
			}
			// Both cards get the same width. An odd column widens the gutter,
			// so the right card still ends flush with the rest of the column.
			right := render(pools[i+1], half)
			if extra := width - gutter - 2*half; extra > 0 {
				for j := range right {
					right[j] = strings.Repeat(" ", extra) + right[j]
				}
			}
			rows = add(rows, join(left, right, half, width-gutter-half))
		}
		return rows
	}
	joinFramed := func(left, right []string, leftWidth, rightWidth int) []string {
		return joinBoxes(left, right, leftWidth, rightWidth, a.p)
	}
	if plan.digits == digitsNone {
		compact := func(pool poolView, boxWidth int) []string { return tuiPoolCompactLines(a.p, pool, now, boxWidth) }
		framed := func(pool poolView, boxWidth int) []string {
			return tuiBox(a.p, compact(pool, maxInt(8, boxWidth-4)), boxWidth, poolBorderColor(a.p, pool))
		}
		if !plan.tight {
			// Without digits each pool still gets its frame, which keeps the
			// pools apart whether they sit side by side or one above the other.
			if plan.panels == panelsSide {
				return side(1, true, framed, joinFramed)
			}
			var rows []string
			for _, pool := range pools {
				rows = stack(rows, framed(pool, width))
			}
			return rows
		}
		if plan.panels == panelsSide {
			return side(3, false, compact, func(left, right []string, leftWidth, rightWidth int) []string {
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
					joined = append(joined, l+" "+a.p.Track+"│"+a.p.Reset+" "+r)
				}
				return joined
			})
		}
		var rows []string
		for _, pool := range pools {
			rows = append(rows, compact(pool, width)...)
		}
		return rows
	}
	box := func(pool poolView, boxWidth int) []string {
		return tuiBox(a.p, tuiPoolPanelScaled(a.p, pool, now, maxInt(8, boxWidth-4), plan.digits, plan.scale, plan.gap, plan.thick), boxWidth, poolBorderColor(a.p, pool))
	}
	if plan.panels == panelsSide {
		return side(1, true, box, joinFramed)
	}
	var rows []string
	for _, pool := range pools {
		rows = stack(rows, box(pool, width))
	}
	return rows
}
