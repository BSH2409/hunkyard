package preferences

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type diffCheck func(path string, addedLines []string) []string

var rubySingleLineDef = []*regexp.Regexp{
	regexp.MustCompile(`^\s*def\s+\S.*;`),
	regexp.MustCompile(`^\s*def\s+\S.*\bend\s*$`),
	regexp.MustCompile(`^\s*def\s+[\w.?!]+(?:\([^)]*\))?\s*=\s*\S`),
}

var rubyQuoted = regexp.MustCompile(`"(?:[^"\\]|\\.)*"|'(?:[^'\\]|\\.)*'`)

var (
	jsConsoleCall   = regexp.MustCompile(`\bconsole\.(?:log|debug|info|trace|dir)\s*\(`)
	todoMarker      = regexp.MustCompile(`\b(?:TODO|FIXME|XXX)\b(\([^)]+\))?`)
	skippedTestCall = regexp.MustCompile(`\bt\.Skip(?:f|Now)?\s*\(|\b(?:it|test|describe)\.skip\s*\(|^\s*x(?:it|describe|test)\b|\bpytest\.mark\.skip|@unittest\.skip|\bskip\s*\(?\s*["']`)
	jsExtensions    = map[string]bool{".js": true, ".mjs": true, ".cjs": true, ".jsx": true, ".ts": true, ".tsx": true}
)

// lineFindings applies one predicate to every added line.
func lineFindings(added []string, message string, match func(string) bool) []string {
	var findings []string
	for i, line := range added {
		if match(line) {
			findings = append(findings, fmt.Sprintf("%d: %s", i, message))
		}
	}
	return findings
}

func isTestPath(path string) bool {
	base := filepath.Base(path)
	return strings.HasSuffix(base, "_test.go") || strings.Contains(base, ".spec.") || strings.Contains(base, ".test.") ||
		strings.HasSuffix(base, "_spec.rb") || strings.HasPrefix(base, "test_") || strings.HasSuffix(base, "_test.py")
}

// diffChecks are mechanical checks a diff_check guard can run on lines the
// agent added. Each returns one message per offending line ("line N: …").
var diffChecks = map[string]diffCheck{
	"js_console_log": func(path string, added []string) []string {
		if !jsExtensions[filepath.Ext(path)] {
			return nil
		}
		return lineFindings(added, "console call left in code", jsConsoleCall.MatchString)
	},
	"todo_without_owner": func(path string, added []string) []string {
		return lineFindings(added, "TODO without an owner", func(line string) bool {
			for _, m := range todoMarker.FindAllStringSubmatch(line, -1) {
				if m[1] == "" {
					return true
				}
			}
			return false
		})
	},
	"skipped_test_added": func(path string, added []string) []string {
		if !isTestPath(path) {
			return nil
		}
		return lineFindings(added, "test skipped", skippedTestCall.MatchString)
	},
	"ruby_single_line_def": func(path string, added []string) []string {
		if filepath.Ext(path) != ".rb" {
			return nil
		}
		var findings []string
		for i, line := range added {
			for _, pattern := range rubySingleLineDef {
				if pattern.MatchString(line) {
					findings = append(findings, fmt.Sprintf("%d: single-line method definition", i))
					break
				}
			}
		}
		return findings
	},
	"ruby_semicolon_statement": func(path string, added []string) []string {
		if filepath.Ext(path) != ".rb" {
			return nil
		}
		var findings []string
		for i, line := range added {
			code := rubyQuoted.ReplaceAllString(line, `""`)
			code, _, _ = strings.Cut(code, "#")
			if strings.Contains(code, ";") {
				findings = append(findings, fmt.Sprintf("%d: statements joined with ';'", i))
			}
		}
		return findings
	},
}

func gitOutput(dir string, args ...string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...).Output()
	if err != nil {
		return ""
	}
	return string(out)
}

// workingDiffDigest fingerprints uncommitted changes so a turn that changed
// nothing, or that was already reviewed, is not reviewed again.
func workingDiffDigest(dir string) string {
	var state strings.Builder
	state.WriteString(gitOutput(dir, "diff", "HEAD"))
	for _, name := range untrackedFiles(dir) {
		if info, err := os.Stat(filepath.Join(dir, name)); err == nil {
			fmt.Fprintf(&state, "\x00%s %d %d", name, info.Size(), info.ModTime().UnixNano())
		}
	}
	return digest(state.String())
}

const untrackedFileLimit = 256 * 1024

func untrackedFiles(dir string) []string {
	return strings.Split(strings.TrimSpace(gitOutput(dir, "ls-files", "--others", "--exclude-standard", "-z")), "\x00")
}

type addedLine struct {
	path   string
	number int
	text   string
}

// key identifies an added line by file and content, so a line the user had
// already written before the turn is not attributed to the agent.
func (l addedLine) key() string {
	return digest(l.path + "\x00" + l.text)[:16]
}

