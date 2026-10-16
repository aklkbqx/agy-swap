package app

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
	"time"
)

type quotaProgress func(index, total int, account Account, state string)

type QuotaService struct {
	http  *HTTPService
	store *Store
	vault AccountVault
	// diagnostics, when set, receives one line per quota bucket explaining
	// whether it was kept. It never receives tokens or account identifiers.
	diagnostics io.Writer
}

func NewQuotaService(httpService *HTTPService, store *Store) *QuotaService {
	return &QuotaService{http: httpService, store: store}
}

func (q *QuotaService) SetVault(vault AccountVault) { q.vault = vault }

func (q *QuotaService) SetDiagnostics(out io.Writer) { q.diagnostics = out }

func (q *QuotaService) Fetch(ctx context.Context, account Account) (map[string]any, error) {
	tokenData, err := accountToken(ctx, account, q.vault)
	if err != nil {
		return nil, err
	}
	access, refreshed, err := q.http.accessTokenData(ctx, tokenData)
	if err != nil {
		return nil, err
	}
	stored := tokenData
	if refreshed != tokenData {
		stored = refreshed
	}
	if !tokenMatchesEmail(stored, getString(account, "email")) {
		return nil, fmt.Errorf("refreshed token identity does not match the account")
	}
	if refreshed != tokenData {
		account["token_data"] = refreshed
		delete(account, "secret_ref")
		rememberTokenExpiry(account, refreshed)
	}
	info, err := q.http.cloudPost(ctx, access, "loadCodeAssist", map[string]any{"metadata": map[string]any{"ideType": "ANTIGRAVITY"}})
	if err != nil {
		return nil, err
	}
	project := getString(info, "cloudaicompanionProject")
	if project == "" {
		return nil, fmt.Errorf("Google returned no Code Assist project") //nolint:staticcheck // "Google" is a proper noun
	}
	summary, err := q.http.cloudPost(ctx, access, "retrieveUserQuotaSummary", map[string]any{"project": project})
	if err != nil {
		return nil, err
	}
	var models map[string]any
	var modelsErr error
	modelsLoaded := false
	loadModels := func() (map[string]any, error) {
		if !modelsLoaded {
			models, modelsErr = q.http.cloudPost(ctx, access, "fetchAvailableModels", map[string]any{"project": project})
			modelsLoaded = true
		}
		return models, modelsErr
	}
	if q.diagnostics != nil {
		q.describeModelList(loadModels())
	}
	tier := getMap(info["paidTier"])
	if tier == nil {
		tier = getMap(info["currentTier"])
	}
	tierID := cleanText(getString(tier, "id"))
	if tierID == "" {
		return nil, fmt.Errorf("Google returned no account tier") //nolint:staticcheck // "Google" is a proper noun
	}
	tierName := tierNames[tierID]
	if tierName == "" {
		tierName = cleanText(firstString(tier["name"], tierID))
	}
	groups, notes := quotaGroupsFromSummary(summary)
	if !hasQuotaGroup(groups, "gemini") {
		// Google's summary can leave Gemini out. The model list still carries
		// the Gemini quota, one value for every Gemini model of the account.
		if list, err := loadModels(); err == nil {
			if gemini := geminiGroupFromModels(list, time.Now()); gemini != nil {
				groups = append([]any{gemini}, groups...)
				notes = append(notes, "gemini: read from the model list, the summary has no Gemini group")
			}
		}
	}
	if q.diagnostics != nil && len(notes) > 0 {
		// One write per account, so concurrent refreshes do not interleave lines.
		fmt.Fprint(q.diagnostics, "quota: summary\n  "+strings.Join(notes, "\n  ")+"\n")
	}
	if len(groups) == 0 {
		return nil, fmt.Errorf("Google returned no quota groups") //nolint:staticcheck // "Google" is a proper noun
	}
	return normalizeQuotaSnapshot(map[string]any{"observed_at": isoTime(time.Now()), "tier": map[string]any{"id": tierID, "name": tierName}, "groups": groups}, getString(account, "email"))
}

