package app

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
)

type quotaProgress func(index, total int, account Account, state string)

type QuotaService struct {
	http  *HTTPService
	store *Store
	vault AccountVault
}

func NewQuotaService(httpService *HTTPService, store *Store) *QuotaService {
	return &QuotaService{http: httpService, store: store}
}

func (q *QuotaService) SetVault(vault AccountVault) { q.vault = vault }

func (q *QuotaService) Fetch(ctx context.Context, account Account) (map[string]any, error) {
	tokenData, err := accountToken(ctx, account, q.vault)
	if err != nil {
		return nil, err
	}
	access, err := q.http.accessToken(ctx, tokenData)
	if err != nil {
		return nil, err
	}
	if claimed := extractVerifiedEmail(tokenData); claimed != "" && claimed != getString(account, "email") {
		return nil, fmt.Errorf("refreshed token identity does not match the account")
	}
	info, err := q.http.cloudPost(ctx, access, "loadCodeAssist", map[string]any{"metadata": map[string]any{"ideType": "ANTIGRAVITY"}})
	if err != nil {
		return nil, err
	}
	project := getString(info, "cloudaicompanionProject")
	if project == "" {
		return nil, fmt.Errorf("Google returned no Code Assist project")
	}
	summary, err := q.http.cloudPost(ctx, access, "retrieveUserQuotaSummary", map[string]any{"project": project})
	if err != nil {
		return nil, err
	}
	tier := getMap(info["paidTier"])
	if tier == nil {
		tier = getMap(info["currentTier"])
	}
	tierID := cleanText(getString(tier, "id"))
	if tierID == "" {
		return nil, fmt.Errorf("Google returned no account tier")
	}
	tierName := tierNames[tierID]
	if tierName == "" {
		tierName = cleanText(firstString(tier["name"], tierID))
	}
	groups := make([]any, 0)
	for _, rawGroup := range getSlice(summary["groups"]) {
		group := getMap(rawGroup)
		groupID := ""
		buckets := make([]any, 0)
		for _, rawBucket := range getSlice(group["buckets"]) {
			bucket := getMap(rawBucket)
			id := cleanText(getString(bucket, "bucketId"))
			if strings.HasPrefix(id, "gemini-") {
				groupID = "gemini"
			} else if strings.HasPrefix(id, "3p-") {
				groupID = "third_party"
			}
			window := getString(bucket, "window")
			fraction, ok := getFloat(bucket["remainingFraction"])
			reset := getString(bucket, "resetTime")
			if !oneOf(window, "weekly", "5h") || !ok || reset == "" {
				continue
			}
			fraction = max(0, min(1, fraction))
			buckets = append(buckets, map[string]any{"id": id, "name": cleanText(firstString(bucket["displayName"], window)), "window": window, "remaining_fraction": fraction, "reset_at": reset})
		}
		if groupID != "" && len(buckets) > 0 {
			groups = append(groups, map[string]any{"id": groupID, "name": cleanText(firstString(group["displayName"], "Model group")), "buckets": buckets})
		}
	}
	if len(groups) == 0 {
		return nil, fmt.Errorf("Google returned no quota groups")
	}
	return normalizeQuotaSnapshot(map[string]any{"observed_at": isoTime(time.Now()), "tier": map[string]any{"id": tierID, "name": tierName}, "groups": groups}, getString(account, "email"))
}

func quotaAge(account Account, now time.Time) (time.Duration, bool) {
	snapshot := getMap(account["quota_snapshot"])
	observed, err := parseUTC(getString(snapshot, "observed_at"))
	if err != nil {
		return 0, false
	}
	age := now.Sub(observed)
	if age < 0 {
		age = 0
	}
	return age, true
}

func (q *QuotaService) Refresh(ctx context.Context, accounts *Accounts, force bool, progress quotaProgress) map[string]string {
	now := time.Now().UTC()
	type item struct {
		email   string
		account Account
	}
	var fetch []item
	done := 0
	for _, email := range accounts.Order {
		account := accounts.ByEmail[email]
		if age, ok := quotaAge(account, now); !force && ok && age < quotaCache {
			done++
			if progress != nil {
				progress(done, accounts.Len(), account, "cached")
			}
		} else {
			fetch = append(fetch, item{email, account})
		}
	}
	errorsByEmail := make(map[string]string)
	if len(fetch) == 0 {
		return errorsByEmail
	}
	workers := minInt(8, len(fetch))
	jobs := make(chan item)
	type result struct {
		item     item
		snapshot map[string]any
		err      error
	}
	results := make(chan result, len(fetch))
	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for it := range jobs {
				snapshot, err := q.Fetch(ctx, it.account)
				results <- result{it, snapshot, err}
			}
		}()
	}
	go func() {
		for _, it := range fetch {
			jobs <- it
		}
		close(jobs)
		wg.Wait()
		close(results)
	}()
	changed := false
	for result := range results {
		done++
		state := "synced"
		if result.err != nil {
			errorsByEmail[result.item.email] = result.err.Error()
			state = "failed"
		} else {
			result.item.account["quota_snapshot"] = result.snapshot
			changed = true
		}
		if progress != nil {
			progress(done, accounts.Len(), result.item.account, state)
		}
	}
	if changed {
		if err := q.store.Save(accounts); err != nil {
			errorsByEmail["store"] = err.Error()
		}
	}
	return errorsByEmail
}

func tokenResetInfo(tokenData string) (string, bool) {
	inner := tokenObject(decodeToken(tokenData))
	if inner == nil {
		return "", false
	}
	expiry, ok := tokenExpiry(inner)
	if !ok {
		return "", false
	}
	diff := time.Until(expiry)
	if diff > 0 {
		mins := int(diff.Minutes())
		if mins >= 60 {
			return fmt.Sprintf("%dh %dm", mins/60, mins%60), true
		}
		return fmt.Sprintf("%dm %ds", mins, int(diff.Seconds())%60), true
	}
	if getString(inner, "refresh_token") != "" {
		return "Access expired · refresh token available", true
	}
	return fmt.Sprintf("Expired %dm ago", int((-diff).Minutes())), true
}
