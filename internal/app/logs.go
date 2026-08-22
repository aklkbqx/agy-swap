package app

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"
)

type LimitRecord struct {
	Model      string `json:"model"`
	Family     string `json:"family"`
	ResetAt    string `json:"reset_at"`
	ObservedAt string `json:"observed_at"`
	Source     string `json:"source"`
	SourceFile string `json:"source_file,omitempty"`
}

func (l LimitRecord) Map() map[string]any {
	result := map[string]any{"model": l.Model, "family": l.Family, "reset_at": l.ResetAt, "observed_at": l.ObservedAt, "source": l.Source}
	if l.SourceFile != "" {
		result["source_file"] = l.SourceFile
	}
	return result
}

type EvidenceRecord struct {
	State      string       `json:"state"`
	ObservedAt string       `json:"observed_at"`
	Model      string       `json:"model"`
	Family     string       `json:"family"`
	Key        string       `json:"key"`
	Limit      *LimitRecord `json:"limit,omitempty"`
}

type logFileCache struct {
	MtimeNS  int64                                `json:"mtime_ns"`
	Size     int64                                `json:"size"`
	Evidence map[string]map[string]EvidenceRecord `json:"evidence"`
}

type persistentLogCache struct {
	Version int                     `json:"version"`
	Files   map[string]logFileCache `json:"files"`
}

type LogScanner struct{ paths Paths }

func NewLogScanner(paths Paths) *LogScanner { return &LogScanner{paths: paths} }

type logCandidate struct {
	path string
	info os.FileInfo
}

func (s *LogScanner) discover() []logCandidate {
	roots := []string{
		filepath.Join(s.paths.Home, "Library", "Logs", "Google Antigravity"), filepath.Join(s.paths.Home, "Library", "Logs", "Antigravity"), filepath.Join(s.paths.Home, ".config", "antigravity", "logs"), filepath.Join(s.paths.Home, ".gemini", "antigravity-cli", "logs"), filepath.Join(s.paths.Home, ".gemini", "antigravity-cli", "log"), filepath.Join(s.paths.Home, "Library", "Application Support", "Antigravity IDE", "logs"),
	}
	if runtime.GOOS == "windows" {
		if appdata := os.Getenv("LOCALAPPDATA"); appdata != "" {
			roots = append(roots, filepath.Join(appdata, "Google", "Antigravity", "logs"), filepath.Join(appdata, "Antigravity", "logs"))
		}
	}
	seen := make(map[string]struct{})
	var result []logCandidate
	add := func(path string, info os.FileInfo) {
		clean := filepath.Clean(path)
		if _, ok := seen[clean]; ok {
			return
		}
		seen[clean] = struct{}{}
		result = append(result, logCandidate{clean, info})
	}
	cli := filepath.Join(s.paths.Home, ".gemini", "antigravity-cli", "cli.log")
	if info, err := os.Stat(cli); err == nil && !info.IsDir() {
		add(cli, info)
	}
	for _, root := range roots {
		_ = filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if !entry.IsDir() && strings.EqualFold(filepath.Ext(path), ".log") {
				if info, infoErr := entry.Info(); infoErr == nil {
					add(path, info)
				}
			}
			return nil
		})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].info.ModTime().After(result[j].info.ModTime()) })
	return result
}

func (s *LogScanner) selected() []logCandidate {
	candidates := s.discover()
	cutoff := time.Now().Add(-maxLimitDuration)
	selected := make([]logCandidate, 0)
	var total int64
	for _, candidate := range candidates {
		if candidate.info.ModTime().Before(cutoff) {
			continue
		}
		bytes := min(candidate.info.Size(), int64(logScanBytes))
		if len(selected) > 0 && total+bytes > logTotalBytes {
			break
		}
		selected = append(selected, candidate)
		total += bytes
	}
	return selected
}

func (s *LogScanner) loadCache() persistentLogCache {
	result := persistentLogCache{Version: 1, Files: make(map[string]logFileCache)}
	data, err := os.ReadFile(s.paths.LogCache)
	if err != nil {
		return result
	}
	if json.Unmarshal(data, &result) != nil || result.Version != 1 || result.Files == nil {
		return persistentLogCache{Version: 1, Files: make(map[string]logFileCache)}
	}
	return result
}

