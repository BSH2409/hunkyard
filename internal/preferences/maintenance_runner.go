package preferences

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/BSH2409/hunkyard/internal/config"
)

//go:embed assets/maintenance-instructions.md
var maintenanceInstructions string

const maintenanceTimeout = 15 * time.Minute

// maintenanceRunner runs the configured model CLI with the prompt on stdin.
type maintenanceRunner func(ctx context.Context, argv, env []string, dir string, stdin []byte) ([]byte, error)

// maintenanceRun is the outcome of the last automatic maintenance attempt.
type maintenanceRun struct {
	At         string `json:"at"`
	Host       string `json:"host"`
	Model      string `json:"model"`
	Effort     string `json:"effort"`
	Outcome    string `json:"outcome"`
	ErrorClass string `json:"error_class,omitempty"`
}

var authFailure = regexp.MustCompile(`(?i)\b401\b|unauthori[sz]ed|not logged in|please (?:run )?log ?in|login required`)

func maintenanceArgv(p config.PreferencesMaintenance) []string {
	if p.Host == "codex" {
		return []string{"codex", "exec", "-m", p.Model, "-c", fmt.Sprintf("model_reasoning_effort=%q", p.Effort), "-s", "read-only", "--skip-git-repo-check", "-"}
	}
	return []string{"claude", "-p", "--model", p.Model, "--effort", p.Effort, "--output-format", "text"}
}

// maintenanceChildEnv strips the launching host session's identity, so a
// nested claude or codex run starts as a fresh, independent session, while
// keeping the configuration directories it needs to authenticate.
func maintenanceChildEnv(environ []string) []string {
	env := []string{}
	for _, entry := range environ {
		name, _, _ := strings.Cut(entry, "=")
		hostSession := name == "CLAUDECODE" || strings.HasPrefix(name, "CLAUDE_CODE_") || strings.HasPrefix(name, "CODEX_") && name != "CODEX_HOME"
		if !hostSession && name != "LEARN_PREFERENCES_MAINTENANCE" {
			env = append(env, entry)
		}
	}
	return append(env, "LEARN_PREFERENCES_MAINTENANCE=1")
}

func execMaintenanceRunner(ctx context.Context, argv, env []string, dir string, stdin []byte) ([]byte, error) {
	command := exec.CommandContext(ctx, argv[0], argv[1:]...)
	command.Env = env
	command.Dir = dir
	command.Stdin = bytes.NewReader(stdin)
	var stderr bytes.Buffer
	command.Stderr = &stderr
	out, err := command.Output()
	if err != nil {
		return out, fmt.Errorf("%w: %s %s", err, strings.TrimSpace(stderr.String()), strings.TrimSpace(string(out)))
	}
	return out, nil
}

// extractProposalJSON takes the single JSON object a model returned, even
// when it is wrapped in Markdown fences or prose.
func extractProposalJSON(output []byte) ([]byte, error) {
	start, end := bytes.IndexByte(output, '{'), bytes.LastIndexByte(output, '}')
	if start < 0 || end < start {
		return nil, fmt.Errorf("no JSON object in output")
	}
	candidate := output[start : end+1]
	var object map[string]any
	if err := json.Unmarshal(candidate, &object); err != nil {
		return nil, err
	}
	return candidate, nil
}

func maintenancePrompt(dry map[string]any, rejection string) ([]byte, error) {
	input, err := json.Marshal(map[string]any{"input_digest": dry["input_digest"], "batch": dry["batch"], "source_pack_files": dry["source_pack_files"]})
	if err != nil {
		return nil, err
	}
	var prompt strings.Builder
	prompt.WriteString(maintenanceInstructions)
	prompt.WriteString("\n\n## Proposal JSON schema\n\n")
	prompt.Write(proposalSchema)
	prompt.WriteString("\n\n## Maintenance input\n\n")
	prompt.Write(input)
	if rejection != "" {
		prompt.WriteString("\n\nYour previous proposal was rejected by the validator: " + rejection + "\nCorrect it.")
	}
	prompt.WriteString("\n\nReturn only the proposal JSON object.\n")
	return []byte(prompt.String()), nil
}

func (s *Store) lastMaintenanceRun() maintenanceRun {
	var run maintenanceRun
	data, err := regularFile(s.dataPath("maintenance-run.json"))
	if err != nil || json.Unmarshal(data, &run) != nil {
		return maintenanceRun{}
	}
	return run
}

