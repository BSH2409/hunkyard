package server

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCommentPathCannotReadOutsideTheRepo(t *testing.T) {
	s, sess := newTestServer(t)
	secret := filepath.Join(filepath.Dir(sess.RepoRoot), "secret.txt")
	if err := os.WriteFile(secret, []byte("TOP-SECRET\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"../secret.txt", "./../secret.txt", secret} {
		post := httptest.NewRecorder()
		s.ServeHTTP(post, httptest.NewRequest(http.MethodPost, "/api/file/comments?path="+path, strings.NewReader(`{"start_line":1,"end_line":1,"body":"x"}`)))
		if post.Code < 400 || strings.Contains(post.Body.String(), "TOP-SECRET") {
			t.Errorf("POST comment on %q = %d %s", path, post.Code, post.Body)
		}
		for _, get := range []string{"/api/file?path=", "/api/file/diff?path=", "/api/file/impact?path=", "/api/file/image?side=head&path="} {
			rec := httptest.NewRecorder()
			s.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, get+path, nil))
			if strings.Contains(rec.Body.String(), "TOP-SECRET") {
				t.Errorf("%s%s leaked the file: %d", get, path, rec.Code)
			}
		}
	}
	// Positive control: a file inside the repo is still commentable.
	ok := httptest.NewRecorder()
	s.ServeHTTP(ok, httptest.NewRequest(http.MethodPost, "/api/file/comments?path=test.md", strings.NewReader(`{"start_line":1,"end_line":1,"body":"x"}`)))
	if ok.Code != http.StatusCreated && ok.Code != http.StatusOK {
		t.Fatalf("comment inside the repo = %d %s", ok.Code, ok.Body)
	}
}
