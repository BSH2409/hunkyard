package preferences

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"github.com/BSH2409/hunkyard/internal/clicmd"
	"github.com/BSH2409/hunkyard/internal/config"
	"io"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
)

// Run exposes the local preference engine without invoking another model process.
//
//nolint:gocyclo // The flat command dispatcher keeps each supported flag beside its operation.
func Run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	emit := func(value any) { _ = json.NewEncoder(stdout).Encode(value) }
	usage := func(class string) int { emit(map[string]any{"status": "usage", "error_class": class}); return 2 }
	if len(args) > 0 && (args[0] == "guard" || args[0] == "allow") {
		return runGuardCLI(args, stdout)
	}
	if len(args) > 0 && args[0] == "check" {
		return runGuardCheck(args[1:], stdout)
	}
	if len(args) > 0 && args[0] == "statusline" {
		return runStatusline(args[1:], stdout)
	}
	if len(args) > 0 && args[0] == "doctor" {
		parser := flag.NewFlagSet("doctor", flag.ContinueOnError)
		parser.SetOutput(io.Discard)
		root := parser.String("root", "", "Canonical preference root")
		parser.Bool("json", false, "JSON output")
		if parser.Parse(args[1:]) != nil {
			return usage("LearnPreferences::CLIUsageError")
		}
		s, err := Open(Config{Root: *root})
		if err != nil {
			emit(map[string]any{"status": "rejected", "error_class": "LearnPreferences::ValidationError"})
			return 3
		}
		emit(s.doctor())
		return 0
	}
	if len(args) == 0 || !slices.Contains(strings.Fields("capture search build audit retire maintain migrate install-hook install-cron status hook select"), args[0]) {
		return usage("LearnPreferences::CLIUsageError")
	}
	command := args[0]
	parser := flag.NewFlagSet(command, flag.ContinueOnError)
	parser.SetOutput(io.Discard)
	root := parser.String("root", "", "Canonical preference root")
	asJSON := parser.Bool("json", false, "JSON output")
	var feedback, directive, scope, repo, cwd, previous, response, query, proposalPath, projectRoot, host, event string
	var explicit, duplicates, dry, due, force, auto bool
	modeCount := 0
	modeFlag := func(name, help string, target *bool) {
		parser.BoolFunc(name, help, func(value string) error {
			if value != "true" {
				return fmt.Errorf("maintenance modes do not take values")
			}
			modeCount++
			*target = true
			return nil
		})
	}
	var selected []string
	surfaceFlag := func(value string) error { selected = append(selected, value); return nil }
	switch command {
	case "capture":
		parser.StringVar(&feedback, "feedback", "", "Feedback text")
		parser.StringVar(&directive, "directive", "", "Normalized directive")
		parser.StringVar(&scope, "scope", "", "global or repository")
		parser.StringVar(&repo, "repo-key", "", "Repository identity")
		parser.StringVar(&cwd, "cwd", "", "Repository directory")
		parser.StringVar(&previous, "previous-user-request", "", "Previous request")
		parser.StringVar(&response, "agent-response", "", "Corrected agent action")
		parser.BoolVar(&explicit, "explicit", false, "Explicit durable directive")
		parser.Func("surface", "Canonical surface", surfaceFlag)
	case "search", "select":
		parser.StringVar(&query, "query", "", "Search or routing text")
		parser.Func("surface", "Canonical surface", surfaceFlag)
		if command == "select" {
			parser.StringVar(&repo, "repo-key", "", "Repository identity")
			parser.StringVar(&cwd, "cwd", "", "Repository directory")
		}
	case "audit":
		parser.BoolVar(&duplicates, "duplicates", false, "Find duplicate rules")
	case "maintain":
		modeFlag("dry-run", "Validate without writes", &dry)
		modeFlag("due", "Run once per local date", &due)
		modeFlag("force", "Run regardless of daily stamp", &force)
		parser.BoolVar(&auto, "auto", false, "Consolidate with the configured model CLI")
		parser.StringVar(&proposalPath, "proposal", "", "Local proposal JSON")
	case "migrate":
		parser.BoolVar(&dry, "dry-run", false, "Inspect legacy inputs")
		parser.StringVar(&projectRoot, "project-root", "", "Project preference root")
		parser.StringVar(&repo, "repo-key", "", "Repository identity")
		parser.StringVar(&proposalPath, "proposal", "", "Local proposal JSON")
	case "hook":
		parser.StringVar(&event, "event", "UserPromptSubmit", "Host hook event")
		parser.StringVar(&host, "host", "", "claude or codex")
	case "install-hook":
		parser.StringVar(&host, "host", "codex", "codex or claude")
	}
	remaining := clicmd.ReorderFlagsFirst(args[1:], map[string]bool{"json": true, "explicit": true, "duplicates": true, "dry-run": true, "due": true, "force": true, "auto": true})
	var ruleID string
	if err := parser.Parse(remaining); err != nil {
		class := "OptionParser::InvalidOption"
		if strings.Contains(err.Error(), "needs an argument") {
			class = "OptionParser::MissingArgument"
		}
		return usage(class)
	}
	if command == "retire" && ruleID == "" && len(parser.Args()) == 1 {
		ruleID = parser.Args()[0]
	} else if len(parser.Args()) != 0 {
		return usage("LearnPreferences::CLIUsageError")
	}
	if command == "retire" && ruleID == "" {
		return usage("LearnPreferences::CLIUsageError")
	}
	if command == "maintain" && (auto && (dry || modeCount > 1) || !auto && modeCount != 1) {
		return usage("LearnPreferences::CLIUsageError")
	}
	if command == "capture" {
		provided := false
		parser.Visit(func(f *flag.Flag) {
			if f.Name == "feedback" {
				provided = true
			}
		})
		if !provided {
			data, err := io.ReadAll(io.LimitReader(stdin, 1024*1024+1))
			if err != nil || len(data) > 1024*1024 {
				return usage("LearnPreferences::CLIUsageError")
			}
			feedback = string(data)
		}
		if strings.TrimSpace(feedback) == "" || explicit && (directive == "" || !slices.Contains([]string{"global", "repository"}, scope) || len(selected) == 0 || scope == "global" && repo != "") {
			return usage("LearnPreferences::CLIUsageError")
		}
	}
	if command == "search" || explicit {
		for _, surface := range selected {
			if !slices.Contains(surfaces, surface) {
				return usage("LearnPreferences::CLIUsageError")
			}
		}
	}
	invalidRoot := false
	parser.Visit(func(option *flag.Flag) {
		if option.Name == "root" && strings.TrimSpace(*root) == "" {
			invalidRoot = true
		}
	})
	var s *Store
	var err error
	if invalidRoot {
		err = fmt.Errorf("preference root cannot be empty")
	} else {
		s, err = Open(Config{Root: *root})
	}
	if err != nil {
		if command == "hook" {
			emit(map[string]any{"continue": true})
			return 0
		}
		emit(map[string]any{"status": "rejected", "error_class": "LearnPreferences::ValidationError"})
		return 3
	}
	if command == "hook" {
		s.hostKind = host
		return s.hook(event, stdin, stdout, stderr)
	}
	var result any
	exit := 0
	switch command {
	case "capture":
		if cwd == "" {
			cwd, _ = os.Getwd()
		}
		if explicit && scope == "repository" && repo == "" {
			repo, err = repositoryKey(cwd)
			if err != nil || repo == "" {
				return usage("LearnPreferences::CLIUsageError")
			}
		}
		var value CaptureResult
		value, err = s.Capture(CaptureInput{Feedback: feedback, Directive: directive, Scope: scope, RepoKey: repo, CWD: cwd, Surfaces: selected, Explicit: explicit, Source: "cli", SessionID: "cli-" + randomID(), TurnID: s.config.Clock().UTC().Format("2006-01-02T15:04:05.999999999Z07:00"), Context: NeighboringContext{Source: "provided", PreviousUserRequest: previous, AgentResponse: response}})
		if err == nil && value.Action == "ignored" && value.Reason != "not_capturable" {
			err = fmt.Errorf("capture rejected")
		}
		status := "success"
		if value.Action == "ignored" {
			status = "no_work"
		}
		result = map[string]any{"status": status, "action": value.Action, "id": nullableString(value.ID)}
	case "search":
		var rules []map[string]any
		rules, err = s.search(query, selected)
		result = map[string]any{"status": "success", "rules": rules}
	case "select":
		result, err = s.SelectPersona(query, cwd, repo)
	case "build":
		result, err = s.build()
	case "retire":
		result, err = s.retire(ruleID)
	case "audit":
		value := s.audit(duplicates)
		result = value
		if value.Status != "success" {
			exit = 5
		}
	case "status":
		result, err = s.status()
	case "install-hook":
		result, err = s.installHook(host)
	case "install-cron":
		result, err = s.installCron(readCrontab, writeCrontab)
	case "maintain":
		var proposal []byte
		if proposalPath != "" {
			proposal, err = readProposal(proposalPath)
		}
		maintenance := config.PreferencesMaintenance{}
		if auto && proposal == nil {
			global, _, _ := config.LoadConfigFile(config.GlobalConfigPath())
			maintenance = global.PreferencesMaintenance
		}
		if err == nil && maintenance.Host != "" {
			var value map[string]any
			value, err = s.runAutomaticMaintenance(context.Background(), maintenance, force, execMaintenanceRunner)
			result = value
			if value["status"] == "failed" {
				exit = 4
			}
		} else if err == nil {
			mode := "due"
			if dry {
				mode = "dry_run"
			}
			if force {
				mode = "force"
			}
			result, err = s.maintain(mode, proposal)
		}
		if err != nil {
			exit = 4
		}
	case "migrate":
		cwd, _ := os.Getwd()
		if projectRoot == "" {
			projectRoot = filepath.Join(cwd, ".codex")
		}
		if repo == "" {
			repo, err = repositoryKey(cwd)
		}
		if err != nil || repo == "" {
			return usage("LearnPreferences::CLIUsageError")
		}
		var proposal []byte
		if proposalPath != "" {
			proposal, err = readProposal(proposalPath)
		}
		if err == nil {
			result, err = s.migrate(projectRoot, repo, dry, proposal)
		}
		if err != nil {
			exit = 4
		}
	default:
		err = fmt.Errorf("command is not available")
	}
	if err != nil {
		if exit == 4 {
			payload, _ := result.(map[string]any)
			if payload == nil {
				payload = map[string]any{"generation_id": nil, "input_digest": nil}
			}
			payload["status"] = "failed"
			payload["error_class"] = "LearnPreferences::ValidationError"
			if errors.Is(err, errStaleProposal) {
				payload["error_class"] = "LearnPreferences::StaleProposalError"
			}
			payload["error_fingerprint"] = digest(err.Error())
			emit(payload)
			return exit
		}
		emit(map[string]any{"status": "rejected", "error_class": "LearnPreferences::ValidationError"})
		if exit == 0 {
			exit = 3
		}
		return exit
	}
	if !*asJSON {
		data, err := json.Marshal(result)
		if err != nil {
			return 3
		}
		var fields map[string]json.RawMessage
		if json.Unmarshal(data, &fields) != nil {
			return 3
		}
		keys := make([]string, 0, len(fields))
		for key := range fields {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for i, key := range keys {
			if i > 0 {
				fmt.Fprint(stdout, " ")
			}
			fmt.Fprintf(stdout, "%s=%s", key, fields[key])
		}
		fmt.Fprintln(stdout)
		return exit
	}
	emit(result)
	return exit
}

