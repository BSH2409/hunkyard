package server

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/BSH2409/hunkyard/internal/vcs"
)

func TestFileImpactEndpoint(t *testing.T) {
	dir := initTestGitRepo(t)
	writeGitFile(t, filepath.Join(dir, "api.go"), "package a\n\nfunc ServeLogin() {}\n")
	writeGitFile(t, filepath.Join(dir, "routes.go"), "package a\n\nvar r = ServeLogin\n")
	gitCommit(t, dir, "add", ".")
	gitCommit(t, dir, "commit", "-m", "impact")
	sess := &Session{Mode: "git", RepoRoot: dir, VCS: &GitVCS{}, Files: []*FileEntry{{Path: "api.go", Status: "modified", FileType: "code",
		DiffHunks: []vcs.DiffHunk{{Lines: []vcs.DiffLine{{Type: "add", Content: "func ServeLogin() {}", NewNum: 3}}}}}}}
	sess.InitTestChannels()
	s, err := NewServer(sess, frontendFS, "", "test", 0, "")
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name, method, target string
		want                 int
		body                 string
	}{
		{"happy", http.MethodGet, "/api/file/impact?path=api.go", http.StatusOK, `"path":"routes.go"`},
		{"missing path", http.MethodGet, "/api/file/impact", http.StatusBadRequest, ""},
		{"unknown file", http.MethodGet, "/api/file/impact?path=nope.go", http.StatusNotFound, ""},
		{"method", http.MethodPost, "/api/file/impact?path=api.go", http.StatusMethodNotAllowed, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			s.ServeHTTP(rec, httptest.NewRequest(tc.method, tc.target, nil))
			if rec.Code != tc.want || !strings.Contains(rec.Body.String(), tc.body) {
				t.Fatalf("%s = %d %s", tc.name, rec.Code, rec.Body)
			}
		})
	}
}
