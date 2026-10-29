package server

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/BSH2409/hunkyard/internal/vcs"
)

func codeReviewRepo(t *testing.T) string {
	t.Helper()
	dir := vcs.InitTestRepo(t)
	if err := os.Mkdir(filepath.Join(dir, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"src/changed.go", "src/deleted.go", "other.go"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("package before\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	vcs.GitRun(t, dir, "add", ".")
	vcs.GitRun(t, dir, "commit", "-m", "base code")
	vcs.GitRun(t, dir, "checkout", "-b", "feature/code-review")
	for _, name := range []string{"src/changed.go", "src/new.go", "other.go"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("package after\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Remove(filepath.Join(dir, "src/deleted.go")); err != nil {
		t.Fatal(err)
	}
	chdirRepo(t, dir)
	return dir
}

func TestCreateSession_CodePathsDefaultToGitDiff(t *testing.T) {
	codeReviewRepo(t)
	sess, err := CreateSession(&DaemonCLIConfig{
		Files: []string{"src/changed.go", "src/new.go", "src/deleted.go"}, BaseBranch: "main",
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(sess.QuiesceForTest)
	if sess.Mode != "git" {
		t.Fatalf("Mode = %q, want git for code paths in a repository", sess.Mode)
	}
	got := make(map[string]string)
	for _, file := range sess.Files {
		got[file.Path] = file.Status
		if file.Status != "deleted" && len(file.DiffHunks) == 0 {
			t.Errorf("%s has no Git diff", file.Path)
		}
	}
	if len(got) != 3 || got["src/changed.go"] != "modified" || got["src/new.go"] != "untracked" || got["src/deleted.go"] != "deleted" {
		t.Fatalf("selected Git files = %v", got)
	}
}

func TestCreateSession_DocumentsAndUnchangedCodeKeepFileView(t *testing.T) {
	dir := vcs.InitTestRepo(t)
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	vcs.GitRun(t, dir, "add", "main.go")
	vcs.GitRun(t, dir, "commit", "-m", "unchanged code")
	chdirRepo(t, dir)
	for _, path := range []string{"README.md", "main.go"} {
		sess, err := CreateSession(&DaemonCLIConfig{Files: []string{path}, BaseBranch: "main"})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(sess.QuiesceForTest)
		if sess.Mode != "files" || len(sess.Files) != 1 {
			t.Fatalf("%s: mode=%s, files=%d", path, sess.Mode, len(sess.Files))
		}
	}
}

func TestCreateSession_CodeDirectoryKeepsSelectionAcrossRefreshAndScopes(t *testing.T) {
	dir := codeReviewRepo(t)
	sess, err := CreateSession(&DaemonCLIConfig{Files: []string{"src"}, BaseBranch: "main"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(sess.QuiesceForTest)
	if sess.Mode != "git" {
		t.Fatalf("Mode = %q, want git", sess.Mode)
	}
	for _, name := range []string{"src/later.go", "outside.go"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("package later\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	sess.RefreshFileList()
	sess.RefreshDiffs()
	for _, scope := range []string{"all", "unstaged"} {
		info := sess.GetSessionInfoScoped(scope, "")
		if len(info.Files) != 4 {
			t.Fatalf("scope %s: got %d files, want four selected changes", scope, len(info.Files))
		}
		for _, file := range info.Files {
			if filepath.Dir(file.Path) != "src" {
				t.Errorf("scope %s leaked unselected file %s", scope, file.Path)
			}
		}
	}
	if err := sess.ChangeBaseBranch("main"); err != nil {
		t.Fatal(err)
	}
	if len(sess.Files) != 4 {
		t.Fatalf("changing base lost the selection: got %d files", len(sess.Files))
	}
	base := vcs.GitRun(t, dir, "rev-parse", "HEAD")
	vcs.GitRun(t, dir, "add", ".")
	if files := sess.GetSessionInfoScoped("staged", "").Files; len(files) != 4 {
		t.Fatalf("staged selection: %+v", files)
	}
	vcs.GitRun(t, dir, "commit", "-m", "selected and unrelated changes")
	head := vcs.GitRun(t, dir, "rev-parse", "HEAD")
	for _, selection := range []struct{ scope, commit string }{{"branch", ""}, {"all", head}} {
		files := sess.GetSessionInfoScoped(selection.scope, selection.commit).Files
		if len(files) != 4 {
			t.Fatalf("scope %s commit %s: %+v", selection.scope, selection.commit, files)
		}
		for _, file := range files {
			if filepath.Dir(file.Path) != "src" {
				t.Fatalf("commit selection leaked %s", file.Path)
			}
		}
	}
	if err := sess.SetFocus(Focus{Kind: FocusRange, BaseSHA: base, HeadSHA: head}); err != nil {
		t.Fatal(err)
	}
	if len(sess.Files) != 4 {
		t.Fatalf("range focus lost the selection: got %d files", len(sess.Files))
	}
}

func TestCreateSession_ExplicitCodeOutsideGitRemainsFileReview(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	chdirRepo(t, dir)
	sess, err := CreateSession(&DaemonCLIConfig{Files: []string{"main.go"}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(sess.QuiesceForTest)
	if sess.Mode != "files" || len(sess.Files) != 1 {
		t.Fatalf("plain file review: mode=%s, files=%d", sess.Mode, len(sess.Files))
	}
}

func TestCreateSession_CodeDirectoryIncludesDeletedCodeWithOnlyMarkdownRemaining(t *testing.T) {
	dir := vcs.InitTestRepo(t)
	if err := os.Mkdir(filepath.Join(dir, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"src/old.go", "src/README.md"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("original\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	vcs.GitRun(t, dir, "add", ".")
	vcs.GitRun(t, dir, "commit", "-m", "base directory")
	if err := os.Remove(filepath.Join(dir, "src/old.go")); err != nil {
		t.Fatal(err)
	}
	chdirRepo(t, dir)
	sess, err := CreateSession(&DaemonCLIConfig{Files: []string{"src"}, BaseBranch: "main"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(sess.QuiesceForTest)
	if sess.Mode != "git" || len(sess.Files) != 1 || sess.Files[0].Path != "src/old.go" || sess.Files[0].Status != "deleted" {
		t.Fatalf("deleted code review: mode=%s, files=%+v", sess.Mode, sess.GetSessionInfo().Files)
	}
}

func TestCreateSession_CodePathsValidateOnlySelectedGitFilters(t *testing.T) {
	dir := codeReviewRepo(t)
	if err := os.WriteFile(filepath.Join(dir, ".gitattributes"), []byte("other.go filter=example\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	sess, err := CreateSession(&DaemonCLIConfig{Files: []string{"src/changed.go"}, BaseBranch: "main"})
	if err != nil {
		t.Fatalf("unrelated Git filter blocked review: %v", err)
	}
	t.Cleanup(sess.QuiesceForTest)
	if sess.Mode != "git" || len(sess.Files) != 1 {
		t.Fatalf("selected code: mode=%s, files=%d", sess.Mode, len(sess.Files))
	}
	sess.RefreshFileList()
	if files := sess.GetSessionInfoScoped("unstaged", "").Files; len(files) != 1 || files[0].Path != "src/changed.go" {
		t.Fatalf("selected scope with unrelated filters: %+v", files)
	}
	if err := os.WriteFile(filepath.Join(dir, ".gitattributes"), []byte("src/changed.go filter=example\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := CreateSession(&DaemonCLIConfig{Files: []string{"src/changed.go"}, BaseBranch: "main"}); err == nil {
		t.Fatal("selected Git filter must still be rejected")
	}
}
