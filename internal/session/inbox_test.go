package session

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/BSH2409/hunkyard/internal/daemon"
	"github.com/BSH2409/hunkyard/internal/testutil"
)

// fakeDaemon answers the health probe and, when stateOK, the inbox state.
func fakeDaemon(t *testing.T, key, cwd, branch string, stateOK bool) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/health":
			fmt.Fprint(w, `{"status":"ok"}`)
		case "/api/inbox/state":
			if !stateOK {
				http.Error(w, "busy", http.StatusServiceUnavailable)
				return
			}
			fmt.Fprint(w, `{"round":2,"waiting_for_agent":true,"unresolved":3,"last_activity":"2026-11-01T10:00:00Z"}`)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	u, _ := url.Parse(server.URL)
	port, _ := strconv.Atoi(u.Port())
	if err := daemon.WriteSessionFile(key, daemon.SessionEntry{PID: os.Getpid(), Port: port, Host: u.Hostname(), CWD: cwd, Branch: branch, StartedAt: "x"}); err != nil {
		t.Fatal(err)
	}
}

func TestListInboxStates(t *testing.T) {
	home := t.TempDir()
	testutil.SetHome(t, home)
	fakeDaemon(t, "aaaaaaaaaaaa", "/work/orion", "feat/login", true)
	fakeDaemon(t, "bbbbbbbbbbbb", "/work/orion", "main", false)
	paused := filepath.Join(home, ".hunkyard", "reviews", "cccccccccccc")
	if err := os.MkdirAll(paused, 0o700); err != nil {
		t.Fatal(err)
	}
	writeJSONFile(t, filepath.Join(paused, "review.json"), HunkyardJSON{CWD: "/work/blogvault", Branch: "fix/x", ReviewRound: 4, UpdatedAt: "2026-10-31T09:00:00Z",
		Files: map[string]HunkyardJSONFile{"a.go": {Comments: []Comment{{ID: "c1", Body: "x"}}}}})
	if err := WritePausedMarker(paused, PausedMarker{PausedAt: time.Now(), Port: 5050, CWD: "/work/blogvault"}); err != nil {
		t.Fatal(err)
	}

	entries, err := ListInbox(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	byKey := map[string]InboxEntry{}
	for _, e := range entries {
		byKey[e.Key] = e
	}
	if len(entries) != 3 {
		t.Fatalf("entries = %+v", entries)
	}
	a := byKey["aaaaaaaaaaaa"]
	if a.State != "agent_working" || a.Round != 2 || a.Unresolved != 3 || a.URL == "" || a.Label != "feat/login" || a.Repository != "orion" {
		t.Fatalf("live = %+v", a)
	}
	if b := byKey["bbbbbbbbbbbb"]; b.State != "unreachable" || b.Label != "main" {
		t.Fatalf("unreachable = %+v", b)
	}
	c := byKey["cccccccccccc"]
	if c.State != "paused" || c.Round != 4 || c.Unresolved != 1 || c.URL != "" || c.Repository != "blogvault" || c.Label != "fix/x" {
		t.Fatalf("paused = %+v", c)
	}
	if entries[0].Key != "aaaaaaaaaaaa" {
		t.Fatalf("live reviews should come first: %+v", entries)
	}
}

func TestInboxStateOfSession(t *testing.T) {
	s := NewTestSession(t)
	s.ReviewRound = 3
	s.Files[0].Comments = []Comment{{ID: "c1"}, {ID: "c2", Resolved: true}}
	s.reviewComments = []Comment{{ID: "r1"}}
	s.setWaitingForAgent(true)
	got := s.InboxState()
	if got.Round != 3 || got.Unresolved != 2 || !got.WaitingForAgent {
		t.Fatalf("state = %+v", got)
	}
}

func TestListInboxPausedReviewWithoutComments(t *testing.T) {
	home := t.TempDir()
	testutil.SetHome(t, home)
	folder := filepath.Join(home, ".hunkyard", "reviews", "dddddddddddd")
	if err := os.MkdirAll(folder, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := WritePausedMarker(folder, PausedMarker{PausedAt: time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC), Port: 5050, CWD: "/work/quiet", CliArgs: []string{"notes.md"}}); err != nil {
		t.Fatal(err)
	}
	entries, err := ListInbox(context.Background())
	if err != nil || len(entries) != 1 {
		t.Fatalf("entries = %+v %v", entries, err)
	}
	if e := entries[0]; e.State != "paused" || e.Repository != "quiet" || e.Label != "notes.md" || e.LastActivity != "2026-11-01T09:00:00Z" {
		t.Fatalf("entry = %+v", e)
	}
}
