package preferences

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/BSH2409/hunkyard/internal/config"
)

func storeWithPending(t *testing.T) *Store {
	t.Helper()
	s := testStore(t)
	if _, err := s.Capture(captureInput()); err != nil {
		t.Fatal(err)
	}
	return s
}

func validProposalRunner(t *testing.T, s *Store, calls *int) maintenanceRunner {
	return func(ctx context.Context, argv, env []string, dir string, stdin []byte) ([]byte, error) {
		*calls++
		if !slices.Contains(env, "LEARN_PREFERENCES_MAINTENANCE=1") || !strings.Contains(string(stdin), "Return only the proposal JSON object") {
			t.Fatal("runner missing maintenance env or instructions")
		}
		batch, _, err := s.buildMaintenanceBatch(false)
		if err != nil {
			t.Fatal(err)
		}
		return ignoredProposal(t, batch), nil
	}
}

var claudeMaintenance = config.PreferencesMaintenance{Host: "claude"}

func TestAutomaticMaintenanceAppliesValidProposal(t *testing.T) {
	s := storeWithPending(t)
	calls := 0
	result, err := s.runAutomaticMaintenance(context.Background(), claudeMaintenance, false, validProposalRunner(t, s, &calls))
	if err != nil || result["status"] != "success" || calls != 1 {
		t.Fatalf("result %v err %v calls %d", result, err, calls)
	}
	if s.unprocessedCount(mustCurrent(t, s)) != 0 {
		t.Fatal("pending work not consumed")
	}
	run := s.lastMaintenanceRun()
	if run.Outcome != "success" || run.Model != "claude-opus-5-5" || run.Effort != "high" {
		t.Fatalf("run not recorded: %+v", run)
	}
}

func mustCurrent(t *testing.T, s *Store) *generation {
	t.Helper()
	current, err := s.loadCurrent()
	if err != nil {
		t.Fatal(err)
	}
	return current
}

func TestAutomaticMaintenanceRetriesOnceOnRejection(t *testing.T) {
	s := storeWithPending(t)
	calls := 0
	valid := validProposalRunner(t, s, &calls)
	runner := func(ctx context.Context, argv, env []string, dir string, stdin []byte) ([]byte, error) {
		if calls == 0 {
			calls++
			return []byte(`{"schema_version":1,"input_digest":"0000000000000000000000000000000000000000000000000000000000000000","actions":[],"summary":{}}`), nil
		}
		if !strings.Contains(string(stdin), "previous proposal was rejected") {
			t.Fatal("retry did not include the validation error")
		}
		return valid(ctx, argv, env, dir, stdin)
	}
	result, err := s.runAutomaticMaintenance(context.Background(), claudeMaintenance, false, runner)
	if err != nil || result["status"] != "success" || calls != 2 {
		t.Fatalf("result %v err %v calls %d", result, err, calls)
	}
}

func TestAutomaticMaintenanceFencedOutput(t *testing.T) {
	s := storeWithPending(t)
	calls := 0
	valid := validProposalRunner(t, s, &calls)
	runner := func(ctx context.Context, argv, env []string, dir string, stdin []byte) ([]byte, error) {
		out, err := valid(ctx, argv, env, dir, stdin)
		return []byte("Here is the proposal:\n```json\n" + string(out) + "\n```\n"), err
	}
	if result, err := s.runAutomaticMaintenance(context.Background(), claudeMaintenance, false, runner); err != nil || result["status"] != "success" {
		t.Fatalf("fenced output rejected: %v %v", result, err)
	}
}