func readProposal(name string) ([]byte, error) {
	info, err := os.Lstat(name)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > 16*1024*1024 {
		return nil, fmt.Errorf("proposal must be a regular file of at most 16 MiB")
	}
	return regularFile(name)
}

// runGuardCLI handles `guard add|list|remove` and `allow RULE_ID --once`.
//
//nolint:gocyclo // One case per guard subcommand keeps each beside its flags.
func runGuardCLI(args []string, stdout io.Writer) int {
	emit := func(value any) { _ = json.NewEncoder(stdout).Encode(value) }
	usage := func() int {
		emit(map[string]any{"status": "usage", "error_class": "LearnPreferences::CLIUsageError"})
		return 2
	}
	command := args[0]
	rest := args[1:]
	if command == "guard" {
		if len(rest) == 0 {
			return usage()
		}
		command, rest = "guard "+rest[0], rest[1:]
	}
	parser := flag.NewFlagSet(command, flag.ContinueOnError)
	parser.SetOutput(io.Discard)
	root := parser.String("root", "", "Canonical preference root")
	parser.Bool("json", false, "JSON output")
	kind := parser.String("kind", "", "Guard kind")
	ruleID := parser.String("rule", "", "Rule identity")
	once := parser.Bool("once", false, "Allow the next blocked action only")
	var patterns, match, notMatch []string
	parser.Func("pattern", "Guard pattern", func(value string) error { patterns = append(patterns, value); return nil })
	parser.Func("match", "Example the guard must trigger on", func(value string) error { match = append(match, value); return nil })
	parser.Func("not-match", "Example the guard must allow", func(value string) error { notMatch = append(notMatch, value); return nil })
	if parser.Parse(clicmd.ReorderFlagsFirst(rest, map[string]bool{"json": true, "once": true})) != nil {
		return usage()
	}
	s, err := Open(Config{Root: *root})
	if err != nil {
		emit(map[string]any{"status": "rejected", "error_class": "LearnPreferences::ValidationError"})
		return 3
	}
	var result map[string]any
	switch {
	case command == "guard add" && parser.NArg() == 0:
		var added guard
		added, err = s.addGuard(*kind, *ruleID, patterns, match, notMatch)
		result = map[string]any{"status": "success", "guard": added}
	case command == "guard test" && parser.NArg() == 0:
		results := s.testGuards()
		status := "success"
		for _, result := range results {
			if result["status"] == "fail" {
				status = "failed"
			}
		}
		result = map[string]any{"status": status, "guards": results}
	case command == "guard list" && parser.NArg() == 0:
		result = map[string]any{"status": "success", "guards": s.loadGuards()}
	case command == "guard remove" && parser.NArg() == 1:
		err = s.removeGuard(parser.Arg(0))
		result = map[string]any{"status": "success"}
	case command == "allow" && parser.NArg() == 1 && *once:
		result, err = s.allowOnce(parser.Arg(0))
	default:
		return usage()
	}
	if err != nil {
		emit(map[string]any{"status": "rejected", "error_class": "LearnPreferences::ValidationError", "error": err.Error()})
		return 3
	}
	emit(result)
	return 0
}