// quotaGroupsFromSummary turns Google's quota summary into saved groups. Each
// bucket is classified by its id prefix, and buckets without a known window,
// a remaining fraction or a reset time are skipped. The notes say what happened
// to every bucket so a missing model group can be explained.
func quotaGroupsFromSummary(summary map[string]any) ([]any, []string) {
	groups := make([]any, 0)
	notes := []string{"response fields: " + sortedKeys(summary)}
	for _, rawGroup := range getSlice(summary["groups"]) {
		group := getMap(rawGroup)
		notes = append(notes, describeQuotaGroup(group))
		groupID := ""
		buckets := make([]any, 0)
		type verdict struct{ id, window, reason string }
		var verdicts []verdict
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
			switch {
			case !oneOf(window, "weekly", "5h"):
				verdicts = append(verdicts, verdict{id, window, "skipped, window is not weekly or 5h"})
				continue
			case bucket["disabled"] == true:
				// Google marks a limit that does not apply right now, for example
				// the 5-hour limit while the weekly limit is used up.
				verdicts = append(verdicts, verdict{id, window, "skipped, disabled"})
				continue
			case !ok:
				verdicts = append(verdicts, verdict{id, window, "skipped, no remaining fraction"})
				continue
			case reset == "":
				verdicts = append(verdicts, verdict{id, window, "skipped, no reset time"})
				continue
			}
			fraction = max(0, min(1, fraction))
			buckets = append(buckets, map[string]any{"id": id, "name": cleanText(firstString(bucket["displayName"], window)), "window": window, "remaining_fraction": fraction, "reset_at": reset})
			verdicts = append(verdicts, verdict{id, window, ""})
		}
		keep := groupID != "" && len(buckets) > 0
		if keep {
			groups = append(groups, map[string]any{"id": groupID, "name": cleanText(firstString(group["displayName"], "Model group")), "buckets": buckets})
		}
		for _, v := range verdicts {
			reason := v.reason
			switch {
			case reason != "":
			case keep:
				reason = "kept as " + groupID
			case groupID == "":
				reason = "group dropped, bucket id has no gemini- or 3p- prefix"
			default:
				reason = "group dropped, no usable buckets"
			}
			notes = append(notes, fmt.Sprintf("bucket %q window %q: %s", v.id, v.window, reason))
		}
	}
	return groups, notes
}

func hasQuotaGroup(groups []any, id string) bool {
	for _, raw := range groups {
		if getString(getMap(raw), "id") == id {
			return true
		}
	}
	return false
}

// geminiGroupFromModels builds the Gemini group from the model list. Every
// Gemini model of an account reports the same quota, so the tightest model
// stands for the group. A model with a reset time but no fraction is empty
// (the protocol omits zero). The list does not name the window, so a reset
// within five hours is read as the 5-hour limit and anything later as weekly.
func geminiGroupFromModels(models map[string]any, now time.Time) map[string]any {
	var (
		found    bool
		fraction float64
		resetAt  time.Time
		resetRaw string
	)
	for id, entry := range getMap(models["models"]) {
		if !strings.HasPrefix(id, "gemini-") {
			continue
		}
		info := getMap(getMap(entry)["quotaInfo"])
		rawReset := getString(info, "resetTime")
		at, err := parseUTC(rawReset)
		if err != nil {
			continue
		}
		value, ok := getFloat(info["remainingFraction"])
		if !ok {
			value = 0
		}
		value = max(0, min(1, value))
		if !found || value < fraction || (value == fraction && at.Before(resetAt)) {
			found, fraction, resetAt, resetRaw = true, value, at, rawReset
		}
	}
	if !found {
		return nil
	}
	window, name := "weekly", "Weekly"
	if resetAt.Sub(now) <= 5*time.Hour+10*time.Minute {
		window, name = "5h", "5-Hour"
	}
	return map[string]any{"id": "gemini", "name": "Gemini models", "buckets": []any{
		map[string]any{"id": "gemini-" + window, "name": name, "window": window, "remaining_fraction": fraction, "reset_at": resetRaw},
	}}
}

// describeQuotaGroup names a group's fields and bucket fields, never values, so
// a missing model group can be traced to the shape of Google's answer.
func describeQuotaGroup(group map[string]any) string {
	buckets := getSlice(group["buckets"])
	line := fmt.Sprintf("group %q: %d buckets, fields: %s", cleanText(getString(group, "displayName")), len(buckets), sortedKeys(group))
	fields := map[string]any{}
	for _, raw := range buckets {
		for key := range getMap(raw) {
			fields[key] = true
		}
	}
	if len(fields) > 0 {
		line += "; bucket fields: " + sortedKeys(fields)
	}
	return line
}

// shapeOf names the fields of a JSON value, never its values, to the given depth.
func shapeOf(value any, depth int) string {
	switch typed := value.(type) {
	case map[string]any:
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		parts := make([]string, 0, len(keys))
		for _, key := range keys {
			child := ""
			if depth > 0 {
				child = shapeOf(typed[key], depth-1)
			}
			if child != "" {
				parts = append(parts, key+":"+child)
			} else {
				parts = append(parts, key)
			}
		}
		return "{" + strings.Join(parts, ", ") + "}"
	case []any:
		if len(typed) == 0 || depth <= 0 {
			return fmt.Sprintf("[%d]", len(typed))
		}
		return fmt.Sprintf("[%d]", len(typed)) + shapeOf(typed[0], depth-1)
	}
	return ""
}

func sortedKeys(values map[string]any) string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return strings.Join(keys, ", ")
}

