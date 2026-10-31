package preferences

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/BSH2409/hunkyard/internal/strictjson"
)

const cronBegin = "# BEGIN CODEX LEARN-PREFERENCES"
const cronEnd = "# END CODEX LEARN-PREFERENCES"

func shellQuote(text string) string { return "'" + strings.ReplaceAll(text, "'", "'\"'\"'") + "'" }

func (s *Store) binaryCommand() (string, error) {
	binary, err := os.Executable()
	if err != nil {
		return "", err
	}
	binary, err = filepath.EvalSymlinks(binary)
	if err != nil {
		return "", err
	}
	if strings.ContainsAny(binary+s.config.Root, "\x00\r\n") {
		return "", fmt.Errorf("hook paths must be single-line")
	}
	return shellQuote(binary), nil
}

func (s *Store) hookCommand(host, event string) (string, error) {
	binary, err := s.binaryCommand()
	if err != nil {
		return "", err
	}
	return binary + " preferences hook --event " + event + " --host " + host + " --root " + shellQuote(s.config.Root), nil
}

// hookEvent is one Hunkyard handler: name is the host's settings event,
// handler the --event it runs (defaults to name), host limits it to one host.
type hookEvent struct {
	name, handler, host, matcher, statusMessage string
	emitsContext, asyncRewake                   bool
	timeout                                     int
}

var hookEvents = []hookEvent{
	{name: "SessionStart", statusMessage: loadingMessages[0], emitsContext: true},
	{name: "UserPromptSubmit", statusMessage: loadingMessages[0], emitsContext: true},
	{name: "PreToolUse", matcher: "Bash|shell|exec_command|Edit|Write|MultiEdit|apply_patch", statusMessage: "Checking your preferences"},
	{name: "Stop", statusMessage: "Reviewing against your preferences"},
	{name: "Interrupt", host: "codex", statusMessage: capturingMessages[2]},
	{name: "Stop", handler: "ReviewWake", host: "claude", statusMessage: reviewWaitingMessages[1], asyncRewake: true, timeout: reviewWakeTimeout},
}

func (e hookEvent) handlerName() string {
	if e.handler != "" {
		return e.handler
	}
	return e.name
}

func hookEventsFor(host string) []hookEvent {
	var events []hookEvent
	for _, event := range hookEvents {
		if event.host == "" || event.host == host {
			events = append(events, event)
		}
	}
	return events
}

func (s *Store) hookPath(host string) (string, error) {
	switch host {
	case "codex":
		if s.config.explicitRoot {
			return filepath.Join(s.config.Root, "hooks.json"), nil
		}
		root, err := hostHome("CODEX_HOME", ".codex")
		if err != nil {
			return "", err
		}
		return filepath.Join(root, "hooks.json"), nil
	case "claude":
		root, err := hostHome("CLAUDE_CONFIG_DIR", ".claude")
		if err != nil {
			return "", err
		}
		return filepath.Join(root, "settings.json"), nil
	}
	return "", fmt.Errorf("hook host must be codex or claude")
}

// hostHome returns a host's configuration directory: the environment
// override when set, otherwise the named directory under the user's home.
func hostHome(env, fallback string) (string, error) {
	if dir := os.Getenv(env); strings.TrimSpace(dir) != "" {
		return dir, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, fallback), nil
}

// isHunkyardPreferenceHook matches any earlier Hunkyard preference hook,
// whatever binary path or store root it was installed with, and the legacy
// Ruby hook it replaced.
func isHunkyardPreferenceHook(command string) bool {
	return strings.Contains(command, " preferences hook") || strings.Contains(command, "learn-preferences/scripts/user_prompt_hook.rb")
}

var trustedField = regexp.MustCompile(`"trusted"\s*:`)

