package preferences

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/BSH2409/hunkyard/internal/strictjson"
)

// hook runs one host hook event and returns the process exit code.
func (s *Store) hook(event string, input io.Reader, output, errorOutput io.Writer) int {
	switch event {
	case "ReviewWake":
		return s.reviewWakeHook(input, errorOutput)
	case "Interrupt":
		s.interruptHook(input, output)
	case "UserPromptSubmit":
		s.promptSubmitHook(input, output, errorOutput)
	case "SessionStart":
		s.sessionStartHook(input, output)
	case "PreToolUse":
		s.preToolUseHook(input, output)
	case "Stop":
		s.stopHook(input, output)
	default:
		_ = json.NewEncoder(output).Encode(map[string]any{"continue": true})
	}
	return 0
}

func readHookPayload(input io.Reader) map[string]any {
	data, err := io.ReadAll(io.LimitReader(input, 1024*1024+1))
	if err != nil || len(data) > 1024*1024 {
		return nil
	}
	var payload map[string]any
	if strictjson.Decode(data, &payload) != nil {
		return nil
	}
	return payload
}

func (s *Store) sessionStartHook(input io.Reader, output io.Writer) {
	response := map[string]any{"continue": true}
	defer func() { _ = json.NewEncoder(output).Encode(response) }()
	payload := readHookPayload(input)
	if payload == nil || os.Getenv("LEARN_PREFERENCES_MAINTENANCE") == "1" {
		return
	}
	cwd, session := textField(payload["cwd"]), textField(payload["session_id"])
	repo, err := repositoryKey(cwd)
	if err != nil {
		return
	}
	current, err := s.loadCurrent()
	if err != nil {
		return
	}
	var rules []rule
	if current != nil {
		rules = current.Rules.rules
	}
	s.startMaintenanceIfDue(current)
	selection := selectCore(rules, repo)
	text := selection.Text
	if health := s.healthLine(current); health != "" {
		text = strings.TrimSpace(text + "\n" + health)
	}
	for _, notice := range s.ownedReviewNotices(session) {
		if notice.Pending > 0 {
			text = strings.TrimSpace(text + "\n" + fmt.Sprintf("Hunkyard review %s has %d published review batch(es) waiting. Run: hunkyard feedback await --session %s --json", notice.ReviewID, notice.Pending, notice.ReviewID))
		}
	}
	if text == "" {
		return
	}
	response["hookSpecificOutput"] = map[string]any{"hookEventName": "SessionStart", "additionalContext": text}
	if session != "" && len(selection.RuleIDs) > 0 {
		_ = s.updateSession(session, func(state *sessionState) {
			state.InjectedRuleIDs = append(state.InjectedRuleIDs, selection.RuleIDs...)
		})
	}
}

// healthLine tells the agent when preference work needs attention.
func (s *Store) healthLine(current *generation) string {
	pending := s.unprocessedCount(current)
	run := s.lastMaintenanceRun()
	maintenance := "maintenance due"
	if run.Outcome == "failed" {
		maintenance = "maintenance failed (" + run.ErrorClass + ")"
	} else if pending == 0 {
		return ""
	}
	return fmt.Sprintf("Hunkyard preferences: %d observations pending; %s.", pending, maintenance)
}

