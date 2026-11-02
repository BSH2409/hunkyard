package main

import (
	"bytes"
	"context"
	"errors"
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
	dir, err := daemon.ReviewDir(pausedKey)
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
	entry, err := liveDaemonOrWaitForResume(context.Background(), pausedKey, true)
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
	_, err := liveDaemonOrWaitForResume(context.Background(), pausedKey, true)
	if err == nil || !strings.Contains(err.Error(), "is not running") {
		t.Fatalf("err = %v, want the review reported as not running", err)
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
	if err == nil || ctx.Err() != nil || errors.Is(err, context.DeadlineExceeded) || !strings.Contains(err.Error(), "127.0.0.1:1") {
		t.Fatalf("follow error = %v (ctx %v), want the transport error from the same daemon, without retrying", err, ctx.Err())
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

// After a pause, the waiter reconnects to the resumed daemon, which answers
// 503 until its session is ready; the waiter keeps waiting for it.
func TestFollowWaiterWaitsForAResumedDaemonToBeReady(t *testing.T) {
	requests := 0
	key := feedbackCLIServer(t, func(w http.ResponseWriter, r *http.Request) {
		requests++
		if requests <= 2 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.Write([]byte(`{"status":"published","batches":[]}`))
	})
	previous, previousReady := lookupSession, readyPollInterval
	readyPollInterval = time.Millisecond
	calls := 0
	lookupSession = func(k string) (daemon.SessionEntry, bool) {
		calls++
		if calls == 1 {
			return daemon.SessionEntry{Host: "127.0.0.1", Port: 1, StartedAt: "paused-run"}, true
		}
		entry, err := daemon.ReadSessionFile(k)
		return entry, err == nil
	}
	t.Cleanup(func() { lookupSession, readyPollInterval = previous, previousReady })
	var out bytes.Buffer
	if err := feedbackCommand(context.Background(), []string{"await", "--session", key, "--timeout", "1ms", "--follow", "--json"}, &out); err != nil {
		t.Fatalf("follow gave up on the resumed daemon: %v", err)
	}
	if !strings.Contains(out.String(), `"published"`) {
		t.Fatalf("output = %s", out.String())
	}
}
