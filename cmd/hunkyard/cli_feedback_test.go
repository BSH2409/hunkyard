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
	"time"

	"github.com/BSH2409/hunkyard/internal/daemon"
	"github.com/BSH2409/hunkyard/internal/feedback"
	"github.com/BSH2409/hunkyard/internal/testutil"
)

func TestFeedbackCLIUsesLaunchingOwner(t *testing.T) {
	key := feedbackCLIServer(t, func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"status": "pending", "batches": []any{}})
	})
	var output bytes.Buffer
	if err := feedbackCommand(context.Background(), []string{"await", "--session", key, "--timeout", "1ms", "--json"}, &output); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), `"status":"pending"`) {
		t.Fatal("timeout was not pending")
	}
	t.Setenv("HUNKYARD_OWNER_ID", "codex:another-session")
	if err := feedbackCommand(context.Background(), []string{"await", "--session", key, "--timeout", "1ms", "--json"}, &output); err == nil {
		t.Fatal("another session consumed feedback")
	}
}

func TestFeedbackCLIFollowWaitsForPublication(t *testing.T) {
	const published = `{"status":"published","session_id":"012345abcdef","batches":[{"id":"batch-1","disposition":"request_changes"}],"selected_skills":{},"blocked_skills":{}}`
	for _, jsonOutput := range []bool{false, true} {
		t.Run(strconv.FormatBool(jsonOutput), func(t *testing.T) {
			requests := 0
			key := feedbackCLIServer(t, func(w http.ResponseWriter, r *http.Request) {
				requests++
				if requests < 3 {
					io.WriteString(w, `{"status":"pending","batches":[]}`)
					return
				}
				io.WriteString(w, published)
			})
			args := []string{"await", "--session", key, "--timeout", "1ms", "--follow"}
			if jsonOutput {
				args = append(args, "--json")
			}
			var output bytes.Buffer
			if err := feedbackCommand(context.Background(), args, &output); err != nil {
				t.Fatal(err)
			}
			if output.String() != published+"\n" {
				t.Fatalf("follow output = %q, want only the publication", output.String())
			}
		})
	}
}

func TestFeedbackCLIFollowStopsOnError(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
	}{
		{"server error", http.StatusServiceUnavailable, "review unavailable"},
		{"authentication error", http.StatusForbidden, "wrong owner"},
		{"malformed JSON", http.StatusOK, "{"},
		{"missing status", http.StatusOK, "{}"},
		{"unknown status", http.StatusOK, `{"status":"approved"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			requests := 0
			key := feedbackCLIServer(t, func(w http.ResponseWriter, r *http.Request) {
				requests++
				if requests == 1 {
					io.WriteString(w, `{"status":"pending","batches":[]}`)
					return
				}
				w.WriteHeader(tc.status)
				io.WriteString(w, tc.body)
			})
			var output bytes.Buffer
			err := feedbackCommand(context.Background(), []string{"await", "--session", key, "--timeout", "1ms", "--follow", "--json"}, &output)
			if err == nil {
				t.Fatal("follow accepted a failed or invalid response")
			}
			if requests != 2 {
				t.Fatalf("follow made %d requests, want one pending wait then the error", requests)
			}
			if strings.Contains(output.String(), `"status":"pending"`) {
				t.Fatalf("follow printed pending output: %s", output.String())
			}
		})
	}
}

func TestFeedbackCLIFollowRequiresAwait(t *testing.T) {
	for _, action := range []string{"learn", "ack"} {
		t.Run(action, func(t *testing.T) {
			var output bytes.Buffer
			err := feedbackCommand(context.Background(), []string{action, "--session", "012345abcdef", "--follow"}, &output)
			if err == nil || !strings.Contains(err.Error(), "--follow is only supported for await") {
				t.Fatalf("error = %v, want an await-only flag error", err)
			}
		})
	}
}

func TestFeedbackCLIFollowCancelsInFlightWait(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	requests := 0
	key := feedbackCLIServer(t, func(w http.ResponseWriter, r *http.Request) {
		requests++
		if requests == 1 {
			io.WriteString(w, `{"status":"pending","batches":[]}`)
			return
		}
		cancel()
		<-r.Context().Done()
	})
	var output bytes.Buffer
	err := feedbackCommand(ctx, []string{"await", "--session", key, "--timeout", "1ms", "--follow", "--json"}, &output)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("follow error = %v, want cancellation", err)
	}
	if output.Len() != 0 {
		t.Fatalf("cancelled follow printed output: %s", output.String())
	}
}

func TestFeedbackCLIFollowStopsOnTransportError(t *testing.T) {
	key := feedbackCLIServer(t, func(w http.ResponseWriter, r *http.Request) {
		panic(http.ErrAbortHandler)
	})
	var output bytes.Buffer
	err := feedbackCommand(context.Background(), []string{"await", "--session", key, "--timeout", "1ms", "--follow", "--json"}, &output)
	if err == nil {
		t.Fatal("follow ignored a transport error")
	}
	if output.Len() != 0 {
		t.Fatalf("failed follow printed output: %s", output.String())
	}
}

func feedbackCLIServer(t *testing.T, handler http.HandlerFunc) string {
	t.Helper()
	home := t.TempDir()
	testutil.SetHome(t, home)
	t.Setenv("HUNKYARD_OWNER_ID", "claude:synthetic")
	key := "012345abcdef"
	owner, err := feedback.OpenOwner(filepath.Join(home, ".hunkyard"), key, feedback.CurrentIdentity())
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/owner/challenge" {
			var request struct {
				Nonce string `json:"nonce"`
			}
			json.NewDecoder(r.Body).Decode(&request)
			proof, _ := owner.Proof(request.Nonce, key, "generation")
			json.NewEncoder(w).Encode(map[string]string{"proof": proof})
			return
		}
		if owner.Verify(r.Header.Get(feedback.OwnerHeader)) != nil {
			t.Error("CLI did not authenticate")
		}
		if r.URL.Path != "/api/feedback/await" || r.URL.Query().Get("timeout") != "1ms" {
			t.Error("wrong feedback command")
		}
		handler(w, r)
	}))
	t.Cleanup(server.Close)
	u, _ := url.Parse(server.URL)
	port, _ := strconv.Atoi(u.Port())
	if err := daemon.WriteSessionFile(key, daemon.SessionEntry{Host: u.Hostname(), Port: port, StartedAt: "generation"}); err != nil {
		t.Fatal(err)
	}
	return key
}
