package app

import (
	"fmt"
	"math"
	"strings"
	"time"
)

func (a *Application) tuiHeaderLines(title, version string, width int) []string {
	return a.tuiHeaderLinesWithVersion(version, nil, width)
}

func (a *Application) tuiActiveLine(state *tuiState, width int) string {
	g := newTUIGeometry(width, 12)
	if state.active == "" {
		return a.barRow(a.p.Orange+"⬢ "+a.p.Bold+a.p.White+"ACTIVE"+a.p.Reset+"  "+a.p.Gray+"— no saved session"+a.p.Reset, g)
	}
	name := "Google User"
	status := a.p.Yellow + "UNSAVED" + a.p.Reset
	if state.accounts != nil {
		if account, ok := state.accounts.Get(state.active); ok {
			name = firstString(tuiText(getString(account, "name")), "Google User")
			status = a.p.Green + "SAVED" + a.p.Reset
		}
	}
	content := a.p.Orange + "⬢ " + a.p.Bold + a.p.White + "ACTIVE" + a.p.Reset + "  " + a.p.Blue + "●" + a.p.Reset + " " + avatar(name, state.active, a.color) + " " + a.p.White + a.p.Bold + tuiText(name) + a.p.Reset + " " + a.p.Gray + "<" + tuiText(state.active) + ">" + a.p.Reset + "  " + "[" + status + "]"
	return a.barRow(content, g)
}

func (a *Application) tuiSectionTitle(value string) string {
	return a.p.Bold + a.p.White + tuiText(value) + a.p.Reset
}

func (a *Application) tuiIdentity(value string) string {
	return a.p.Bold + a.p.White + tuiText(value) + a.p.Reset
}

func (a *Application) tuiSecondary(value string) string {
	return a.p.Gray + tuiText(value) + a.p.Reset
}

func tuiHealthToneForGroups(groups []quotaGroupHealth, account Account, now time.Time) tuiHealthTone {
	if best, ok := bestAvailableQuotaGroup(groups); ok {
		if best.fraction <= .1 {
			return tuiHealthCritical
		}
		if best.fraction <= .3 {
			return tuiHealthWarning
		}
		return tuiHealthReady
	}
	if len(groups) > 0 {
		return tuiHealthLimited
	}
	if len(activeLimits(account, now)) > 0 {
		return tuiHealthCooldown
	}
	return tuiHealthPending
}

func (a *Application) tuiAccountView(state *tuiState, email string, now time.Time) tuiAccountView {
	account := state.accounts.ByEmail[email]
	name := firstString(tuiText(getString(account, "name")), "Google User")
	groups := quotaGroupHealths(account)
	view := tuiAccountView{
		email:    email,
		name:     name,
		avatar:   avatar(name, email, a.color),
		selected: strings.EqualFold(email, state.selectedEmail),
		active:   strings.EqualFold(email, state.active),
		health:   accountHealthCompactForGroups(account, groups, now),
		tone:     tuiHealthToneForGroups(groups, account, now),
		pools:    poolViews(account, now),
	}
	view.status = accountStatusFor(view.pools, false)
	if state != nil {
		view.pools = state.tweenPools("meter:"+email, view.pools, now)
		if needs, reason := state.accountNeedsAttention(email); needs {
			view.needsAttention = true
			view.attentionReason = reason
			view.tone = tuiHealthWarning
			if strings.Contains(strings.ToLower(reason), "vault") || strings.Contains(strings.ToLower(reason), "token") {
				view.health = "⚠ Token unavailable"
				view.status = statusNoToken
			} else {
				view.health = "⚠ Needs attention"
				view.status = statusAttention
			}
		}
	}
	if ineligible, ok := accountEligibility(account); ok && view.status != statusNoToken {
		view.verify = ineligible
		view.status = statusVerify
		view.needsAttention = false
		view.health = "Needs Google verification"
		view.tone = tuiHealthWarning
	}
	view.activeFx = 1
	if state != nil {
		if view.active {
			view.activeFx = state.fxProgress(state.activeChangedAt, tuiMotionPulse, now)
			if state.ambient {
				phase := float64(now.UnixNano()%int64(tuiBreathPeriod)) / float64(tuiBreathPeriod)
				view.breath = 0.5 - 0.5*math.Cos(2*math.Pi*phase)
			}
		}
		// A status that changes flashes once; the first one seen does not.
		if state.statusSeen == nil {
			state.statusSeen, state.statusChangedAt = map[string]tuiAccountStatus{}, map[string]time.Time{}
		}
		if previous, seen := state.statusSeen[email]; seen && previous != view.status {
			state.statusChangedAt[email] = now
		}
		state.statusSeen[email] = view.status
		view.statusFlash = state.fxProgress(state.statusChangedAt[email], tuiMotionStatus, now) < 1
	}
	return view
}

