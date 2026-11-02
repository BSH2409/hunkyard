package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/BSH2409/hunkyard/internal/daemon"
	"github.com/BSH2409/hunkyard/internal/testutil"
)

// A review started at the repository root is found from a subdirectory, the
// way `hunkyard comment` finds it.
func TestAnnotateFindsTheReviewFromASubdirectory(t *testing.T) {
	testutil.SetHome(t, t.TempDir())
	repo, err := filepath.EvalSymlinks(testutil.InitTestRepo(t))
	if err != nil {
		t.Fatal(err)
	}
	health := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}))
	defer health.Close()
	port, _ := strconv.Atoi(health.URL[strings.LastIndex(health.URL, ":")+1:])
	if err := daemon.WriteSessionFile("aaaaaaaaaaaa", daemon.SessionEntry{PID: os.Getpid(), Port: port, CWD: repo}); err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(repo, "internal", "api")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(sub)
	entry, err := annotateSession("")
	if err != nil || entry.Port != port {
		t.Fatalf("annotate from a subdirectory: %+v %v", entry, err)
	}
}

func TestAnnotateRejectsAnInvalidSessionID(t *testing.T) {
	testutil.SetHome(t, t.TempDir())
	if _, err := annotateSession("../../etc"); err == nil || !strings.Contains(err.Error(), "valid") {
		t.Fatalf("invalid --session accepted: %v", err)
	}
}