// runStatusline prints one short line for Claude's status bar. It never
// fails loudly: on any error it prints nothing.
func runStatusline(args []string, stdout io.Writer) int {
	parser := flag.NewFlagSet("statusline", flag.ContinueOnError)
	parser.SetOutput(io.Discard)
	root := parser.String("root", "", "Canonical preference root")
	if parser.Parse(args) != nil {
		return 0
	}
	s, err := Open(Config{Root: *root})
	if err != nil {
		return 0
	}
	current, err := s.loadCurrent()
	if err != nil {
		return 0
	}
	rules := 0
	if current != nil {
		for _, r := range current.Rules.rules {
			if r["status"] == "active" || r["status"] == "core" {
				rules++
			}
		}
	}
	pending := s.unprocessedCount(current)
	maintenance := "ok"
	if run := s.lastMaintenanceRun(); run.Outcome == "failed" {
		maintenance = "failed"
	} else if pending > 0 {
		maintenance = "due"
	}
	fmt.Fprintf(stdout, "◆ yard: %d rules · %d pending · maint %s\n", rules, pending, maintenance)
	return 0
}

// runGuardCheck reports whether guards would block a command, without
// recording activity or consuming an allow-once override.
func runGuardCheck(args []string, stdout io.Writer) int {
	emit := func(value any) { _ = json.NewEncoder(stdout).Encode(value) }
	separator := slices.Index(args, "--")
	found := separator >= 0
	flags, command := args, []string(nil)
	if found {
		flags, command = args[:separator], args[separator+1:]
	}
	parser := flag.NewFlagSet("check", flag.ContinueOnError)
	parser.SetOutput(io.Discard)
	root := parser.String("root", "", "Canonical preference root")
	cwd := parser.String("cwd", "", "Directory the command runs in")
	parser.Bool("json", false, "JSON output")
	if !found || len(command) == 0 || parser.Parse(flags) != nil {
		emit(map[string]any{"status": "usage", "error_class": "LearnPreferences::CLIUsageError"})
		return 2
	}
	s, err := Open(Config{Root: *root})
	if err != nil {
		emit(map[string]any{"status": "rejected", "error_class": "LearnPreferences::ValidationError"})
		return 3
	}
	if *cwd == "" {
		*cwd, _ = os.Getwd()
	}
	text := command[0]
	if len(command) > 1 {
		quoted := make([]string, len(command))
		for i, word := range command {
			quoted[i] = shellQuote(word)
		}
		text = strings.Join(quoted, " ")
	}
	result := map[string]any{"status": "success", "blocked": false}
	if runsPreferenceOverride(text) {
		result["blocked"], result["reason"] = true, "only the user can grant preference overrides"
	} else if action, ok := parseGitInvocation(text); ok {
		if verdict := s.evaluateCommand(s.loadGuards(), action, text, *cwd, sessionState{}, "", false); verdict.blocked {
			result["blocked"], result["rule_id"], result["guard_id"], result["reason"] = true, verdict.ruleID, verdict.guardID, verdict.message()
		}
	}
	emit(result)
	return 0
}