// tuiSelectionBar marks the selected account down the left edge of every line
// of its block, so the selection reads as a band rather than a single glyph.
const tuiSelectionBar = "┃"

func (a *Application) tuiSelectionMarker(view tuiAccountView, state *tuiState) string {
	if !view.selected || view.banded {
		return " "
	}
	marker := tuiSelectionBar
	if state != nil && state.animation.kind == "focus" && state.animation.active && state.animationPhase()%2 == 1 {
		marker = "*"
	}
	return a.p.Orange + marker + a.p.Reset
}

// tuiSelectionGutter is the left edge of a block's continuation lines.
func (a *Application) tuiSelectionGutter(view tuiAccountView) string {
	if !view.selected || view.banded {
		return " "
	}
	return a.p.Orange + tuiSelectionBar + a.p.Reset
}

func (a *Application) tuiActiveMarker(view tuiAccountView) string {
	if view.active && view.activeFx < 1 && int(view.activeFx*6)%2 == 1 {
		// Just switched to: the dot pulses a few times.
		return a.p.Blue + "◉" + a.p.Reset
	}
	if view.active {
		if view.needsAttention {
			return a.p.Yellow + "●" + a.p.Reset
		}
		if view.breath > 0 {
			// While nothing else moves, the live account's dot breathes.
			return lerpColor(a.p.Blue, "\x1b[38;2;10;12;16m", 0.55*view.breath) + "●" + a.p.Reset
		}
		return a.p.Blue + "●" + a.p.Reset
	}
	if view.needsAttention {
		return a.p.Yellow + "⚠" + a.p.Reset
	}
	return " " // only the live account carries a dot
}

// tuiActiveName draws the account name and, for the account Antigravity is
// using, the word ACTIVE beside it. The word is blue, like the active dot:
// green, yellow and red are kept for how much quota is left. A live account
// that needs attention stays yellow.
func (a *Application) tuiActiveName(view tuiAccountView) (name, word string) {
	name = a.tuiIdentity(view.name)
	if view.selected {
		name = a.p.Bold + a.p.White + view.name + a.p.Reset
	}
	if !view.active {
		return name, ""
	}
	color := a.p.Blue
	if view.needsAttention {
		color = a.p.Yellow
	}
	word = "ACTIVE"
	if sweep := view.activeFx * float64(tuiMotionPulse) / float64(tuiMotionSweep); sweep < 1 {
		// Just switched to: the word sweeps in, keeping its width.
		shown := int(float64(len(word)) * easeOutCubic(sweep))
		word = word[:shown] + strings.Repeat(" ", len(word)-shown)
	}
	return name, color + a.p.Bold + word + a.p.Reset
}

func (a *Application) tuiAccountIdentityLine(view tuiAccountView, state *tuiState) string {
	return fmt.Sprintf("%s %s %s %s %s", a.tuiSelectionMarker(view, state), a.tuiActiveMarker(view), view.avatar, a.tuiIdentity(view.name), a.tuiSecondary("<"+view.email+">"))
}

func (a *Application) renderAccountRow(view tuiAccountView, width int) string {
	line := a.tuiAccountIdentityLine(view, nil)
	return fitVisible(line, width, a.p)
}