func TestAutomaticMaintenanceAuthFailureRecorded(t *testing.T) {
	s := storeWithPending(t)
	runner := func(ctx context.Context, argv, env []string, dir string, stdin []byte) ([]byte, error) {
		return nil, errors.New("exit status 1: 401 Unauthorized: please run login")
	}
	result, _ := s.runAutomaticMaintenance(context.Background(), claudeMaintenance, false, runner)
	if result["status"] != "failed" || s.lastMaintenanceRun().ErrorClass != "auth_failed" {
		t.Fatalf("auth failure not classified: %v %+v", result, s.lastMaintenanceRun())
	}
	events, _ := s.Activity(1)
	if len(events) != 1 || events[0].Kind != activityMaintenanceFailed {
		t.Fatalf("failure not in activity: %+v", events)
	}
	if !strings.Contains(s.healthLine(mustCurrent(t, s)), "auth_failed") {
		t.Fatalf("health line silent about failure: %q", s.healthLine(mustCurrent(t, s)))
	}
}

func TestAutomaticMaintenanceNoWork(t *testing.T) {
	s := testStore(t)
	runner := func(ctx context.Context, argv, env []string, dir string, stdin []byte) ([]byte, error) {
		t.Fatal("runner called without pending work")
		return nil, nil
	}
	if result, err := s.runAutomaticMaintenance(context.Background(), claudeMaintenance, false, runner); err != nil || result["status"] != "no_work" {
		t.Fatalf("result %v err %v", result, err)
	}
}

func TestMaintenanceArgv(t *testing.T) {
	claude := maintenanceArgv(config.PreferencesMaintenance{Host: "claude"}.Resolved())
	if strings.Join(claude, " ") != "claude -p --model claude-opus-5-5 --effort high --output-format text" {
		t.Fatalf("claude argv %v", claude)
	}
	codex := maintenanceArgv(config.PreferencesMaintenance{Host: "codex"}.Resolved())
	if strings.Join(codex, " ") != `codex exec -m gpt-6.1-sol -c model_reasoning_effort="high" -s read-only --skip-git-repo-check -` {
		t.Fatalf("codex argv %v", codex)
	}
}

func writeGlobalConfig(t *testing.T, body string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	if err := os.WriteFile(filepath.Join(home, ".hunkyard.config.json"), []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestSessionStartStartsMaintenanceWhenDue(t *testing.T) {
	s := storeWithPending(t)
	writeGlobalConfig(t, `{"preferences_maintenance":{"host":"codex"}}`)
	var started []string
	previous := startDetached
	startDetached = func(argv []string, logPath string) error { started = argv; return nil }
	t.Cleanup(func() { startDetached = previous })
	sessionStartResponse(t, s, t.TempDir())
	if !strings.Contains(strings.Join(started, " "), "preferences maintain --auto --json --root") {
		t.Fatalf("maintenance not started: %v", started)
	}
}

func TestSessionStartSkipsMaintenanceWhenNotConfigured(t *testing.T) {
	s := storeWithPending(t)
	writeGlobalConfig(t, `{}`)
	called := false
	previous := startDetached
	startDetached = func(argv []string, logPath string) error { called = true; return nil }
	t.Cleanup(func() { startDetached = previous })
	sessionStartResponse(t, s, t.TempDir())
	if called {
		t.Fatal("maintenance started without a configured host")
	}
}

func TestMaintainAutoWithoutHostKeepsLocalRequest(t *testing.T) {
	s := storeWithPending(t)
	writeGlobalConfig(t, `{}`)
	var out bytes.Buffer
	if exit := Run([]string{"maintain", "--auto", "--root", s.config.Root, "--json"}, strings.NewReader(""), &out, &bytes.Buffer{}); exit != 0 {
		t.Fatalf("exit %d: %s", exit, out.String())
	}
	if !strings.Contains(out.String(), `"needs_proposal"`) {
		t.Fatalf("without a host, --auto must behave like --due: %s", out.String())
	}
}

func TestCronUsesAutoMaintenance(t *testing.T) {
	s := testStore(t)
	rendered, err := s.renderCron("")
	if err != nil || !strings.Contains(rendered, "preferences maintain --auto --json --root") {
		t.Fatalf("cron line: %q %v", rendered, err)
	}
}