// describeModelList asks for the model list and writes only its field names,
// to see where Google reports quota that the summary does not carry.
func (q *QuotaService) describeModelList(models map[string]any, err error) {
	if err != nil {
		fmt.Fprintf(q.diagnostics, "quota: fetchAvailableModels failed: %v\n", err)
		return
	}
	shape := shapeOf(models, 1)
	fmt.Fprintf(q.diagnostics, "quota: fetchAvailableModels fields: %s\n", shape)
	if lines := modelQuotaLines(models); len(lines) > 0 {
		fmt.Fprint(q.diagnostics, "quota: models\n  "+strings.Join(lines, "\n  ")+"\n")
	}
}

func quotaInfoSummary(info map[string]any) string {
	parts := make([]string, 0, len(info))
	for _, key := range strings.Split(sortedKeys(info), ", ") {
		if number, ok := getFloat(info[key]); ok {
			parts = append(parts, fmt.Sprintf("%s=%.4g", key, number))
		} else if text, ok := info[key].(string); ok {
			parts = append(parts, key+"="+cleanText(text))
		} else {
			parts = append(parts, fmt.Sprintf("%s=(%T)", key, info[key]))
		}
	}
	return strings.Join(parts, ", ")
}

// modelQuotaLines lists each model that carries quotaInfo (field names only)
// and how the model groups are made up.
func modelQuotaLines(models map[string]any) []string {
	lines := make([]string, 0)
	byID := getMap(models["models"])
	ids := make([]string, 0, len(byID))
	for id := range byID {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		if info := getMap(getMap(byID[id])["quotaInfo"]); info != nil {
			// Quota percentages and reset times are what `list` prints; no identity here.
			lines = append(lines, fmt.Sprintf("model %s: quotaInfo %s", cleanText(id), quotaInfoSummary(info)))
		}
	}
	for _, rawSort := range getSlice(models["agentModelSorts"]) {
		sorted := getMap(rawSort)
		for index, rawGroup := range getSlice(sorted["groups"]) {
			members := make([]string, 0)
			for _, id := range getSlice(getMap(rawGroup)["modelIds"]) {
				if text, ok := id.(string); ok {
					members = append(members, cleanText(text))
				}
			}
			lines = append(lines, fmt.Sprintf("model group %q %d: %s", cleanText(getString(sorted, "displayName")), index+1, strings.Join(members, ", ")))
		}
	}
	return lines
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
	return q.refreshSelected(ctx, accounts, force, progress, "")
}

func (q *QuotaService) refreshSelected(ctx context.Context, accounts *Accounts, force bool, progress quotaProgress, selected string) map[string]string {
	now := time.Now().UTC()
	type item struct {
		email   string
		account Account
	}
	var fetch []item
	done := 0
	for _, email := range accounts.Order {
		if selected != "" && selected != email {
			continue
		}
		account := accounts.ByEmail[email]
		if age, ok := quotaAge(account, now); !force && ok && age < quotaCache {
			done++
			if progress != nil {
				progress(done, accounts.Len(), account, "cached")
			}
		} else {
			copyAccount := make(Account, len(account))
			for k, v := range account {
				copyAccount[k] = v
			}
			fetch = append(fetch, item{email, copyAccount})
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
	var replaced []string
	for result := range results {
		done++
		state := "synced"
		original := accounts.ByEmail[result.item.email]
		if updated := getString(result.item.account, "token_data"); updated != "" && updated != getString(original, "token_data") {
			if q.vault != nil {
				app := &Application{vault: q.vault}
				old, ok := app.saveAccountSecret(ctx, result.item.account, updated)
				if !ok {
					errorsByEmail["storage"] = "Vault unavailable; refreshed token stored in private accounts file"
				} else if old != "" {
					replaced = append(replaced, old)
				}
			} else {
				result.item.account["token_hash"] = hashToken(updated)
				rememberTokenExpiry(result.item.account, updated)
			}
			for _, key := range []string{"secret_ref", "token_data", "token_hash", "access_expires_at"} {
				if value, ok := result.item.account[key]; ok {
					original[key] = value
				} else {
					delete(original, key)
				}
			}
			changed = true
		}
		if result.err != nil {
			errorsByEmail[result.item.email] = result.err.Error()
			state = "failed"
		} else {
			original["quota_snapshot"] = result.snapshot
			changed = true
		}
		if progress != nil {
			progress(done, accounts.Len(), original, state)
		}
	}
	if changed {
		if err := q.store.Save(accounts); err != nil {
			errorsByEmail["store"] = err.Error()
		} else if q.vault != nil {
			(&Application{vault: q.vault}).deleteReplacedSecrets(ctx, replaced)
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
	return formatTokenReset(expiry, getString(inner, "refresh_token") != "")
}

func tokenResetFromExpiry(value string) (string, bool) {
	expiry, err := parseUTC(strings.TrimSpace(value))
	if err != nil || expiry.IsZero() {
		return "", false
	}
	return formatTokenReset(expiry, false)
}

func formatTokenReset(expiry time.Time, refreshAvailable bool) (string, bool) {
	if expiry.IsZero() {
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
	if refreshAvailable {
		return "Access expired · refresh token available", true
	}
	return fmt.Sprintf("Expired %dm ago", int((-diff).Minutes())), true
}
