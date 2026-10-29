package daemon

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/BSH2409/hunkyard/internal/testutil"
	"time"
)

func TestDaemonLifecycle(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping daemon lifecycle test in short mode")
	}
	if runtime.GOOS == "windows" {
		// TODO(windows): re-evaluate now that terminateProcess uses
		// GenerateConsoleCtrlEvent(CTRL_BREAK_EVENT) before falling back to
		// TerminateProcess — the daemon's signal handler should now run on
		// shutdown and let cmd.Wait return cleanly. The detached child +
		// readiness-pipe (ExtraFiles FD 3) combo still produces
		// subprocess-lifecycle behavior that differs from POSIX, so this
		// suite remains skipped pending hands-on validation on a Windows
		// runner. The runtime daemon code path is still exercised by
		// daemon_test.go unit tests on Windows.
		t.Skip("daemon E2E spawn/wait/kill semantics differ on Windows; covered by daemon_test.go unit tests; TODO: revisit after CTRL_BREAK shutdown lands")
	}

	// Build hunkyard binary
	dir := t.TempDir()
	binaryName := "hunkyard"
	if runtime.GOOS == "windows" {
		binaryName = "hunkyard.exe"
	}
	binary := filepath.Join(dir, binaryName)
	_, testFile, _, _ := runtime.Caller(0)
	moduleRoot := filepath.Clean(filepath.Join(filepath.Dir(testFile), "..", ".."))
	build := exec.Command("go", "build", "-o", binary, "./cmd/hunkyard")
	build.Dir = moduleRoot
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build failed: %v\n%s", err, out)
	}
	if err := os.Chmod(binary, 0o755); err != nil {
		t.Fatalf("chmod binary: %v", err)
	}

	// Create a test repo with a file
	repoDir := t.TempDir()
	testutil.Git(t, repoDir, "init")
	testutil.Git(t, repoDir, "config", "user.email", "test@test.com")
	testutil.Git(t, repoDir, "config", "user.name", "Test")
	testutil.Git(t, repoDir, "checkout", "-b", "main")
	testutil.WriteFile(t, filepath.Join(repoDir, "test.md"), "# Hello\n")
	testutil.Git(t, repoDir, "add", ".")
	testutil.Git(t, repoDir, "commit", "-m", "init")

	// Make a change so hunkyard has something to review
	testutil.WriteFile(t, filepath.Join(repoDir, "test.md"), "# Hello\n\nWorld\n")

	// Resolve symlinks so the session key matches (macOS: /var → /private/var)
	repoDir, _ = filepath.EvalSymlinks(repoDir)

	// Use temp HOME so session files don't pollute real HOME
	homeDir := t.TempDir()
	homeDir, _ = filepath.EvalSymlinks(homeDir)
	t.Setenv("HOME", homeDir)
	if runtime.GOOS == "windows" {
		t.Setenv("USERPROFILE", homeDir)
	}
	// A git process can outlive the daemon by a moment and still be writing
	// into these dirs, which makes t.TempDir's RemoveAll fail with "directory
	// not empty". Cleanups run in reverse order, so this one runs first and
	// retries until the dirs go.
	t.Cleanup(func() { removeAllEventually(t, dir, repoDir, homeDir) })

	// Compute expected session key (cwd=repoDir, branch=main, args=[])
	key := SessionKey(repoDir, "main", nil)
	sessDir := filepath.Join(homeDir, ".hunkyard", "sessions")
	sessionPath := filepath.Join(sessDir, key+".json")

	// Start daemon via _serve
	cmd := exec.Command(binary, "_serve", "--no-open", "--port", "0")
	cmd.Dir = repoDir
	// Filter existing HOME (and Windows equivalents) so our override takes
	// effect. On Windows os.UserHomeDir reads USERPROFILE / HOMEDRIVE+HOMEPATH;
	// without filtering them, the spawned daemon would still resolve the
	// runner's real profile and write its session file there.
	var env []string
	for _, e := range os.Environ() {
		if strings.HasPrefix(e, "HOME=") {
			continue
		}
		if runtime.GOOS == "windows" && (strings.HasPrefix(e, "USERPROFILE=") ||
			strings.HasPrefix(e, "HOMEDRIVE=") || strings.HasPrefix(e, "HOMEPATH=")) {
			continue
		}
		env = append(env, e)
	}
	// The startup integration check runs `<agent> --version` for every AI CLI
	// on the developer's PATH (seconds), delaying readiness past the waits
	// below; the update check calls the network.
	//
	// Git mode looks up the branch's PR (`gh auth status`, `gh pr view`);
	// failing stubs keep that off the developer's real gh and the network.
	env = append(env, "HOME="+homeDir, "HUNKYARD_NO_INTEGRATION_CHECK=1", "HUNKYARD_NO_UPDATE_CHECK=1",
		testutil.FailingForgeCLIsPathEnv(t))
	if runtime.GOOS == "windows" {
		env = append(env, "USERPROFILE="+homeDir)
	}
	cmd.Env = env
	var stderrBuf bytes.Buffer
	cmd.Stderr = &stderrBuf
	if err := cmd.Start(); err != nil {
		t.Fatalf("start daemon: %v", err)
	}
	defer cmd.Process.Kill()

	// Wait for session file
	var entry SessionEntry
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		time.Sleep(100 * time.Millisecond)
		data, err := os.ReadFile(sessionPath)
		if err != nil {
			continue
		}
		if err := json.Unmarshal(data, &entry); err != nil {
			continue
		}
		if entry.Port > 0 {
			break
		}
	}
	if entry.Port == 0 {
		t.Fatalf("daemon did not write session file at %s\nstderr: %s", sessionPath, stderrBuf.String())
	}

	// Verify session file contents
	if entry.PID != cmd.Process.Pid {
		t.Errorf("session PID %d doesn't match process PID %d", entry.PID, cmd.Process.Pid)
	}
	if entry.Host != "0.0.0.0" {
		t.Fatalf("listener host = %q, want Docker wildcard", entry.Host)
	}
	if entry.CWD != repoDir {
		t.Errorf("session CWD %q doesn't match repo dir %q", entry.CWD, repoDir)
	}

	// Verify health endpoint
	resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/api/health", entry.Port))
	if err != nil {
		t.Fatalf("health check failed: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("health check: got %d, want 200", resp.StatusCode)
	}

	// Wait for the session to be fully initialized (server returns 503 while loading)
	readyDeadline := time.Now().Add(10 * time.Second)
	sessionReady := false
	for time.Now().Before(readyDeadline) {
		resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/api/session", entry.Port))
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == 200 {
				sessionReady = true
				break
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !sessionReady {
		t.Fatalf("session did not become ready within 10s")
	}

	// Publish with no waiter, then reconnect through the authenticated queue.
	finishURL := fmt.Sprintf("http://127.0.0.1:%d/api/finish", entry.Port)
	published, err := http.Post(finishURL, "application/json", strings.NewReader(`{"request_id":"lifecycle-test","disposition":"approve","summary":""}`))
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(published.Body)
	published.Body.Close()
	if published.StatusCode != 200 {
		t.Fatalf("publish: %d %s", published.StatusCode, body)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	response, err := OwnerRequest(ctx, entry, key, http.MethodGet, "/api/feedback/await?timeout=1ms", nil)
	if err != nil {
		t.Fatal(err)
	}
	body, _ = io.ReadAll(response.Body)
	response.Body.Close()
	var result struct {
		Status  string `json:"status"`
		Batches []struct {
			Disposition string `json:"disposition"`
			ID          string `json:"id"`
		} `json:"batches"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatal(err)
	}
	if result.Status != "published" || len(result.Batches) != 1 || result.Batches[0].Disposition != "approve" {
		t.Fatalf("queued publication: %s", body)
	}
	response, err = OwnerRequest(ctx, entry, key, http.MethodPost, "/api/feedback/ack", strings.NewReader(`{"batch_id":"`+result.Batches[0].ID+`"}`))
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != 200 {
		t.Fatalf("ack: %d", response.StatusCode)
	}
	response, err = OwnerRequest(ctx, entry, key, http.MethodGet, "/api/feedback/await?timeout=1ms", nil)
	if err != nil {
		t.Fatal(err)
	}
	body, _ = io.ReadAll(response.Body)
	response.Body.Close()
	if !strings.Contains(string(body), `"status":"pending"`) {
		t.Fatalf("acknowledged batch returned: %s", body)
	}

	// Kill daemon
	cmd.Process.Signal(os.Interrupt)
	exited := make(chan error, 1)
	go func() { exited <- cmd.Wait() }()
	select {
	case <-exited:
	case <-time.After(10 * time.Second):
		t.Fatalf("daemon did not exit within 10s of SIGINT\nstderr: %s", stderrBuf.String())
	}

	// Session file should be cleaned up
	if _, err := os.Stat(sessionPath); !os.IsNotExist(err) {
		t.Error("session file not cleaned up after daemon shutdown")
	}
}

// removeAllEventually removes dirs, retrying for a few seconds while a
// process that outlived the test finishes writing into them.
func removeAllEventually(t *testing.T, dirs ...string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for _, dir := range dirs {
		for {
			err := os.RemoveAll(dir)
			if err == nil {
				break
			}
			if time.Now().After(deadline) {
				var left []string
				_ = filepath.WalkDir(dir, func(path string, _ os.DirEntry, _ error) error {
					left = append(left, path)
					return nil
				})
				t.Errorf("remove %s: %v\nstill present: %v", dir, err, left)
				break
			}
			time.Sleep(50 * time.Millisecond)
		}
	}
}
