package main

import (
	"bytes"
	"context"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/BSH2409/hunkyard/internal/daemon"
	"github.com/BSH2409/hunkyard/internal/session"
)

const pausedKey = "abcdef012345"

func pausedReview(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	dir, err := daemon.ReviewFilePath(pausedKey)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := session.WritePausedMarker(dir, session.PausedMarker{PausedAt: time.Now(), Port: 4321}); err != nil {
		t.Fatal(err)
	}
}

func fakeLiveness(t *testing.T, alive *atomic.Bool) {
	t.Helper()
	previous, previousInterval := lookupSession, pausedPollInterval
	lookupSession = func(key string) (daemon.SessionEntry, bool) {
		if alive.Load() {
			return daemon.SessionEntry{Port: 4321, Host: "127.0.0.1"}, true
		}
		return daemon.SessionEntry{}, false
	}
	pausedPollInterval = 10 * time.Millisecond
	t.Cleanup(func() { lookupSession, pausedPollInterval = previous, previousInterval })
}

func TestFollowWaiterWaitsThroughPause(t *testing.T) {
	pausedReview(t)
	var alive atomic.Bool
	fakeLiveness(t, &alive)
	go func() { time.Sleep(60 * time.Millisecond); alive.Store(true) }()
	entry, err := resumedSession(context.Background(), pausedKey, true)
	if err != nil || entry.Port != 4321 {
		t.Fatalf("waiter did not reconnect after the resume: %+v %v", entry, err)
	}
}

func TestWaiterErrorsWithoutPausedMarker(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	var alive atomic.Bool
	fakeLiveness(t, &alive)
	if _, err := resumedSession(context.Background(), pausedKey, true); err == nil {
		t.Fatal("a stopped review without a paused marker should be an error")
	}
}

func TestOneShotAwaitReportsPaused(t *testing.T) {
	pausedReview(t)
	var alive atomic.Bool
	fakeLiveness(t, &alive)
	var out bytes.Buffer
	if err := feedbackCommand(context.Background(), []string{"await", "--session", pausedKey, "--json"}, &out); err != nil {
		t.Fatalf("await on a paused review: %v", err)
	}
	if !strings.Contains(out.String(), `"status":"paused"`) || !strings.Contains(out.String(), "hunkyard --session "+pausedKey) {
		t.Fatalf("output = %s", out.String())
	}
}

func TestFollowWaiterStopsWhenTheSameDaemonKeepsFailing(t *testing.T) {
	key := feedbackCLIServer(t, func(w http.ResponseWriter, r *http.Request) {})
	entry, _ := daemon.ReadSessionFile(key)
	entry.Port = 1 // nothing listens: every request fails at the transport
	if err := daemon.WriteSessionFile(key, entry); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	var output bytes.Buffer
	err := feedbackCommand(ctx, []string{"await", "--session", key, "--timeout", "1ms", "--follow", "--json"}, &output)
	if err == nil || ctx.Err() != nil {
		t.Fatalf("follow error = %v (ctx %v), want the request error without retrying the same daemon", err, ctx.Err())
	}
}

func TestOneShotFeedbackKeepsASlowDaemonsSessionFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("HUNKYARD_OWNER_ID", "claude:synthetic")
	// A live process whose health check does not answer (busy or slow).
	if err := daemon.WriteSessionFile(pausedKey, daemon.SessionEntry{PID: os.Getpid(), Port: 1, Host: "127.0.0.1"}); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := feedbackCommand(context.Background(), []string{"ack", "--session", pausedKey, "--batch", "b1", "--json"}, &output); err == nil {
		t.Fatal("ack against an unreachable daemon succeeded")
	}
	if _, err := daemon.ReadSessionFile(pausedKey); err != nil {
		t.Fatalf("a one-shot command removed the daemon's session file: %v", err)
	}
}
