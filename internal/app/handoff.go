package app

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

const (
	defaultHandoffMaxBytes = 8 * 1024
	maxRetainedHandoffs    = 20
)

var (
	handoffEmailRE = regexp.MustCompile(`[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}`)
	handoffSlugRE  = regexp.MustCompile(`[^a-z0-9]+`)
)

// findLatestConversationID locates the latest Antigravity CLI conversation ID
// for cwd using pure-Go JSON lookups (zero SQLite dependency):
//  1. ~/.gemini/antigravity-cli/cache/last_conversations.json
//  2. fallback: ~/.gemini/antigravity-cli/history.jsonl
func findLatestConversationID(home, cwd string) string {
	home = strings.TrimSpace(home)
	if home == "" {
		return ""
	}
	cleanCwd := ""
	if strings.TrimSpace(cwd) != "" {
		cleanCwd = filepath.Clean(cwd)
	}
	resolvedCwd := ""
	if cleanCwd != "" {
		if eval, err := filepath.EvalSymlinks(cleanCwd); err == nil {
			resolvedCwd = filepath.Clean(eval)
		}
	}

	lastPath := filepath.Join(home, ".gemini", "antigravity-cli", "cache", "last_conversations.json")
	if raw, err := os.ReadFile(lastPath); err == nil && len(raw) > 0 {
		var mapping map[string]string
		if json.Unmarshal(raw, &mapping) == nil && len(mapping) > 0 {
			for _, candidate := range []string{cleanCwd, resolvedCwd} {
				if candidate == "" {
					continue
				}
				if id := strings.TrimSpace(mapping[candidate]); id != "" {
					return id
				}
			}
			for key, id := range mapping {
				cleanedKey := filepath.Clean(key)
				if (cleanCwd != "" && cleanedKey == cleanCwd) || (resolvedCwd != "" && cleanedKey == resolvedCwd) {
					if trimmed := strings.TrimSpace(id); trimmed != "" {
						return trimmed
					}
				}
			}
		}
	}

	historyPath := filepath.Join(home, ".gemini", "antigravity-cli", "history.jsonl")
	f, err := os.Open(historyPath)
	if err != nil {
		return ""
	}
	defer func() { _ = f.Close() }()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	latestMatched := ""
	latestAny := ""
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var entry map[string]any
		if json.Unmarshal([]byte(line), &entry) != nil || entry == nil {
			continue
		}
		id := strings.TrimSpace(firstNonEmptyText(
			entry["conversation_id"],
			entry["conversationId"],
			entry["session_id"],
			entry["id"],
		))
		if id == "" {
			continue
		}
		latestAny = id
		ws := strings.TrimSpace(firstNonEmptyText(entry["cwd"], entry["workspace"], entry["project_dir"], entry["dir"]))
		if ws != "" {
			cleanedWS := filepath.Clean(ws)
			if (cleanCwd != "" && cleanedWS == cleanCwd) || (resolvedCwd != "" && cleanedWS == resolvedCwd) {
				latestMatched = id
			}
		}
	}
	if latestMatched != "" {
		return latestMatched
	}
	return latestAny
}

type handoffTurn struct {
	role string // "user" or "assistant"
	text string
}

func redactHandoffText(text, convID string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	if convID != "" {
		text = strings.ReplaceAll(text, convID, "[redacted-session]")
	}
	text = handoffEmailRE.ReplaceAllString(text, "[redacted-email]")
	return text
}

func extractEditedFilesFromEntry(entry map[string]any, seen map[string]bool, out *[]string) {
	record := func(p string) {
		p = strings.TrimSpace(p)
		if p == "" || seen[p] {
			return
		}
		seen[p] = true
		*out = append(*out, p)
	}
	checkCall := func(call map[string]any) {
		if call == nil {
			return
		}
		name := strings.ToLower(strings.TrimSpace(firstNonEmptyText(call["name"], call["tool"], call["tool_name"])))
		if !strings.Contains(name, "edit") && !strings.Contains(name, "write") && !strings.Contains(name, "replace") && !strings.Contains(name, "patch") {
			return
		}
		args := getMap(call["arguments"])
		if args == nil {
			args = getMap(call["args"])
		}
		if args == nil {
			args = call
		}
		for _, k := range []string{"Path", "path", "TargetFile", "target_file", "file_path", "file"} {
			if v := strings.TrimSpace(getString(args, k)); v != "" {
				record(v)
			}
		}
	}
	checkCall(entry)
	for _, key := range []string{"tool_calls", "toolCalls", "calls"} {
		for _, item := range getSlice(entry[key]) {
			checkCall(getMap(item))
		}
	}
}