//nolint:gocyclo // Validation, selection, session state, and capture share one advisory hook boundary.
func (s *Store) promptSubmitHook(input io.Reader, output, errorOutput io.Writer) {
	response := map[string]any{"continue": true}
	health := map[string]any{"capture_action": "not_attempted", "context_emitted": false, "payload_fields": []string{}}
	defer func() {
		_ = json.NewEncoder(output).Encode(response)
		if os.Getenv("LEARN_PREFERENCES_MAINTENANCE") == "1" {
			return
		}
		if info, err := os.Lstat(s.dataPath()); err == nil && info.IsDir() && info.Mode()&os.ModeSymlink == 0 {
			health["last_seen_at"] = s.config.Clock().UTC().Format(time.RFC3339)
			if data, err := canonicalJSON(health); err == nil {
				_ = atomicReplace(s.dataPath("hook-health.json"), append(data, '\n'))
			}
		}
	}()
	if os.Getenv("LEARN_PREFERENCES_MAINTENANCE") == "1" {
		return
	}
	diagnose := func(code string) {
		health["last_error_code"] = code
		_ = json.NewEncoder(errorOutput).Encode(map[string]any{"timestamp": s.config.Clock().UTC().Format(time.RFC3339), "error_class": "LearnPreferences::ValidationError", "error_code": code})
	}
	data, err := io.ReadAll(io.LimitReader(input, 1024*1024+1))
	if err != nil {
		diagnose("internal_error")
		return
	}
	if len(data) > 1024*1024 {
		diagnose("input_too_large")
		return
	}
	var payload map[string]any
	if strictjson.Decode(data, &payload) != nil || payload == nil || payload["hook_event_name"] != "UserPromptSubmit" {
		diagnose("malformed_input")
		return
	}
	for _, key := range []string{"session_id", "cwd", "prompt"} {
		if _, ok := payload[key].(string); !ok {
			diagnose("malformed_input")
			return
		}
	}
	for _, key := range []string{"turn_id", "transcript_path"} {
		if payload[key] != nil {
			if _, ok := payload[key].(string); !ok {
				diagnose("malformed_input")
				return
			}
		}
	}
	fields := []string{}
	for key := range payload {
		if slices.Contains(strings.Fields("hook_event_name session_id turn_id cwd prompt transcript_path model permission_mode agent_id agent_type"), key) {
			fields = append(fields, key)
		}
	}
	sort.Strings(fields)
	health["payload_fields"] = fields
	cwd := textField(payload["cwd"])
	prompt := textField(payload["prompt"])
	repo, err := repositoryKey(cwd)
	if err != nil {
		diagnose("internal_error")
		return
	}
	authored := userAuthoredText(prompt)
	session := textField(payload["session_id"])
	state := s.loadSession(session)
	interrupted := !isTrivialPrompt(authored) && (state.InterruptedAt != "" || s.hostKind == "claude" && transcriptShowsInterrupt(s.transcriptTail(payload), session, authored))
	context := NeighboringContext{}
	_, _, capture := analyzeSignals(authored)
	if capture || interrupted {
		context = s.transcriptContext(payload)
	}
	commitRequested := requestsCommit(authored)
	if !isTrivialPrompt(authored) {
		selection, err := s.promptSelection(authored+" "+context.PreviousUserRequest, repo, state.InjectedRuleIDs)
		if err != nil {
			diagnose("selection_failed")
		} else if len(selection.RuleIDs) > 0 && selection.Text != "" {
			response["hookSpecificOutput"] = map[string]any{"hookEventName": "UserPromptSubmit", "additionalContext": selection.Text}
			response["systemMessage"] = fillCount(pickMessage(afterHookNotes, session+prompt), len(selection.RuleIDs))
			health["context_emitted"] = true
			health["selected_rule_count"] = len(selection.RuleIDs)
			health["omitted_rule_count"] = selection.OmittedCount
			state.InjectedRuleIDs = append(state.InjectedRuleIDs, selection.RuleIDs...)
		}
	}
	if session != "" {
		injected := state.InjectedRuleIDs
		turnDigest := ""
		var baseline []string
		for _, g := range s.loadGuards() {
			if g.Kind == "diff_check" {
				turnDigest = workingDiffDigest(cwd)
				for _, line := range addedLines(cwd) {
					baseline = append(baseline, line.key())
				}
				break
			}
		}
		_ = s.updateSession(session, func(stored *sessionState) {
			stored.InjectedRuleIDs = append(stored.InjectedRuleIDs, injected...)
			stored.CommitRequested = stored.CommitRequested || commitRequested
			stored.TurnDiffDigest = turnDigest
			stored.TurnAddedLines = baseline
			if interrupted {
				stored.InterruptedAt = ""
			}
		})
	}
	if authored == "" {
		health["capture_action"] = "ignored"
		return
	}
	result, err := s.capture(CaptureInput{Feedback: authored, Source: "hook", CWD: cwd, RepoKey: repo, SessionID: textField(payload["session_id"]), TurnID: textField(payload["turn_id"]), Context: context, Interrupted: interrupted}, true)
	if err != nil {
		diagnose("capture_failed")
	} else {
		health["capture_action"] = result.Action
		if result.Action == "created" {
			response["systemMessage"] = pickMessage(capturingMessages, session+prompt)
		}
	}
}

