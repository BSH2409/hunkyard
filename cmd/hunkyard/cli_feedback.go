package main

import (
	"context"
	"encoding/json"
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
	"github.com/BSH2409/hunkyard/internal/strictjson"
)

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
	entry, err := daemon.ReadSessionFile(*key)
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
	for {
		requestCtx, cancel := context.WithTimeout(ctx, *timeout+5*time.Second)
		resp, err := daemon.OwnerRequest(requestCtx, entry, *key, method, path, strings.NewReader(body))
		if err != nil {
			cancel()
			return err
		}
		content, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
		resp.Body.Close()
		cancel()
		if err != nil {
			return err
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