func (s *LogScanner) Scan() (map[string]map[string]LimitRecord, map[string]map[string]EvidenceRecord, error) {
	selected := s.selected()
	if len(selected) == 0 {
		return map[string]map[string]LimitRecord{}, map[string]map[string]EvidenceRecord{}, nil
	}
	cache := s.loadCache()
	next := persistentLogCache{Version: 1, Files: make(map[string]logFileCache, len(selected))}
	changed := false
	for _, candidate := range selected {
		cached, ok := cache.Files[candidate.path]
		if ok && cached.MtimeNS == candidate.info.ModTime().UnixNano() && cached.Size == candidate.info.Size() {
			next.Files[candidate.path] = cached
			continue
		}
		evidence, err := scanLogFile(candidate.path, candidate.info)
		if err != nil {
			if ok {
				next.Files[candidate.path] = cached
			}
			continue
		}
		next.Files[candidate.path] = logFileCache{MtimeNS: candidate.info.ModTime().UnixNano(), Size: candidate.info.Size(), Evidence: evidence}
		changed = true
	}
	if len(cache.Files) != len(next.Files) {
		changed = true
	}
	if changed {
		_ = atomicWriteJSON(s.paths.LogCache, next)
	}
	merged := make(map[string]map[string]EvidenceRecord)
	for _, file := range next.Files {
		for email, events := range file.Evidence {
			target := merged[email]
			if target == nil {
				target = make(map[string]EvidenceRecord)
				merged[email] = target
			}
			for identity, event := range events {
				existing, ok := target[identity]
				newTime, _ := parseUTC(event.ObservedAt)
				oldTime, _ := parseUTC(existing.ObservedAt)
				if !ok || newTime.After(oldTime) {
					target[identity] = event
				}
			}
		}
	}
	limits := make(map[string]map[string]LimitRecord)
	for email, events := range merged {
		for _, event := range events {
			if event.State == "limited" && event.Limit != nil {
				if limits[email] == nil {
					limits[email] = make(map[string]LimitRecord)
				}
				limits[email][event.Key] = *event.Limit
			}
		}
	}
	return limits, merged, nil
}

type modelContext struct{ Key, Label, Family string }
type pendingRequest struct {
	Email            string
	Model            modelContext
	SentAt           time.Time
	Response, Failed bool
}

var (
	emailLogPattern    = regexp.MustCompile(`(?i)applyAuthResult:\s*email=([^\s,]+)`)
	errorLogPattern    = regexp.MustCompile(`(?i)RESOURCE_EXHAUSTED.*?Resets\s+in\s+((?:\d+\s*[dhms]\s*)+)`)
	sendLogPattern     = regexp.MustCompile(`(?i)Sending user message to conversation\s+([0-9a-f-]+)`)
	completeLogPattern = regexp.MustCompile(`(?i)Stream completed for\s+([0-9a-f-]+)`)
	resolvedPattern    = regexp.MustCompile(`(?i)\bresolving model\s+(.+?)\s*$`)
	labelPattern       = regexp.MustCompile(`(?i)\blabel\s*=\s*["']([^"']+)`)
	ideTimePattern     = regexp.MustCompile(`(\d{4})-(\d{2})-(\d{2})\s+(\d{2}):(\d{2}):(\d{2})`)
	glogTimePattern    = regexp.MustCompile(`\b[IWEF](\d{2})(\d{2})\s+(\d{2}):(\d{2}):(\d{2})`)
)

