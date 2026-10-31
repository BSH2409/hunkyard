package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/BSH2409/hunkyard/internal/config"
	"github.com/BSH2409/hunkyard/internal/localonly"
	"github.com/BSH2409/hunkyard/internal/preferences"
)

type commandDescriptor struct {
	name     string
	handler  func([]string)
	help     string
	helpFn   func()
	hidden   bool
	bareHelp bool
}

// commandRegistry is the single source of truth for command dispatch, help,
// ordering, and public visibility.
var commandRegistry = []commandDescriptor{
	{name: "preferences", handler: func(args []string) { os.Exit(preferences.Run(args, os.Stdin, os.Stdout, os.Stderr)) }, help: "Usage: hunkyard preferences <capture|search|select|build|audit|retire|maintain|migrate|install-hook|install-cron|status|hook|guard|allow|doctor|statusline> [options]\n\nLocal Go preference engine. Maintenance accepts --proposal FILE, or --auto to use the Claude or Codex CLI set in preferences_maintenance (global config)."},
	{name: "feedback", handler: runFeedback, help: "Usage: hunkyard feedback <await|learn|ack> --session ID [--timeout 45s] [--follow] [--batch ID] [--decisions FILE] [--json]\n\nReceive or acknowledge locally published feedback in the launching CLI session.\nFor await, --follow silently renews pending waits until feedback is published; --timeout bounds each request."},
	{name: "install", handler: runInstall, helpFn: printInstallUsage},
	{name: "config", handler: runConfig, bareHelp: true, helpFn: config.PrintConfigHelp},
	{name: "yard", handler: runYard, help: "Usage: hunkyard yard [--port N] [--host 127.0.0.1] [--no-open]\n\nOpen the Preference Yard: your learned rules, persona, activity and preference health, without a review session."},
	{name: "check", handler: func([]string) { runCheck() }, help: `Usage: hunkyard check

Check installed integrations for missing or stale configuration.`},
	{name: "pr", handler: runPR, help: `Usage: hunkyard pr <num|url>

Open a GitHub pull request for review.`},
	{name: "pull", handler: runPull, help: `Usage: hunkyard pull [--session <id>] [--output <dir>] [number|url]

Fetch PR comments into the local review file.`},
	{name: "push", handler: runPush, help: `Usage: hunkyard push [options] [number|url]

Post local comments as a PR review.

Options:
      --session <id>     Target an active review session
      --dry-run          Preview without posting
  -e, --event <type>     comment, approve, or request-changes
  -m, --message <text>   Review-level message
  -o, --output <dir>     Hunkyard data root for reviews`},
	{name: "comment", handler: runComment, help: `Usage: hunkyard comment [options] <body>
       hunkyard comment [options] <path> <body>
       hunkyard comment [options] <path>:<line[-end]> <body>
       hunkyard comment [options] --selector <css> [--route <path>] <body>
       hunkyard comment [options] --reply-to <id> [--resolve] <body>
       hunkyard comment [options] --json [--file <path>]
       hunkyard comment [options] --clear

Add, reply to, bulk import, or clear review comments.
Live reviews and HTML preview use --selector to pin a DOM element.

Options:
  -o, --output <dir>     Hunkyard data root for reviews
      --author <name>    Comment author
      --plan <name>      Target a stored plan review
      --session <id>     Target an active review session (all comment modes)
      --reply-to <id>    Reply to an existing comment
      --resolve          Resolve the parent after replying
      --path <path>      File path for a reply
      --selector <css>   CSS selector for a live or preview pin
      --route <path>     Page path for that pin
      --json             Read bulk comments as JSON
  -f, --file <path>      Read JSON from a file
      --scope <mode>     Override comment focus scope`},
	{name: "comments", handler: runComments, help: `Usage: hunkyard comments [--session <id>] [--json] [--all] [review]

List unresolved comments, with review-level comments first.`},
	{name: "review", handler: runReview, help: `Usage: hunkyard review [options] [file|dir...]

Open an inline review for git changes, a commit range, a PR, or files.

Options:
      --pr <num|url>          Review a GitHub pull request
      --range <base>..<head>  Review a commit range
      --base-branch <branch>  Override the diff base
      --no-open               Do not open a browser
  -o, --output <dir>          Hunkyard data root for reviews`},
	{name: "live", handler: runLive, help: `Usage: hunkyard live [options] <url>

Review a running web application in live mode.

Options:
  -p, --port <port>        Port to listen on
      --host <host>        Host to listen on
      --public-url <url>   Advertised base URL
      --allow-unauthenticated-network
                           Allow non-loopback --host or --public-url
      --cookie <value>     Forward a Cookie header
      --cookie-file <path> Read cookies from a file
      --cdp-url <url>      Reuse cookies from Chrome DevTools
      --no-open            Do not open a browser
  -q, --quiet              On success, suppress connect/start status, tips, and session summary`},
	{name: "preview", handler: runPreview, help: `Usage: hunkyard preview [options] <file.html>

Review a local HTML file in preview mode.

Options:
  -p, --port <port>       Port to listen on
      --host <host>       Host to listen on
      --public-url <url>  Advertised base URL
      --allow-unauthenticated-network
                          Allow non-loopback --host or --public-url
      --no-open           Do not open a browser
  -q, --quiet             On success, suppress connect/start status, tips, and session summary`},
	{name: "plan", handler: runPlan, help: `Usage: hunkyard plan [--name <slug>] [options] <file>
       echo "content" | hunkyard plan [--name <slug>] [options]

Create or continue a plan-file review. If --name is omitted, hunkyard derives it
from the plan content.

With --no-wait, hunkyard saves the plan as its next version and exits without
starting a review (no version is added when the content is unchanged). It
prints the slug, for use with hunkyard comment --plan <slug>.

Options:
      --host <host>       Host to listen on
      --public-url <url>  Advertised base URL
      --allow-unauthenticated-network
                          Allow non-loopback --host or --public-url
  -p, --port <port>       Port to listen on
      --no-open           Do not open a browser
      --no-wait           Save the version and exit; do not start a review
  -q, --quiet             On success, suppress connect/start status, tips, and session summary`},
	{name: "story", handler: runStory, helpFn: printStoryUsage, bareHelp: true},
	{name: "stop", handler: runStop, help: `Usage: hunkyard stop [--session ID | --all] [file...]

Stop the review daemon for the current session. Specify files to target an
exact file-mode session, or use --all to stop every daemon.`},
	{name: "status", handler: runStatus, help: `Usage: hunkyard status [--json]

Show active session IDs and review paths, daemon status, and comment counts.`},
	{name: "resume", handler: runResume, help: `Usage: hunkyard resume [--list | <id>] [review options]

Pick a stored review to reopen. Without arguments this shows an interactive
list of every review in ~/.hunkyard/reviews, newest first, and reconnects to the
one you choose — restarting its daemon in the directory it came from when that
daemon has stopped. Pass a session ID to skip the picker.

Options:
  -l, --list               Print the list instead of opening the picker

Any other option is passed through to the review, so flags like --no-open work.
Reviews stored outside ~/.hunkyard/reviews are not listed: reopen a plan with
hunkyard plan --name <slug>, and an --output review with hunkyard --session <id>.`},
	{name: "stats", handler: runStats, help: `Usage: hunkyard stats [--json]

Show lifetime review statistics.`},
	{name: "cleanup", handler: runCleanup, help: `Usage: hunkyard cleanup [--days N] [--force]

Delete stale review files. The default age is seven days.`},
	{name: "plan-hook", handler: runPlanHookCommand, help: `Usage: hunkyard plan-hook [--mode claude|codex]

Run the internal plan hook.`, hidden: true},
	{name: "_serve", handler: runServe, help: `Usage: hunkyard _serve [options]

Run the internal foreground review server.`, hidden: true},
}