// hostInsertedBlocks match text the host adds to a prompt on the user's
// behalf (background task results, slash-command expansions, reminders,
// pasted documents). Their wording is not the user's, so it must not count
// as correction or durable-preference evidence.
var hostInsertedBlocks = func() []*regexp.Regexp {
	tags := strings.Fields("task-notification system-reminder command-message command-name command-args local-command-stdout local-command-stderr local-command-caveat bash-input bash-stdout bash-stderr pasted_content environment_context user_instructions")
	blocks := make([]*regexp.Regexp, 0, len(tags))
	for _, tag := range tags {
		blocks = append(blocks, regexp.MustCompile(`(?s)<`+tag+`(?:\s[^>]*)?>.*?</`+tag+`(?:\s[^>]*)?>`))
	}
	return blocks
}()

// userAuthoredText returns the prompt with host-inserted blocks removed, or
// "" when nothing the user typed remains.
func userAuthoredText(prompt string) string {
	for _, block := range hostInsertedBlocks {
		prompt = block.ReplaceAllString(prompt, " ")
	}
	return strings.TrimSpace(prompt)
}

var harnessMessage = regexp.MustCompile(`^(?:<environment_context>|<user_instructions>|<local-command-|<command-name>|<system-reminder>|# AGENTS\.md instructions)`)