func (a *Application) tuiSectionDividerRow(title string, count int, isAttention bool, width int) string {
	width = maxInt(1, width)
	label := fmt.Sprintf("%s (%d)", title, count)
	color := a.p.Bold + a.p.White
	if isAttention {
		color = a.p.Bold + a.p.Yellow
	}
	prefix := a.p.DarkGray + "── " + color + label + a.p.Reset + " " + a.p.DarkGray
	prefixLen := visibleWidth(prefix)
	dashCount := maxInt(0, width-prefixLen)
	content := prefix + strings.Repeat("─", dashCount) + a.p.Reset
	return fitVisible(content, width, a.p)
}

func (a *Application) tuiWelcomeRows(width, maxRows int) []string {
	rows := []string{a.p.Bold + a.p.White + "Welcome to agy-swap" + a.p.Reset, "No saved accounts yet."}
	if width >= 50 {
		rows = append(rows, "Press "+a.p.Orange+"a"+a.p.Reset+" to sign in, or "+a.p.Cyan+"Ctrl-K"+a.p.Reset+" for every action.")
	} else {
		rows = append(rows, "Press "+a.p.Orange+"a"+a.p.Reset+" to sign in,", "or "+a.p.Cyan+"Ctrl-K"+a.p.Reset+" for every action.")
	}
	rows = append(rows, wrapWords("Your tokens go to a private local vault after sign-in.", width)...)
	if maxRows < len(rows) {
		rows = rows[:maxRows]
	}
	for i := range rows {
		rows[i] = fitVisible(rows[i], width, a.p)
	}
	return rows
}

// wrapWords breaks plain text into lines of at most width columns.
func wrapWords(text string, width int) []string {
	var lines []string
	line := ""
	for _, word := range strings.Fields(text) {
		switch {
		case line == "":
			line = word
		case len(line)+1+len(word) <= width:
			line += " " + word
		default:
			lines = append(lines, line)
			line = word
		}
	}
	if line != "" {
		lines = append(lines, line)
	}
	return lines
}

func sessionTokenReset(account Account) (string, bool) {
	if reset, ok := tokenResetInfo(tuiText(getString(account, "token_data"))); ok {
		return reset, true
	}
	return tokenResetFromExpiry(tuiText(getString(account, "access_expires_at")))
}

