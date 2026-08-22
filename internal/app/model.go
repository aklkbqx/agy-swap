package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
)

type Account map[string]any

type Accounts struct {
	Order    []string
	ByEmail  map[string]Account
	Revision int64
}

func NewAccounts() *Accounts {
	return &Accounts{ByEmail: make(map[string]Account)}
}

func (a *Accounts) Len() int { return len(a.Order) }

func (a *Accounts) Get(email string) (Account, bool) {
	v, ok := a.ByEmail[email]
	return v, ok
}

func (a *Accounts) Set(email string, account Account) {
	if _, ok := a.ByEmail[email]; !ok {
		a.Order = append(a.Order, email)
	}
	a.ByEmail[email] = account
}

func (a *Accounts) Delete(email string) {
	if _, ok := a.ByEmail[email]; !ok {
		return
	}
	delete(a.ByEmail, email)
	for i, key := range a.Order {
		if key == email {
			a.Order = append(a.Order[:i], a.Order[i+1:]...)
			break
		}
	}
}

func decodeOrderedAccounts(data []byte) (*Accounts, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	token, err := dec.Token()
	if err != nil {
		return nil, err
	}
	if delim, ok := token.(json.Delim); !ok || delim != '{' {
		return nil, fmt.Errorf("accounts.json must contain an object")
	}
	accounts := NewAccounts()
	for dec.More() {
		keyToken, err := dec.Token()
		if err != nil {
			return nil, err
		}
		key, ok := keyToken.(string)
		if !ok {
			return nil, fmt.Errorf("accounts.json contains an invalid account entry")
		}
		var account Account
		if err := dec.Decode(&account); err != nil || account == nil {
			return nil, fmt.Errorf("accounts.json contains an invalid account entry")
		}
		email := normalizeEmail(firstString(account["email"], key))
		if email == "" {
			return nil, fmt.Errorf("invalid account email: %q", key)
		}
		if _, exists := accounts.ByEmail[email]; exists {
			return nil, fmt.Errorf("duplicate account email: %s", email)
		}
		account["email"] = email
		name := cleanText(firstString(account["name"], "Google User"))
		if name == "" {
			name = "Google User"
		}
		account["name"] = name
		if rawToken, exists := account["token_data"]; exists && rawToken != nil {
			token, ok := rawToken.(string)
			if !ok || decodeToken(token) == nil {
				return nil, fmt.Errorf("invalid saved token for %s", email)
			}
			if claimed := extractVerifiedEmail(token); claimed != "" && claimed != email {
				return nil, fmt.Errorf("saved token email does not match %s", email)
			}
		}
		if err := normalizeQuotaLimits(account, email); err != nil {
			return nil, err
		}
		if raw, ok := account["legacy_quota"]; ok {
			if _, ok := raw.(map[string]any); !ok {
				return nil, fmt.Errorf("invalid legacy quota for %s", email)
			}
		}
		if raw, ok := account["quota_snapshot"]; ok {
			norm, err := normalizeQuotaSnapshot(raw, email)
			if err != nil {
				return nil, err
			}
			account["quota_snapshot"] = norm
		}
		accounts.Set(email, account)
	}
	if _, err := dec.Token(); err != nil {
		return nil, err
	}
	var trailing any
	if err := dec.Decode(&trailing); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("accounts.json contains trailing data")
		}
		return nil, err
	}
	return accounts, nil
}

func encodeOrderedAccounts(accounts *Accounts) ([]byte, error) {
	var compact bytes.Buffer
	compact.WriteByte('{')
	for i, email := range accounts.Order {
		account, ok := accounts.ByEmail[email]
		if !ok {
			continue
		}
		if i > 0 {
			compact.WriteByte(',')
		}
		key, _ := json.Marshal(email)
		value, err := json.Marshal(account)
		if err != nil {
			return nil, err
		}
		compact.Write(key)
		compact.WriteByte(':')
		compact.Write(value)
	}
	compact.WriteByte('}')
	var pretty bytes.Buffer
	if err := json.Indent(&pretty, compact.Bytes(), "", "  "); err != nil {
		return nil, err
	}
	pretty.WriteByte('\n')
	return pretty.Bytes(), nil
}

func cleanText(value string) string {
	return strings.TrimSpace(strings.Map(func(r rune) rune {
		if r == '\t' || r == ' ' || !unicode.IsControl(r) && unicode.In(r, unicode.Cf) == false {
			return r
		}
		return -1
	}, value))
}

var emailPattern = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)

func normalizeEmail(value string) string {
	email := strings.ToLower(cleanText(value))
	if !emailPattern.MatchString(email) {
		return ""
	}
	return email
}

func firstString(value any, fallback string) string {
	if str, ok := value.(string); ok && str != "" {
		return str
	}
	return fallback
}

func getString(m map[string]any, key string) string {
	value, _ := m[key].(string)
	return value
}

func getMap(value any) map[string]any {
	m, _ := value.(map[string]any)
	return m
}

func getSlice(value any) []any {
	v, _ := value.([]any)
	return v
}

func getFloat(value any) (float64, bool) {
	switch n := value.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case json.Number:
		v, err := n.Float64()
		return v, err == nil
	default:
		return 0, false
	}
}

func parseUTC(value string) (time.Time, error) {
	if value == "" {
		return time.Time{}, fmt.Errorf("empty timestamp")
	}
	if strings.HasSuffix(value, "Z") {
		value = strings.TrimSuffix(value, "Z") + "+00:00"
	}
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02T15:04:05.999999999", "2006-01-02 15:04:05"} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed.UTC(), nil
		}
	}
	return time.Time{}, fmt.Errorf("invalid timestamp")
}

func isoTime(value time.Time) string { return value.UTC().Format(time.RFC3339Nano) }

func numberInt(value any) (int64, bool) {
	switch n := value.(type) {
	case int64:
		return n, true
	case int:
		return int64(n), true
	case float64:
		if math.Trunc(n) == n {
			return int64(n), true
		}
	case json.Number:
		v, err := strconv.ParseInt(string(n), 10, 64)
		return v, err == nil
	}
	return 0, false
}

func sortedMapKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
