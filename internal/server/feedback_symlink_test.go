package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/BSH2409/hunkyard/internal/feedback"
	sesspkg "github.com/BSH2409/hunkyard/internal/session"
	"github.com/BSH2409/hunkyard/internal/testutil"
)

func TestPublishWithUntrackedDirectorySymlink(t *testing.T) {
	for _, disposition := range []string{"approve", "comment", "request_changes"} {
		t.Run(disposition, func(t *testing.T) {
			root := testutil.InitTestRepo(t)
			t.Chdir(root)
			testutil.SetHome(t, t.TempDir())
			testutil.WriteFile(t, filepath.Join(root, ".gitignore"), "node_modules/\n")
			testutil.Git(t, root, "add", ".gitignore")
			testutil.Git(t, root, "commit", "-m", "ignore directories")
			for i := 0; i < 30; i++ {
				testutil.WriteFile(t, filepath.Join(root, fmt.Sprintf("file-%02d.txt", i)), "new content\n")
			}
			if err := os.Symlink(t.TempDir(), filepath.Join(root, "node_modules")); err != nil {
				t.Skipf("symlinks unavailable: %v", err)
			}
			sess, err := sesspkg.NewSessionFromGit(nil)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(sess.QuiesceForTest)
			lazy := false
			for _, file := range sess.Files {
				if file.Path == "node_modules" {
					t.Fatal("directory symlink included in review")
				}
				lazy = lazy || file.Lazy
			}
			if !lazy {
				t.Fatal("fixture does not exercise lazy loading")
			}
			s, _ := newTestServer(t)
			s.SetSession(sess)
			s.reviewPath = t.TempDir()
			owner, err := feedback.OpenOwner(t.TempDir(), "abcd1234ef01", feedback.Identity{Key: "test", Kind: "codex"})
			if err != nil {
				t.Fatal(err)
			}
			if err := s.BindOwner(owner, "abcd1234ef01", "generation"); err != nil {
				t.Fatal(err)
			}
			rec := httptest.NewRecorder()
			body := fmt.Sprintf(`{"request_id":"symlink-test","disposition":%q}`, disposition)
			s.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/finish", strings.NewReader(body)))
			if rec.Code != http.StatusOK {
				t.Fatalf("publish: %d %s", rec.Code, rec.Body)
			}
			var result struct {
				Approved bool           `json:"approved"`
				Batch    feedback.Batch `json:"batch"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			if result.Approved != (disposition == "approve") || result.Batch.Disposition != disposition {
				t.Fatalf("unexpected publication: %+v", result)
			}
		})
	}
}
