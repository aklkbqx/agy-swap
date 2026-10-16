package app

import (
	"testing"
	"time"
)

var poolNow = time.Date(2026, 10, 16, 14, 0, 0, 0, time.UTC)

func poolBucket(window string, fraction float64, resetIn time.Duration) map[string]any {
	return map[string]any{"id": window, "name": window, "window": window, "remaining_fraction": fraction, "reset_at": poolNow.Add(resetIn).Format(time.RFC3339)}
}

func poolAccount(groups ...map[string]any) Account {
	list := make([]any, 0, len(groups))
	for _, group := range groups {
		list = append(list, group)
	}
	return Account{"quota_snapshot": map[string]any{"observed_at": poolNow.Format(time.RFC3339), "tier": map[string]any{"id": "t", "name": "T"}, "groups": list}}
}

func poolGroup(id, name string, buckets ...map[string]any) map[string]any {
	list := make([]any, 0, len(buckets))
	for _, bucket := range buckets {
		list = append(list, bucket)
	}
	return map[string]any{"id": id, "name": name, "buckets": list}
}

func TestPoolViewsSplitWeeklyFromFiveHourAndPickTheTighterLimit(t *testing.T) {
	account := poolAccount(poolGroup("gemini", "Gemini models",
		poolBucket("weekly", 0.36, 125*time.Hour), poolBucket("5h", 0.93, 4*time.Hour)))
	pools := poolViews(account, poolNow)
	if len(pools) != 1 {
		t.Fatalf("pools = %#v", pools)
	}
	pool := pools[0]
	if pool.id != "gemini" || pool.label != "Gemini" {
		t.Fatalf("pool identity = %q %q", pool.id, pool.label)
	}
	if !pool.weekly.present || pool.weekly.fraction != 0.36 || !pool.fiveHour.present || pool.fiveHour.fraction != 0.93 {
		t.Fatalf("limits = %#v %#v", pool.weekly, pool.fiveHour)
	}
	if pool.limitingWindow != "weekly" || pool.limiting.fraction != 0.36 {
		t.Fatalf("limiting = %q %v", pool.limitingWindow, pool.limiting.fraction)
	}
	if pool.state != poolPlenty {
		t.Fatalf("state = %v", pool.state)
	}
}

func TestPoolStateUsesTheSameThresholdsAsTheQuotaBars(t *testing.T) {
	cases := []struct {
		fraction float64
		want     poolState
	}{
		{1, poolPlenty}, {0.31, poolPlenty}, {0.30, poolLow}, {0.11, poolLow},
		{0.10, poolNearlyOut}, {0.01, poolNearlyOut}, {0, poolUsedUp},
	}
	for _, tc := range cases {
		if got := poolStateFor(tc.fraction); got != tc.want {
			t.Fatalf("poolStateFor(%v) = %v, want %v", tc.fraction, got, tc.want)
		}
	}
}

func TestAnEmptyWeeklyLimitPausesTheMissingFiveHourLimitAndSetsBackIn(t *testing.T) {
	account := poolAccount(poolGroup("third_party", "Claude and GPT models", poolBucket("weekly", 0, 22*time.Hour+43*time.Minute)))
	pool := poolViews(account, poolNow)[0]
	if pool.label != "Claude/GPT" {
		t.Fatalf("label = %q", pool.label)
	}
	if pool.state != poolUsedUp || !pool.paused {
		t.Fatalf("state = %v paused = %v", pool.state, pool.paused)
	}
	if pool.backIn != 22*time.Hour+43*time.Minute {
		t.Fatalf("backIn = %v", pool.backIn)
	}
}

func TestAPoolWithQuotaIsNotPausedAndHasNoBackIn(t *testing.T) {
	pool := poolViews(poolAccount(poolGroup("gemini", "Gemini models", poolBucket("5h", 0.6, 90*time.Minute))), poolNow)[0]
	if pool.paused || pool.backIn != 0 {
		t.Fatalf("paused = %v backIn = %v", pool.paused, pool.backIn)
	}
	if pool.limitingWindow != "5h" || pool.weekly.present {
		t.Fatalf("limits = %#v window %q", pool.weekly, pool.limitingWindow)
	}
}

