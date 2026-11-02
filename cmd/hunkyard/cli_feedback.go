package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/BSH2409/hunkyard/internal/clicmd"
	"github.com/BSH2409/hunkyard/internal/daemon"
	"github.com/BSH2409/hunkyard/internal/feedback"
	"github.com/BSH2409/hunkyard/internal/session"
	"github.com/BSH2409/hunkyard/internal/strictjson"
)

// errReviewPaused means the review's daemon paused after idle_pause_minutes;
// `hunkyard --session ID` resumes it.
var errReviewPaused = errors.New("review is paused")

// lookupSession and pausedPollInterval are variables so tests can stand in
// for a daemon that pauses and comes back. The lookup only reads the session
// file: a health probe would delete the entry of a daemon too busy to answer.
// A crashed daemon's leftover entry fails at the request, and a follow
// waiter retries only when a different daemon run is registered.
var (
	lookupSession = func(key string) (daemon.SessionEntry, bool) {
		entry, err := daemon.ReadSessionFile(key)
		return entry, err == nil
	}
	pausedPollInterval = 5 * time.Second
	readyPollInterval  = 500 * time.Millisecond
	// resumeInitWaits bounds how long a follow waiter lets a resumed daemon
	// answer 503 while it initializes (about 30s at readyPollInterval).
	resumeInitWaits = 60
)

// resumedSession finds the review's live daemon. A paused review (its marker
// is in the review folder) is not an error: a follow waiter keeps polling
// until the review resumes; a one-shot caller gets errReviewPaused. A review
// that is neither live nor paused stopped for good.
func resumedSession(ctx context.Context, key string, follow bool) (daemon.SessionEntry, error) {
	for {
		if entry, alive := lookupSession(key); alive {
			return entry, nil
		}
		if _, paused := session.PausedReviewDir(key); !paused {
			return daemon.SessionEntry{}, fmt.Errorf("review session %s is not running", key)
		}
		if !follow {
			return daemon.SessionEntry{}, errReviewPaused
		}
		select {
		case <-ctx.Done():
			return daemon.SessionEntry{}, ctx.Err()
		case <-time.After(pausedPollInterval):
		}
	}
}

// sameDaemon reports whether two session entries name the same daemon run.
func sameDaemon(a, b daemon.SessionEntry) bool {
	return a.PID == b.PID && a.Port == b.Port && a.StartedAt == b.StartedAt
}

// reportPaused tells a one-shot await caller the review is paused.
func reportPaused(out io.Writer, key string, jsonOutput bool) error {
	command := "hunkyard --session " + key
	if jsonOutput {
		encoded, _ := json.Marshal(map[string]string{"status": "paused", "review_id": key, "command": command})
		_, err := fmt.Fprintln(out, string(encoded))
		return err
	}
	_, err := fmt.Fprintf(out, "Review %s is paused. Resume it with: %s\n", key, command)
	return err
}

func runFeedback(args []string) {
	ctx, stop := signal.NotifyContext(context.Background(), daemon.ShutdownSignals()...)
	defer stop()
	clicmd.Exit(feedbackCommand(ctx, args, os.Stdout))
}

