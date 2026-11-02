package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/BSH2409/hunkyard/internal/annotate"
	"github.com/BSH2409/hunkyard/internal/clicmd"
	"github.com/BSH2409/hunkyard/internal/daemon"
)

const annotateUsage = `Usage: hunkyard annotate --format <coverprofile|lcov|sarif|go-test-json> [--tool NAME] [--session ID] [--json] <file|->

Import a local tool's output into the running review: findings become
comments by the tool, uncovered changed lines are tinted. Running it again
for the same tool replaces that tool's earlier findings.`

func runAnnotate(args []string) {
	clicmd.Exit(annotateCommand(args, os.Stdin, os.Stdout))
}

func annotateUsageError(msg string) error {
	return clicmd.ExitError{Code: 2, Err: fmt.Errorf("%s\n\n%s", msg, annotateUsage)}
}

type annotateResult struct {
	Added    int                       `json:"added"`
	Kept     int                       `json:"kept"`
	Removed  int                       `json:"removed"`
	Dropped  int                       `json:"dropped"`
	Coverage map[string]map[string]int `json:"coverage,omitempty"`
}

func annotateCommand(args []string, stdin io.Reader, out io.Writer) error {
	fs := flag.NewFlagSet("annotate", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	format := fs.String("format", "", "")
	tool := fs.String("tool", "", "")
	sessionKey := fs.String("session", "", "")
	jsonOutput := fs.Bool("json", false, "")
	if err := fs.Parse(clicmd.ReorderFlagsFirst(args, map[string]bool{"json": true})); err != nil {
		return annotateUsageError(err.Error())
	}
	if *format == "" || fs.NArg() != 1 {
		return annotateUsageError("annotate needs --format and one input file (- for stdin)")
	}
	if !slices.Contains(annotate.Formats, *format) {
		return annotateUsageError(fmt.Sprintf("unknown format %q", *format))
	}
	entry, err := annotateSession(*sessionKey)
	if err != nil {
		return err
	}
	input := stdin
	if name := fs.Arg(0); name != "-" {
		f, err := os.Open(name)
		if err != nil {
			return err
		}
		defer func() { _ = f.Close() }()
		input = f
	}
	// Paths are resolved against the reviewed repository, not the directory
	// annotate happens to run in (an agent may be in a subdirectory).
	root := repositoryRoot(entry.CWD)
	parsed, err := annotate.Parse(*format, input, *tool, annotate.Options{ModulePath: goModulePath(root), Root: root})
	if err != nil {
		return err
	}
	body, err := json.Marshal(map[string]any{"tool": parsed.Tool, "findings": parsed.Findings, "marks": parsed.Marks})
	if err != nil {
		return err
	}
	client := &http.Client{Timeout: 30 * time.Second}
	if err := waitForReviewReady(client, entry.ConnURL()); err != nil {
		return err
	}
	resp, err := client.Post(entry.ConnURL()+"/api/annotations", "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("sending annotations to the review: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	reply, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("review rejected the annotations: %s %s", resp.Status, strings.TrimSpace(string(reply)))
	}
	if *jsonOutput {
		_, err := fmt.Fprintln(out, strings.TrimSpace(string(reply)))
		return err
	}
	return printAnnotateResult(out, parsed, reply)
}

// printAnnotateResult writes the review's answer as one summary line plus a
// coverage line per file.
func printAnnotateResult(out io.Writer, parsed annotate.Annotations, reply []byte) error {
	var result annotateResult
	if err := json.Unmarshal(reply, &result); err != nil {
		return fmt.Errorf("unexpected review response: %w", err)
	}
	fmt.Fprintf(out, "%s: added=%d kept=%d removed=%d dropped=%d", parsed.Tool, result.Added, result.Kept, result.Removed, result.Dropped)
	if parsed.IgnoredFiles > 0 {
		fmt.Fprintf(out, " ignored=%d", parsed.IgnoredFiles)
	}
	fmt.Fprintln(out)
	paths := make([]string, 0, len(result.Coverage))
	for p := range result.Coverage {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	for _, p := range paths {
		c := result.Coverage[p]
		fmt.Fprintf(out, "  %s: %d of %d changed lines uncovered\n", p, c["uncovered"], c["changed"])
	}
	return nil
}

// readinessPoll is how often annotate re-checks a daemon that is still
// loading (a variable so tests need not wait).
var readinessPoll = 200 * time.Millisecond

// waitForReviewReady polls /api/session until the daemon has finished
// loading the review (it answers 503 until then), for up to 30 seconds.
func waitForReviewReady(client *http.Client, base string) error {
	deadline := time.Now().Add(30 * time.Second)
	for {
		resp, err := client.Get(base + "/api/session")
		if err == nil {
			_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
			_ = resp.Body.Close()
			if resp.StatusCode != http.StatusServiceUnavailable {
				return nil
			}
		}
		if time.Now().After(deadline) {
			return errors.New("the review is still loading; try again in a moment")
		}
		time.Sleep(readinessPoll)
	}
}

// annotateSession finds the review daemon: the named session, or the only
// review running in this directory.
func annotateSession(key string) (daemon.SessionEntry, error) {
	if key != "" {
		entry, err := daemon.ReadSessionFile(key)
		if err != nil {
			return daemon.SessionEntry{}, fmt.Errorf("no active review session %s", key)
		}
		return entry, nil
	}
	cwd, err := os.Getwd()
	if err != nil {
		return daemon.SessionEntry{}, err
	}
	entries, keys, err := daemon.ListSessionsForCWDWithKeys(cwd)
	if err != nil {
		return daemon.SessionEntry{}, err
	}
	switch len(entries) {
	case 0:
		return daemon.SessionEntry{}, errors.New("no active review session in this directory; start one with hunkyard, or pass --session ID")
	case 1:
		return entries[0], nil
	default:
		return daemon.SessionEntry{}, fmt.Errorf("several reviews run in this directory (%s); pass --session ID", strings.Join(keys, ", "))
	}
}

// repositoryRoot is the git top level of dir (the review's directory, or the
// current directory when the session does not record one).
func repositoryRoot(dir string) string {
	if dir == "" {
		dir, _ = os.Getwd()
	}
	if out, err := exec.Command("git", "-C", dir, "rev-parse", "--show-toplevel").Output(); err == nil {
		return strings.TrimSpace(string(out))
	}
	return dir
}

// goModulePath reads the module path from root/go.mod, if there is one.
func goModulePath(root string) string {
	data, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		if rest, ok := strings.CutPrefix(strings.TrimSpace(line), "module "); ok {
			return strings.Trim(strings.TrimSpace(rest), `"`)
		}
	}
	return ""
}