func TestAnEmptyFiveHourLimitReturnsAtItsOwnReset(t *testing.T) {
	account := poolAccount(poolGroup("gemini", "Gemini models", poolBucket("weekly", 0.5, 100*time.Hour), poolBucket("5h", 0, 2*time.Hour)))
	pool := poolViews(account, poolNow)[0]
	if pool.state != poolUsedUp || pool.paused || pool.backIn != 2*time.Hour {
		t.Fatalf("state = %v paused = %v backIn = %v", pool.state, pool.paused, pool.backIn)
	}
}

func TestStaleQuotaIsFlaggedWhenAResetHasPassed(t *testing.T) {
	account := poolAccount(poolGroup("gemini", "Gemini models", poolBucket("weekly", 0.5, -time.Hour)))
	if !poolViews(account, poolNow)[0].stale {
		t.Fatal("expected a stale pool")
	}
	fresh := poolAccount(poolGroup("gemini", "Gemini models", poolBucket("weekly", 0.5, time.Hour)))
	if poolViews(fresh, poolNow)[0].stale {
		t.Fatal("a pool with a future reset is not stale")
	}
}

func TestPoolViewsListGeminiFirstAndSkipEmptyGroups(t *testing.T) {
	account := poolAccount(
		poolGroup("third_party", "Claude and GPT models", poolBucket("weekly", 1, 100*time.Hour)),
		poolGroup("gemini", "Gemini models", poolBucket("weekly", 1, 100*time.Hour)),
		poolGroup("gemini", "Empty"),
	)
	pools := poolViews(account, poolNow)
	if len(pools) != 2 || pools[0].id != "gemini" || pools[1].id != "third_party" {
		t.Fatalf("pools = %#v", pools)
	}
	if got := poolViews(Account{}, poolNow); len(got) != 0 {
		t.Fatalf("an account without a snapshot has pools: %#v", got)
	}
}

func TestAccountStatusWords(t *testing.T) {
	gem := func(fraction float64) poolView {
		return poolView{id: "gemini", label: "Gemini", limiting: poolLimit{present: true, fraction: fraction}, state: poolStateFor(fraction)}
	}
	cg := func(fraction float64) poolView {
		return poolView{id: "third_party", label: "Claude/GPT", limiting: poolLimit{present: true, fraction: fraction}, state: poolStateFor(fraction)}
	}
	cases := []struct {
		name       string
		pools      []poolView
		needsToken bool
		want       tuiAccountStatus
		word       string
	}{
		{"both plenty", []poolView{gem(0.86), cg(1)}, false, statusReady, "READY"},
		{"gemini low is low", []poolView{gem(0.09), cg(0.41)}, false, statusLow, "LOW"},
		{"gemini plenty claude low is low", []poolView{gem(0.9), cg(0.3)}, false, statusLow, "LOW"},
		{"claude used up", []poolView{gem(0.36), cg(0)}, false, statusGeminiOnly, "GEMINI ONLY"},
		{"gemini used up", []poolView{gem(0), cg(0.64)}, false, statusClaudeGPTOnly, "CLAUDE/GPT ONLY"},
		{"both used up", []poolView{gem(0), cg(0)}, false, statusLimited, "LIMITED"},
		{"a single plenty pool", []poolView{cg(1)}, false, statusReady, "READY"},
		{"a single empty pool", []poolView{cg(0)}, false, statusLimited, "LIMITED"},
		{"no token wins", []poolView{gem(1), cg(1)}, true, statusNoToken, "NO TOKEN"},
		{"no snapshot yet", nil, false, statusPending, "…"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := accountStatusFor(tc.pools, tc.needsToken)
			if got != tc.want || got.String() != tc.word {
				t.Fatalf("status = %v %q, want %v %q", got, got.String(), tc.want, tc.word)
			}
		})
	}
}

func TestAccountBackInIsTheSoonestUsedUpPool(t *testing.T) {
	pools := []poolView{
		{state: poolUsedUp, backIn: 22 * time.Hour},
		{state: poolUsedUp, backIn: 2 * time.Hour},
		{state: poolPlenty},
	}
	if got := accountBackIn(pools); got != 2*time.Hour {
		t.Fatalf("backIn = %v", got)
	}
	if got := accountBackIn([]poolView{{state: poolPlenty}}); got != 0 {
		t.Fatalf("backIn = %v, want 0", got)
	}
}
