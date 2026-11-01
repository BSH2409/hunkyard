package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/BSH2409/hunkyard/internal/session"
)

func pausableServer(t *testing.T) (*Server, *Session, *atomic.Int32, chan session.SSEEvent) {
	t.Helper()
	s, sess := newTestServer(t)
	s.reviewPath = t.TempDir()
	stops := &atomic.Int32{}
	s.SetStopFunc(func() { stops.Add(1) })
	sess.MarkActivity("start")
	events := sess.Subscribe()
	t.Cleanup(func() { sess.Unsubscribe(events) })
	return s, sess, stops, events
}

func TestIdleMonitorPausesAfterIdle(t *testing.T) {
	s, _, stops, events := pausableServer(t)
	later := time.Now().Add(31 * time.Minute)
	if !s.pauseIfIdle(30*time.Minute, later) {
		t.Fatal("idle review did not pause")
	}
	if stops.Load() != 1 {
		t.Fatal("daemon was not stopped")
	}
	marker, ok := session.ReadPausedMarker(s.reviewPath)
	if !ok || marker.Port != s.port {
		t.Fatalf("paused marker = %+v, %v", marker, ok)
	}
	select {
	case ev := <-events:
		if ev.Type != "paused" {
			t.Fatalf("first event = %q, want paused", ev.Type)
		}
	case <-time.After(time.Second):
		t.Fatal("tabs were not told about the pause")
	}
}

func TestIdleMonitorWaitsForActivityAndInFlightRequests(t *testing.T) {
	s, _, stops, _ := pausableServer(t)
	if s.pauseIfIdle(30*time.Minute, time.Now().Add(10*time.Minute)) {
		t.Fatal("paused before the idle time passed")
	}
	s.inFlight.Add(1)
	if s.pauseIfIdle(30*time.Minute, time.Now().Add(31*time.Minute)) {
		t.Fatal("paused during an in-flight request")
	}
	s.inFlight.Add(-1)
	if stops.Load() != 0 {
		t.Fatal("daemon stopped")
	}
}

func TestIdleMonitorSkipsDuringRoundTransition(t *testing.T) {
	s, sess, _, _ := pausableServer(t)
	sess.SetRoundBusyForTest(true)
	if s.pauseIfIdle(30*time.Minute, time.Now().Add(31*time.Minute)) {
		t.Fatal("paused during a round transition")
	}
}

func TestIdleDisabledWhenZero(t *testing.T) {
	s, _, _, _ := pausableServer(t)
	if s.pauseIfIdle(0, time.Now().Add(24*time.Hour)) {
		t.Fatal("paused with pausing disabled")
	}
}

func TestIdleMonitorWaitsForAgentTurn(t *testing.T) {
	s, sess, _, _ := pausableServer(t)
	path := filepath.Join(t.TempDir(), "activity")
	sess.SetActivityFile(path)
	if err := os.WriteFile(path, []byte("turn\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-40 * time.Minute)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}
	// 40 minutes past the last activity, but the agent's turn (started 80
	// minutes before this clock, inside the 3×idle window) is still open.
	if s.pauseIfIdle(30*time.Minute, time.Now().Add(40*time.Minute)) {
		t.Fatal("paused while the owning agent was mid-turn")
	}
}

func TestPausedMarkerRecordsHowToResume(t *testing.T) {
	s, sess, _, _ := pausableServer(t)
	sess.CWD = t.TempDir()
	sess.CLIArgs = []string{"--range", "a..b"}
	if !s.pauseIfIdle(30*time.Minute, time.Now().Add(31*time.Minute)) {
		t.Fatal("idle review did not pause")
	}
	marker, _ := session.ReadPausedMarker(s.reviewPath)
	if marker.CWD != sess.CWD || len(marker.CliArgs) != 2 || marker.CliArgs[1] != "a..b" {
		t.Fatalf("paused marker = %+v", marker)
	}
}

func TestRunIdleMonitorPausesOnceAndStops(t *testing.T) {
	s, _, stops, _ := pausableServer(t)
	done := make(chan struct{})
	go func() {
		s.RunIdleMonitor(context.Background(), 20*time.Millisecond, 5*time.Millisecond)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("the idle monitor never paused the idle review")
	}
	if stops.Load() != 1 {
		t.Fatalf("daemon stopped %d times, want 1", stops.Load())
	}
}

func TestRunIdleMonitorReturnsWhenTheDaemonStops(t *testing.T) {
	s, _, stops, _ := pausableServer(t)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		s.RunIdleMonitor(ctx, time.Hour, 5*time.Millisecond)
		close(done)
	}()
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("the idle monitor kept running after the daemon stopped")
	}
	if stops.Load() != 0 {
		t.Fatal("a busy review paused")
	}
}

// Once the pause is decided, a request that slips in is refused instead of
// being served by a daemon that is about to stop.
func TestPauseRefusesRequestDuringDecision(t *testing.T) {
	s, _, stops, _ := pausableServer(t)
	var late int
	s.SetStopFunc(func() {
		stops.Add(1)
		rec := httptest.NewRecorder()
		s.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/activity", nil))
		late = rec.Code
	})
	if !s.pauseIfIdle(30*time.Minute, time.Now().Add(31*time.Minute)) {
		t.Fatal("idle review did not pause")
	}
	if late != http.StatusServiceUnavailable {
		t.Errorf("request during the pause = %d, want 503", late)
	}
}
