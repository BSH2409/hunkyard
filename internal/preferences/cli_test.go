package preferences

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestPreferencesCLIReferenceParity(t *testing.T) {
	var fixture struct {
		Cases []struct {
			Args          []string
			Stdin, Stdout string
			Exit          int
		}
	}
	readContract(t, "cli.json", &fixture)
	for _, c := range fixture.Cases {
		t.Run(strings.Join(c.Args, " "), func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "codex")
			t.Setenv("HOME", t.TempDir())
			t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(t.TempDir(), "claude"))
			if c.Args[0] == "status" && len(c.Args) == 2 && c.Args[1] == "--json" {
				if runtime.GOOS == "windows" {
					t.Skip("POSIX crontab fixture")
				}
				bin := t.TempDir()
				if err := os.WriteFile(filepath.Join(bin, "crontab"), []byte("#!/bin/sh\nprintf '%s\\n' '"+cronBegin+"' '"+cronEnd+"'\n"), 0700); err != nil {
					t.Fatal(err)
				}
				t.Setenv("PATH", bin)
			}
			var out, errOut bytes.Buffer
			args := append([]string{}, c.Args...)
			if args[0] != "missing-command" {
				args = append(args, "--root", root)
			}
			exit := Run(args, strings.NewReader(c.Stdin), &out, &errOut)
			var expected, actual any
			if json.Unmarshal([]byte(c.Stdout), &expected) != nil || json.Unmarshal(out.Bytes(), &actual) != nil {
				t.Fatal("invalid JSON output")
			}
			if exit != c.Exit || !sameValue(actual, expected) {
				t.Fatalf("exit %d output %s; expected exit %d output %s", exit, out.String(), c.Exit, c.Stdout)
			}
		})
	}
}

func TestPreferencesCLIFlagOrderAndRepeatedModes(t *testing.T) {
	s := testStore(t)
	input := captureInput()
	input.Explicit = true
	input.Scope = "global"
	input.RepoKey = ""
	input.Directive = "Prefer early returns."
	input.Surfaces = []string{"ruby"}
	if _, err := s.Capture(input); err != nil {
		t.Fatal(err)
	}
	current, err := s.loadCurrent()
	if err != nil {
		t.Fatal(err)
	}
	id := textField(current.Rules.rules[0]["id"])
	var out, errOut bytes.Buffer
	if exit := Run([]string{"retire", "--root", s.config.Root, id, "--json"}, strings.NewReader(""), &out, &errOut); exit != 0 {
		t.Fatalf("interspersed flags rejected: exit %d %s", exit, out.String())
	}
	for _, args := range [][]string{{"maintain", "--due", "--due"}, {"maintain", "--force", "--due"}, {"maintain", "--due=false"}} {
		out.Reset()
		args = append(args, "--root", s.config.Root, "--json")
		if exit := Run(args, strings.NewReader(""), &out, &errOut); exit != 2 {
			t.Fatalf("invalid mode selection mutated maintenance: exit %d %s", exit, out.String())
		}
	}
	if _, err := os.Stat(s.dataPath("proposals")); !os.IsNotExist(err) {
		t.Fatal("invalid mode created proposal state")
	}
}

func TestPreferencesCLIStateAndSafeFailureCodes(t *testing.T) {
	root := filepath.Join(t.TempDir(), "codex")
	t.Setenv("HOME", t.TempDir())
	t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(t.TempDir(), "claude"))
	if runtime.GOOS == "windows" {
		t.Skip("POSIX crontab fixture")
	}
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "crontab"), []byte("#!/bin/sh\nprintf 'no crontab for isolated fixture\\n' >&2\nexit 1\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	run := func(want int, input string, args ...string) map[string]any {
		t.Helper()
		var out, errOut bytes.Buffer
		exit := Run(append(args, "--root", root, "--json"), strings.NewReader(input), &out, &errOut)
		if exit != want {
			t.Fatalf("%s: exit %d, want %d; %s %s", args[0], exit, want, out.String(), errOut.String())
		}
		var result map[string]any
		if err := json.Unmarshal(out.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(out.String(), "Always prefer focused tests") || strings.Contains(out.String(), "password=synthetic-value") {
			t.Fatal("CLI output exposed feedback")
		}
		return result
	}
	if run(0, "Always prefer focused tests", "capture")["action"] != "created" {
		t.Fatal("stdin feedback lost")
	}
	run(2, "", "capture", "--explicit")
	run(3, "", "capture", "--feedback", "Always use password=synthetic-value")
	run(0, "", "capture", "--feedback", "Always use explicit service contracts", "--explicit", "--directive", "Use explicit service contracts.", "--scope", "global", "--surface", "services")
	run(0, "", "build")
	run(0, "", "audit")
	status := run(0, "", "status")
	if status["pending_count"] != float64(2) || len(status) != 12 || status["hook_trust"] != "unknown" || status["cron_installed"] != false {
		t.Fatal("status leaked records or lost operational fields", status)
	}
	found := run(0, "", "search", "--query", "explicit service")
	rules, err := mapArray(found["rules"])
	if err != nil || len(rules) != 1 {
		t.Fatal("CLI search failed", err)
	}
	run(0, "", "retire", textField(rules[0]["id"]))
	run(0, "", "audit")
	if err := os.WriteFile(filepath.Join(root, "persona", "core.md"), []byte("manual guidance"), 0600); err != nil {
		t.Fatal(err)
	}
	run(5, "", "audit")
	s, err := Open(Config{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	legacyFixture(t, root)
	project := t.TempDir()
	legacyFixture(t, project)
	plan, err := s.planMigration(project, "example-a1b2c3d4e5f6")
	if err != nil {
		t.Fatal(err)
	}
	batch, _, err := s.migrationBatch(plan)
	if err != nil {
		t.Fatal(err)
	}
	proposal := ignoredProposal(t, batch)
	var document map[string]any
	if err := json.Unmarshal(proposal, &document); err != nil {
		t.Fatal(err)
	}
	document["input_digest"] = digest("stale fixture input")
	proposal, err = json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	name := filepath.Join(t.TempDir(), "proposal.json")
	if err := os.WriteFile(name, proposal, 0600); err != nil {
		t.Fatal(err)
	}
	result := run(4, "", "migrate", "--project-root", project, "--repo-key", "example-a1b2c3d4e5f6", "--proposal", name)
	if result["error_class"] != "LearnPreferences::StaleProposalError" || result["error_fingerprint"] != digest("proposal input digest does not match the locked batch") {
		t.Fatal("migration lost safe stale-proposal error identity", result)
	}
}