//nolint:gocyclo // Settings merge, verified backup, readback, and rollback form one installation transaction.
func (s *Store) installHook(host string) (map[string]any, error) {
	target, err := s.hookPath(host)
	if err != nil {
		return nil, err
	}
	commands := map[string]any{}
	for _, event := range hookEventsFor(host) {
		command, err := s.hookCommand(host, event.handlerName())
		if err != nil {
			return nil, err
		}
		commands[event.handlerName()] = command
	}
	if err := ensureDirectory(filepath.Dir(target)); err != nil {
		return nil, err
	}
	var result map[string]any
	err = withFileLock(target+".hunkyard.lock", false, func() error {
		original, err := regularFile(target)
		existed := err == nil
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		document := map[string]any{}
		if len(bytes.TrimSpace(original)) > 0 {
			if err := strictjson.Decode(original, &document); err != nil {
				return err
			}
			if document == nil {
				return fmt.Errorf("hook configuration must be an object")
			}
		}
		hooks := map[string]any{}
		if value, exists := document["hooks"]; exists {
			hooks = record(value)
			if hooks == nil {
				return fmt.Errorf("hooks must be an object")
			}
		}
		for name, value := range hooks {
			groups, err := mapArray(value)
			if err != nil {
				return err
			}
			retained := []any{}
			for _, group := range groups {
				entries, err := mapArray(group["hooks"])
				if err != nil {
					return err
				}
				keep := []any{}
				for _, entry := range entries {
					if !isHunkyardPreferenceHook(textField(entry["command"])) {
						keep = append(keep, entry)
					}
				}
				if len(keep) > 0 {
					group["hooks"] = keep
					retained = append(retained, group)
				}
			}
			hooks[name] = retained
		}
		for _, event := range hookEventsFor(host) {
			timeout := event.timeout
			if timeout == 0 {
				timeout = 5
			}
			handler := map[string]any{"type": "command", "command": commands[event.handlerName()], "timeout": timeout, "statusMessage": event.statusMessage}
			if host == "codex" && event.emitsContext {
				handler["additionalContextLimit"] = 1500
			}
			if event.asyncRewake {
				handler["asyncRewake"] = true
			}
			group := map[string]any{"hooks": []any{handler}}
			if event.matcher != "" {
				group["matcher"] = event.matcher
			}
			existing, _ := hooks[event.name].([]any)
			hooks[event.name] = append(existing, group)
		}
		for name, value := range hooks {
			if groups, _ := value.([]any); len(groups) == 0 {
				delete(hooks, name)
			}
		}
		document["hooks"] = hooks
		if host == "claude" {
			statusCommand, err := s.binaryCommand()
			if err != nil {
				return err
			}
			mergeClaudeSpinnerSettings(document, statusCommand+" preferences statusline --root "+shellQuote(s.config.Root))
		}
		rendered, err := json.MarshalIndent(document, "", "  ")
		if err != nil {
			return err
		}
		rendered = append(rendered, '\n')
		if bytes.Contains(rendered, []byte("dangerously-bypass-hook-trust")) || trustedField.Match(rendered) {
			return fmt.Errorf("hook configuration cannot synthesize trust")
		}
		var backup any
		if existed {
			if err := ensureDirectory(s.dataPath("backups")); err != nil {
				return err
			}
			name := s.dataPath("backups", filepath.Base(target)+"."+s.config.Clock().UTC().Format("20060102T150405Z")+"-"+randomID()+".bak")
			if err := writeNewFile(name, original); err != nil {
				return err
			}
			backup = name
		}
		installErr := atomicReplace(target, rendered)
		if installErr == nil {
			installErr = s.interrupt("hook_written")
		}
		if installErr == nil {
			actual, err := regularFile(target)
			if err != nil {
				installErr = err
			} else if !bytes.Equal(actual, rendered) {
				installErr = fmt.Errorf("hook readback mismatch")
			}
		}
		if installErr != nil {
			var rollback error
			if existed {
				rollback = atomicReplace(target, original)
			} else {
				rollback = os.Remove(target)
				if rollback == nil {
					rollback = syncDirectory(filepath.Dir(target))
				}
			}
			return errors.Join(installErr, rollback)
		}
		result = map[string]any{"installed": true, "path": target, "backup_path": backup, "command": commands["UserPromptSubmit"], "commands": commands, "trust_required": true, "trust_instruction": "Inspect each Hunkyard preferences command in the host's /hooks view and trust it only if its absolute paths match this report."}
		return nil
	})
	return result, err
}

func (s *Store) renderCron(existing string) (string, error) {
	binary, err := s.binaryCommand()
	if err != nil {
		return "", err
	}
	command := binary + " preferences maintain --auto --json --root " + shellQuote(s.config.Root)
	line := "17 * * * * " + command + " >> " + shellQuote(s.dataPath("logs", "cron.log")) + " 2>&1"
	line = strings.ReplaceAll(line, "%", `\%`)
	block := cronBegin + "\n" + line + "\n" + cronEnd + "\n"
	lines := strings.SplitAfter(existing, "\n")
	start, end := -1, -1
	for i, line := range lines {
		switch strings.TrimSpace(line) {
		case cronBegin:
			if start != -1 || end != -1 {
				return "", fmt.Errorf("duplicate or nested cron markers")
			}
			start = i
		case cronEnd:
			if start == -1 || end != -1 {
				return "", fmt.Errorf("unmatched cron marker")
			}
			end = i
		}
	}
	if (start == -1) != (end == -1) {
		return "", fmt.Errorf("unmatched cron marker")
	}
	if start >= 0 {
		return strings.Join(lines[:start], "") + block + strings.Join(lines[end+1:], ""), nil
	}
	if existing != "" && !strings.HasSuffix(existing, "\n") {
		existing += "\n"
	}
	return existing + block, nil
}