func quotaKey(label string) string {
	lower := strings.ToLower(label)
	var b strings.Builder
	dash := false
	for _, r := range lower {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			b.WriteRune(r)
			dash = false
		} else if b.Len() > 0 && !dash {
			b.WriteByte('-')
			dash = true
		}
	}
	return strings.Trim(b.String(), "-")
}
func modelIdentity(label string) string { return strings.TrimSuffix(quotaKey(label), "-thinking") }
func modelFamily(label string) string {
	lower := strings.ToLower(label)
	for _, word := range []string{"gemini", "flash"} {
		if wordPresent(lower, word) {
			return "gemini"
		}
	}
	for _, word := range []string{"claude", "anthropic"} {
		if wordPresent(lower, word) {
			return "claude"
		}
	}
	for _, word := range []string{"gpt", "openai"} {
		if wordPresent(lower, word) {
			return "gpt"
		}
	}
	return ""
}
func wordPresent(value, word string) bool {
	re := regexp.MustCompile(`\b` + regexp.QuoteMeta(word) + `\b`)
	return re.MatchString(value)
}
func detectModel(pattern *regexp.Regexp, line string) (modelContext, bool) {
	match := pattern.FindStringSubmatch(line)
	if match == nil {
		return modelContext{}, false
	}
	label := cleanText(match[1])
	family := modelFamily(label)
	if family == "" {
		return modelContext{}, false
	}
	return modelContext{quotaKey(label), label, family}, true
}

func containsASCIIFold(value, lowerNeedle string) bool {
	if len(lowerNeedle) == 0 || len(value) < len(lowerNeedle) {
		return len(lowerNeedle) == 0
	}
	first := lowerNeedle[0]
	for start := 0; start <= len(value)-len(lowerNeedle); start++ {
		candidate := value[start]
		if candidate >= 'A' && candidate <= 'Z' {
			candidate += 'a' - 'A'
		}
		if candidate != first {
			continue
		}
		matched := true
		for offset := 1; offset < len(lowerNeedle); offset++ {
			candidate = value[start+offset]
			if candidate >= 'A' && candidate <= 'Z' {
				candidate += 'a' - 'A'
			}
			if candidate != lowerNeedle[offset] {
				matched = false
				break
			}
		}
		if matched {
			return true
		}
	}
	return false
}

