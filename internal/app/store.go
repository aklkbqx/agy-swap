package app

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Store struct {
	paths   Paths
	scanner *LogScanner
}

func NewStore(paths Paths) *Store {
	store := &Store{paths: paths}
	store.scanner = NewLogScanner(paths)
	return store
}

func (s *Store) Load(syncLogs bool) (*Accounts, error) {
	data, err := os.ReadFile(s.paths.Accounts)
	if errors.Is(err, os.ErrNotExist) {
		return NewAccounts(), nil
	}
	if err != nil {
		return nil, fmt.Errorf("cannot read %s: %w", s.paths.Accounts, err)
	}
	accounts, err := decodeOrderedAccounts(data)
	if err != nil {
		return nil, fmt.Errorf("cannot read %s: %w", s.paths.Accounts, err)
	}
	if stat, statErr := os.Stat(s.paths.Accounts); statErr == nil {
		accounts.Revision = stat.ModTime().UnixNano()
	}
	if syncLogs && accounts.Len() > 0 {
		changed := migrateAndExpire(accounts, time.Now().UTC())
		limits, evidence, scanErr := s.scanner.Scan()
		if scanErr == nil {
			for _, email := range accounts.Order {
				account := accounts.ByEmail[email]
				if reconcileLogLimits(account, limits[email], evidence[email]) {
					changed = true
				}
			}
		}
		if changed {
			if saveErr := s.Save(accounts); saveErr != nil && !errors.Is(saveErr, errStoreConflict) {
				return nil, saveErr
			}
		}
	}
	return accounts, nil
}

func (s *Store) Save(accounts *Accounts) error {
	if err := validateAccounts(accounts); err != nil {
		return err
	}
	payload, err := encodeOrderedAccounts(accounts)
	if err != nil {
		return err
	}
	lock, err := acquireFileLock(s.paths.AccountsLock)
	if err != nil {
		return err
	}
	defer lock.Close()
	currentRevision := int64(0)
	if stat, statErr := os.Stat(s.paths.Accounts); statErr == nil {
		currentRevision = stat.ModTime().UnixNano()
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return statErr
	}
	if accounts.Revision != 0 && accounts.Revision != currentRevision {
		return errStoreConflict
	}
	if currentRevision != 0 {
		previous, readErr := os.ReadFile(s.paths.Accounts)
		if readErr != nil {
			return readErr
		}
		if err := atomicWrite(s.paths.AccountsBackup, previous, 0o600); err != nil {
			return err
		}
	}
	if err := atomicWrite(s.paths.Accounts, payload, 0o600); err != nil {
		return err
	}
	stat, err := os.Stat(s.paths.Accounts)
	if err != nil {
		return err
	}
	accounts.Revision = stat.ModTime().UnixNano()
	return nil
}

func validateAccounts(accounts *Accounts) error {
	seen := make(map[string]struct{}, accounts.Len())
	for _, key := range accounts.Order {
		account, ok := accounts.ByEmail[key]
		if !ok || account == nil {
			return fmt.Errorf("accounts.json contains an invalid account entry")
		}
		email := normalizeEmail(firstString(account["email"], key))
		if email == "" {
			return fmt.Errorf("invalid account email: %q", key)
		}
		if _, exists := seen[email]; exists {
			return fmt.Errorf("duplicate account email: %s", email)
		}
		seen[email] = struct{}{}
		account["email"] = email
		account["name"] = firstString(account["name"], "Google User")
		if token, ok := account["token_data"].(string); ok {
			if decodeToken(token) == nil {
				return fmt.Errorf("invalid saved token for %s", email)
			}
			if claimed := extractVerifiedEmail(token); claimed != "" && claimed != email {
				return fmt.Errorf("saved token email does not match %s", email)
			}
		}
		if err := normalizeQuotaLimits(account, email); err != nil {
			return err
		}
		if raw, ok := account["quota_snapshot"]; ok {
			norm, err := normalizeQuotaSnapshot(raw, email)
			if err != nil {
				return err
			}
			account["quota_snapshot"] = norm
		}
	}
	return nil
}

func normalizeQuotaLimits(account Account, email string) error {
	raw, exists := account["quota_limits"]
	if !exists || raw == nil {
		return nil
	}
	limits, ok := raw.(map[string]any)
	if !ok {
		return fmt.Errorf("invalid quota limits for %s", email)
	}
	normalized := make(map[string]any)
	for key, rawLimit := range limits {
		limit, ok := rawLimit.(map[string]any)
		if !ok {
			return fmt.Errorf("invalid quota limit for %s", email)
		}
		model := cleanText(getString(limit, "model"))
		family := getString(limit, "family")
		source := getString(limit, "source")
		if model == "" || !oneOf(family, "claude", "gemini", "gpt") || !oneOf(source, "log", "manual") {
			return fmt.Errorf("invalid quota metadata for %s", email)
		}
		resetAt, err1 := parseUTC(getString(limit, "reset_at"))
		observedAt, err2 := parseUTC(getString(limit, "observed_at"))
		if err1 != nil || err2 != nil {
			return fmt.Errorf("invalid quota timestamp for %s", email)
		}
		norm := map[string]any{
			"model": model, "family": family, "reset_at": isoTime(resetAt),
			"observed_at": isoTime(observedAt), "source": source,
		}
		if sourceFile := filepath.Base(strings.ReplaceAll(cleanText(getString(limit, "source_file")), "\\", "/")); sourceFile != "." && sourceFile != "" {
			norm["source_file"] = sourceFile
		}
		normalized[key] = norm
	}
	if len(normalized) == 0 {
		delete(account, "quota_limits")
	} else {
		account["quota_limits"] = normalized
	}
	return nil
}

