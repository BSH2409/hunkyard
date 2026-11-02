package daemon

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/BSH2409/hunkyard/internal/feedback"
	"github.com/BSH2409/hunkyard/internal/localonly"
	"github.com/BSH2409/hunkyard/internal/testutil"
)

// Another agent session's review in the same repository must not receive this
// session's `hunkyard comment` from a subdirectory.
func TestListSessionsForRepoRootSkipsOtherOwners(t *testing.T) {
	if os.Getenv("HUNKYARD_REPO_OWNER_TEST") != "1" {
		command := exec.Command(os.Args[0], "-test.run=^TestListSessionsForRepoRootSkipsOtherOwners$", "-test.count=1")
		command.Env = append(os.Environ(), "HUNKYARD_REPO_OWNER_TEST=1")
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("%s: %v", output, err)
		}
		return
	}
	localonly.Install()
	home := t.TempDir()
	testutil.SetHome(t, home)
	t.Setenv("HUNKYARD_OWNER_ID", "claude:mine")
	health := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}))
	defer health.Close()
	port, _ := strconv.Atoi(health.URL[strings.LastIndex(health.URL, ":")+1:])
	owners := map[string]feedback.Identity{
		"aaaaaaaaaaaa": feedback.CurrentIdentity(),
		"bbbbbbbbbbbb": {Key: "other", Kind: "claude"},
	}
	for key, identity := range owners {
		if _, err := feedback.OpenOwner(filepath.Join(home, ".hunkyard"), key, identity); err != nil {
			t.Fatal(err)
		}
		if err := WriteSessionFile(key, SessionEntry{PID: os.Getpid(), Port: port, CWD: "/tmp/repo/api"}); err != nil {
			t.Fatal(err)
		}
	}
	_, keys := ListSessionsForRepoRoot("/tmp/repo")
	if len(keys) != 1 || keys[0] != "aaaaaaaaaaaa" {
		t.Fatalf("keys = %v, want only this session's review", keys)
	}
}
