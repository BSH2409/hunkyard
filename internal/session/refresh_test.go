package session

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/BSH2409/hunkyard/internal/vcs"
)

func TestRefreshWorkingTreePicksUpEditsAndKeepsComments(t *testing.T) {
	dir := newBudgetRepo(t, 25, 0)
	writeFile(t, filepath.Join(dir, "src", "file3.go"), "package p\n\nfunc F3() int { return 33 }\n")
	sess, err := NewGitSession(&vcs.GitVCS{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	// The comment schedules a debounced write into the repository; stop it
	// before the temp directory is removed (Windows cannot remove a
	// directory that is being written).
	t.Cleanup(sess.QuiesceForTest)
	if _, ok := sess.AddComment("src/file3.go", 3, 3, "", "keep me", "", ""); !ok {
		t.Fatal("comment rejected")
	}
	writeFile(t, filepath.Join(dir, "src", "file3.go"), "package p\n\nfunc F3() int { return 333 }\n")
	writeFile(t, filepath.Join(dir, "src", "file4.go"), "package p\n\nfunc F4() int { return 44 }\n")
	sess.markStale()
	if err := sess.RefreshWorkingTree(); err != nil {
		t.Fatal(err)
	}
	info := sess.GetSessionInfo()
	if info.Stale {
		t.Fatal("still stale after refresh")
	}
	var paths []string
	for _, f := range info.Files {
		paths = append(paths, f.Path)
	}
	if !strings.Contains(strings.Join(paths, ","), "src/file4.go") {
		t.Fatalf("refresh missed the newly edited file: %v", paths)
	}
	if got := sess.GetComments("src/file3.go"); len(got) != 1 || got[0].Body != "keep me" {
		t.Fatalf("comments lost on refresh: %+v", got)
	}
	sess.mu.RLock()
	defer sess.mu.RUnlock()
	for _, f := range sess.Files {
		if f.Path == "src/file3.go" && !strings.Contains(f.Content, "333") {
			t.Fatalf("file3 content not refreshed: %q", f.Content)
		}
	}
}

func TestFilesWatcherMarksStaleOutsideAgentWork(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "plan.md")
	writeFile(t, path, "# Plan\n")
	s := &Session{
		Mode: "files", RepoRoot: dir, ReviewRound: 1, pollInterval: testPollInterval,
		Files:       []*FileEntry{{Path: "plan.md", AbsPath: path, FileType: "markdown", Content: "# Plan\n", FileHash: fileHash([]byte("# Plan\n")), Comments: []Comment{}}},
		subscribers: make(map[chan SSEEvent]struct{}), roundComplete: make(chan struct{}, 1),
	}
	events := s.Subscribe()
	stop := make(chan struct{})
	go s.Watch(stop)
	t.Cleanup(func() { close(stop) })
	time.Sleep(5 * testPollInterval)
	writeFileNewMtime(t, path, "# Plan\n\nEdited by hand.\n")
	if !waitForEvent(events, "review-stale", 2*time.Second) {
		t.Fatal("no review-stale event for a hand edit outside agent work")
	}
	if !s.GetSessionInfo().Stale {
		t.Fatal("files session not marked stale")
	}
}

func TestRefreshFilesReadsDiskBeforeWatcherTick(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	path := filepath.Join(dir, "plan.md")
	writeFile(t, path, "# Original\n")
	sess, err := NewSessionFromFiles([]string{path}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(sess.QuiesceForTest)
	name := sess.GetSessionInfo().Files[0].Path
	if _, ok := sess.AddComment(name, 1, 1, "", "keep this thread", "", ""); !ok {
		t.Fatal("comment rejected")
	}
	writeFile(t, path, "# Updated\n")
	events := sess.Subscribe()
	defer sess.Unsubscribe(events)
	if err := sess.RefreshWorkingTree(); err != nil {
		t.Fatal(err)
	}
	file, ok := sess.GetFileSnapshot(name)
	if !ok || file["content"] != "# Updated\n" {
		t.Fatalf("content not refreshed: %+v", file)
	}
	if len(sess.GetComments(name)) != 1 || sess.ReviewRound != 1 {
		t.Fatal("refresh changed comments or round")
	}
	select {
	case event := <-events:
		if event.Type != "base-changed" {
			t.Fatalf("unexpected event: %s", event.Type)
		}
	default:
		t.Fatal("other tabs were not notified")
	}
}

func TestRefreshPinnedRangeRequiresWorkingTree(t *testing.T) {
	sess := &Session{Mode: "git", Focus: Focus{Kind: FocusRange}, stale: true}
	if err := sess.RefreshWorkingTree(); err == nil || !strings.Contains(err.Error(), "pinned") {
		t.Fatalf("pinned refresh: %v", err)
	}
	if !sess.stale {
		t.Fatal("pinned refresh cleared pending updates")
	}
}

func TestRefreshFilesRecomputesCodeDiff(t *testing.T) {
	dir := newBudgetRepo(t, 25, 0)
	path := filepath.Join(dir, "src", "file3.go")
	sess, err := NewSessionFromFiles([]string{path}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(sess.QuiesceForTest)
	name := sess.GetSessionInfo().Files[0].Path
	writeFile(t, path, "package p\n\nfunc F3() int { return 333 }\n")
	if err := sess.RefreshWorkingTree(); err != nil {
		t.Fatal(err)
	}
	diff, ok := sess.GetFileDiffSnapshot(name, false)
	if !ok || !strings.Contains(fmt.Sprint(diff), "333") {
		t.Fatalf("code diff was not refreshed: %+v", diff)
	}
}
