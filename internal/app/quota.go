package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"
)

type quotaProgress func(index, total int, account Account, state string)

type QuotaService struct {
	http         *HTTPService
	store        *Store
	vault        AccountVault
	pollInterval time.Duration
	// diagnostics, when set, receives one line per quota bucket explaining
	// whether it was kept. It never receives tokens or account identifiers.
	diagnostics io.Writer
	// now is the clock, for tests.
	now func() time.Time

	mu sync.Mutex
	// projects remembers each account's Code Assist project and tier, so a
	// repeat read asks Google once instead of twice. Any failed read forgets
	// it, and it lasts quotaProjectMemo, so eligibility is checked again.
	projects map[string]quotaProject
	// backoffs holds the accounts Google refused (429, 5xx) and until when.
	backoffs map[string]*quotaBackoffState
}

const (
	quotaProjectMemo = 15 * time.Minute
	// quotaDueSlack lets a snapshot a few seconds short of quotaCache count as
	// due, so a timer that fires as it turns a minute old never skips it.
	quotaDueSlack = 5 * time.Second
)

type quotaProject struct {
	project string
	tier    map[string]any
	at      time.Time
}

type quotaBackoffState struct {
	backoff quotaBackoff
	until   time.Time
}

func (q *QuotaService) clock() time.Time {
	if q.now != nil {
		return q.now()
	}
	return time.Now()
}

func (q *QuotaService) rememberedProject(email string) (quotaProject, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	memo, ok := q.projects[email]
	if !ok || q.clock().Sub(memo.at) >= quotaProjectMemo {
		return quotaProject{}, false
	}
	return memo, true
}

func (q *QuotaService) rememberProject(email string, memo quotaProject) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.projects == nil {
		q.projects = map[string]quotaProject{}
	}
	memo.at = q.clock()
	q.projects[email] = memo
}

func (q *QuotaService) forgetProject(email string) {
	q.mu.Lock()
	defer q.mu.Unlock()
	delete(q.projects, email)
}

// BackoffUntil is when an account Google refused may be read again; zero
// when it is not backing off.
func (q *QuotaService) BackoffUntil(email string) time.Time {
	q.mu.Lock()
	defer q.mu.Unlock()
	if state, ok := q.backoffs[email]; ok {
		return state.until
	}
	return time.Time{}
}

func (q *QuotaService) noteRead(email string, err error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	switch {
	case err == nil:
		delete(q.backoffs, email)
	case backsOff(err):
		if q.backoffs == nil {
			q.backoffs = map[string]*quotaBackoffState{}
		}
		state, ok := q.backoffs[email]
		if !ok {
			state = &quotaBackoffState{}
			q.backoffs[email] = state
		}
		state.until = state.backoff.fail(q.clock(), rand.Float64)
	}
}

