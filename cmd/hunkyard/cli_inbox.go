package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/BSH2409/hunkyard/internal/clicmd"
	"github.com/BSH2409/hunkyard/internal/session"
)

const inboxUsage = `Usage: hunkyard inbox [--json]

List every review on this machine: running reviews (waiting for you, or the
agent is working) and paused ones, with round, open comments and the URL or
the command that resumes it. Every review page also serves this list at /inbox.`

func runInbox(args []string) {
	clicmd.Exit(inboxCommand(context.Background(), args, os.Stdout))
}

var inboxStateText = map[string]string{
	"waiting_for_reviewer": "waiting for you",
	"agent_working":        "agent working",
	"paused":               "paused",
	"unreachable":          "not answering",
}

func inboxCommand(ctx context.Context, args []string, out io.Writer) error {
	jsonOutput := false
	for _, a := range args {
		if a != "--json" {
			return clicmd.ExitError{Code: 2, Err: fmt.Errorf("unknown argument %q\n\n%s", a, inboxUsage)}
		}
		jsonOutput = true
	}
	entries, err := session.ListInbox(ctx)
	if err != nil {
		return err
	}
	if jsonOutput {
		if entries == nil {
			entries = []session.InboxEntry{}
		}
		encoded, _ := json.Marshal(entries)
		_, err := fmt.Fprintln(out, string(encoded))
		return err
	}
	if len(entries) == 0 {
		_, err := fmt.Fprintln(out, "No reviews are running or paused.")
		return err
	}
	for _, e := range entries {
		where := e.URL
		if e.State == "paused" {
			where = "hunkyard --session " + e.Key
		}
		fmt.Fprintf(out, "%-16s %s  %s · round %d · %d open\n                 %s\n", inboxStateText[e.State], e.Label, e.Repository, e.Round, e.Unresolved, where)
	}
	return nil
}
