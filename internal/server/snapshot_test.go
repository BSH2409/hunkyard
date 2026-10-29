package server

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSnapshotStaleCommentIsRejected(t *testing.T) {
	for _, field := range []string{"content_id", "view_id"} {
		t.Run(field, func(t *testing.T) {
			server, sess := newTestServer(t)
			body := `{"start_line":1,"end_line":1,"body":"Feedback","` + field + `":"stale"}`
			response := httptest.NewRecorder()
			server.ServeHTTP(response, httptest.NewRequest("POST", "/api/file/comments?path=test.md", strings.NewReader(body)))
			if response.Code != 409 {
				t.Fatalf("stale comment status = %d", response.Code)
			}
			if len(sess.GetComments("test.md")) != 0 {
				t.Fatal("stale comment attached to changed code")
			}
		})
	}
}

func TestSnapshotCommentRecordsViewAndContent(t *testing.T) {
	server, sess := newTestServer(t)
	file, ok := sess.GetFileSnapshot("test.md")
	if !ok {
		t.Fatal("file missing")
	}
	body, err := json.Marshal(map[string]any{"start_line": 1, "end_line": 1, "body": "Feedback", "content_id": file["file_hash"], "view_id": file["view_id"]})
	if err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	server.ServeHTTP(response, httptest.NewRequest("POST", "/api/file/comments?path=test.md", strings.NewReader(string(body))))
	if response.Code != 201 {
		t.Fatalf("comment status = %d: %s", response.Code, response.Body.String())
	}
	var saved map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &saved); err != nil {
		t.Fatal(err)
	}
	if saved["content_id"] != file["file_hash"] || saved["view_id"] == nil || saved["view_id"] == "" {
		t.Fatal("comment lost immutable view/content identity")
	}
}
