package vcs

import (
	"context"
	"path/filepath"
	"testing"
)

func TestGrepTrackedIgnoresUntrackedAndRespectsExclude(t *testing.T) {
	dir := initTestRepo(t)
	writeFile(t, filepath.Join(dir, "api.go"), "package a\n\nfunc ServeLogin() {}\n")
	writeFile(t, filepath.Join(dir, "caller.go"), "package a\n\nfunc x() { ServeLogin() }\n// ServeLoginLater is a different word\n")
	writeFile(t, filepath.Join(dir, "web: odd name.go"), "ServeLogin()\n")
	gitT(t, dir, "add", ".")
	gitT(t, dir, "commit", "-m", "grep fixture")
	writeFile(t, filepath.Join(dir, "untracked.go"), "ServeLogin()\n")

	hits, err := GrepTracked(context.Background(), dir, []string{"ServeLogin"}, "api.go")
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]int{"caller.go": 3, "web: odd name.go": 1}
	if len(hits) != len(want) {
		t.Fatalf("hits = %+v", hits)
	}
	for _, h := range hits {
		if want[h.Path] != h.Line || h.Text == "" {
			t.Fatalf("hit = %+v", h)
		}
	}
	none, err := GrepTracked(context.Background(), dir, []string{"NothingMatchesThis"}, "")
	if err != nil || len(none) != 0 {
		t.Fatalf("no-match = %+v %v", none, err)
	}
	if empty, err := GrepTracked(context.Background(), dir, nil, ""); err != nil || len(empty) != 0 {
		t.Fatalf("no words = %+v %v", empty, err)
	}
}
