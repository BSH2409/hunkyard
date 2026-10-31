package session

import (
	"net"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/BSH2409/hunkyard/internal/daemon"
)

func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	return port
}

func TestResumeReusesPausedPort(t *testing.T) {
	dir := t.TempDir()
	port := freePort(t)
	if err := WritePausedMarker(dir, PausedMarker{PausedAt: time.Now(), Port: port, Host: "127.0.0.1"}); err != nil {
		t.Fatal(err)
	}
	stale, args := pausedResumeArgs(dir, daemon.SessionEntry{}, nil)
	if !slices.Equal(args, []string{"--port", strconv.Itoa(port)}) || stale.Host != "127.0.0.1" {
		t.Fatalf("free paused port: args=%v host=%q", args, stale.Host)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:"+strconv.Itoa(port))
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	if _, args := pausedResumeArgs(dir, daemon.SessionEntry{}, nil); !slices.Equal(args, []string{"--port", "0"}) {
		t.Fatalf("busy paused port: args=%v, want a free port", args)
	}
	explicit := []string{"--port", "5555"}
	if _, args := pausedResumeArgs(dir, daemon.SessionEntry{}, explicit); !slices.Equal(args, explicit) {
		t.Fatalf("an explicit --port was overridden: %v", args)
	}
	if _, args := pausedResumeArgs(t.TempDir(), daemon.SessionEntry{}, nil); len(args) != 0 {
		t.Fatalf("a review that is not paused got port args: %v", args)
	}
}

func TestStatusShowsPaused(t *testing.T) {
	dir := t.TempDir()
	at := time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC)
	if err := WritePausedMarker(dir, PausedMarker{PausedAt: at, Port: 4321}); err != nil {
		t.Fatal(err)
	}
	info := daemonStatusInfo(nil, dir)
	if info["running"] != false || info["paused"] != true || info["port"] != 4321 {
		t.Fatalf("status for a paused review = %v", info)
	}
	if line := daemonStatusLine(nil, dir); line != "paused since 2026-10-31 12:00 UTC (port 4321)" {
		t.Fatalf("status line = %q", line)
	}
	if line := daemonStatusLine(nil, t.TempDir()); line != "not running" {
		t.Fatalf("status line without a pause = %q", line)
	}
	running := &daemon.SessionEntry{PID: 7, Port: 9}
	if line := daemonStatusLine(running, dir); line != "running (PID 7, port 9)" {
		t.Fatalf("status line while running = %q", line)
	}
}

func TestStopClearsPausedMarker(t *testing.T) {
	home := t.TempDir()
	setHome(t, home)
	key := "abcdef012345"
	dir, err := daemon.ReviewFilePath(key)
	if err != nil {
		t.Fatal(err)
	}
	if err := WritePausedMarker(dir, PausedMarker{PausedAt: time.Now(), Port: 1}); err != nil {
		t.Fatal(err)
	}
	if stopped, err := stopPausedReview(key); err != nil || !stopped {
		t.Fatalf("stopping a paused review: %v %v", stopped, err)
	}
	if _, paused := ReadPausedMarker(dir); paused {
		t.Fatal("marker still present after stop")
	}
	if stopped, _ := stopPausedReview(key); stopped {
		t.Fatal("a review that is not paused reported as stopped")
	}
}

func TestDeadSessionLaunchKeepsPausedListener(t *testing.T) {
	dir := t.TempDir()
	if err := WritePausedMarker(dir, PausedMarker{PausedAt: time.Now(), Port: 4321, Host: "127.0.0.1"}); err != nil {
		t.Fatal(err)
	}
	stale := daemon.SessionEntry{ReviewPath: dir}
	defaults := &CLIReviewConfig{Port: 5000, Host: "0.0.0.0"}
	tests := []struct {
		name      string
		reviewDir string
		sc        *CLIReviewConfig
		wantHost  string
		wantArgs  []string
	}{
		{"paused review keeps its own listener", dir, defaults, "", nil},
		{"explicit port and host win over the pause", dir, &CLIReviewConfig{Port: 6000, Host: "10.0.0.1", PortExplicit: true, HostExplicit: true}, "10.0.0.1", []string{"--port", "6000"}},
		{"stopped review uses the resolved listener", t.TempDir(), defaults, "0.0.0.0", []string{"--port", "5000"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entry := stale
			entry.ReviewPath = tt.reviewDir
			got, args := deadSessionListener("abcdef012345", entry, tt.sc)
			if got.Host != tt.wantHost || !slices.Equal(args, tt.wantArgs) {
				t.Fatalf("host=%q args=%v, want host=%q args=%v", got.Host, args, tt.wantHost, tt.wantArgs)
			}
		})
	}
}

func TestPausedReviewWithoutCommentsCanResume(t *testing.T) {
	dir := t.TempDir()
	cwd := t.TempDir()
	if err := WritePausedMarker(dir, PausedMarker{PausedAt: time.Now(), Port: 4321, CWD: cwd, CliArgs: []string{"--range", "a..b"}}); err != nil {
		t.Fatal(err)
	}
	stale := daemon.SessionEntry{ReviewPath: dir}
	revDir, err := resolveReconnectReviewDir("abcdef012345", stale)
	if err != nil || revDir != dir {
		t.Fatalf("review dir = %q, %v", revDir, err)
	}
	cj, err := readReconnectReview("abcdef012345", revDir)
	if err != nil {
		t.Fatal(err)
	}
	if cj.CWD != cwd || !slices.Equal(cj.CliArgs, []string{"--range", "a..b"}) {
		t.Fatalf("review rebuilt from the pause = cwd %q args %v", cj.CWD, cj.CliArgs)
	}
	if _, err := readReconnectReview("abcdef012345", t.TempDir()); err == nil {
		t.Fatal("a folder with neither a review nor a pause resumed")
	}
}
