package preferences

import (
	"bufio"
	"bytes"
	"encoding/json"
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

//nolint:gocyclo // Transcript confinement and advisory failure handling share one bounded input boundary.
func (s *Store) hook(input io.Reader, output, errorOutput io.Writer) {
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
	context := NeighboringContext{}
	_, _, capture := analyzeSignals(prompt)
	if capture {
		context = s.transcriptContext(payload)
	}
	selection, err := s.SelectPersona(prompt+" "+context.PreviousUserRequest, cwd, repo)
	if err != nil {
		diagnose("selection_failed")
	} else if len(selection.RuleIDs) > 0 && selection.Text != "" {
		response["hookSpecificOutput"] = map[string]any{"hookEventName": "UserPromptSubmit", "additionalContext": selection.Text}
		health["context_emitted"] = true
		health["selected_rule_count"] = len(selection.RuleIDs)
		health["omitted_rule_count"] = selection.OmittedCount
	}
	result, err := s.capture(CaptureInput{Feedback: prompt, Source: "hook", CWD: cwd, RepoKey: repo, SessionID: textField(payload["session_id"]), TurnID: textField(payload["turn_id"]), Context: context}, true)
	if err != nil {
		diagnose("capture_failed")
	} else {
		health["capture_action"] = result.Action
	}
}

var harnessMessage = regexp.MustCompile(`^(?:<environment_context>|<user_instructions>|<local-command-|<command-name>|<system-reminder>|# AGENTS\.md instructions)`)

//nolint:gocyclo // Transcript confinement and advisory failure handling share one bounded input boundary.
func (s *Store) transcriptContext(payload map[string]any) NeighboringContext {
	var empty NeighboringContext
	name := textField(payload["transcript_path"])
	cwd := textField(payload["cwd"])
	session := textField(payload["session_id"])
	if !filepath.IsAbs(name) || filepath.Ext(name) != ".jsonl" || filepath.Clean(name) != name {
		return empty
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
		return empty
	}
	resolved, err := filepath.EvalSymlinks(name)
	if err != nil || resolved != name {
		return empty
	}
	info, err := os.Lstat(name)
	if err != nil || !info.Mode().IsRegular() || !ownedFile(info) {
		return empty
	}
	file, err := os.Open(name)
	if err != nil {
		return empty
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !os.SameFile(info, opened) {
		return empty
	}
	first, _ := bufio.NewReaderSize(file, 256*1024).ReadSlice('\n')
	var header map[string]any
	if strictjson.Decode(first, &header) == nil {
		recorded := textField(header["sessionId"])
		if header["type"] == "session_meta" {
			recorded = textField(record(header["payload"])["id"])
		}
		if recorded != "" && recorded != session {
			return empty
		}
	}
	offset := max(opened.Size()-256*1024, 0)
	if _, err := file.Seek(offset, io.SeekStart); err != nil {
		return empty
	}
	data, err := io.ReadAll(io.LimitReader(file, 256*1024))
	if err != nil {
		return empty
	}
	if offset > 0 {
		if i := bytes.IndexByte(data, '\n'); i >= 0 {
			data = data[i+1:]
		} else {
			return empty
		}
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
		if text != "" && !harnessMessage.MatchString(strings.TrimLeft(text, " \t\r\n")) {
			messages = append(messages, message{textField(content["role"]), text})
		}
	}
	if len(messages) > 40 {
		messages = messages[len(messages)-40:]
	}
	for len(messages) > 0 && messages[len(messages)-1].role == "user" && normalize(messages[len(messages)-1].text) == normalize(textField(payload["prompt"])) {
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
