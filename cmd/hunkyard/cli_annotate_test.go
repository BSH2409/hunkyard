package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/BSH2409/hunkyard/internal/clicmd"
	"github.com/BSH2409/hunkyard/internal/daemon"
	"github.com/BSH2409/hunkyard/internal/testutil"
)

// annotateDaemon registers a fake review daemon that records the
// annotations it receives.
func annotateDaemon(t *testing.T, received *map[string]any) string {
	return annotateDaemonIn(t, received, "")
}

// annotateDaemonIn registers the fake daemon as serving a review of cwd.
func annotateDaemonIn(t *testing.T, received *map[string]any, cwd string) string {
	t.Helper()
	testutil.SetHome(t, t.TempDir())
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/session" {
			_, _ = io.WriteString(w, `{}`)
			return
		}
		if r.URL.Path != "/api/annotations" || r.Method != http.MethodPost {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		_ = json.NewDecoder(r.Body).Decode(received)
		_, _ = io.WriteString(w, `{"added":1,"kept":0,"removed":0,"dropped":0,"coverage":{"server.go":{"changed":2,"uncovered":1}}}`)
	}))
	t.Cleanup(server.Close)
	u, _ := url.Parse(server.URL)
	port, _ := strconv.Atoi(u.Port())
	key := "0a1b2c3d4e5f"
	if err := daemon.WriteSessionFile(key, daemon.SessionEntry{Host: u.Hostname(), Port: port, PID: os.Getpid(), CWD: cwd, StartedAt: "generation"}); err != nil {
		t.Fatal(err)
	}
	return key
}

func TestAnnotateCLIPostsParsedAnnotations(t *testing.T) {
	var received map[string]any
	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "go.mod"), []byte("module example.com/demo\n\ngo 1.26\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	key := annotateDaemonIn(t, &received, repo)
	profile := filepath.Join(t.TempDir(), "cover.out")
	if err := os.WriteFile(profile, []byte("mode: set\nexample.com/demo/server.go:2.1,3.2 1 0\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(t.TempDir()) // the agent runs annotate from somewhere else
	var out bytes.Buffer
	if err := annotateCommand([]string{"--format", "coverprofile", "--session", key, profile}, strings.NewReader(""), &out); err != nil {
		t.Fatal(err)
	}
	if received["tool"] != "coverage" {
		t.Fatalf("tool = %v", received["tool"])
	}
	marks, _ := received["marks"].(map[string]any)
	if _, ok := marks["server.go"]; !ok {
		t.Fatalf("module prefix not stripped: %v", received["marks"])
	}
	if !strings.Contains(out.String(), "added=1") || !strings.Contains(out.String(), "server.go: 1 of 2 changed lines uncovered") {
		t.Fatalf("output = %q", out.String())
	}
}

func TestAnnotateCLIReadsStdin(t *testing.T) {
	var received map[string]any
	key := annotateDaemon(t, &received)
	sarif := `{"runs":[{"tool":{"driver":{"name":"eslint"}},"results":[{"ruleId":"no-console","message":{"text":"no console"},"locations":[{"physicalLocation":{"artifactLocation":{"uri":"web/app.js"},"region":{"startLine":4}}}]}]}]}`
	var out bytes.Buffer
	if err := annotateCommand([]string{"--format", "sarif", "--session", key, "--json", "-"}, strings.NewReader(sarif), &out); err != nil {
		t.Fatal(err)
	}
	if received["tool"] != "eslint" || !strings.HasPrefix(out.String(), "{") {
		t.Fatalf("tool = %v, output = %q", received["tool"], out.String())
	}
}

func TestAnnotateCLIRequiresDaemon(t *testing.T) {
	testutil.SetHome(t, t.TempDir())
	t.Chdir(t.TempDir())
	err := annotateCommand([]string{"--format", "lcov", "-"}, strings.NewReader("SF:a.js\nDA:1,0\nend_of_record\n"), io.Discard)
	if err == nil || !strings.Contains(err.Error(), "no active review session") {
		t.Fatalf("err = %v", err)
	}
}

func TestAnnotateCLIUsage(t *testing.T) {
	for _, args := range [][]string{{}, {"cover.out"}, {"--format", "junit", "x"}} {
		err := annotateCommand(args, strings.NewReader(""), io.Discard)
		var exit clicmd.ExitError
		if err == nil || !errors.As(err, &exit) || exit.Code != 2 {
			t.Fatalf("%v: err = %v, want usage error", args, err)
		}
	}
}

func TestAnnotateCLIWaitsForTheReviewToBeReady(t *testing.T) {
	testutil.SetHome(t, t.TempDir())
	readyAfter := 2
	posted := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/session":
			if readyAfter > 0 {
				readyAfter--
				http.Error(w, "loading", http.StatusServiceUnavailable)
				return
			}
			_, _ = io.WriteString(w, `{}`)
		case "/api/annotations":
			if readyAfter > 0 {
				t.Error("annotations posted before the review was ready")
			}
			posted = true
			_, _ = io.WriteString(w, `{"added":0,"kept":0,"removed":0,"dropped":0}`)
		}
	}))
	t.Cleanup(server.Close)
	u, _ := url.Parse(server.URL)
	port, _ := strconv.Atoi(u.Port())
	if err := daemon.WriteSessionFile("0a1b2c3d4e5f", daemon.SessionEntry{Host: u.Hostname(), Port: port, PID: os.Getpid(), StartedAt: "g"}); err != nil {
		t.Fatal(err)
	}
	readinessPoll = time.Millisecond
	if err := annotateCommand([]string{"--format", "lcov", "--session", "0a1b2c3d4e5f", "-"}, strings.NewReader("SF:a.js\nDA:1,0\nend_of_record\n"), io.Discard); err != nil || !posted {
		t.Fatalf("err = %v, posted = %v", err, posted)
	}
}
