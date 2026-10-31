package preferences

import (
	"bufio"
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

// diffChecks are mechanical checks a diff_check guard can run on lines the
// agent added. Each returns one message per offending line ("line N: …").
var diffChecks = map[string]diffCheck{
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
	return digest(gitOutput(dir, "diff", "HEAD"))
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
	var checks []guard
	for _, g := range s.loadGuards() {
		if g.Kind == "diff_check" && len(g.Patterns) > 0 && diffChecks[g.Patterns[0]] != nil && s.activeRule(g.RuleID) != nil {
			checks = append(checks, g)
		}
	}
	if len(checks) == 0 {
		return
	}
	cwd, session := textField(payload["cwd"]), textField(payload["session_id"])
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
	repo, _ := repositoryKey(cwd)
	_ = s.recordActivity(ActivityEvent{Kind: activityBlocked, RepoKey: repo, SessionKey: digest(session), Detail: strings.Join(findings, "; ")})
	response["decision"] = "block"
	response["reason"] = "Before finishing, fix these preference violations:\n- " + strings.Join(findings, "\n- ")
}
