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

func (s *Store) hookCommand() (string, error) {
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
	return shellQuote(binary) + " preferences hook --root " + shellQuote(s.config.Root), nil
}

func (s *Store) hookPath(host string) (string, error) {
	switch host {
	case "codex":
		return filepath.Join(s.config.Root, "hooks.json"), nil
	case "claude":
		root := os.Getenv("CLAUDE_CONFIG_DIR")
		if root == "" {
			home, err := os.UserHomeDir()
			if err != nil {
				return "", err
			}
			root = filepath.Join(home, ".claude")
		}
		return filepath.Join(root, "settings.json"), nil
	}
	return "", fmt.Errorf("hook host must be codex or claude")
}

var trustedField = regexp.MustCompile(`"trusted"\s*:`)

//nolint:gocyclo // Settings merge, verified backup, readback, and rollback form one installation transaction.
func (s *Store) installHook(host string) (map[string]any, error) {
	target, err := s.hookPath(host)
	if err != nil {
		return nil, err
	}
	command, err := s.hookCommand()
	if err != nil {
		return nil, err
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
		groups := []map[string]any{}
		if value, exists := hooks["UserPromptSubmit"]; exists {
			var err error
			groups, err = mapArray(value)
			if err != nil {
				return err
			}
		}
		retained := []any{}
		for _, group := range groups {
			entries, err := mapArray(group["hooks"])
			if err != nil {
				return err
			}
			keep := []any{}
			for _, entry := range entries {
				old := textField(entry["command"])
				if old == command || strings.Contains(old, "learn-preferences/scripts/user_prompt_hook.rb") {
					continue
				}
				keep = append(keep, entry)
			}
			if len(keep) > 0 {
				group["hooks"] = keep
				retained = append(retained, group)
			}
		}
		handler := map[string]any{"type": "command", "command": command, "timeout": 5}
		if host == "codex" {
			handler["statusMessage"] = "Loading coding preferences"
			handler["additionalContextLimit"] = 1500
		}
		hooks["UserPromptSubmit"] = append(retained, map[string]any{"hooks": []any{handler}})
		document["hooks"] = hooks
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
		result = map[string]any{"installed": true, "path": target, "backup_path": backup, "command": command, "trust_required": true, "trust_instruction": "Inspect the UserPromptSubmit command in the host's /hooks view and trust it only if its absolute paths match this report."}
		return nil
	})
	return result, err
}

func (s *Store) renderCron(existing string) (string, error) {
	command, err := s.hookCommand()
	if err != nil {
		return "", err
	}
	command = strings.Replace(command, " preferences hook --root ", " preferences maintain --due --json --root ", 1)
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
	command, err := s.hookCommand()
	if err != nil {
		return false
	}
	for _, host := range []string{"codex", "claude"} {
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