func (s *Store) recordMaintenanceRun(p config.PreferencesMaintenance, outcome, errorClass string) {
	run := maintenanceRun{At: s.config.Clock().UTC().Format(time.RFC3339), Host: p.Host, Model: p.Model, Effort: p.Effort, Outcome: outcome, ErrorClass: errorClass}
	if encoded, err := json.Marshal(run); err == nil && ensureDirectory(s.dataPath()) == nil {
		_ = atomicReplace(s.dataPath("maintenance-run.json"), append(encoded, '\n'))
	}
	kind, detail := activityMaintenanceSucceeded, p.Host+" "+p.Model
	if outcome != "success" {
		kind, detail = activityMaintenanceFailed, errorClass
	}
	_ = s.recordActivity(ActivityEvent{Kind: kind, Detail: detail})
}

// runAutomaticMaintenance consolidates pending observations with the
// configured model CLI. The engine still validates every proposal; a
// rejected proposal is retried once with the validation error.
func (s *Store) runAutomaticMaintenance(ctx context.Context, p config.PreferencesMaintenance, force bool, run maintenanceRunner) (map[string]any, error) {
	p = p.Resolved()
	if p.Host == "" {
		return map[string]any{"status": "disabled"}, nil
	}
	if !p.Supported() {
		s.recordMaintenanceRun(p, "failed", "invalid_config")
		return map[string]any{"status": "failed", "error_class": "invalid_config"}, nil
	}
	if !force && readJSONFile(s.dataPath("state.json"))["last_success_local_date"] == s.config.Clock().Local().Format("2006-01-02") {
		return map[string]any{"status": "already_complete"}, nil
	}
	var result map[string]any
	err := withFileLock(s.dataPath(".auto-maintenance.lock"), true, func() error {
		dry, err := s.maintain("dry_run", nil)
		if err != nil {
			return err
		}
		observations := 0
		switch values := record(dry["batch"])["observations"].(type) {
		case []any:
			observations = len(values)
		case []map[string]any:
			observations = len(values)
		}
		sources := record(dry["source_pack_files"])
		if dry["status"] != "needs_proposal" || observations == 0 && len(sources) == 0 {
			result = map[string]any{"status": "no_work"}
			return nil
		}
		fail := func(class string) {
			s.recordMaintenanceRun(p, "failed", class)
			result = map[string]any{"status": "failed", "error_class": class}
		}
		rejection := ""
		for attempt := 0; attempt < 2; attempt++ {
			prompt, err := maintenancePrompt(dry, rejection)
			if err != nil {
				return err
			}
			runCtx, cancel := context.WithTimeout(ctx, maintenanceTimeout)
			output, err := run(runCtx, maintenanceArgv(p), maintenanceChildEnv(os.Environ()), s.config.Root, prompt)
			timedOut := errors.Is(runCtx.Err(), context.DeadlineExceeded)
			cancel()
			switch {
			case timedOut:
				fail("timeout")
				return nil
			case err != nil && (authFailure.MatchString(err.Error()) || authFailure.Match(output)):
				fail("auth_failed")
				return nil
			case err != nil:
				fail("command_failed")
				return nil
			}
			proposal, err := extractProposalJSON(output)
			if err != nil {
				fail("invalid_output")
				return nil
			}
			mode := "due"
			if force {
				mode = "force"
			}
			applied, err := s.maintain(mode, proposal)
			if err == nil {
				s.recordMaintenanceRun(p, "success", "")
				result = applied
				return nil
			}
			rejection = err.Error()
		}
		fail("rejected")
		return nil
	})
	if lockBusy(err) {
		return map[string]any{"status": "already_running"}, nil
	}
	return result, err
}

// startDetached launches a background process that outlives the hook.
var startDetached = func(argv []string, logPath string) error {
	log, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return err
	}
	defer log.Close()
	command := exec.Command(argv[0], argv[1:]...)
	command.Stdout, command.Stderr = log, log
	command.Env = maintenanceChildEnv(os.Environ())
	command.Dir = filepath.Dir(logPath)
	detachProcess(command)
	if err := command.Start(); err != nil {
		return err
	}
	return command.Process.Release()
}

// startMaintenanceIfDue starts automatic maintenance in the background when
// a model host is configured and unprocessed observations are waiting.
func (s *Store) startMaintenanceIfDue(current *generation) {
	global, _, _ := config.LoadConfigFile(config.GlobalConfigPath())
	if global.PreferencesMaintenance.Host == "" || s.unprocessedCount(current) == 0 {
		return
	}
	state := readJSONFile(s.dataPath("state.json"))
	if state["last_success_local_date"] == s.config.Clock().Local().Format("2006-01-02") {
		return
	}
	binary, err := os.Executable()
	if err != nil || ensureDirectory(s.dataPath("logs")) != nil {
		return
	}
	_ = startDetached([]string{binary, "preferences", "maintain", "--auto", "--json", "--root", s.config.Root}, s.dataPath("logs", "auto-maintenance.log"))
}