// transcriptTail reads the last 256 KiB of the host transcript named in the
// payload, only when it is a regular file the user owns, inside a known host
// directory, and recorded for the same session.
//
//nolint:gocyclo // Transcript confinement and advisory failure handling share one bounded input boundary.
func (s *Store) transcriptTail(payload map[string]any) []byte {
	name := textField(payload["transcript_path"])
	cwd := textField(payload["cwd"])
	session := textField(payload["session_id"])
	if !filepath.IsAbs(name) || filepath.Ext(name) != ".jsonl" || filepath.Clean(name) != name {
		return nil
	}
	roots := []string{s.config.Root, filepath.Join(cwd, ".codex"), filepath.Join(cwd, ".claude")}
	if codex, err := hostHome("CODEX_HOME", ".codex"); err == nil {
		roots = append(roots, codex)
	}
	if claude, err := hostHome("CLAUDE_CONFIG_DIR", ".claude"); err == nil {
		roots = append(roots, filepath.Join(claude, "projects"))
	}
	allowed := false
	for _, root := range roots {
		root, err := filepath.Abs(root)
		if err == nil && strings.HasPrefix(name, root+string(filepath.Separator)) {
			allowed = true
			break
		}
	}
	if !allowed {
		return nil
	}
	resolved, err := filepath.EvalSymlinks(name)
	if err != nil || resolved != name {
		return nil
	}
	info, err := os.Lstat(name)
	if err != nil || !info.Mode().IsRegular() || !ownedFile(info) {
		return nil
	}
	file, err := os.Open(name)
	if err != nil {
		return nil
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !os.SameFile(info, opened) {
		return nil
	}
	first, _ := bufio.NewReaderSize(file, 256*1024).ReadSlice('\n')
	var header map[string]any
	if strictjson.Decode(first, &header) == nil {
		recorded := textField(header["sessionId"])
		if header["type"] == "session_meta" {
			recorded = textField(record(header["payload"])["id"])
		}
		if recorded != "" && recorded != session {
			return nil
		}
	}
	offset := max(opened.Size()-256*1024, 0)
	if _, err := file.Seek(offset, io.SeekStart); err != nil {
		return nil
	}
	data, err := io.ReadAll(io.LimitReader(file, 256*1024))
	if err != nil {
		return nil
	}
	if offset > 0 {
		if i := bytes.IndexByte(data, '\n'); i >= 0 {
			data = data[i+1:]
		} else {
			return nil
		}
	}
	return data
}

//nolint:gocyclo // Host transcript formats and message filtering are parsed in one bounded pass.
func (s *Store) transcriptContext(payload map[string]any) NeighboringContext {
	var empty NeighboringContext
	session := textField(payload["session_id"])
	data := s.transcriptTail(payload)
	if data == nil {
		return empty
	}
	type message struct{ role, text string }
	messages := []message{}
	for _, line := range bytes.Split(data, []byte("\n")) {
		var entry map[string]any
		if strictjson.Decode(line, &entry) != nil || entry == nil {
			continue
		}
		if recorded := textField(entry["sessionId"]); recorded != "" && recorded != session {
			continue
		}
		if entry["isMeta"] == true || entry["isSidechain"] == true || entry["isCompactSummary"] == true {
			continue
		}
		var content map[string]any
		switch entry["type"] {
		case "response_item":
			content = record(entry["payload"])
		case "user", "assistant":
			content = record(entry["message"])
		case "message":
			content = entry
		}
		if content == nil || !slices.Contains([]string{"user", "assistant"}, textField(content["role"])) {
			continue
		}
		text := textField(content["content"])
		if blocks, ok := content["content"].([]any); ok {
			parts := []string{}
			for _, block := range blocks {
				value := record(block)
				if slices.Contains([]string{"text", "input_text", "output_text"}, textField(value["type"])) {
					if part, ok := value["text"].(string); ok {
						parts = append(parts, part)
					}
				}
			}
			text = strings.Join(parts, "\n")
		}
		if textField(content["role"]) == "user" {
			text = userAuthoredText(text)
		}
		if text != "" && !harnessMessage.MatchString(strings.TrimLeft(text, " \t\r\n")) {
			messages = append(messages, message{textField(content["role"]), text})
		}
	}
	if len(messages) > 40 {
		messages = messages[len(messages)-40:]
	}
	for len(messages) > 0 && messages[len(messages)-1].role == "user" && normalize(messages[len(messages)-1].text) == normalize(userAuthoredText(textField(payload["prompt"]))) {
		messages = messages[:len(messages)-1]
	}
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].role != "user" {
			continue
		}
		parts := []string{}
		for _, m := range messages[i+1:] {
			if m.role == "assistant" {
				parts = append(parts, m.text)
			}
		}
		if len(parts) > 3 {
			parts = parts[len(parts)-3:]
		}
		if len(parts) == 0 {
			return empty
		}
		if context := sanitizeContext(NeighboringContext{Source: "transcript", PreviousUserRequest: messages[i].text, AgentResponse: strings.Join(parts, "\n")}); context != nil {
			return *context
		}
		return empty
	}
	return empty
}

func (s *Store) unprocessedCount(current *generation) int {
	data, err := regularFile(s.dataPath("observations", "pending.jsonl"))
	if err != nil {
		return 0
	}
	processed := map[string]bool{}
	if current != nil {
		for _, id := range current.ProcessedIDs {
			processed[id] = true
		}
	}
	count := 0
	for _, line := range bytes.Split(data, []byte("\n")) {
		var entry struct {
			ID string `json:"id"`
		}
		if json.Unmarshal(line, &entry) == nil && entry.ID != "" && !processed[entry.ID] {
			count++
		}
	}
	return count
}

var commitIntent = regexp.MustCompile(`(?i)\b(?:commit|push)\b`)
var negatedCommit = regexp.MustCompile(`(?i)\b(?:don'?t|do\s+not|never|no|not|without|stop|avoid)\b(?:\W+\w+){0,2}?\W+(?:commit|push)\w*`)
var commitNoun = regexp.MustCompile(`(?i)\b(?:the|a|an|this|that|last|previous|latest|each|every|my|your|its|initial|first)\s+(?:commit|push)(?:es|s)?\b`)