// tuiDetailTableLines mirrors the account table with key/value columns for
// status, quota buckets, and token state.
func (a *Application) tuiDetailTableLines(state *tuiState, width, maxRows int) []string {
	width = maxInt(1, width)
	if maxRows <= 0 {
		return nil
	}
	email, account, ok := state.selectedAccount()
	if !ok {
		return []string{fitVisible(a.tuiSecondary("Select an account to inspect usage."), width, a.p)}
	}
	now := a.renderTime()
	name := firstString(tuiText(getString(account, "name")), "Google User")
	labelWidth := maxInt(12, minInt(22, width/3))
	statusStr := accountStatus(account, a.p, now)
	var actionStr string
	ineligible, needsVerification := accountEligibility(account)
	if needs, reason := state.accountNeedsAttention(email); needs && !needsVerification {
		statusStr = a.p.Yellow + "[Needs Re-auth]" + a.p.Reset + " · " + reason
		actionStr = "Run 'agy-swap add' to re-authenticate or 'd' to remove"
	}
	if needsVerification {
		statusStr = a.p.Yellow + "Verification required" + a.p.Reset
		if ineligible.Message != "" {
			statusStr += " · " + tuiText(ineligible.Message)
		}
		actionStr = "Run 'agy' to see Google's link, then press r"
		if ineligible.URL != "" {
			actionStr = "[w] open Google's verification page in the browser, then press r"
		}
	}
	rows := []string{
		a.tuiSectionTitle("SELECTED ACCOUNT"),
		a.tuiIdentity(name),
		a.tuiSecondary(email),
		a.p.DarkGray + strings.Repeat("─", width) + a.p.Reset,
		tuiDetailKV("STATUS", statusStr, width, labelWidth, a.p),
	}
	if actionStr != "" {
		rows = append(rows, tuiDetailKV("ACTION", a.p.White+actionStr+a.p.Reset, width, labelWidth, a.p))
	}
	for _, rawGroup := range quotaGroups(account) {
		group := getMap(rawGroup)
		groupName := firstString(tuiText(getString(group, "name")), "Model group")
		rows = append(rows, a.p.Bold+a.p.Blue+tuiText(groupName)+a.p.Reset)
		for _, rawBucket := range getSlice(group["buckets"]) {
			bucket := getMap(rawBucket)
			valueWidth := maxInt(8, width-labelWidth-3)
			barWidth := tuiQuotaBarWidth(valueWidth)
			rows = append(rows, tuiDetailKV(tuiText(getString(bucket, "name")), formatQuotaBarResponsive(bucket, a.p, now, barWidth, valueWidth), width, labelWidth, a.p))
		}
	}
	if len(quotaGroups(account)) == 0 {
		for _, limit := range activeLimits(account, now) {
			valueWidth := maxInt(8, width-labelWidth-3)
			bar := formatCooldownBar(limit.Limit, a.p, now, tuiQuotaBarWidth(valueWidth))
			if bar != "" {
				rows = append(rows, tuiDetailKV(tuiText(getString(limit.Limit, "model")), bar, width, labelWidth, a.p))
			}
		}
	}
	if reset, ok := sessionTokenReset(account); ok {
		rows = append(rows, tuiDetailKV("SESSION TOKEN", reset, width, labelWidth, a.p))
	}
	if reason := tuiText(state.quotaErrors[email]); reason != "" && !strings.HasPrefix(reason, eligibilityErrorPrefix) {
		rows = append(rows, tuiDetailKV("USAGE", a.p.Yellow+"Unavailable"+a.p.Reset+" · "+reason, width, labelWidth, a.p))
	}
	if len(rows) > maxRows {
		rows = rows[:maxRows]
	}
	for i := range rows {
		rows[i] = fitVisible(rows[i], width, a.p)
	}
	return rows
}

func tuiQuotaBarWidth(valueWidth int) int {
	return minInt(18, maxInt(4, (maxInt(8, valueWidth)-8)/2))
}

func tuiDetailKV(label, value string, width, labelWidth int, p palette) string {
	valueWidth := maxInt(1, width-labelWidth-3)
	left := p.Cyan + fitVisible(strings.ToUpper(tuiText(label)), labelWidth, p) + p.Reset
	rule := p.DarkGray + "│" + p.Reset
	right := fitVisible(value, valueWidth, p)
	return fitVisible(left+" "+rule+" "+right, width, p)
}

func tuiText(value string) string {
	value = strings.ToValidUTF8(value, "?")
	value = strings.ReplaceAll(value, "\uFFFD", "?")
	return cleanText(value)
}

func accountHealthCompact(account Account, now time.Time) string {
	return accountHealthCompactForGroups(account, quotaGroupHealths(account), now)
}

func accountHealthCompactForGroups(account Account, groups []quotaGroupHealth, now time.Time) string {
	if best, ok := bestAvailableQuotaGroup(groups); ok {
		return fmt.Sprintf("%s %.0f%% ready", best.label, best.fraction*100)
	}
	if len(groups) > 0 {
		return "limited"
	}
	if len(activeLimits(account, now)) > 0 {
		return "cooldown"
	}
	return "pending"
}

func (a *Application) tuiDetailLines(state *tuiState, width, maxRows int) []string {
	if maxRows <= 0 {
		return nil
	}
	rows := a.tuiDetailTableLines(state, width, maxRows+2)
	if width <= 78 {
		compact := make([]string, 0, len(rows))
		for _, row := range rows {
			plain := ansiPattern.ReplaceAllString(row, "")
			if strings.TrimSpace(plain) == "SELECTED ACCOUNT" || strings.Trim(plain, "─ ") == "" {
				continue
			}
			compact = append(compact, row)
		}
		rows = compact
	}
	if len(rows) > maxRows {
		rows = rows[:maxRows]
	}
	return rows
}