func extractTranscriptText(entry map[string]any, fields ...string) string {
	for _, field := range fields {
		raw, ok := entry[field]
		if !ok || raw == nil {
			continue
		}
		switch typed := raw.(type) {
		case string:
			if trimmed := strings.TrimSpace(typed); trimmed != "" {
				return trimmed
			}
		case map[string]any:
			if s := extractTranscriptText(typed, "text", "content", "summary", "message", "response", "user_response"); s != "" {
				return s
			}
		case []any:
			var parts []string
			for _, elem := range typed {
				switch e := elem.(type) {
				case string:
					if s := strings.TrimSpace(e); s != "" {
						parts = append(parts, s)
					}
				case map[string]any:
					// Explicitly skip thinking/thought blocks so internal scratchpads never leak.
					partType := strings.ToLower(strings.TrimSpace(getString(e, "type")))
					if partType == "thinking" || partType == "thought" {
						continue
					}
					if s := extractTranscriptText(e, "text", "content"); s != "" {
						parts = append(parts, s)
					}
				}
			}
			if len(parts) > 0 {
				return strings.Join(parts, "\n")
			}
		}
	}
	return ""
}

// buildCleanHandoffMarkdown creates a bounded, redacted Markdown context brief
// from ~/.gemini/antigravity-cli/brain/<convID>/.system_generated/logs/transcript.jsonl
// without SQLite or network calls. It strips conversation IDs, step_index,
// emails, SYSTEM_MESSAGE, thinking blocks, and ERROR_MESSAGE entries so a
// freshly spawned CLI session starts a clean trajectory.
func buildCleanHandoffMarkdown(home, convID, cwd string, maxBytes int) (string, error) {
	if maxBytes <= 0 {
		maxBytes = defaultHandoffMaxBytes
	}
	var latestCheckpoint string
	var turns []handoffTurn
	editedSeen := map[string]bool{}
	var editedFiles []string

	if strings.TrimSpace(home) != "" && strings.TrimSpace(convID) != "" {
		logsDir := filepath.Join(home, ".gemini", "antigravity-cli", "brain", convID, ".system_generated", "logs")
		transcriptPath := filepath.Join(logsDir, "transcript.jsonl")
		if _, err := os.Stat(transcriptPath); err != nil {
			transcriptPath = filepath.Join(logsDir, "transcript_full.jsonl")
		}
		if f, err := os.Open(transcriptPath); err == nil {
			defer func() { _ = f.Close() }()
			scanner := bufio.NewScanner(f)
			scanner.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
			for scanner.Scan() {
				line := strings.TrimSpace(scanner.Text())
				if line == "" {
					continue
				}
				var entry map[string]any
				if json.Unmarshal([]byte(line), &entry) != nil || entry == nil {
					continue
				}
				// Strip step_index and thinking keys defensively before extraction.
				delete(entry, "step_index")
				delete(entry, "thinking")
				delete(entry, "thought")
				extractEditedFilesFromEntry(entry, editedSeen, &editedFiles)

				kind := strings.ToUpper(strings.TrimSpace(getString(entry, "type")))
				switch kind {
				case "SYSTEM_MESSAGE", "ERROR_MESSAGE":
					continue
				case "CHECKPOINT":
					if text := redactHandoffText(extractTranscriptText(entry, "content", "summary", "text", "checkpoint"), convID); text != "" {
						latestCheckpoint = text
					}
				case "USER_INPUT":
					source := strings.ToUpper(strings.TrimSpace(firstNonEmptyText(entry["source"], entry["input_source"])))
					if source != "" && source != "USER_EXPLICIT" && source != "USER" {
						continue
					}
					if text := redactHandoffText(extractTranscriptText(entry, "content", "text", "user_input", "message"), convID); text != "" {
						turns = append(turns, handoffTurn{role: "user", text: text})
					}
				case "PLANNER_RESPONSE":
					if text := redactHandoffText(extractTranscriptText(entry, "content", "response", "text", "message"), convID); text != "" {
						turns = append(turns, handoffTurn{role: "assistant", text: text})
					}
				}
			}
		}
	}

	sort.Strings(editedFiles)
	gitSummary := collectShortGitStatus(cwd)

	var sb strings.Builder
	sb.WriteString("# Clean Session Handoff\n\n")
	sb.WriteString("Continue the current task cleanly in this fresh session. Previous account and trajectory metadata have been omitted.\n")
	if strings.TrimSpace(cwd) != "" {
		sb.WriteString("\n## Workspace\n")
		sb.WriteString("- Path: `" + filepath.Clean(cwd) + "`\n")
	}
	if latestCheckpoint != "" {
		sb.WriteString("\n## Latest Checkpoint\n")
		sb.WriteString(truncateUTF8(latestCheckpoint, maxBytes/3) + "\n")
	}
	if len(editedFiles) > 0 {
		sb.WriteString("\n## Recently Modified Files\n")
		for _, p := range editedFiles {
			sb.WriteString("- `" + redactHandoffText(p, convID) + "`\n")
		}
	}
	if gitSummary != "" {
		sb.WriteString("\n## Working Tree (`git status --short`)\n```text\n")
		sb.WriteString(redactHandoffText(gitSummary, convID))
		if !strings.HasSuffix(gitSummary, "\n") {
			sb.WriteString("\n")
		}
		sb.WriteString("```\n")
	}
	if len(turns) > 0 {
		// Keep up to the 6 most recent turns, prioritizing the newest turns first.
		start := 0
		if len(turns) > 6 {
			start = len(turns) - 6
		}
		recent := turns[start:]
		budget := maxBytes - sb.Len() - 64 - len("\n## Recent Exchange\n")
		selected := make([]string, 0, len(recent))
		for idx := len(recent) - 1; idx >= 0; idx-- {
			turn := recent[idx]
			label := "User"
			if turn.role == "assistant" {
				label = "Assistant"
			}
			entry := fmt.Sprintf("\n### %s\n%s\n", label, truncateUTF8(turn.text, 1200))
			if len(entry) > budget {
				if len(selected) == 0 && budget > 48 {
					avail := budget - len(fmt.Sprintf("\n### %s\n\n", label))
					if avail > 24 {
						entry = fmt.Sprintf("\n### %s\n%s\n", label, truncateUTF8(turn.text, avail))
						selected = append(selected, entry)
					}
				}
				break
			}
			selected = append(selected, entry)
			budget -= len(entry)
		}
		if len(selected) > 0 {
			sb.WriteString("\n## Recent Exchange\n")
			for i := len(selected) - 1; i >= 0; i-- {
				sb.WriteString(selected[i])
			}
		}
	}

	result := sb.String()
	if len(result) > maxBytes {
		result = truncateUTF8(result, maxBytes-16) + "\n[truncated]\n"
	}
	return result, nil
}

