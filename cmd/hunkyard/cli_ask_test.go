package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/BSH2409/hunkyard/internal/clicmd"
	"github.com/BSH2409/hunkyard/internal/daemon"
	"github.com/BSH2409/hunkyard/internal/feedback"
	"github.com/BSH2409/hunkyard/internal/testutil"
)

// askDaemon is a fake owner-bound review daemon that records the decision
// request it receives.
func askDaemon(t *testing.T, received *map[string]any) string {
	t.Helper()
	home := t.TempDir()
	testutil.SetHome(t, home)
	t.Setenv("HUNKYARD_OWNER_ID", "claude:synthetic")
	key := "0123456789ab"
	owner, err := feedback.OpenOwner(filepath.Join(home, ".hunkyard"), key, feedback.CurrentIdentity())
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/owner/challenge" {
			var request struct {
				Nonce string `json:"nonce"`
			}
			_ = json.NewDecoder(r.Body).Decode(&request)
			proof, _ := owner.Proof(request.Nonce, key, "generation")
			_ = json.NewEncoder(w).Encode(map[string]string{"proof": proof})
			return
		}
		if owner.Verify(r.Header.Get(feedback.OwnerHeader)) != nil || r.URL.Path != "/api/feedback/ask" || r.Method != http.MethodPost {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		_ = json.NewDecoder(r.Body).Decode(received)
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, `{"id":"d_abc123","question":"Which auth?","options":["Session","JWT"],"status":"open","asked_round":1,"asked_at":"2026-11-01T00:00:00Z"}`)
	}))
	t.Cleanup(server.Close)
	u, _ := url.Parse(server.URL)
	port, _ := strconv.Atoi(u.Port())
	if err := daemon.WriteSessionFile(key, daemon.SessionEntry{Host: u.Hostname(), Port: port, StartedAt: "generation"}); err != nil {
		t.Fatal(err)
	}
	return key
}

func TestAskCLIPostsRequest(t *testing.T) {
	var received map[string]any
	key := askDaemon(t, &received)
	var out bytes.Buffer
	err := askCommand(context.Background(), []string{"--session", key, "--option", "Session", "--option", "JWT", "--blocking", "--context", "internal/auth.go:12", "Which auth?"}, &out)
	if err != nil {
		t.Fatal(err)
	}
	if received["question"] != "Which auth?" || received["blocking"] != true {
		t.Fatalf("received = %v", received)
	}
	if options, _ := received["options"].([]any); len(options) != 2 || options[1] != "JWT" {
		t.Fatalf("options = %v", received["options"])
	}
	if ctx, _ := received["context"].(map[string]any); ctx["path"] != "internal/auth.go" || ctx["line"] != float64(12) {
		t.Fatalf("context = %v", received["context"])
	}
	if !strings.Contains(out.String(), "d_abc123") {
		t.Fatalf("output = %q", out.String())
	}
	out.Reset()
	if err := askCommand(context.Background(), []string{"--session", key, "--allow-other", "--json", "Name it?"}, &out); err != nil {
		t.Fatal(err)
	}
	if received["allow_other"] != true || !strings.HasPrefix(out.String(), `{"decision":`) {
		t.Fatalf("received = %v, output = %q", received, out.String())
	}
}

func TestAskCLIUsage(t *testing.T) {
	cases := [][]string{
		{"--session", "0123456789ab", "--option", "only", "Which?"},
		{"--session", "0123456789ab", "--option", "a", "--option", "b"},
		{"--option", "a", "--option", "b", "Which?"},
		{"--session", "0123456789ab", "--option", "a", "--option", "b", "--context", "file.go:x", "Which?"},
	}
	for _, args := range cases {
		err := askCommand(context.Background(), args, io.Discard)
		var exit clicmd.ExitError
		if !errors.As(err, &exit) || exit.Code != 2 {
			t.Fatalf("%v: err = %v, want usage error", args, err)
		}
	}
}

func TestAskCLIRequiresDaemon(t *testing.T) {
	testutil.SetHome(t, t.TempDir())
	err := askCommand(context.Background(), []string{"--session", "0123456789ab", "--option", "a", "--option", "b", "Which?"}, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "not running") {
		t.Fatalf("err = %v", err)
	}
}