// requestsCommit reports whether the user asked for a commit or push, as
// opposed to forbidding one ("don't commit yet") or mentioning one ("the
// last commit broke tests").
func requestsCommit(authored string) bool {
	text := negatedCommit.ReplaceAllString(authored, " ")
	text = commitNoun.ReplaceAllString(text, " ")
	return commitIntent.MatchString(text)
}

// isTrivialPrompt reports short acknowledgements ("next", "status?") that
// carry no topic, so no preferences need loading for them.
func isTrivialPrompt(authored string) bool {
	if len(strings.Fields(authored)) >= 4 {
		return false
	}
	if _, _, correction := analyzeSignals(authored); correction {
		return false
	}
	for _, pattern := range personaSurfacePatterns {
		if pattern.MatchString(authored) {
			return false
		}
	}
	return true
}

// promptSelection selects rules for this prompt and keeps every non-core
// rule injected earlier in the session, so a rule never drops out mid-task.
// Core rules were loaded at SessionStart and stay in the conversation.
func (s *Store) promptSelection(query, repo string, injected []string) (Selection, error) {
	current, err := s.loadCurrent()
	if err != nil || current == nil {
		return Selection{}, err
	}
	rules := current.Rules.rules
	selection := selectPersona(rules, query, repo)
	chosen := map[string]bool{}
	for _, id := range selection.RuleIDs {
		chosen[id] = true
	}
	sticky := map[string]bool{}
	for _, id := range injected {
		sticky[id] = true
	}
	var selected []rule
	for _, r := range rules {
		id := textField(r["id"])
		if isCoreRule(r) && sticky[id] {
			continue
		}
		if chosen[id] || sticky[id] && (r["status"] == "active" || r["status"] == "core") {
			selected = append(selected, r)
		}
	}
	ids := []string{}
	for _, r := range selected {
		ids = append(ids, textField(r["id"]))
	}
	if len(selected) == 0 {
		return Selection{RuleIDs: ids, OmittedCount: selection.OmittedCount}, nil
	}
	text := selectionText(selected)
	return Selection{Text: text, RuleIDs: ids, OmittedCount: selection.OmittedCount, EstimatedTokens: estimatedTokens(text)}, nil
}

var interruptMarkers = []string{"[Request interrupted by user", "The user doesn't want to proceed with this tool use"}

// transcriptShowsInterrupt reports whether the last user turn in a Claude
// transcript is an interruption or a rejected tool call.
func transcriptShowsInterrupt(data []byte, session, prompt string) bool {
	lines := bytes.Split(bytes.TrimSpace(data), []byte("\n"))
	skippedPrompt := false
	for i := len(lines) - 1; i >= 0; i-- {
		var entry map[string]any
		if strictjson.Decode(lines[i], &entry) != nil || entry["type"] != "user" {
			continue
		}
		if recorded := textField(entry["sessionId"]); recorded != "" && recorded != session {
			continue
		}
		text := string(lines[i])
		// The host may already have recorded the prompt being submitted.
		if !skippedPrompt && prompt != "" && strings.Contains(text, prompt) {
			skippedPrompt = true
			continue
		}
		for _, marker := range interruptMarkers {
			if strings.Contains(text, marker) {
				return true
			}
		}
		return false
	}
	return false
}

// interruptHook records a Codex interrupt so the user's next prompt counts
// as a correction. Codex honours only systemMessage here, so it prints {}.
func (s *Store) interruptHook(input io.Reader, output io.Writer) {
	defer func() { _, _ = output.Write([]byte("{}\n")) }()
	payload := readHookPayload(input)
	session := textField(payload["session_id"])
	if payload == nil || session == "" || os.Getenv("LEARN_PREFERENCES_MAINTENANCE") == "1" {
		return
	}
	_ = s.updateSession(session, func(state *sessionState) {
		state.InterruptedAt = s.config.Clock().UTC().Format(time.RFC3339)
	})
	repo, _ := repositoryKey(textField(payload["cwd"]))
	_ = s.recordActivity(ActivityEvent{Kind: activityInterrupted, RepoKey: repo, SessionKey: digest(session)})
}