func truncateUTF8(s string, maxBytes int) string {
	if maxBytes <= 0 || len(s) <= maxBytes {
		return s
	}
	out := s[:maxBytes]
	for len(out) > 0 && (out[len(out)-1]&0xC0) == 0x80 {
		out = out[:len(out)-1]
	}
	return strings.TrimRight(out, "\n ")
}

func collectShortGitStatus(cwd string) string {
	cwd = strings.TrimSpace(cwd)
	if cwd == "" {
		return ""
	}
	gitBin, err := exec.LookPath("git")
	if err != nil || gitBin == "" {
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, gitBin, "-C", cwd, "status", "--short")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) == 1 && lines[0] == "" {
		return ""
	}
	if len(lines) > 40 {
		lines = append(lines[:40], fmt.Sprintf("... (%d more)", len(lines)-40))
	}
	return strings.Join(lines, "\n")
}

func handoffSlug(cwd string) string {
	base := strings.ToLower(strings.TrimSpace(filepath.Base(filepath.Clean(cwd))))
	slug := strings.Trim(handoffSlugRE.ReplaceAllString(base, "-"), "-")
	if slug == "" {
		return "workspace"
	}
	if len(slug) > 32 {
		slug = strings.TrimRight(slug[:32], "-")
	}
	return slug
}

// writeHandoffBrief persists a clean handoff brief to
// ~/.gemini/agy-swap/handoffs/handoff-<slug>-<ts>.md with mode 0600 and prunes
// stale handoff files so disk usage stays bounded.
func writeHandoffBrief(home, cwd, content string) (string, error) {
	home = strings.TrimSpace(home)
	if home == "" {
		return "", fmt.Errorf("home directory is empty")
	}
	dir := filepath.Join(home, ".gemini", "agy-swap", "handoffs")
	if err := ensurePrivateDir(dir); err != nil {
		return "", err
	}
	name := fmt.Sprintf("handoff-%s-%d.md", handoffSlug(cwd), time.Now().UTC().UnixNano())
	target := filepath.Join(dir, name)
	if err := atomicWrite(target, []byte(content), 0o600); err != nil {
		return "", err
	}
	pruneOldHandoffs(dir, maxRetainedHandoffs)
	return target, nil
}

