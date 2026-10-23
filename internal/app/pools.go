package app

import (
	"regexp"
	"strings"
)

// Quota pools are the groups Google reports per account. Gemini and Claude/GPT
// are shared by most models; Argon is a weekly pool of its own on some Pro
// accounts (Ultra counts Argon against the Gemini pool). Pools Google adds
// later are kept under an id taken from their bucket ids, so they show up
// without a release.

const (
	poolGemini     = "gemini"
	poolArgon      = "argon"
	poolThirdParty = "third_party"
)

// quotaFamilies are the model families a user can ask for with --family.
var quotaFamilies = []string{"claude", "gemini", "gpt", "argon"}

func isQuotaFamily(value string) bool { return oneOf(value, quotaFamilies...) }

var (
	quotaIDPattern     = regexp.MustCompile(`^[a-z0-9][a-z0-9_]{0,31}$`)
	quotaWindowPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_]{0,15}$`)
)

func validQuotaID(id string) bool         { return quotaIDPattern.MatchString(id) }
func validQuotaWindow(window string) bool { return quotaWindowPattern.MatchString(window) }

// quotaPoolForBucket names the pool a summary bucket belongs to. Argon is
// checked before Gemini because its ids also start with "gemini-". An unknown
// bucket keeps its own name minus the window: "veo-preview-monthly" is
// "veo_preview".
func quotaPoolForBucket(id, window string) string {
	switch {
	case strings.HasPrefix(id, "gemini-argon-"):
		return poolArgon
	case strings.HasPrefix(id, "gemini-"):
		return poolGemini
	case strings.HasPrefix(id, "3p-"):
		return poolThirdParty
	}
	name := strings.ToLower(strings.TrimSuffix(id, "-"+window))
	name = strings.NewReplacer("-", "_", ".", "_").Replace(name)
	if !validQuotaID(name) {
		return ""
	}
	return name
}

// familyPools lists the pools a request for a model family draws from on this
// account. No family means any model, which is judged by the shared Gemini and
// Claude/GPT pools: a used-up Argon pool must not bench the whole account.
func familyPools(account Account, family string) []string {
	switch family {
	case "":
		return []string{poolGemini, poolThirdParty}
	case "claude", "gpt", poolThirdParty:
		return []string{poolThirdParty}
	case poolArgon:
		if hasQuotaGroup(quotaGroups(account), poolArgon) {
			return []string{poolArgon}
		}
		return []string{poolGemini}
	}
	return []string{family}
}

func poolInFamily(account Account, pool, family string) bool {
	return oneOf(pool, familyPools(account, family)...)
}

// familiesOverlap says whether a limit recorded for one family also holds back
// a request for another, e.g. an Argon limit on Ultra holds back Gemini.
func familiesOverlap(account Account, limitFamily, family string) bool {
	for _, pool := range familyPools(account, limitFamily) {
		if poolInFamily(account, pool, family) {
			return true
		}
	}
	return false
}
