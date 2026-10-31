package preferences

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"
)

var guardKinds = []string{"staged_paths", "no_coauthor_trailer", "commit_message_format", "no_unrequested_commit", "diff_check"}

// guard ties one of the user's rules to a check at action time.
type guard struct {
	ID       string   `json:"id"`
	Kind     string   `json:"kind"`
	RuleID   string   `json:"rule_id"`
	Patterns []string `json:"patterns"`
}

type guardFile struct {
	SchemaVersion int     `json:"schema_version"`
	Guards        []guard `json:"guards"`
}

type override struct {
	RuleID    string `json:"rule_id"`
	ExpiresAt string `json:"expires_at"`
}

const overrideLifetime = 30 * time.Minute

var coauthorTrailer = regexp.MustCompile(`(?i)co-authored-by:`)

func (s *Store) loadGuards() []guard {
	var file guardFile
	data, err := regularFile(s.dataPath("guards.json"))
	if err != nil || json.Unmarshal(data, &file) != nil {
		return nil
	}
	return file.Guards
}

func (s *Store) writeGuards(guards []guard) error {
	if err := ensureDirectory(s.dataPath()); err != nil {
		return err
	}
	encoded, err := json.MarshalIndent(guardFile{SchemaVersion: schemaVersion, Guards: guards}, "", "  ")
	if err != nil {
		return err
	}
	return atomicReplace(s.dataPath("guards.json"), append(encoded, '\n'))
}

func (s *Store) addGuard(kind, ruleID string, patterns []string) (guard, error) {
	if !slices.Contains(guardKinds, kind) {
		return guard{}, fmt.Errorf("unknown guard kind")
	}
	if s.activeRule(ruleID) == nil {
		return guard{}, fmt.Errorf("guards must reference an active rule")
	}
	if (kind == "staged_paths" || kind == "commit_message_format" || kind == "diff_check") && len(patterns) == 0 {
		return guard{}, fmt.Errorf("%s needs at least one pattern", kind)
	}
	if kind == "commit_message_format" {
		if _, err := regexp.Compile(patterns[0]); err != nil {
			return guard{}, err
		}
	}
	if kind == "diff_check" && diffChecks[patterns[0]] == nil {
		return guard{}, fmt.Errorf("unknown diff check")
	}
	var added guard
	err := withFileLock(s.dataPath("guards.json")+".lock", false, func() error {
		added = guard{ID: "g_" + digest(kind + "\x00" + ruleID + "\x00" + strings.Join(patterns, "\x00"))[:12], Kind: kind, RuleID: ruleID, Patterns: patterns}
		guards := slices.DeleteFunc(s.loadGuards(), func(g guard) bool { return g.ID == added.ID })
		return s.writeGuards(append(guards, added))
	})
	return added, err
}

func (s *Store) removeGuard(id string) error {
	return withFileLock(s.dataPath("guards.json")+".lock", false, func() error {
		guards := s.loadGuards()
		kept := slices.DeleteFunc(slices.Clone(guards), func(g guard) bool { return g.ID == id })
		if len(kept) == len(guards) {
			return fmt.Errorf("unknown guard")
		}
		return s.writeGuards(kept)
	})
}

func (s *Store) activeRule(id string) rule {
	current, err := s.loadCurrent()
	if err != nil || current == nil {
		return nil
	}
	for _, r := range current.Rules.rules {
		if r["id"] == id && (r["status"] == "active" || r["status"] == "core") {
			return r
		}
	}
	return nil
}

// allowOnce lets the next action blocked by ruleID through. Only the user
// runs it: the PreToolUse guard refuses agent tool calls that invoke it.
func (s *Store) allowOnce(ruleID string) (map[string]any, error) {
	if s.activeRule(ruleID) == nil {
		return nil, fmt.Errorf("unknown or inactive rule")
	}
	expires := s.config.Clock().Add(overrideLifetime).UTC().Format(time.RFC3339)
	err := withFileLock(s.dataPath("overrides.json")+".lock", false, func() error {
		overrides := append(s.loadOverrides(), override{RuleID: ruleID, ExpiresAt: expires})
		return s.writeOverrides(overrides)
	})
	return map[string]any{"status": "success", "rule_id": ruleID, "expires_at": expires}, err
}

func (s *Store) loadOverrides() []override {
	var overrides []override
	data, err := regularFile(s.dataPath("overrides.json"))
	if err != nil || json.Unmarshal(data, &overrides) != nil {
		return nil
	}
	return overrides
}

func (s *Store) writeOverrides(overrides []override) error {
	encoded, err := json.Marshal(overrides)
	if err != nil {
		return err
	}
	return atomicReplace(s.dataPath("overrides.json"), append(encoded, '\n'))
}

// consumeOverride removes and reports an unexpired override for ruleID.
func (s *Store) consumeOverride(ruleID string) bool {
	consumed := false
	_ = withFileLock(s.dataPath("overrides.json")+".lock", false, func() error {
		now := s.config.Clock()
		kept := []override{}
		for _, o := range s.loadOverrides() {
			expires, err := time.Parse(time.RFC3339, o.ExpiresAt)
			if err != nil || !now.Before(expires) {
				continue
			}
			if !consumed && o.RuleID == ruleID {
				consumed = true
				continue
			}
			kept = append(kept, o)
		}
		return s.writeOverrides(kept)
	})
	return consumed
}