//nolint:gocyclo // Log privacy, backup, readback, and rollback remain one installation transaction.
func (s *Store) installCron(read func() (string, error), write func(string) error) (map[string]any, error) {
	original, err := read()
	if err != nil {
		return nil, err
	}
	rendered, err := s.renderCron(original)
	if err != nil {
		return nil, err
	}
	if err := ensureDirectory(s.dataPath("logs")); err != nil {
		return nil, err
	}
	logPath := s.dataPath("logs", "cron.log")
	if _, err := regularFile(logPath); os.IsNotExist(err) {
		if err := writeNewFile(logPath, nil); err != nil {
			return nil, err
		}
	} else if err != nil {
		return nil, err
	}
	if err := os.Chmod(logPath, 0600); err != nil {
		return nil, err
	}
	if err := ensureDirectory(s.dataPath("backups")); err != nil {
		return nil, err
	}
	if err := writeNewFile(s.dataPath("backups", "crontab."+randomID()+".bak"), []byte(original)); err != nil {
		return nil, err
	}
	installErr := write(rendered)
	if installErr == nil {
		actual, err := read()
		if err != nil {
			installErr = err
		} else if actual != rendered {
			installErr = fmt.Errorf("crontab readback mismatch")
		}
	}
	if installErr != nil {
		rollback := write(original)
		if rollback == nil {
			actual, err := read()
			if err != nil {
				rollback = err
			} else if actual != original {
				rollback = fmt.Errorf("crontab rollback mismatch")
			}
		}
		return nil, errors.Join(installErr, rollback)
	}
	_, block, _ := strings.Cut(rendered, cronBegin+"\n")
	command, _, _ := strings.Cut(block, "\n")
	return map[string]any{"installed": true, "schedule": "17 * * * *", "command": command}, nil
}

func readCrontab() (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, "crontab", "-l")
	var stderr bytes.Buffer
	command.Stderr = &stderr
	out, err := command.Output()
	if err == nil {
		return string(out), nil
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) && exit.ExitCode() == 1 && strings.Contains(strings.ToLower(string(out)+stderr.String()), "no crontab for") {
		return "", nil
	}
	return "", fmt.Errorf("unable to read crontab")
}
func writeCrontab(text string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, "crontab", "-")
	command.Stdin = strings.NewReader(text)
	if command.Run() != nil {
		return fmt.Errorf("unable to install crontab")
	}
	return nil
}

func (s *Store) hookInstalled() bool {
	for _, host := range []string{"codex", "claude"} {
		command, err := s.hookCommand(host, "UserPromptSubmit")
		if err != nil {
			return false
		}
		target, err := s.hookPath(host)
		if err != nil {
			continue
		}
		document := readJSONFile(target)
		groups, err := mapArray(record(document["hooks"])["UserPromptSubmit"])
		if err != nil {
			continue
		}
		for _, group := range groups {
			entries, _ := mapArray(group["hooks"])
			for _, entry := range entries {
				value := textField(entry["command"])
				if value == command || strings.Contains(value, "learn-preferences/scripts/user_prompt_hook.rb") {
					return true
				}
			}
		}
	}
	return false
}

// mergeClaudeSpinnerSettings adds Hunkyard's spinner verbs and tips to the
// user's own, and offers a status line only when none is configured.
func mergeClaudeSpinnerSettings(document map[string]any, statusCommand string) {
	appendMissing := func(existing []string, additions []string) []any {
		merged := []any{}
		seen := map[string]bool{}
		for _, value := range append(existing, additions...) {
			if !seen[value] {
				seen[value] = true
				merged = append(merged, value)
			}
		}
		return merged
	}
	verbs := record(document["spinnerVerbs"])
	if verbs == nil {
		verbs = map[string]any{"mode": "append"}
	}
	verbs["verbs"] = appendMissing(stringValues(verbs["verbs"]), spinnerVerbs)
	document["spinnerVerbs"] = verbs
	tips := record(document["spinnerTipsOverride"])
	if tips == nil {
		tips = map[string]any{}
	}
	existing, _ := tips["tips"].([]any)
	plain := []string{}
	objects := []any{}
	for _, tip := range existing {
		if text, ok := tip.(string); ok {
			plain = append(plain, text)
		} else {
			objects = append(objects, tip)
		}
	}
	tips["tips"] = append(objects, appendMissing(plain, spinnerTips)...)
	document["spinnerTipsOverride"] = tips
	if _, exists := document["statusLine"]; !exists {
		document["statusLine"] = map[string]any{"type": "command", "command": statusCommand}
	}
}
