package app

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// eligibilityError is Google's answer that an account cannot use Antigravity
// until its owner acts in a browser: verify the account or appeal a block.
type eligibilityError struct {
	Reason  string
	Message string
	URL     string // a checked Google page, or empty
}

func (e *eligibilityError) Error() string {
	if e.Message != "" {
		return eligibilityErrorPrefix + ": " + e.Message
	}
	return eligibilityErrorPrefix + " before Antigravity can use it"
}

// eligibilityErrorPrefix starts every eligibilityError message.
const eligibilityErrorPrefix = "Google needs this account verified"

// verificationReason says whether Google's reason code asks for verification.
func verificationReason(reason string) bool {
	reason = strings.ToUpper(reason)
	return strings.Contains(reason, "VALIDATION") || strings.Contains(reason, "VERIF")
}

// verificationText matches either a verification reason code or explicit
// verification/appeal wording in Google's user-facing message.
func verificationText(reason, message string) bool {
	if verificationReason(reason) {
		return true
	}
	lower := strings.ToLower(message)
	return strings.Contains(lower, "verify") || strings.Contains(lower, "validation") || strings.Contains(lower, "appeal")
}

// eligibilityFromErrorBody reads a Google RPC error. Only a 400 or 403
// google.rpc.ErrorInfo detail carrying a verification/appeal page or
// verification intent counts.
func eligibilityFromErrorBody(status int, body []byte) *eligibilityError {
	if status != http.StatusBadRequest && status != http.StatusForbidden {
		return nil
	}
	var envelope struct {
		Error struct {
			Details []struct {
				Type     string            `json:"@type"`
				Reason   string            `json:"reason"`
				Metadata map[string]string `json:"metadata"`
			} `json:"details"`
		} `json:"error"`
	}
	if json.Unmarshal(body, &envelope) != nil {
		return nil
	}
	for _, detail := range envelope.Error.Details {
		if detail.Type != "" && !strings.HasSuffix(detail.Type, "/google.rpc.ErrorInfo") {
			continue
		}
		meta := detail.Metadata
		link := firstNonEmpty(meta["validation_url"], meta["appeal_url"])
		message := cleanText(meta["uiMessage"])
		if link == "" && !verificationText(detail.Reason, message) {
			continue
		}
		return &eligibilityError{Reason: cleanText(detail.Reason), Message: message, URL: safeGoogleURL(link)}
	}
	return nil
}

// eligibilityFromOperationError extracts an eligibilityError from a Google
// Long-Running Operation error payload (onboardUser).
func eligibilityFromOperationError(opErr map[string]any) *eligibilityError {
	if opErr == nil {
		return nil
	}
	raw, err := json.Marshal(map[string]any{"error": opErr})
	if err != nil {
		return nil
	}
	return eligibilityFromErrorBody(http.StatusForbidden, raw)
}

// allowedTierID returns a usable Antigravity tier offered by loadCodeAssist,
// preferring free-tier (matching Antigravity's ValidateAndOnboardAccount).
func allowedTierID(info map[string]any) string {
	tiers := getSlice(info["allowedTiers"])
	for _, raw := range tiers {
		if cleanText(getString(getMap(raw), "id")) == "free-tier" {
			return "free-tier"
		}
	}
	for _, raw := range tiers {
		tier := getMap(raw)
		if isDefault, _ := tier["isDefault"].(bool); isDefault {
			if id := cleanText(getString(tier, "id")); id != "" {
				return id
			}
		}
	}
	return ""
}

