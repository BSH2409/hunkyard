package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"

	"github.com/BSH2409/hunkyard/internal/clicmd"
	"github.com/BSH2409/hunkyard/internal/daemon"
	"github.com/BSH2409/hunkyard/internal/feedback"
)

const askUsage = `Usage: hunkyard ask --session ID --option TEXT --option TEXT [--allow-other] [--blocking] [--context PATH:LINE] [--json] "question"

Ask the reviewer to decide something in the running review. Questions appear
one at a time in the review's bottom-right panel: the reviewer picks an
option or writes an answer, then submits all answers together. They arrive
in the next feedback batch as "decisions" ("chosen" and/or "text").
--allow-other allows fewer than two options (a question answered in words);
--blocking keeps Approve disabled until it is answered.`

// repeatedFlag collects every value of a flag given several times.
type repeatedFlag []string

func (f *repeatedFlag) String() string     { return strings.Join(*f, ", ") }
func (f *repeatedFlag) Set(v string) error { *f = append(*f, v); return nil }

func runAsk(args []string) {
	ctx, stop := signal.NotifyContext(context.Background(), daemon.ShutdownSignals()...)
	defer stop()
	clicmd.Exit(askCommand(ctx, args, os.Stdout))
}

func askUsageError(msg string) error {
	return clicmd.ExitError{Code: 2, Err: fmt.Errorf("%s\n\n%s", msg, askUsage)}
}

func askCommand(ctx context.Context, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("ask", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	key := fs.String("session", "", "")
	var options repeatedFlag
	fs.Var(&options, "option", "")
	allowOther := fs.Bool("allow-other", false, "")
	blocking := fs.Bool("blocking", false, "")
	contextRef := fs.String("context", "", "")
	replyTo := fs.String("reply-to", "", "")
	jsonOutput := fs.Bool("json", false, "")
	if err := fs.Parse(clicmd.ReorderFlagsFirst(args, map[string]bool{"allow-other": true, "blocking": true, "json": true})); err != nil {
		return askUsageError(err.Error())
	}
	if fs.NArg() != 1 || strings.TrimSpace(fs.Arg(0)) == "" {
		return askUsageError("ask needs exactly one question")
	}
	if !daemon.ValidSessionKey(*key) {
		return askUsageError("provide a valid --session ID")
	}
	request := feedback.DecisionRequest{Question: fs.Arg(0), Options: options, AllowOther: *allowOther, Blocking: *blocking, ParentID: *replyTo}
	if *contextRef != "" {
		path, line, ok := strings.Cut(*contextRef, ":")
		n, err := strconv.Atoi(line)
		if !ok || err != nil || n < 1 {
			return askUsageError("--context takes PATH:LINE")
		}
		request.Context = &feedback.DecisionContext{Path: path, Line: n}
	}
	if err := feedback.ValidateDecisionRequest(request); err != nil {
		return askUsageError(err.Error())
	}
	asked, err := sendDecisionRequest(ctx, *key, request)
	if err != nil {
		return err
	}
	if *jsonOutput {
		encoded, _ := json.Marshal(map[string]any{"decision": asked, "status": "asked"})
		_, err := fmt.Fprintln(out, string(encoded))
		return err
	}
	_, err = fmt.Fprintf(out, "Asked %s. The answer arrives with the next published review.\n", asked.ID)
	return err
}

// sendDecisionRequest posts the request to the review's daemon as its owner.
func sendDecisionRequest(ctx context.Context, key string, request feedback.DecisionRequest) (feedback.DecisionRequest, error) {
	var asked feedback.DecisionRequest
	entry, err := resumedSession(ctx, key, false)
	if errors.Is(err, errReviewPaused) {
		return asked, fmt.Errorf("review %s is paused; resume it with: hunkyard --session %s", key, key)
	}
	if err != nil {
		return asked, err
	}
	body, _ := json.Marshal(request)
	resp, err := daemon.OwnerRequest(ctx, entry, key, http.MethodPost, "/api/feedback/ask", bytes.NewReader(body))
	if err != nil {
		return asked, err
	}
	defer func() { _ = resp.Body.Close() }()
	reply, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusCreated {
		return asked, fmt.Errorf("review rejected the question: %s %s", resp.Status, strings.TrimSpace(string(reply)))
	}
	if err := json.Unmarshal(reply, &asked); err != nil {
		return asked, fmt.Errorf("unexpected review response: %w", err)
	}
	return asked, nil
}