//nolint:gocyclo // The flat bridge dispatcher validates owner, batch, timeout, and decision flags together.
func feedbackCommand(ctx context.Context, args []string, out io.Writer) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: hunkyard feedback <await|learn|ack> --session ID [--timeout 45s] [--follow] [--batch ID] [--json]")
	}
	action := args[0]
	flags := flag.NewFlagSet("feedback "+action, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	key := flags.String("session", "", "review session ID")
	timeout := flags.Duration("timeout", 45*time.Second, "maximum wait per request")
	follow := flags.Bool("follow", false, "keep waiting silently until feedback is published (await only)")
	decisionsFile := flags.String("decisions", "", "Local JSON classification file")
	batch := flags.String("batch", "", "publication batch ID")
	jsonOutput := flags.Bool("json", false, "structured output")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	if flags.NArg() != 0 || !daemon.ValidSessionKey(*key) {
		return fmt.Errorf("provide a valid --session ID")
	}
	if *timeout <= 0 || *timeout > 45*time.Second {
		return fmt.Errorf("timeout must be greater than zero and at most 45s")
	}
	if *follow && action != "await" {
		return fmt.Errorf("--follow is only supported for await")
	}
	entry, err := resumedSession(ctx, *key, *follow)
	if errors.Is(err, errReviewPaused) {
		if action == "await" {
			return reportPaused(out, *key, *jsonOutput)
		}
		return fmt.Errorf("review %s is paused; resume it with: hunkyard --session %s", *key, *key)
	}
	if err != nil {
		return fmt.Errorf("cannot read review session: %w", err)
	}
	method, path, body := http.MethodGet, "/api/feedback/await?timeout="+url.QueryEscape(timeout.String()), ""
	switch action {
	case "await":
	case "learn":
		if *batch == "" || *decisionsFile == "" {
			return fmt.Errorf("provide --batch ID and --decisions FILE")
		}
		info, err := os.Lstat(*decisionsFile)
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() || info.Size() > 2<<20 {
			return fmt.Errorf("invalid preference decision file")
		}
		data, err := os.ReadFile(*decisionsFile)
		if err != nil {
			return err
		}
		var decisions []feedback.PreferenceDecision
		if strictjson.Decode(data, &decisions) != nil {
			return fmt.Errorf("invalid preference decision JSON")
		}
		encoded, err := json.Marshal(map[string]any{"batch_id": *batch, "decisions": decisions})
		if err != nil {
			return err
		}
		method, path, body = http.MethodPost, "/api/feedback/learn", string(encoded)
	case "ack":
		if *batch == "" {
			return fmt.Errorf("provide --batch ID")
		}
		method, path = http.MethodPost, "/api/feedback/ack"
		encoded, _ := json.Marshal(map[string]string{"batch_id": *batch})
		body = string(encoded)
	default:
		return fmt.Errorf("unknown feedback action %q", action)
	}
	initWaitsLeft := 0
	for {
		requestCtx, cancel := context.WithTimeout(ctx, *timeout+5*time.Second)
		resp, err := daemon.OwnerRequest(requestCtx, entry, *key, method, path, strings.NewReader(body))
		if err != nil {
			cancel()
			// A follow waiter rides through a pause: wait for the resume and
			// reconnect to the new daemon. The same daemon failing again is a
			// real error, not a pause.
			if *follow && ctx.Err() == nil {
				if next, resumeErr := resumedSession(ctx, *key, true); resumeErr == nil && !sameDaemon(next, entry) {
					entry = next
					initWaitsLeft = resumeInitWaits
					continue
				}
			}
			return err
		}
		content, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
		resp.Body.Close()
		cancel()
		if err != nil {
			return err
		}
		// A daemon that just resumed answers 503 until its session is ready;
		// a follow waiter gives it a bounded while rather than giving up. A
		// 503 from the daemon it was already talking to is a real error.
		if resp.StatusCode == http.StatusServiceUnavailable && *follow && initWaitsLeft > 0 {
			initWaitsLeft--
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(readyPollInterval):
			}
			continue
		}
		if resp.StatusCode != 200 && *jsonOutput {
			fmt.Fprintln(out, string(content))
			return fmt.Errorf("feedback request failed (HTTP %d)", resp.StatusCode)
		}
		if resp.StatusCode != 200 {
			return fmt.Errorf("feedback request failed (HTTP %d): %s", resp.StatusCode, strings.TrimSpace(string(content)))
		}
		var result struct {
			Status string `json:"status"`
		}
		if *follow || !*jsonOutput {
			if err := json.Unmarshal(content, &result); err != nil {
				return fmt.Errorf("invalid feedback response")
			}
		}
		if *follow {
			switch result.Status {
			case "pending":
				continue
			case "published":
			default:
				return fmt.Errorf("invalid feedback response status %q", result.Status)
			}
		}
		if result.Status == "pending" && !*jsonOutput {
			_, err = fmt.Fprintf(out, "Review pending. Reconnect with: hunkyard feedback await --session %s --json\n", *key)
		} else {
			_, err = fmt.Fprintln(out, string(content))
		}
		return err
	}
}
