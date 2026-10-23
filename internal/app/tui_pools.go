package app

import (
	"time"
)

// The pool model behind the dashboard: each account has up to two model pools
// (Gemini, Claude/GPT), each with a weekly and an optional 5-hour limit. These
// functions only read the saved quota snapshot; they draw nothing.

// Thresholds match the quota bars and the CLI list: green above 30%, yellow at
// 30% or less, red at 10% or less.
const (
	poolLowFraction       = .3
	poolNearlyOutFraction = .1
)

type poolState uint8

const (
	poolPlenty poolState = iota
	poolLow
	poolNearlyOut
	poolUsedUp
)

func poolStateFor(fraction float64) poolState {
	switch {
	case fraction <= 0:
		return poolUsedUp
	case fraction <= poolNearlyOutFraction:
		return poolNearlyOut
	case fraction <= poolLowFraction:
		return poolLow
	}
	return poolPlenty
}

type poolLimit struct {
	present  bool
	fraction float64
	resetAt  time.Time
}

type poolView struct {
	id             string
	label          string
	weekly         poolLimit
	fiveHour       poolLimit
	limiting       poolLimit
	limitingWindow string
	state          poolState
	// paused: the weekly limit is used up and Google reports no active 5-hour
	// limit, so the 5-hour limit waits for the weekly reset.
	paused bool
	// backIn is how long until a used-up pool has quota again, 0 otherwise.
	backIn time.Duration
	// stale: a reset time has already passed, so the saved numbers are old.
	stale bool
}

func poolLabel(id, name string) string {
	switch id {
	case "gemini":
		return "Gemini"
	case "third_party":
		return "Claude/GPT"
	}
	return quotaGroupLabel(map[string]any{"id": id, "name": name})
}

func poolViews(account Account, now time.Time) []poolView {
	var gemini, others []poolView
	for _, raw := range quotaGroups(account) {
		group := getMap(raw)
		view := poolView{id: getString(group, "id")}
		view.label = poolLabel(view.id, getString(group, "name"))
		for _, rawBucket := range getSlice(group["buckets"]) {
			bucket := getMap(rawBucket)
			fraction, ok := getFloat(bucket["remaining_fraction"])
			if !ok {
				continue
			}
			resetAt, _ := parseUTC(getString(bucket, "reset_at"))
			limit := poolLimit{present: true, fraction: max(0, min(1, fraction)), resetAt: resetAt}
			switch getString(bucket, "window") {
			case "weekly":
				view.weekly = limit
			case "5h":
				view.fiveHour = limit
			default:
				continue
			}
			if !resetAt.IsZero() && resetAt.Before(now) {
				view.stale = true
			}
		}
		if !view.weekly.present && !view.fiveHour.present {
			continue
		}
		view.limiting, view.limitingWindow = view.weekly, "weekly"
		if !view.weekly.present || (view.fiveHour.present && view.fiveHour.fraction < view.weekly.fraction) {
			view.limiting, view.limitingWindow = view.fiveHour, "5h"
		}
		view.state = poolStateFor(view.limiting.fraction)
		view.paused = view.weekly.present && view.weekly.fraction <= 0 && !view.fiveHour.present
		if view.state == poolUsedUp {
			if left := view.limiting.resetAt.Sub(now); left > 0 {
				view.backIn = left
			}
		}
		if view.id == "gemini" {
			gemini = append(gemini, view)
		} else {
			others = append(others, view)
		}
	}
	return append(gemini, others...)
}

type tuiAccountStatus uint8

const (
	statusPending tuiAccountStatus = iota
	statusReady
	statusLow
	statusGeminiOnly
	statusClaudeGPTOnly
	statusLimited
	statusNoToken
	statusAttention
	statusVerify
)

func (s tuiAccountStatus) String() string {
	switch s {
	case statusReady:
		return "READY"
	case statusLow:
		return "LOW"
	case statusGeminiOnly:
		return "GEMINI ONLY"
	case statusClaudeGPTOnly:
		return "CLAUDE/GPT ONLY"
	case statusLimited:
		return "LIMITED"
	case statusNoToken:
		return "NO TOKEN"
	case statusAttention:
		return "ATTENTION"
	case statusVerify:
		return "VERIFY"
	}
	return "…"
}

// accountStatusFor says what an account can be used for right now.
func accountStatusFor(pools []poolView, needsToken bool) tuiAccountStatus {
	if needsToken {
		return statusNoToken
	}
	if len(pools) == 0 {
		return statusPending
	}
	usable, usedUp, low := 0, 0, false
	var geminiUsable, otherUsable bool
	for _, pool := range pools {
		if pool.state == poolUsedUp {
			usedUp++
			continue
		}
		usable++
		if pool.state != poolPlenty {
			low = true
		}
		if pool.id == "gemini" {
			geminiUsable = true
		} else {
			otherUsable = true
		}
	}
	switch {
	case usable == 0:
		return statusLimited
	case usedUp > 0 && geminiUsable && !otherUsable:
		return statusGeminiOnly
	case usedUp > 0 && otherUsable && !geminiUsable:
		return statusClaudeGPTOnly
	case low:
		return statusLow
	}
	return statusReady
}

// accountBackIn is the soonest time a used-up pool has quota again, 0 when
// nothing is used up.
func accountBackIn(pools []poolView) time.Duration {
	var soonest time.Duration
	for _, pool := range pools {
		if pool.state != poolUsedUp || pool.backIn <= 0 {
			continue
		}
		if soonest == 0 || pool.backIn < soonest {
			soonest = pool.backIn
		}
	}
	return soonest
}
