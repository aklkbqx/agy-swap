package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
)

func metricAccountID(email string) string {
	digest := sha256.Sum256([]byte(strings.ToLower(email)))
	return hex.EncodeToString(digest[:])[:12]
}

func (a *Application) metricsSnapshot(ctx context.Context, refresh bool) (map[string]any, error) {
	accounts, err := a.store.Load(true)
	if err != nil {
		return nil, err
	}
	failures := map[string]string{}
	if refresh {
		failures = a.quota.Refresh(ctx, accounts, true, nil)
	}
	samples := make([]map[string]any, 0)
	for _, email := range accounts.Order {
		account := accounts.ByEmail[email]
		item := map[string]any{"account_id": metricAccountID(email), "email": email}
		for _, rawGroup := range quotaGroups(account) {
			group := getMap(rawGroup)
			for _, rawBucket := range getSlice(group["buckets"]) {
				bucket := getMap(rawBucket)
				fraction, _ := getFloat(bucket["remaining_fraction"])
				item["family_"+getString(group, "id")] = fraction
				item["reset_"+getString(bucket, "id")] = getString(bucket, "reset_at")
			}
		}
		if failure := failures[email]; failure != "" {
			item["error"] = failure
		}
		samples = append(samples, item)
	}
	return map[string]any{"schema": stateSchema, "generated_at": isoTime(time.Now().UTC()), "accounts": samples}, nil
}

func (a *Application) cmdMetrics(ctx context.Context, opts extendedOptions, positional []string) int {
	sub := "render"
	if len(positional) > 0 {
		sub = positional[0]
	}
	snapshot, err := a.metricsSnapshot(ctx, opts.Refresh)
	if err != nil {
		return a.extendedError("metrics", opts, err)
	}
	if sub == "json" || opts.JSON {
		return a.extendedResult("metrics "+sub, opts, snapshot, nil)
	}
	if sub != "render" && sub != "prometheus" {
		return a.extendedError("metrics", opts, errors.New("usage: metrics [render|prometheus|json]"))
	}
	var lines []string
	lines = append(lines, "# HELP agy_swap_up agy-swap local metrics exporter status", "# TYPE agy_swap_up gauge", "agy_swap_up 1")
	for _, raw := range snapshot["accounts"].([]map[string]any) {
		id := raw["account_id"]
		if errorText, ok := raw["error"].(string); ok {
			_ = errorText
			lines = append(lines, fmt.Sprintf("agy_swap_account_error{account=\"%v\"} 1", id))
		}
		for key, value := range raw {
			keyText := key
			if !strings.HasPrefix(keyText, "family_") {
				continue
			}
			family := strings.TrimPrefix(keyText, "family_")
			fraction, ok := value.(float64)
			if !ok {
				continue
			}
			lines = append(lines, fmt.Sprintf("agy_swap_quota_remaining{account=\"%v\",family=\"%s\"} %g", id, family, fraction))
		}
	}
	for _, line := range lines {
		fmt.Fprintln(a.Out, line)
	}
	return 0
}
