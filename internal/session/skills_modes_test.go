package session

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/BSH2409/hunkyard/internal/daemon"
)

func TestSkillCatalogOnReviewModes(t *testing.T) {
	if testing.Short() {
		t.Skip("compiled CLI review")
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
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, "<html><body>test</body></html>")
	}))
	defer upstream.Close()

	for _, mode := range []string{"plan", "live", "preview"} {
		for _, reconnect := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/reconnect=%t", mode, reconnect), func(t *testing.T) {
				home := t.TempDir()
				repo := t.TempDir()
				target := filepath.Join(repo, "page.html")
				if err := os.WriteFile(target, []byte("<html><body>test</body></html>"), 0600); err != nil {
					t.Fatal(err)
				}
				skill := filepath.Join(home, "SKILL.md")
				if err := os.WriteFile(skill, []byte("# Naming Analyzer"), 0600); err != nil {
					t.Fatal(err)
				}
				manifest := filepath.Join(home, "skills.json")
				data, _ := json.Marshal(map[string]any{"version": 1, "skills": []map[string]string{{
					"id": "naming-analyzer", "name": "Naming Analyzer", "source": "codex", "path": skill,
				}}})
				if err := os.WriteFile(manifest, data, 0600); err != nil {
					t.Fatal(err)
				}
				listener, err := net.Listen("tcp", "127.0.0.1:0")
				if err != nil {
					t.Fatal(err)
				}
				port := listener.Addr().(*net.TCPAddr).Port
				listener.Close()
				if mode != "plan" {
					next, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port+1))
					if err != nil {
						t.Skipf("adjacent port unavailable: %v", err)
					}
					next.Close()
				}
				args := []string{mode, target, "--host", "127.0.0.1", "--port", fmt.Sprint(port), "--no-open"}
				if mode == "live" {
					args[1] = upstream.URL
				}
				if !reconnect {
					args = append(args, "--skills-file", manifest)
				}
				env := append(clientFinishEnv(t, home), "HUNKYARD_OWNER_ID=codex:skills-modes", "HUNKYARD_IDLE_PAUSE_MINUTES=0")
				start := func(args []string) *exec.Cmd {
					t.Helper()
					cmd := exec.Command(binary, args...)
					cmd.Dir = repo
					cmd.Env = env
					if err := cmd.Start(); err != nil {
						t.Fatal(err)
					}
					t.Cleanup(func() {
						_ = cmd.Process.Kill()
						_ = cmd.Wait()
					})
					return cmd
				}
				start(args)
				var entry daemon.SessionEntry
				key := ""
				pollHost(t, func() bool {
					files, _ := os.ReadDir(filepath.Join(home, ".hunkyard", "sessions"))
					for _, file := range files {
						if !strings.HasSuffix(file.Name(), ".json") {
							continue
						}
						content, err := os.ReadFile(filepath.Join(home, ".hunkyard", "sessions", file.Name()))
						if err == nil && json.Unmarshal(content, &entry) == nil && entry.Port > 0 {
							key = strings.TrimSuffix(file.Name(), ".json")
							return true
						}
					}
					return false
				})
				t.Cleanup(func() {
					stop := exec.Command(binary, "stop", "--session", key)
					stop.Dir = repo
					stop.Env = env
					_ = stop.Run()
				})
				waitForSessionReady(t, entry.Port)
				if reconnect {
					start(append(args, "--skills-file", manifest))
				}
				pollHost(t, func() bool {
					response, err := http.Get(entry.ConnURL() + "/api/skills")
					if err != nil {
						return false
					}
					defer response.Body.Close()
					var catalog struct {
						Skills []struct {
							ID string `json:"id"`
						} `json:"skills"`
					}
					return response.StatusCode == http.StatusOK && json.NewDecoder(response.Body).Decode(&catalog) == nil &&
						len(catalog.Skills) == 1 && catalog.Skills[0].ID == "naming-analyzer"
				})
				response, err := http.Post(entry.ConnURL()+"/api/finish", "application/json", bytes.NewBufferString(`{"request_id":"skills-check","disposition":"comment","skill_ids":["naming-analyzer"]}`))
				if err != nil {
					t.Fatal(err)
				}
				defer response.Body.Close()
				var result struct {
					Batch struct {
						SkillIDs []string `json:"skill_ids"`
					} `json:"batch"`
				}
				if response.StatusCode != http.StatusOK || json.NewDecoder(response.Body).Decode(&result) != nil ||
					len(result.Batch.SkillIDs) != 1 || result.Batch.SkillIDs[0] != "naming-analyzer" {
					t.Fatalf("publication status %d, skills %v", response.StatusCode, result.Batch.SkillIDs)
				}
			})
		}
	}
}