func dispatchCLI(args []string) (bool, error) {
	return dispatchWithRegistry(args, commandRegistry)
}

func dispatchWithRegistry(args []string, registry []commandDescriptor) (bool, error) {
	if len(args) == 0 {
		return false, nil
	}
	switch args[0] {
	case "--help", "-h":
		printHelp()
		return true, nil
	case "--version", "-v":
		printVersion()
		return true, nil
	case "help":
		return true, printRequestedHelp(args[1:], registry)
	}

	command, ok := findCommand(registry, args[0])
	if !ok {
		return false, nil
	}
	commandArgs := args[1:]
	if len(commandArgs) > 0 && (isHelpFlag(commandArgs[0]) || command.bareHelp && commandArgs[0] == "help") {
		printCommandHelp(command)
		return true, nil
	}
	command.handler(commandArgs)
	return true, nil
}

func printRequestedHelp(args []string, registry []commandDescriptor) error {
	if len(args) == 0 {
		printHelp()
		return nil
	}
	command, ok := findCommand(registry, args[0])
	if !ok {
		return fmt.Errorf("unknown help topic %q", strings.Join(args, " "))
	}
	if len(args) > 1 {
		return fmt.Errorf("unknown help topic %q", strings.Join(args, " "))
	}
	printCommandHelp(command)
	return nil
}

func findCommand(registry []commandDescriptor, name string) (commandDescriptor, bool) {
	for _, command := range registry {
		if command.name == name {
			return command, true
		}
	}
	return commandDescriptor{}, false
}

