package session

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/BSH2409/hunkyard/internal/vcs"
)

func impactFixture(t *testing.T) *Session {
	t.Helper()
	root := initTestRepo(t)
	writeFile(t, filepath.Join(root, "api.go"), "package a\n\nfunc ServeLogin() {}\n")
	writeFile(t, filepath.Join(root, "routes.go"), "package a\n\nvar r = ServeLogin\n")
	writeFile(t, filepath.Join(root, "api_test.go"), "package a\n\nfunc TestX() { ServeLogin() }\n")
	gitT(t, root, "add", ".")
	gitT(t, root, "commit", "-m", "impact fixture")
	s := NewTestSession(t)
	s.Mode, s.RepoRoot, s.VCS = "git", root, &vcs.GitVCS{}
	s.Files = []*FileEntry{{Path: "api.go", Status: "modified", FileType: "code", FileHash: "h1", Content: "package a\n\nfunc ServeLogin() {}\n",
		DiffHunks: []vcs.DiffHunk{{Lines: []vcs.DiffLine{{Type: "add", Content: "func ServeLogin() {}", NewNum: 3}}}}}}
	return s
}

func TestFileImpactFindsCallersAndTests(t *testing.T) {
	s := impactFixture(t)
	got, err := s.FileImpact(context.Background(), "api.go")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Mentions) != 1 || got.Mentions[0].Path != "routes.go" || len(got.Tests) != 1 || got.Tests[0].Path != "api_test.go" {
		t.Fatalf("impact = %+v", got)
	}
}

func TestFileImpactCachesPerFileVersion(t *testing.T) {
	s := impactFixture(t)
	calls := 0
	s.impactGrep = func(ctx context.Context, dir string, words []string, exclude string) ([]vcs.GrepHit, error) {
		calls++
		return nil, nil
	}
	for i := 0; i < 3; i++ {
		if _, err := s.FileImpact(context.Background(), "api.go"); err != nil {
			t.Fatal(err)
		}
	}
	if calls != 1 {
		t.Fatalf("grep ran %d times for one file version", calls)
	}
	s.Files[0].FileHash = "h2"
	_, _ = s.FileImpact(context.Background(), "api.go")
	s.ReviewRound++
	_, _ = s.FileImpact(context.Background(), "api.go")
	if calls != 3 {
		t.Fatalf("cache not refreshed after a change or a new round: %d calls", calls)
	}
}

func TestFileImpactUnavailable(t *testing.T) {
	s := impactFixture(t)
	if _, err := s.FileImpact(context.Background(), "missing.go"); !errors.Is(err, ErrImpactUnavailable) {
		t.Fatalf("unknown file: %v", err)
	}
	s.Mode = "files"
	if _, err := s.FileImpact(context.Background(), "api.go"); !errors.Is(err, ErrImpactUnavailable) {
		t.Fatalf("files mode: %v", err)
	}
}