func normalizeQuotaSnapshot(raw any, email string) (map[string]any, error) {
	snapshot, ok := raw.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("invalid quota snapshot for %s", email)
	}
	observed, err := parseUTC(getString(snapshot, "observed_at"))
	if err != nil {
		return nil, fmt.Errorf("invalid quota snapshot timestamp for %s", email)
	}
	tier := getMap(snapshot["tier"])
	tierID, tierName := cleanText(getString(tier, "id")), cleanText(getString(tier, "name"))
	if tierID == "" || tierName == "" {
		return nil, fmt.Errorf("invalid quota tier for %s", email)
	}
	rawGroups := getSlice(snapshot["groups"])
	if len(rawGroups) == 0 {
		return nil, fmt.Errorf("invalid quota groups for %s", email)
	}
	groups := make([]any, 0, minInt(len(rawGroups), 8))
	for _, rawGroup := range rawGroups[:minInt(len(rawGroups), 8)] {
		group := getMap(rawGroup)
		groupID, name := getString(group, "id"), cleanText(getString(group, "name"))
		bucketsRaw := getSlice(group["buckets"])
		if !oneOf(groupID, "gemini", "third_party") || name == "" || len(bucketsRaw) == 0 {
			return nil, fmt.Errorf("invalid quota group for %s", email)
		}
		buckets := make([]any, 0, minInt(len(bucketsRaw), 8))
		for _, rawBucket := range bucketsRaw[:minInt(len(bucketsRaw), 8)] {
			bucket := getMap(rawBucket)
			window := getString(bucket, "window")
			id, bucketName := cleanText(getString(bucket, "id")), cleanText(getString(bucket, "name"))
			fraction, validFraction := getFloat(bucket["remaining_fraction"])
			reset, resetErr := parseUTC(getString(bucket, "reset_at"))
			if !oneOf(window, "weekly", "5h") || id == "" || bucketName == "" || !validFraction || fraction < 0 || fraction > 1 || resetErr != nil {
				return nil, fmt.Errorf("invalid quota bucket for %s", email)
			}
			buckets = append(buckets, map[string]any{"id": id, "name": bucketName, "window": window, "remaining_fraction": fraction, "reset_at": isoTime(reset)})
		}
		groups = append(groups, map[string]any{"id": groupID, "name": name, "buckets": buckets})
	}
	return map[string]any{"observed_at": isoTime(observed), "tier": map[string]any{"id": tierID, "name": tierName}, "groups": groups}, nil
}

func migrateAndExpire(accounts *Accounts, now time.Time) bool {
	changed := false
	legacyKeys := []string{"limit_reset", "limit_reset_claude", "limit_reset_gemini", "claude_pct", "gemini_pct"}
	for _, email := range accounts.Order {
		account := accounts.ByEmail[email]
		schema, _ := numberInt(account["quota_schema"])
		if schema != quotaSchema {
			legacy := getMap(account["legacy_quota"])
			if legacy == nil {
				legacy = make(map[string]any)
			}
			for _, key := range legacyKeys {
				if value, ok := account[key]; ok {
					legacy[key] = value
					delete(account, key)
					changed = true
				}
			}
			if len(legacy) > 0 {
				account["legacy_quota"] = legacy
			}
			account["quota_schema"] = quotaSchema
			changed = true
		}
		limits := getMap(account["quota_limits"])
		for key, raw := range limits {
			limit := getMap(raw)
			reset, err := parseUTC(getString(limit, "reset_at"))
			if err != nil || !reset.After(now) || reset.Sub(now) > maxLimitDuration {
				delete(limits, key)
				changed = true
			}
		}
		if len(limits) == 0 {
			delete(account, "quota_limits")
		}
	}
	return changed
}

func reconcileLogLimits(account Account, detected map[string]LimitRecord, evidence map[string]EvidenceRecord) bool {
	limits := getMap(account["quota_limits"])
	if limits == nil {
		limits = make(map[string]any)
	}
	changed := false
	for key, raw := range limits {
		limit := getMap(raw)
		if getString(limit, "source") != "log" {
			continue
		}
		event, ok := evidence[modelIdentity(getString(limit, "model"))]
		if !ok {
			continue
		}
		eventTime, _ := parseUTC(event.ObservedAt)
		limitTime, _ := parseUTC(getString(limit, "observed_at"))
		if !eventTime.Before(limitTime) {
			delete(limits, key)
			changed = true
		}
	}
	for key, record := range detected {
		existing := getMap(limits[key])
		newTime, _ := parseUTC(record.ObservedAt)
		oldTime, oldErr := parseUTC(getString(existing, "observed_at"))
		if existing == nil || oldErr != nil || newTime.After(oldTime) {
			limits[key] = record.Map()
			changed = true
		}
	}
	if len(limits) == 0 {
		delete(account, "quota_limits")
	} else {
		account["quota_limits"] = limits
	}
	return changed
}

func oneOf(value string, options ...string) bool {
	for _, option := range options {
		if value == option {
			return true
		}
	}
	return false
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