// eligibilityFromLoad reads loadCodeAssist's ineligible tiers. An account
// with a project and a tier (or an offered allowed tier awaiting onboarding)
// is not blocked by unrelated tier notices.
func eligibilityFromLoad(info map[string]any) *eligibilityError {
	if extractCloudProject(info["cloudaicompanionProject"]) != "" && (getMap(info["currentTier"]) != nil || getMap(info["paidTier"]) != nil) {
		return nil
	}
	if allowedTierID(info) != "" {
		return nil
	}
	ineligible := getSlice(info["ineligibleTiers"])
	checkTier := func(tier map[string]any) *eligibilityError {
		link := firstNonEmpty(getString(tier, "validationUrl"), getString(tier, "appealUrl"))
		message := cleanText(firstNonEmpty(getString(tier, "reasonMessage"), getString(tier, "validationErrorMessage")))
		reason := getString(tier, "reasonCode")
		if link == "" && !verificationText(reason, message) {
			return nil
		}
		return &eligibilityError{Reason: cleanText(reason), Message: message, URL: safeGoogleURL(link)}
	}
	for _, raw := range ineligible {
		tier := getMap(raw)
		tierID := strings.ToLower(strings.TrimSpace(getString(tier, "tierId")))
		if tierID == "" || tierID == "free-tier" {
			if hit := checkTier(tier); hit != nil {
				return hit
			}
		}
	}
	for _, raw := range ineligible {
		if hit := checkTier(getMap(raw)); hit != nil {
			return hit
		}
	}
	return nil
}

// safeGoogleURL returns raw only when it is an https page on google.com,
// *.google.com, or *.google.
func safeGoogleURL(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme != "https" || u.User != nil {
		return ""
	}
	host := strings.ToLower(u.Hostname())
	isGoogleCom := host == "google.com" || strings.HasSuffix(host, ".google.com")
	isDotGoogle := strings.HasSuffix(host, ".google") && strings.Count(host, ".") >= 1
	if !isGoogleCom && !isDotGoogle {
		return ""
	}
	return u.String()
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func (e *eligibilityError) record(now time.Time) map[string]any {
	return map[string]any{"state": "verify_required", "reason": e.Reason, "message": e.Message, "url": e.URL, "checked_at": isoTime(now)}
}

// accountEligibility reads the stored record and checks its link again.
func accountEligibility(account Account) (*eligibilityError, bool) {
	record := getMap(account["eligibility"])
	if getString(record, "state") != "verify_required" {
		return nil, false
	}
	return &eligibilityError{Reason: cleanText(getString(record, "reason")), Message: cleanText(getString(record, "message")), URL: safeGoogleURL(getString(record, "url"))}, true
}

// eligibilityPassedError marks errors occurring after Google confirmed the
// account has a valid Code Assist project and passed eligibility checks.
type eligibilityPassedError struct {
	err error
}

func (e eligibilityPassedError) Error() string { return e.err.Error() }
func (e eligibilityPassedError) Unwrap() error { return e.err }

func markEligibilityPassed(err error) error {
	if err == nil {
		return nil
	}
	var ineligible *eligibilityError
	if errors.As(err, &ineligible) {
		return err
	}
	return eligibilityPassedError{err: err}
}

// shouldClearEligibility clears a stored verify_required record only when
// Google positively confirmed eligibility or when credentials failed
// permanently (so sign-in errors replace VERIFY, while OAuth 5xx/429 and
// captive portals preserve the saved verification link).
func shouldClearEligibility(err error) bool {
	if err == nil {
		return true
	}
	var passed eligibilityPassedError
	if errors.As(err, &passed) {
		return true
	}
	return authInvalidationError(err)
}

func authInvalidationError(err error) bool {
	var cloud *cloudError
	if errors.As(err, &cloud) && cloud.Status == http.StatusUnauthorized {
		return true
	}
	msg := err.Error()
	return strings.Contains(msg, "refresh token is unavailable") ||
		strings.Contains(msg, "OAuth refresh failed (HTTP 400)") ||
		strings.Contains(msg, "OAuth refresh failed (HTTP 401)") ||
		strings.Contains(msg, "refreshed token identity does not match") ||
		strings.Contains(msg, "account has no saved token") ||
		strings.Contains(msg, "secret vault entry is unavailable")
}