func gitLines(dir string, args ...string) []string {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...).Output()
	if err != nil {
		return nil
	}
	return strings.Fields(string(out))
}

// guardFinding explains why a guard blocks an action, or "" when it passes.
//
//nolint:gocyclo // One case per guard kind keeps each check beside its message.
func guardFinding(g guard, action gitAction, command, dir string, state sessionState) string {
	switch g.Kind {
	case "staged_paths":
		if action.Verb != "commit" {
			return ""
		}
		paths := gitLines(dir, "diff", "--cached", "--name-only")
		if action.All {
			paths = append(paths, gitLines(dir, "diff", "--name-only")...)
		}
		for _, name := range paths {
			for _, pattern := range g.Patterns {
				if matchGuardPattern(pattern, name) {
					return fmt.Sprintf("the commit includes %s. Unstage it (git restore --staged %s) and commit only the code", name, name)
				}
			}
		}
	case "no_coauthor_trailer":
		if action.Verb == "commit" && (coauthorTrailer.MatchString(action.Message) || coauthorTrailer.MatchString(command)) {
			return "the commit message has a Co-authored-by trailer. Remove the trailer"
		}
	case "commit_message_format":
		if action.Verb == "commit" && action.Message != "" && len(g.Patterns) > 0 {
			if pattern, err := regexp.Compile(g.Patterns[0]); err == nil && !pattern.MatchString(action.Message) {
				return fmt.Sprintf("the commit message does not match %s. Rewrite the message in that format", g.Patterns[0])
			}
		}
	case "no_unrequested_commit":
		if !state.CommitRequested {
			return fmt.Sprintf("the user has not asked for a %s in this session. Leave the changes uncommitted and ask the user", action.Verb)
		}
	}
	return ""
}

func shellCommandText(value any) string {
	switch command := value.(type) {
	case string:
		return command
	case []any:
		parts := stringValues(command)
		if len(parts) > 0 && shellInterpreters[filepath.Base(parts[0])] {
			for i := 1; i+1 < len(parts); i++ {
				if strings.HasPrefix(parts[i], "-") && strings.HasSuffix(parts[i], "c") {
					return parts[i+1]
				}
			}
		}
		return strings.Join(parts, " ")
	}
	return ""
}

//nolint:gocyclo // Override denial, git detection, scope, and overrides form one advisory decision.
func (s *Store) preToolUseHook(input io.Reader, output io.Writer) {
	response := map[string]any{"continue": true}
	defer func() { _ = json.NewEncoder(output).Encode(response) }()
	deny := func(reason string) {
		response["hookSpecificOutput"] = map[string]any{"hookEventName": "PreToolUse", "permissionDecision": "deny", "permissionDecisionReason": reason}
	}
	payload := readHookPayload(input)
	if payload == nil || os.Getenv("LEARN_PREFERENCES_MAINTENANCE") == "1" {
		return
	}
	command := shellCommandText(record(payload["tool_input"])["command"])
	if command == "" {
		return
	}
	if runsPreferenceOverride(command) {
		deny("Only the user can grant preference overrides. Ask them to run: ! hunkyard preferences allow <rule-id> --once")
		return
	}
	action, ok := parseGitInvocation(command)
	if !ok {
		return
	}
	if info, err := os.Lstat(s.dataPath()); err != nil || !info.IsDir() {
		return
	}
	guards := s.loadGuards()
	if len(guards) == 0 {
		return
	}
	cwd := textField(payload["cwd"])
	dir := cwd
	if action.Dir != "" {
		dir = action.Dir
		if !filepath.IsAbs(dir) {
			dir = filepath.Join(cwd, dir)
		}
	}
	session := textField(payload["session_id"])
	state := s.loadSession(session)
	repo, _ := repositoryKey(cwd)
	for _, g := range guards {
		r := s.activeRule(g.RuleID)
		if r == nil {
			continue
		}
		if scope := record(r["scope"]); scope["kind"] == "repository" && scope["repo_key"] != repo {
			continue
		}
		finding := guardFinding(g, action, command, dir, state)
		if finding == "" {
			continue
		}
		if s.consumeOverride(g.RuleID) {
			_ = s.recordActivity(ActivityEvent{Kind: activityOverridden, RuleID: g.RuleID, RepoKey: repo, SessionKey: digest(session), Detail: finding})
			continue
		}
		_ = s.recordActivity(ActivityEvent{Kind: activityBlocked, RuleID: g.RuleID, RepoKey: repo, SessionKey: digest(session), Detail: finding})
		deny(fmt.Sprintf("Blocked by your preference %q (%s): %s. If the user wants this anyway, ask them to run: ! hunkyard preferences allow %s --once", textField(r["directive"]), g.RuleID, finding, g.RuleID))
		return
	}
}
