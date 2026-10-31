package session

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/BSH2409/hunkyard/internal/daemon"
	"github.com/BSH2409/hunkyard/internal/feedback"
	"github.com/BSH2409/hunkyard/internal/preferences"
)

// This exercises the real binary/protocol; it does not claim a live model applied a skill.
func TestHostRoundLoopProtocol(t *testing.T) {
	if testing.Short() {
		t.Skip("compiled CLI round loop")
	}
	if !preferences.SupportedPlatform {
		t.Skip("the round loop learns preferences: " + preferences.ErrUnsupportedPlatform.Error())
	}
	_, file, _, _ := runtime.Caller(0)
	module := filepath.Clean(filepath.Join(filepath.Dir(file), "../.."))
	binary := filepath.Join(t.TempDir(), "hunkyard")
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	build := exec.Command("go", "build", "-o", binary, "./cmd/hunkyard")
	build.Dir = module
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v %s", err, output)
	}
	for _, kind := range []string{"codex", "claude"} {
		t.Run(kind, func(t *testing.T) {
			home := t.TempDir()
			repo := initTestRepo(t)
			writeFile(t, filepath.Join(repo, "doc.md"), "# Review me\n")
			gitT(t, repo, "add", "doc.md")
			gitT(t, repo, "commit", "-m", "review fixture")
			writeFile(t, filepath.Join(repo, "doc.md"), "# Review me\n\nChanged\n")
			skill := filepath.Join(home, "naming.md")
			writeFile(t, skill, "# Naming\nPrefer names describing domain behavior.\n")
			manifest := filepath.Join(home, "skills.json")
			data, _ := json.Marshal(map[string]any{"version": 1, "skills": []feedback.SkillRef{{ID: "naming", Name: "Naming", Path: skill, Source: kind}}})
			writeFile(t, manifest, string(data))
			env := append(clientFinishEnv(t, home), "HUNKYARD_OWNER_ID="+kind+":round-loop", "LEARN_PREFERENCES_HOME="+filepath.Join(home, "agents"))
			start := exec.Command(binary, "_serve", "--no-open", "--port", "0", "--skills-file", manifest)
			start.Dir = repo
			start.Env = env
			if err := start.Start(); err != nil {
				t.Fatal(err)
			}
			serverStopped := false
			t.Cleanup(func() {
				if serverStopped {
					return
				}
				start.Process.Signal(os.Interrupt)
				done := make(chan error, 1)
				go func() { done <- start.Wait() }()
				select {
				case <-done:
				case <-time.After(5 * time.Second):
					start.Process.Kill()
					<-done
				}
			})
			var entry daemon.SessionEntry
			key := ""
			pollHost(t, func() bool {
				files, _ := os.ReadDir(filepath.Join(home, ".hunkyard", "sessions"))
				for _, file := range files {
					if !strings.HasSuffix(file.Name(), ".json") {
						continue
					}
					data, _ := os.ReadFile(filepath.Join(home, ".hunkyard", "sessions", file.Name()))
					var candidate daemon.SessionEntry
					if json.Unmarshal(data, &candidate) == nil && candidate.PID == start.Process.Pid {
						entry = candidate
						key = strings.TrimSuffix(file.Name(), ".json")
						return true
					}
				}
				return false
			})
			waitForSessionReady(t, entry.Port)
			command := func(args ...string) []byte {
				t.Helper()
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				cmd := exec.CommandContext(ctx, binary, args...)
				cmd.Dir = repo
				cmd.Env = env
				var output, errors bytes.Buffer
				cmd.Stdout = &output
				cmd.Stderr = &errors
				if err := cmd.Run(); err != nil {
					t.Fatalf("hunkyard %s: %v %s %s", strings.Join(args, " "), err, output.String(), errors.String())
				}
				return output.Bytes()
			}
			pending := command("feedback", "await", "--session", key, "--timeout", "1ms", "--json")
			if !bytes.Contains(pending, []byte(`"status":"pending"`)) {
				t.Fatalf("initial wait: %s", pending)
			}
			post := func(path string, value any) []byte {
				t.Helper()
				body, _ := json.Marshal(value)
				resp, err := http.Post(entry.ConnURL()+path, "application/json", bytes.NewReader(body))
				if err != nil {
					t.Fatal(err)
				}
				defer resp.Body.Close()
				output, _ := io.ReadAll(resp.Body)
				if resp.StatusCode < 200 || resp.StatusCode > 299 {
					t.Fatalf("POST %s: %d %s", path, resp.StatusCode, output)
				}
				return output
			}
			posted := post("/api/file/comments?path=doc.md", map[string]any{"start_line": 1, "end_line": 1, "body": "Always prefer names describing domain behavior."})
			var comment struct {
				ID string `json:"id"`
			}
			if err := json.Unmarshal(posted, &comment); err != nil || comment.ID == "" {
				t.Fatalf("comment: %s %v", posted, err)
			}

			post("/api/finish", map[string]any{"request_id": "host-first", "disposition": "request_changes", "skill_ids": []string{"naming"}})
			foreign := exec.Command(binary, "comment", "--session", key, "--reply-to", comment.ID, "--author", kind, "Wrong session must not reply")
			foreign.Dir = repo
			foreign.Env = append(append([]string{}, env...), "HUNKYARD_OWNER_ID="+kind+":other-session")
			if output, err := foreign.CombinedOutput(); err == nil {
				t.Fatalf("another host session replied to this review: %s", output)
			} else {
				t.Logf("other-owner reply rejected: %s", output)
			}
			// Restart after publishing, with no waiter; queued feedback and the original skills must survive.
			// The shutdown endpoint stops the daemon gracefully on every OS; Windows cannot send os.Interrupt.
			if resp, err := http.Post(entry.ConnURL()+"/api/shutdown?pid="+strconv.Itoa(start.Process.Pid), "application/json", nil); err == nil {
				resp.Body.Close()
			}
			stopped := make(chan error, 1)
			go func() { stopped <- start.Wait() }()
			select {
			case <-stopped:
				serverStopped = true
			case <-time.After(5 * time.Second):
				t.Fatal("daemon did not stop for reconnect test")
			}
			output := command("--session", key, "--port", "0", "--no-open")
			entryData, err := os.ReadFile(filepath.Join(home, ".hunkyard", "sessions", key+".json"))
			if err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(entryData, &entry); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { command("stop", "--session", key) })
			if entry.Port == 5000 {
				t.Fatal("explicit --port 0 was ignored on reconnect")
			}

			var result struct {
				Batches []feedback.Batch               `json:"batches"`
				Skills  map[string][]feedback.SkillRef `json:"selected_skills"`
			}
			if err := json.Unmarshal(output, &result); err != nil || len(result.Batches) != 1 {
				t.Fatalf("feedback: %s %v", output, err)
			}
			batch := result.Batches[0]
			if len(result.Skills[batch.ID]) != 1 || result.Skills[batch.ID][0].Path != skill {
				t.Fatal("selected host skill lost provenance")
			}
			decisions := filepath.Join(home, "decisions.json")
			data, _ = json.Marshal([]feedback.PreferenceDecision{{CommentID: comment.ID, Revision: 1, Kind: "directive", Directive: "Prefer names describing domain behavior.", Scope: "repository", Surfaces: []string{"naming"}}})
			writeFile(t, decisions, string(data))
			command("feedback", "learn", "--session", key, "--batch", batch.ID, "--decisions", decisions, "--json")
			if _, err := os.Stat(filepath.Join(home, "agents", "learn-preferences", "current", "rules.yml")); err != nil {
				t.Fatal(err)
			}
			command("comment", "--session", key, "--reply-to", comment.ID, "--author", kind, "Applied the naming guidance.")
			command("comment", "--session", key, "--author", kind, "Always prefer this synthetic agent-only instruction.")
			command("feedback", "ack", "--session", key, "--batch", batch.ID, "--json")
			next := exec.Command(binary, "--session", key, "--no-open")
			next.Env = env
			next.Dir = repo
			var nextOutput bytes.Buffer
			next.Stdout = &nextOutput
			var nextError bytes.Buffer
			next.Stderr = &nextError
			if err := next.Start(); err != nil {
				t.Fatal(err)
			}
			done := make(chan error, 1)
			go func() { done <- next.Wait() }()
			t.Cleanup(func() {
				next.Process.Kill()
				select {
				case <-done:
				default:
				}
			})
			pollHost(t, func() bool {
				resp, err := http.Get(entry.ConnURL() + "/api/session")
				if err != nil {
					return false
				}
				defer resp.Body.Close()
				var session struct {
					Round int `json:"review_round"`
				}
				json.NewDecoder(resp.Body).Decode(&session)
				return session.Round > batch.Round
			})
			post("/api/finish", map[string]any{"request_id": "host-approval", "disposition": "approve"})
			select {
			case err := <-done:
				if err != nil {
					t.Fatalf("next round: %v %s", err, nextError.String())
				}
			case <-time.After(10 * time.Second):
				t.Fatal("second-round waiter did not receive approval")
			}
			if !strings.Contains(nextOutput.String(), `"disposition":"approve"`) {
				t.Fatalf("second round: %s", nextOutput.String())
			}
			var approval struct {
				Batches []feedback.Batch `json:"batches"`
			}
			if err := json.Unmarshal(nextOutput.Bytes(), &approval); err != nil || len(approval.Batches) != 1 {
				t.Fatalf("approval: %s %v", nextOutput.String(), err)
			}
			if approval.Batches[0].Round <= batch.Round || len(approval.Batches[0].Comments) != 0 {
				t.Fatalf("second round repeated human revisions or learned from agent replies: first=%+v next=%+v", batch, approval.Batches[0])
			}
			command("feedback", "ack", "--session", key, "--batch", approval.Batches[0].ID, "--json")
			if output := command("feedback", "await", "--session", key, "--timeout", "1ms", "--json"); !bytes.Contains(output, []byte(`"status":"pending"`)) {
				t.Fatalf("approval queue not acknowledged: %s", output)
			}
		})
	}
}

func pollHost(t *testing.T, ready func() bool) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	for {
		if ready() {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatal("host protocol did not reach expected state within 15s")
		case <-ticker.C:
		}
	}
}

func TestHostCarryForwardPreservesAuthoredContentIdentity(t *testing.T) {
	original := Comment{ID: "human", ContentID: "authored-content", ViewID: "authored-view", Body: "Use clear names"}
	carried := carryForwardComment(original, "2026-10-18T00:00:00Z")
	if carried.ContentID != original.ContentID || carried.ViewID != original.ViewID {
		t.Fatal("round carry-forward lost the authored content/view identity")
	}
}