func addedLines(dir string) []addedLine {
	var lines []addedLine
	path, number := "", 0
	scanner := bufio.NewScanner(strings.NewReader(gitOutput(dir, "diff", "HEAD", "-U0")))
	scanner.Buffer(make([]byte, 4096), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case strings.HasPrefix(line, "+++ "):
			path = strings.TrimPrefix(strings.TrimPrefix(line, "+++ "), "b/")
		case strings.HasPrefix(line, "@@"):
			if _, after, ok := strings.Cut(line, "+"); ok {
				start, _, _ := strings.Cut(strings.Fields(after)[0], ",")
				number, _ = strconv.Atoi(start)
			}
		case strings.HasPrefix(line, "+"):
			lines = append(lines, addedLine{path: path, number: number, text: line[1:]})
			number++
		}
	}
	// New files the agent has not staged are not in `git diff HEAD`; every
	// line of a small untracked text file counts as added.
	for _, name := range untrackedFiles(dir) {
		info, err := os.Stat(filepath.Join(dir, name))
		if name == "" || err != nil || !info.Mode().IsRegular() || info.Size() > untrackedFileLimit {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil || bytes.IndexByte(data, 0) >= 0 {
			continue
		}
		for i, text := range strings.Split(strings.TrimSuffix(string(data), "\n"), "\n") {
			lines = append(lines, addedLine{path: filepath.ToSlash(name), number: i + 1, text: text})
		}
	}
	return lines
}

//nolint:gocyclo // Guard selection, turn baselines, and finding collection share one advisory hook boundary.
func (s *Store) stopHook(input io.Reader, output io.Writer) {
	response := map[string]any{"continue": true}
	defer func() { _ = json.NewEncoder(output).Encode(response) }()
	payload := readHookPayload(input)
	if payload == nil || payload["stop_hook_active"] == true || os.Getenv("LEARN_PREFERENCES_MAINTENANCE") == "1" {
		return
	}
	if reason := s.claimUnseenPublication(textField(payload["session_id"])); reason != "" {
		response["decision"] = "block"
		response["reason"] = reason
		return
	}
	cwd, session := textField(payload["cwd"]), textField(payload["session_id"])
	repo, _ := RepositoryKey(cwd)
	var checks []guard
	for _, binding := range s.diffChecksFor(repo) {
		for _, g := range s.loadGuards() {
			if g.Kind == "diff_check" && g.RuleID == binding.ruleID && len(g.Patterns) > 0 && g.Patterns[0] == binding.name {
				checks = append(checks, g)
				break
			}
		}
	}
	if len(checks) == 0 {
		return
	}
	state := s.loadSession(session)
	current := workingDiffDigest(cwd)
	if current == state.TurnDiffDigest || current == state.StopBlockedTurn {
		return
	}
	existing := map[string]bool{}
	for _, key := range state.TurnAddedLines {
		existing[key] = true
	}
	byPath := map[string][]addedLine{}
	for _, line := range addedLines(cwd) {
		if !existing[line.key()] {
			byPath[line.path] = append(byPath[line.path], line)
		}
	}
	var findings []string
	for _, g := range checks {
		directive := textField(s.activeRule(g.RuleID)["directive"])
		for path, lines := range byPath {
			texts := make([]string, len(lines))
			for i, line := range lines {
				texts[i] = line.text
			}
			for _, finding := range diffChecks[g.Patterns[0]](path, texts) {
				index, message, _ := strings.Cut(finding, ": ")
				i, _ := strconv.Atoi(index)
				findings = append(findings, fmt.Sprintf("%s:%d %s (%q)", path, lines[i].number, message, directive))
			}
		}
	}
	if len(findings) == 0 {
		return
	}
	_ = s.updateSession(session, func(stored *sessionState) { stored.StopBlockedTurn = current })
	_ = s.recordActivity(ActivityEvent{Kind: activityBlocked, RepoKey: repo, SessionKey: digest(session), Detail: strings.Join(findings, "; ")})
	response["decision"] = "block"
	response["reason"] = "Before finishing, fix these preference violations:\n- " + strings.Join(findings, "\n- ")
}

// claimUnseenPublication returns a stop reason when a review this session owns
// has a published batch the agent has not been told about yet, so feedback
// published mid-turn is never left unread.
func (s *Store) claimUnseenPublication(session string) string {
	notices := s.ownedReviewNotices(session)
	if len(notices) == 0 {
		return ""
	}
	for _, notice := range notices {
		if notice.Pending == 0 || !s.claimBatch(session, notice.BatchID) {
			continue
		}
		return fmt.Sprintf("The reviewer published Hunkyard review %s (%s). Run: hunkyard feedback await --session %s --json and process it before finishing.", notice.ReviewID, notice.Disposition, notice.ReviewID)
	}
	return ""
}