func scanLogFile(path string, info os.FileInfo) (map[string]map[string]EvidenceRecord, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	offset := max(int64(0), info.Size()-logScanBytes)
	if _, err := file.Seek(offset, io.SeekStart); err != nil {
		return nil, err
	}
	reader := bufio.NewReaderSize(file, 128*1024)
	if offset > 0 {
		_, _ = reader.ReadString('\n')
	}
	evidence := make(map[string]map[string]EvidenceRecord)
	labels := make(map[string]modelContext)
	pending := make(map[string]*pendingRequest)
	currentEmail := ""
	var current modelContext
	pendingResolved := ""
	record := func(email string, model modelContext, state string, observed time.Time, limit *LimitRecord) {
		identity := modelIdentity(model.Label)
		if evidence[email] == nil {
			evidence[email] = make(map[string]EvidenceRecord)
		}
		existing, ok := evidence[email][identity]
		old, _ := parseUTC(existing.ObservedAt)
		if ok && !observed.After(old) {
			return
		}
		evidence[email][identity] = EvidenceRecord{State: state, ObservedAt: isoTime(observed), Model: model.Label, Family: model.Family, Key: model.Key, Limit: limit}
	}
	for {
		line, readErr := reader.ReadString('\n')
		if len(line) > 0 {
			hasEmail := containsASCIIFold(line, "applyauthresult:")
			hasResolved := containsASCIIFold(line, "resolving model")
			hasLabel := containsASCIIFold(line, "label") && strings.Contains(line, "=")
			hasSend := containsASCIIFold(line, "sending user message to conversation")
			hasResponse := strings.Contains(line, "streamGenerateContent") && strings.Contains(line, "ResponseID:")
			hasError := containsASCIIFold(line, "resource_exhausted") && containsASCIIFold(line, "resets")
			hasComplete := containsASCIIFold(line, "stream completed for")
			if hasEmail {
				if match := emailLogPattern.FindStringSubmatch(line); match != nil {
					currentEmail = normalizeEmail(match[1])
					current = modelContext{}
					pendingResolved = ""
				}
			}
			if hasResolved {
				if resolved, ok := detectModel(resolvedPattern, line); ok {
					pendingResolved = resolved.Key
					if known, found := labels[resolved.Key]; found {
						current = known
					} else {
						current = resolved
					}
				}
			}
			if hasLabel {
				if labeled, ok := detectModel(labelPattern, line); ok {
					key := labeled.Key
					if pendingResolved != "" && current.Family == labeled.Family {
						key = pendingResolved
					}
					labeled.Key = key
					current = labeled
					labels[key] = labeled
					pendingResolved = ""
				}
			}
			if hasSend && currentEmail != "" && current.Family != "" {
				if match := sendLogPattern.FindStringSubmatch(line); match != nil {
					if sent, ok := parseLogTimestamp(line, info.ModTime()); ok {
						pending[match[1]] = &pendingRequest{Email: currentEmail, Model: current, SentAt: sent}
					}
				}
			}
			var latest *pendingRequest
			if len(pending) > 0 && (hasResponse || hasError) {
				latest = latestPending(pending)
			}
			if latest != nil && hasResponse {
				latest.Response = true
			}
			if hasError {
				match := errorLogPattern.FindStringSubmatch(line)
				if match != nil {
					if latest != nil {
						latest.Failed = true
					}
					email, model := currentEmail, current
					if latest != nil {
						email, model = latest.Email, latest.Model
					}
					duration, okDuration := parseDuration(match[1])
					observed, okTime := parseLogTimestamp(line, info.ModTime())
					if email != "" && model.Family != "" && okDuration && duration > 0 && okTime {
						reset := observed.Add(duration)
						now := time.Now()
						if reset.After(now) && reset.Sub(now) <= maxLimitDuration {
							limit := &LimitRecord{Model: model.Label, Family: model.Family, ResetAt: isoTime(reset), ObservedAt: isoTime(observed), Source: "log", SourceFile: filepath.Base(path)}
							record(email, model, "limited", observed, limit)
						}
					}
				}
			}
			if hasComplete {
				if match := completeLogPattern.FindStringSubmatch(line); match != nil {
					request := pending[match[1]]
					delete(pending, match[1])
					completed, ok := parseLogTimestamp(line, info.ModTime())
					if request != nil && request.Response && !request.Failed && ok {
						record(request.Email, request.Model, "available", completed, nil)
					}
				}
			}
		}
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			return evidence, readErr
		}
	}
	return evidence, nil
}

func latestPending(pending map[string]*pendingRequest) *pendingRequest {
	var latest *pendingRequest
	for _, request := range pending {
		if latest == nil || request.SentAt.After(latest.SentAt) {
			latest = request
		}
	}
	return latest
}

func parseLogTimestamp(line string, reference time.Time) (time.Time, bool) {
	if match := ideTimePattern.FindStringSubmatch(line); match != nil {
		value := fmt.Sprintf("%s-%s-%s %s:%s:%s", match[1], match[2], match[3], match[4], match[5], match[6])
		parsed, err := time.ParseInLocation("2006-01-02 15:04:05", value, time.Local)
		if err == nil {
			return parsed.UTC(), true
		}
	}
	match := glogTimePattern.FindStringSubmatch(line)
	if match == nil {
		return time.Time{}, false
	}
	var month, day, hour, minute, second int
	_, err := fmt.Sscanf(strings.Join(match[1:], " "), "%d %d %d %d %d", &month, &day, &hour, &minute, &second)
	if err != nil {
		return time.Time{}, false
	}
	ref := reference.In(time.Local)
	var candidates []time.Time
	for _, year := range []int{ref.Year() - 1, ref.Year(), ref.Year() + 1} {
		candidate := time.Date(year, time.Month(month), day, hour, minute, second, 0, time.Local)
		if int(candidate.Month()) == month && candidate.Day() == day {
			candidates = append(candidates, candidate)
		}
	}
	if len(candidates) == 0 {
		return time.Time{}, false
	}
	sort.Slice(candidates, func(i, j int) bool { return absDuration(candidates[i].Sub(ref)) < absDuration(candidates[j].Sub(ref)) })
	return candidates[0].UTC(), true
}
func absDuration(d time.Duration) time.Duration {
	if d < 0 {
		return -d
	}
	return d
}