func pruneOldHandoffs(dir string, keep int) {
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) <= keep {
		return
	}
	type item struct {
		path    string
		modTime time.Time
	}
	var files []item
	for _, e := range entries {
		if e.IsDir() || !strings.HasPrefix(e.Name(), "handoff-") || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		if info, infoErr := e.Info(); infoErr == nil {
			files = append(files, item{path: filepath.Join(dir, e.Name()), modTime: info.ModTime()})
		}
	}
	if len(files) <= keep {
		return
	}
	sort.Slice(files, func(i, j int) bool { return files[i].modTime.After(files[j].modTime) })
	for _, old := range files[keep:] {
		_ = os.Remove(old.path)
	}
}

// unwrapManagedAgyBinary resolves an agy-swap session-guard shell wrapper back
// to the underlying real `agy` executable so supervised runs execute `agy`
// directly without wrapper re-entry or silent `--dangerously-skip-permissions`.
func unwrapManagedAgyBinary(home, candidate string) string {
	candidate = strings.TrimSpace(candidate)
	if candidate == "" {
		return ""
	}
	header, err := readHeaderBytes(candidate, 8192)
	if err != nil || len(header) == 0 {
		return candidate
	}
	text := string(header)
	if !strings.HasPrefix(text, "#!") || !strings.Contains(text, "Managed by agy-swap session guard") {
		return candidate
	}
	realPath := extractWrapperRealCommand(text)
	if realPath == "" && strings.TrimSpace(home) != "" {
		realPath = filepath.Join(home, ".local", "libexec", "agy")
	}
	if realPath != "" {
		if info, statErr := os.Stat(realPath); statErr == nil && !info.IsDir() {
			return realPath
		}
	}
	return candidate
}

// buildSupervisedAgyEnv constructs a clean child environment for direct `agy`
// execution: forces file token storage via the official agy 1.5.18 variable,
// marks the process as supervised to prevent wrapper re-entry, disables nested
// wrapper sync, and removes inherited WSL_INTEROP on macOS.
func buildSupervisedAgyEnv(base []string, goos string) []string {
	env := append([]string(nil), base...)
	if goos == "darwin" {
		env = withoutEnvKey(env, "WSL_INTEROP")
	}
	env = withoutEnvKey(env, "AGY_SWAP_SKIP_SYNC")
	env = withoutEnvKey(env, "AGY_SWAP_SUPERVISED")
	env = withoutEnvKey(env, "AGY_CLI_FORCE_FILE_TOKEN_STORAGE")
	env = append(env, "AGY_SWAP_SKIP_SYNC=1", "AGY_SWAP_SUPERVISED=1", "AGY_CLI_FORCE_FILE_TOKEN_STORAGE=1")
	return env
}

// extractConversationArg returns an explicit conversation ID passed via
// agy 1.5.18 flags (--conversation, --resume, or -r), if any.
func extractConversationArg(args []string) string {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--conversation" || arg == "--resume" || arg == "-r":
			if i+1 < len(args) {
				if v := strings.TrimSpace(args[i+1]); v != "" && !strings.HasPrefix(v, "-") {
					return v
				}
			}
		case strings.HasPrefix(arg, "--conversation="):
			if v := strings.TrimSpace(strings.TrimPrefix(arg, "--conversation=")); v != "" {
				return v
			}
		case strings.HasPrefix(arg, "--resume="):
			if v := strings.TrimSpace(strings.TrimPrefix(arg, "--resume=")); v != "" {
				return v
			}
		case strings.HasPrefix(arg, "-r="):
			if v := strings.TrimSpace(strings.TrimPrefix(arg, "-r=")); v != "" {
				return v
			}
		}
	}
	return ""
}