func isHelpFlag(arg string) bool {
	return arg == "--help" || arg == "-h"
}

func printCommandHelp(command commandDescriptor) {
	if command.helpFn != nil {
		command.helpFn()
		return
	}
	fmt.Fprintln(os.Stderr, command.help)
}

func printHelp() {
	fmt.Fprintf(os.Stderr, `hunkyard — local browser diff review for Codex and Claude CLI

Review:
  hunkyard                            Review working changes in the current repository
  hunkyard --range START..END          Compare exactly two commits
  hunkyard --range START...END         Compare END against the merge base with START
  hunkyard FILE [...]                 Review local files
  hunkyard --session ID               Reconnect or start the next requested review round
  hunkyard plan --name NAME FILE      Review a local plan
  hunkyard story --story-file FILE    Review a host-authored story

CLI feedback:
  hunkyard feedback await --session ID --follow --json
  hunkyard feedback learn --session ID --batch ID --decisions FILE --json
  hunkyard feedback ack --session ID --batch ID --json
  hunkyard comment --session ID --reply-to ID --author NAME 'Reply'
  hunkyard comments --session ID --json

Setup and management:
  hunkyard install --global codex      Install the Codex review skills
  hunkyard install --global claude     Install the Claude review skills
  hunkyard install --global preferences  Install the Go-backed learn-preferences skill
  hunkyard preferences COMMAND        Capture, select, audit, maintain, or migrate preferences
  hunkyard status --json              List local review sessions
  hunkyard stop --session ID          Stop an owned review daemon
  hunkyard cleanup --days N           Clean eligible old review files
  hunkyard config                    Inspect local configuration
  hunkyard help COMMAND              Show command options

Options:
  -p, --port PORT          Listen port (default: 5000; 0 chooses a free port)
      --host HOST         Listen address (default: 0.0.0.0; loopback: 127.0.0.1)
      --allow-unauthenticated-network
                          Allow wildcard binding (enabled by default)
      --range RANGE       Exact START..END or merge-base START...END comparison
      --skills-file FILE  Trusted skill catalog supplied by the launching host
      --session ID        Existing review session
      --base-branch NAME  Local base branch
  -o, --output DIR        Review data root (default: ~/.hunkyard)
      --no-open           Print the local URL without opening a browser
  -q, --quiet             Suppress connection/status tips
  -v, --version           Print version

Environment:
  HUNKYARD_PORT           Default port override
  HUNKYARD_HOST           Listen address override
  HUNKYARD_ALLOW_UNAUTHENTICATED_NETWORK=1
                          Allow wildcard binding (default: 1; 0 disables it)
  HUNKYARD_OWNER_ID       Stable launching host identity, such as codex:SESSION or claude:SESSION
  CODEX_HOME             Canonical preference home (default: ~/.codex)

All assets, review data, and preference operations remain local. The launching
Codex or Claude session owns any model-provider communication.

Commands: %s
Agents: %s
Source: https://github.com/BSH2409/hunkyard
`, strings.Join(visibleCommandNames(), ", "), strings.Join(availableIntegrations(), ", "))
}

func visibleCommandNames() []string {
	var names []string
	for _, command := range commandRegistry {
		if !command.hidden && (!localonly.Enabled() || localonly.ValidateArgs([]string{command.name, "--help"}) == nil) {
			names = append(names, command.name)
		}
	}
	return names
}

func runPlanHookCommand(args []string) {
	mode := "claude"
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--mode":
			if i+1 >= len(args) {
				fmt.Fprintln(os.Stderr, "Error: --mode requires a value")
				os.Exit(1)
			}
			i++
			mode = args[i]
		case strings.HasPrefix(arg, "--mode="):
			mode = strings.TrimPrefix(arg, "--mode=")
		default:
			fmt.Fprintf(os.Stderr, "Unknown plan-hook flag: %s\n", arg)
			os.Exit(1)
		}
	}

	switch mode {
	case "claude", "":
		runPlanHook()
	case "codex":
		runCodexPlanHook()
	default:
		fmt.Fprintf(os.Stderr, "Unknown plan-hook mode: %s\n", mode)
		os.Exit(1)
	}
}

func printVersion() {
	line := "hunkyard " + version
	var details []string
	if date != "unknown" {
		details = append(details, date)
	}
	if commit != "unknown" {
		short := commit
		if len(short) > 7 {
			short = short[:7]
		}
		details = append(details, short)
	}
	if len(details) > 0 {
		line += " (" + strings.Join(details, ", ") + ")"
	}
	fmt.Println(line)
	fmt.Println("Inline code review for AI agent workflows")
}