func NewQuotaService(httpService *HTTPService, store *Store) *QuotaService {
	return &QuotaService{http: httpService, store: store, pollInterval: 500 * time.Millisecond}
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
	email := getString(account, "email")
	memo, remembered := q.rememberedProject(email)
	project, tier := memo.project, memo.tier
	if !remembered {
		info, loadErr := q.http.cloudPost(ctx, access, "loadCodeAssist", map[string]any{"metadata": map[string]any{"ideType": "ANTIGRAVITY"}})
		if q.diagnostics != nil {
			q.describeLoadCodeAssist(info, loadErr)
		}
		if loadErr != nil {
			return nil, ineligibleOr(loadErr)
		}
		if ineligible := eligibilityFromLoad(info); ineligible != nil {
			return nil, ineligible
		}
		project = extractCloudProject(info["cloudaicompanionProject"])
		var onboardedTier map[string]any
		if project == "" {
			var onboardErr error
			project, onboardedTier, onboardErr = q.onboardProject(ctx, access, info)
			if onboardErr != nil {
				return nil, onboardErr
			}
		}
		if project == "" {
			return nil, fmt.Errorf("Google returned no Code Assist project") //nolint:staticcheck // "Google" is a proper noun
		}
		tier = getMap(info["paidTier"])
		if tier == nil {
			tier = getMap(info["currentTier"])
		}
		if tier == nil {
			tier = onboardedTier
		}
	}
	summary, err := q.http.cloudPost(ctx, access, "retrieveUserQuotaSummary", map[string]any{"project": project})
	if err != nil {
		q.forgetProject(email)
		return nil, markEligibilityPassed(ineligibleOr(err))
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
	tierID := cleanText(getString(tier, "id"))
	if tierID == "" {
		q.forgetProject(email)
		return nil, markEligibilityPassed(fmt.Errorf("Google returned no account tier")) //nolint:staticcheck // "Google" is a proper noun
	}
	if !remembered {
		q.rememberProject(email, quotaProject{project: project, tier: tier})
	}
	tierName := tierNames[tierID]
	if tierName == "" {
		tierName = cleanText(firstString(tier["name"], tierID))
	}
	groups, notes := quotaGroupsFromSummary(summary)
	if !hasQuotaGroup(groups, "gemini") {
		// Google's summary can leave Gemini out. The model list still carries
		// the Gemini quota, one value for every Gemini model of the account.
		if list, loadErr := loadModels(); loadErr == nil {
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
		return nil, markEligibilityPassed(fmt.Errorf("Google returned no quota groups")) //nolint:staticcheck // "Google" is a proper noun
	}
	// Reset times are Google's; on a machine whose clock is off they are moved
	// onto this clock, so countdowns and schedules agree with what Google means.
	shiftQuotaResets(groups, -q.http.clockSkew())
	snapshot, err := normalizeQuotaSnapshot(map[string]any{"observed_at": isoTime(time.Now()), "tier": map[string]any{"id": tierID, "name": tierName}, "groups": groups}, getString(account, "email"))
	if err != nil {
		q.forgetProject(email)
		return nil, markEligibilityPassed(err)
	}
	return snapshot, nil
}

func extractCloudProject(raw any) string {
	if s, ok := raw.(string); ok {
		return strings.TrimSpace(s)
	}
	if m := getMap(raw); m != nil {
		return strings.TrimSpace(getString(m, "id"))
	}
	return ""
}

func (q *QuotaService) onboardProject(ctx context.Context, access string, info map[string]any) (string, map[string]any, error) {
	tierID := allowedTierID(info)
	if tierID == "" {
		return "", nil, nil
	}
	op, err := q.http.cloudPost(ctx, access, "onboardUser", map[string]any{"tierId": tierID, "metadata": map[string]any{"ideType": "ANTIGRAVITY"}})
	if err != nil {
		return "", nil, ineligibleOr(err)
	}
	delay := q.pollInterval
	if delay <= 0 {
		delay = 500 * time.Millisecond
	}
	for range 5 {
		if done, _ := op["done"].(bool); done {
			break
		}
		name := getString(op, "name")
		if name == "" {
			break
		}
		select {
		case <-ctx.Done():
			return "", nil, ctx.Err()
		case <-time.After(delay):
		}
		op, err = q.http.cloudGet(ctx, access, name)
		if err != nil {
			return "", nil, ineligibleOr(err)
		}
	}
	if opErr := getMap(op["error"]); opErr != nil {
		if ineligible := eligibilityFromOperationError(opErr); ineligible != nil {
			return "", nil, ineligible
		}
		if msg := cleanText(getString(opErr, "message")); msg != "" {
			return "", nil, fmt.Errorf("onboardUser failed: %s", msg)
		}
		return "", nil, fmt.Errorf("onboardUser failed")
	}
	project := extractCloudProject(getMap(op["response"])["cloudaicompanionProject"])
	return project, map[string]any{"id": tierID}, nil
}

// quotaGroupsFromSummary turns Google's quota summary into saved groups. Each
// group is named by its bucket ids (see quotaPoolForBucket), groups that share
// a name are merged, and buckets without a well-formed window, a remaining
// fraction or a reset time are skipped. The notes say what happened to every
// bucket so a missing model group can be explained.
func quotaGroupsFromSummary(summary map[string]any) ([]any, []string) {
	groups := make([]any, 0)
	byID := make(map[string]map[string]any)
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
			window := getString(bucket, "window")
			if groupID == "" {
				groupID = quotaPoolForBucket(id, window)
			}
			fraction, ok := getFloat(bucket["remainingFraction"])
			reset := getString(bucket, "resetTime")
			switch {
			case !validQuotaWindow(window):
				verdicts = append(verdicts, verdict{id, window, "skipped, window is not a plain name"})
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
			if existing := byID[groupID]; existing != nil {
				existing["buckets"] = append(getSlice(existing["buckets"]), buckets...)
			} else {
				saved := map[string]any{"id": groupID, "name": cleanText(firstString(group["displayName"], "Model group")), "buckets": buckets}
				byID[groupID] = saved
				groups = append(groups, saved)
			}
		}
		for _, v := range verdicts {
			reason := v.reason
			switch {
			case reason != "":
			case keep:
				reason = "kept as " + groupID
			case groupID == "":
				reason = "group dropped, bucket id gives no pool name"
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
		if !strings.HasPrefix(id, "gemini-") || strings.Contains(id, "-argon") {
			// Argon models carry the Argon pool, not the Gemini one.
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
			safeKey := cleanText(key)
			child := ""
			if depth > 0 {
				child = shapeOf(typed[key], depth-1)
			}
			if child != "" {
				parts = append(parts, safeKey+":"+child)
			} else {
				parts = append(parts, safeKey)
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
		keys = append(keys, cleanText(key))
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

// ineligibleOr turns a Google error that says the account must be verified
// into an *eligibilityError and returns any other error unchanged.
func ineligibleOr(err error) error {
	var cloud *cloudError
	if errors.As(err, &cloud) {
		if ineligible := eligibilityFromErrorBody(cloud.Status, cloud.Body); ineligible != nil {
			return ineligible
		}
	}
	return err
}

// describeLoadCodeAssist writes the shape of Google's answer, its reason codes
// and the hosts of any links: never a token, email, project or full link.
func (q *QuotaService) describeLoadCodeAssist(info map[string]any, err error) {
	var cloud *cloudError
	if errors.As(err, &cloud) {
		var body map[string]any
		_ = json.Unmarshal(cloud.Body, &body)
		lines := []string{fmt.Sprintf("loadCodeAssist HTTP %d body fields: %s", cloud.Status, shapeOf(body, 3))}
		for _, raw := range getSlice(getMap(body["error"])["details"]) {
			detail := getMap(raw)
			lines = append(lines, "reason: "+cleanText(getString(detail, "reason")))
			lines = append(lines, linkHosts(getMap(detail["metadata"]))...)
		}
		fmt.Fprint(q.diagnostics, "quota: "+strings.Join(lines, "\n  ")+"\n")
		return
	}
	if err != nil {
		fmt.Fprintf(q.diagnostics, "quota: loadCodeAssist failed: %v\n", err)
		return
	}
	lines := []string{"loadCodeAssist fields: " + shapeOf(info, 2)}
	for _, raw := range getSlice(info["ineligibleTiers"]) {
		tier := getMap(raw)
		lines = append(lines, "ineligible tier reason: "+cleanText(getString(tier, "reasonCode")))
		lines = append(lines, linkHosts(tier)...)
	}
	fmt.Fprint(q.diagnostics, "quota: "+strings.Join(lines, "\n  ")+"\n")
}

// linkHosts names each link field with its host only.
func linkHosts(fields map[string]any) []string {
	keys := make([]string, 0, len(fields))
	for key := range fields {
		lower := strings.ToLower(key)
		if strings.HasSuffix(lower, "url") || strings.HasSuffix(lower, "uri") {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	lines := make([]string, 0, len(keys))
	for _, key := range keys {
		host := "(unparsed)"
		if u, err := url.Parse(getString(fields, key)); err == nil && u.Hostname() != "" {
			host = cleanText(u.Hostname())
		}
		lines = append(lines, cleanText(key)+" host "+host)
	}
	return lines
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
	return q.refreshWhere(ctx, accounts, force, progress, func(string) bool { return true })
}

func (q *QuotaService) refreshSelected(ctx context.Context, accounts *Accounts, force bool, progress quotaProgress, selected string) map[string]string {
	return q.refreshWhere(ctx, accounts, force, progress, func(email string) bool { return selected == "" || selected == email })
}

// RefreshEmails reads the quota of these accounts now, however fresh it is.
func (q *QuotaService) RefreshEmails(ctx context.Context, accounts *Accounts, emails []string) map[string]string {
	want := make(map[string]bool, len(emails))
	for _, email := range emails {
		want[email] = true
	}
	return q.refreshWhere(ctx, accounts, true, nil, func(email string) bool { return want[email] })
}

func (q *QuotaService) refreshWhere(ctx context.Context, accounts *Accounts, force bool, progress quotaProgress, pick func(email string) bool) map[string]string {
	now := time.Now().UTC()
	type item struct {
		email   string
		account Account
	}
	var fetch []item
	done := 0
	for _, email := range accounts.Order {
		if !pick(email) {
			continue
		}
		account := accounts.ByEmail[email]
		if age, ok := quotaAge(account, now); !force && ok && age < quotaCache-quotaDueSlack {
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
		var ineligible *eligibilityError
		switch {
		case errors.As(result.err, &ineligible):
			original["eligibility"] = ineligible.record(now)
			changed = true
		case shouldClearEligibility(result.err):
			if _, ok := original["eligibility"]; ok {
				delete(original, "eligibility")
				changed = true
			}
		}
		q.noteRead(result.item.email, result.err)
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

// shiftQuotaResets moves every bucket's reset time by shift.
func shiftQuotaResets(groups []any, shift time.Duration) {
	if shift == 0 {
		return
	}
	for _, group := range groups {
		for _, raw := range getSlice(getMap(group)["buckets"]) {
			bucket := getMap(raw)
			if reset, err := parseUTC(getString(bucket, "reset_at")); err == nil {
				bucket["reset_at"] = isoTime(reset.Add(shift))
			}
		}
	}
}