// buildHandoffRelaunchArgs builds the argument vector for relaunching `agy`
// after an account swap while preserving user model/output flags, stripping
// prior prompt and conversation-binding flags (--conversation, -r, --resume,
// -c, --continue) so clean-summary starts a fresh trajectory, and never
// silently injecting `--dangerously-skip-permissions`.
func buildHandoffRelaunchArgs(origArgs []string, handoffMode, handoffFile, convID string) []string {
	var preserved []string
	hadNonInteractivePrompt := false
	for i := 0; i < len(origArgs); i++ {
		arg := origArgs[i]
		switch {
		case arg == "-p" || arg == "--prompt" || arg == "--print":
			hadNonInteractivePrompt = true
			if i+1 < len(origArgs) && !strings.HasPrefix(origArgs[i+1], "-") {
				i++
			}
		case strings.HasPrefix(arg, "--prompt=") || strings.HasPrefix(arg, "--print=") || strings.HasPrefix(arg, "-p="):
			hadNonInteractivePrompt = true
		case arg == "-c" || arg == "--continue":
			// Strip continue-last-conversation flag so clean-summary does not
			// accidentally resume the previous account's trajectory.
		case arg == "-i" || arg == "--prompt-interactive" || arg == "--resume" || arg == "--conversation" || arg == "-r":
			if i+1 < len(origArgs) {
				i++
			}
		case strings.HasPrefix(arg, "--prompt-interactive=") || strings.HasPrefix(arg, "-i=") ||
			strings.HasPrefix(arg, "--resume=") || strings.HasPrefix(arg, "--conversation=") || strings.HasPrefix(arg, "-r="):
		default:
			preserved = append(preserved, arg)
		}
	}

	if handoffMode == "resume" && strings.TrimSpace(convID) != "" {
		return append(preserved, "--resume", convID)
	}
	if strings.TrimSpace(handoffFile) != "" {
		prompt := fmt.Sprintf("Read %s and continue.", handoffFile)
		if hadNonInteractivePrompt {
			return append(preserved, "-p", prompt)
		}
		return append(preserved, "-i", prompt)
	}
	return preserved
}

func isTransientUpstreamLine(lower string) bool {
	for _, transient := range []string{"500 internal", "502 bad gateway", "503 service", "504 gateway", "no such host", "connection reset", "i/o timeout", "tls handshake"} {
		if strings.Contains(lower, transient) {
			return true
		}
	}
	return false
}

// isQuotaExhaustedOutputLine detects real quota exhaustion signals in `agy`
// stdout/stderr/stream-json output while ignoring transient 5xx or DNS errors.
func isQuotaExhaustedOutputLine(line string) bool {
	lower := strings.ToLower(strings.TrimSpace(line))
	if lower == "" || isTransientUpstreamLine(lower) {
		return false
	}
	for _, marker := range []string{
		"resource_exhausted",
		"resourceexhausted, http 429",
		"resourceexhausted",
		"model_capacity_exhausted",
		"quota_exhausted",
		"individual quota reached",
		"third-party quota reached",
		"quota exceeded",
		"rate limit exceeded",
		"exhausted your capacity",
	} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

// isAuthExpiredOutputLine detects permanent auth/token failure signals in `agy`
// output/logs while ignoring transient upstream or network errors.
func isAuthExpiredOutputLine(line string) bool {
	lower := strings.ToLower(strings.TrimSpace(line))
	if lower == "" || isTransientUpstreamLine(lower) {
		return false
	}
	for _, marker := range []string{
		"http 401",
		"unauthenticated",
		"invalid_grant",
		"token expired",
		"expired=true",
	} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

// isRealAgyLogErrorLine filters out informational monitor/runner lines (which
// may quote keywords like RESOURCE_EXHAUSTED inside tool arguments) so only
// genuine error log entries trigger auto-swap.
func isRealAgyLogErrorLine(line string) bool {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return false
	}
	if strings.Contains(trimmed, "llm_monitor.go") || strings.Contains(trimmed, "runner.go") {
		return false
	}
	return strings.HasPrefix(trimmed, "E") ||
		strings.Contains(trimmed, "errorreport.go") ||
		strings.Contains(trimmed, "agent executor error:") ||
		strings.Contains(trimmed, "transport error (")
}

// inferExhaustedFamilyFromLine infers the affected quota family from an
// exhaustion line ("claude" for third-party, "gemini" for individual/Gemini).
func inferExhaustedFamilyFromLine(line string) string {
	lower := strings.ToLower(line)
	if strings.Contains(lower, "third-party quota") || strings.Contains(lower, "3p quota") ||
		strings.Contains(lower, "claude") || strings.Contains(lower, "anthropic") ||
		strings.Contains(lower, "gpt") || strings.Contains(lower, "openai") {
		return "claude"
	}
	if strings.Contains(lower, "individual quota") || strings.Contains(lower, "gemini") {
		return "gemini"
	}
	return ""
}

func hasLogFileFlag(args []string) (string, bool) {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--log-file" && i+1 < len(args) {
			return strings.TrimSpace(args[i+1]), true
		}
		if strings.HasPrefix(arg, "--log-file=") {
			return strings.TrimSpace(strings.TrimPrefix(arg, "--log-file=")), true
		}
	}
	return "", false
}
