package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	sesspkg "github.com/BSH2409/hunkyard/internal/session"
	"github.com/BSH2409/hunkyard/internal/testutil"
)

func discussionHTTP(s *Server, method string, body any) *httptest.ResponseRecorder {
	raw, _ := json.Marshal(body)
	request := httptest.NewRequest(method, "/api/discussion", bytes.NewReader(raw))
	request.Header.Set("Sec-Fetch-Site", "same-origin")
	recorder := httptest.NewRecorder()
	s.ServeHTTP(recorder, request)
	return recorder
}

func TestDiscussionOptInValidationAndRestart(t *testing.T) {
	s, _ := newTestServer(t)
	s.reviewPath = t.TempDir()
	s.ownerGeneration = "new"
	response := discussionHTTP(s, http.MethodGet, nil)
	if response.Code != 200 || !strings.Contains(response.Body.String(), `"participants":[]`) {
		t.Fatalf("not disabled by default: %d %s", response.Code, response.Body)
	}
	if got := discussionHTTP(s, http.MethodPut, map[string]any{"participants": []string{"gemini"}}); got.Code != 400 {
		t.Fatalf("unknown agent: %d", got.Code)
	}
	if got := discussionHTTP(s, http.MethodPost, map[string]any{"request_id": "test", "comment_id": "r_none", "recipients": []string{"codex"}}); got.Code != 409 {
		t.Fatalf("uninvited agent: %d", got.Code)
	}
	raw := httptest.NewRecorder()
	s.ServeHTTP(raw, httptest.NewRequest(http.MethodPut, "/api/discussion", strings.NewReader(`{"participants":["codex"]}`)))
	if raw.Code != 403 {
		t.Fatal("invitation bypassed the review page")
	}
	state := discussionState{Participants: []string{"codex"}, Requests: []discussionRequest{{ID: "prior", Generation: "old", Responses: []discussionResponse{{Agent: "codex", Status: "thinking"}}}}}
	if err := s.writeDiscussion(state); err != nil {
		t.Fatal(err)
	}
	recovered, err := s.readDiscussion()
	if err != nil || recovered.Requests[0].Responses[0].Status != "interrupted" {
		t.Fatalf("restart recovery: %+v %v", recovered, err)
	}
}

func TestDiscussionBothCLIsReplyOnceAndFailuresStayVisible(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix CLI fixtures")
	}
	dir := t.TempDir()
	for name, body := range map[string]string{
		"claude": "#!/bin/sh\ncat >/dev/null\nprintf '%s' '{\"result\":\"Claude naming advice\",\"is_error\":false}'\n",
		"codex":  "#!/bin/sh\nwhile [ $# -gt 0 ]; do\nif [ \"$1\" = --output-last-message ]; then shift; target=$1; fi\nshift\ndone\ncat >/dev/null\nprintf '%s' 'Codex contract advice' > \"$target\"\n",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	s, sess := newTestServer(t)
	s.reviewPath = t.TempDir()
	s.ownerGeneration = "test"
	comment := sess.AddReviewComment("Discuss test.md:1", "Reviewer")
	if got := discussionHTTP(s, http.MethodPut, map[string]any{"participants": []string{"claude", "codex"}}); got.Code != 200 {
		t.Fatal(got.Body)
	}
	input := map[string]any{"request_id": "group-one", "comment_id": comment.ID, "recipients": []string{"claude", "codex"}}
	for range 2 {
		response := discussionHTTP(s, http.MethodPost, input)
		if response.Code != 202 && response.Code != 200 {
			t.Fatalf("send: %d %s", response.Code, response.Body)
		}
	}
	if !s.WaitBackground(5 * time.Second) {
		t.Fatal("CLI requests did not finish")
	}
	comments := sess.GetReviewComments()
	if len(comments) != 1 || len(comments[0].Replies) != 2 {
		t.Fatalf("group replies: %+v", comments)
	}
	for _, reply := range comments[0].Replies {
		if reply.AuthorKind != "agent" {
			t.Fatalf("agent reply marked human: %+v", reply)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "claude"), []byte("#!/bin/sh\necho 'authentication expired' >&2\nexit 1\n"), 0700); err != nil {
		t.Fatal(err)
	}
	input["request_id"] = "group-two"
	input["recipients"] = []string{"claude"}
	if got := discussionHTTP(s, http.MethodPost, input); got.Code != 202 {
		t.Fatalf("retry: %d %s", got.Code, got.Body)
	}
	if !s.WaitBackground(5 * time.Second) {
		t.Fatal("failed participant hung")
	}
	response := discussionHTTP(s, http.MethodGet, nil)
	if !strings.Contains(response.Body.String(), `"status":"authentication"`) {
		t.Fatalf("failure hidden: %s", response.Body)
	}
}

func TestDiscussionOmitsDeletedPrivateKey(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix CLI fixture")
	}
	root := testutil.InitTestRepo(t)
	t.Chdir(root)
	testutil.SetHome(t, t.TempDir())
	testutil.WriteFile(t, filepath.Join(root, "config.txt"), "-----BEGIN PRIVATE KEY-----\nFAKE_DELETED_KEY_MATERIAL\n-----END PRIVATE KEY-----\n")
	testutil.Git(t, root, "add", "config.txt")
	testutil.Git(t, root, "commit", "-m", "fixture baseline")
	testutil.WriteFile(t, filepath.Join(root, "config.txt"), "public configuration\n")
	sess, err := sesspkg.NewSessionFromGit(nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(sess.QuiesceForTest)
	directory := t.TempDir()
	capture := filepath.Join(directory, "prompt.txt")
	t.Setenv("HUNKYARD_DISCUSSION_CAPTURE", capture)
	if err := os.WriteFile(filepath.Join(directory, "claude"), []byte("#!/bin/sh\ncat >\"$HUNKYARD_DISCUSSION_CAPTURE\"\nprintf '%s' '{\"result\":\"Context received\",\"is_error\":false}'\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", directory+string(os.PathListSeparator)+os.Getenv("PATH"))
	s, _ := newTestServer(t)
	s.SetSession(sess)
	s.reviewPath = t.TempDir()
	comment := sess.AddReviewComment("Discuss the configuration", "Reviewer")
	if got := discussionHTTP(s, http.MethodPut, map[string]any{"participants": []string{"claude"}}); got.Code != 200 {
		t.Fatal(got.Body)
	}
	if got := discussionHTTP(s, http.MethodPost, map[string]any{"request_id": "deleted-key", "comment_id": comment.ID, "recipients": []string{"claude"}}); got.Code != 202 {
		t.Fatalf("send: %d %s", got.Code, got.Body)
	}
	if !s.WaitBackground(5 * time.Second) {
		t.Fatal("participant did not finish")
	}
	prompt, err := os.ReadFile(capture)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(prompt), "FAKE_DELETED_KEY_MATERIAL") || !strings.Contains(string(prompt), "config.txt: sensitive file") {
		t.Fatal("deleted private-key evidence was not omitted")
	}
}
