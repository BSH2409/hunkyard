package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/BSH2409/hunkyard/internal/comment"
	"github.com/BSH2409/hunkyard/internal/focus"
	"github.com/BSH2409/hunkyard/internal/review"
	"github.com/BSH2409/hunkyard/internal/session"
)

func TestPrintHelpMentionsSession(t *testing.T) {
	var stderr strings.Builder
	old := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w
	done := make(chan struct{})
	go func() {
		io.Copy(&stderr, r)
		close(done)
	}()
	printHelp()
	w.Close()
	<-done
	os.Stderr = old
	out := stderr.String()
	if !strings.Contains(out, "--session") {
		t.Fatalf("help missing --session:\n%s", out)
	}
	for _, removed := range []string{"--public-url", "HUNKYARD_PUBLIC_URL", "default: random"} {
		if strings.Contains(out, removed) {
			t.Fatalf("help advertises unavailable option %s", removed)
		}
	}
	for _, supported := range []string{"default: 5000", "START...END", "preferences", "feedback await", "--skills-file", "--allow-unauthenticated-network", "HUNKYARD_ALLOW_UNAUTHENTICATED_NETWORK"} {
		if !strings.Contains(out, supported) {
			t.Fatalf("help missing %s", supported)
		}
	}
	if !strings.Contains(out, "hunkyard story") {
		t.Fatalf("help missing hunkyard story:\n%s", out)
	}
}

// TestSubcommandDispatch_Help verifies that help flags are recognized.
func TestSubcommandDispatch_Help(t *testing.T) {
	for _, arg := range []string{"help", "--help", "-h"} {
		t.Run(arg, func(t *testing.T) {
			cmd := exec.Command(os.Args[0], "-test.run=TestHelperProcess_Help", "--")
			cmd.Env = append(os.Environ(), "GO_TEST_HELPER=1", "GO_TEST_HELP_ARG="+arg)
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("help %q exited with error: %v\noutput: %s", arg, err, out)
			}
		})
	}
}

func TestHelperProcess_Help(t *testing.T) {
	if os.Getenv("GO_TEST_HELPER") != "1" {
		return
	}
	arg := os.Getenv("GO_TEST_HELP_ARG")
	os.Args = []string{"hunkyard", arg}
	var stderr strings.Builder
	old := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w
	done := make(chan struct{})
	go func() {
		io.Copy(&stderr, r)
		close(done)
	}()
	printHelp()
	w.Close()
	<-done
	os.Stderr = old
	if !strings.Contains(stderr.String(), "--session") {
		t.Fatalf("help missing --session:\n%s", stderr.String())
	}
}

// TestSubcommandDispatch_Version verifies the version flag.
func TestSubcommandDispatch_Version(t *testing.T) {
	cmd := exec.Command(os.Args[0], "-test.run=TestHelperProcess_Version", "--")
	cmd.Env = append(os.Environ(), "GO_TEST_HELPER=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("version exited with error: %v\noutput: %s", err, out)
	}
	if !strings.HasPrefix(string(out), "hunkyard "+version) {
		t.Fatalf("version output = %q, want it to start with %q", out, "hunkyard "+version)
	}
}

func TestHelperProcess_Version(t *testing.T) {
	if os.Getenv("GO_TEST_HELPER") != "1" {
		return
	}
	printVersion()
}

// TestSubcommandDispatch_Config verifies that "hunkyard config --generate" produces output.
func TestSubcommandDispatch_Config(t *testing.T) {
	cmd := exec.Command(os.Args[0], "-test.run=TestHelperProcess_Config", "--")
	cmd.Env = append(os.Environ(), "GO_TEST_HELPER=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("config --generate exited with error: %v\noutput: %s", err, out)
	}
	if !strings.Contains(string(out), `"port":`) || !strings.Contains(string(out), `"host":`) {
		t.Fatalf("config --generate output lacks the config keys:\n%s", out)
	}
}

func TestHelperProcess_Config(t *testing.T) {
	if os.Getenv("GO_TEST_HELPER") != "1" {
		return
	}
	runConfig([]string{"--generate"})
}

// expectHelperUsageError runs a helper child and requires exit code 1 with
// want in its output: any other failure (a panic exits 2, a t.Fatal prints
// FAIL) is a different bug.
func expectHelperUsageError(t *testing.T, helper, want string) {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^"+helper+"$", "--")
	cmd.Env = append(os.Environ(), "GO_TEST_HELPER=1")
	out, err := cmd.CombinedOutput()
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 1 {
		t.Fatalf("%s: err = %v, want exit code 1\n%s", helper, err, out)
	}
	if !strings.Contains(string(out), want) {
		t.Fatalf("%s output lacks %q:\n%s", helper, want, out)
	}
}

func TestRunComment_MissingArgs(t *testing.T) {
	expectHelperUsageError(t, "TestHelperProcess_CommentMissing", "Usage: hunkyard comment")
}

func TestHelperProcess_CommentMissing(t *testing.T) {
	if os.Getenv("GO_TEST_HELPER") != "1" {
		return
	}
	runComment([]string{})
}

// TestRunComment_InvalidLocation verifies that a bad location format exits with error.
func TestRunComment_InvalidLocation(t *testing.T) {
	expectHelperUsageError(t, "TestHelperProcess_CommentBadLoc", `invalid location "noColonHere"`)
}

func TestHelperProcess_CommentBadLoc(t *testing.T) {
	if os.Getenv("GO_TEST_HELPER") != "1" {
		return
	}
	// No colon in location
	runComment([]string{"noColonHere", "some body"})
}

// TestRunComment_InvalidLineNumber verifies that a non-numeric line exits with error.
func TestRunComment_InvalidLineNumber(t *testing.T) {
	expectHelperUsageError(t, "TestHelperProcess_CommentBadLine", `invalid line spec in "file.go:abc"`)
}

func TestHelperProcess_CommentBadLine(t *testing.T) {
	if os.Getenv("GO_TEST_HELPER") != "1" {
		return
	}
	runComment([]string{"file.go:abc", "some body"})
}

// TestRunInstall_MissingAgent verifies that runInstall with no args exits with usage.
func TestRunInstall_MissingAgent(t *testing.T) {
	expectHelperUsageError(t, "TestHelperProcess_InstallMissing", "Usage: hunkyard install")
}

func TestHelperProcess_InstallMissing(t *testing.T) {
	if os.Getenv("GO_TEST_HELPER") != "1" {
		return
	}
	runInstall([]string{})
}

// runCommentHelper runs a comment helper child in a folder the parent owns and
// returns that folder's review file after the child exits.
func runCommentHelper(t *testing.T, helper string, prepare func(dir string)) (string, session.HunkyardJSON) {
	t.Helper()
	dir := t.TempDir()
	if prepare != nil {
		prepare(dir)
	}
	cmd := exec.Command(os.Args[0], "-test.run=^"+helper+"$", "--")
	cmd.Env = append(os.Environ(), "GO_TEST_HELPER=1", "GO_TEST_DIR="+dir)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%s exited with error: %v\noutput: %s", helper, err, out)
	}
	path, err := review.ResolveReviewPath(dir)
	if err != nil {
		t.Fatal(err)
	}
	cj, _ := review.LoadHunkyardJSON(path)
	return path, cj
}

// TestRunComment_FlagParsing verifies that --output and --author are applied.
func TestRunComment_FlagParsing(t *testing.T) {
	_, cj := runCommentHelper(t, "TestHelperProcess_CommentFlags", func(dir string) {
		os.WriteFile(filepath.Join(dir, "test.go"), []byte("package main\n"), 0o644)
	})
	comments := cj.Files["test.go"].Comments
	if len(comments) != 1 || comments[0].Author != "TestBot" || comments[0].Body != "test body" || comments[0].StartLine != 1 {
		t.Fatalf("written comment = %+v, want TestBot's comment on line 1", comments)
	}
}

func TestHelperProcess_CommentFlags(t *testing.T) {
	if os.Getenv("GO_TEST_HELPER") != "1" {
		return
	}
	runComment([]string{"--output", os.Getenv("GO_TEST_DIR"), "--author", "TestBot", "test.go:1", "test body"})
}

// TestRunComment_ClearFlag verifies that --clear removes the review.
func TestRunComment_ClearFlag(t *testing.T) {
	path, cj := runCommentHelper(t, "TestHelperProcess_CommentClear", func(dir string) {
		path, err := review.ResolveReviewPath(dir)
		if err != nil {
			t.Fatal(err)
		}
		seed := session.HunkyardJSON{Files: map[string]session.HunkyardJSONFile{"a.go": {Comments: []session.Comment{{ID: "c_1", StartLine: 1, EndLine: 1, Body: "x"}}}}}
		if err := review.SaveHunkyardJSON(path, seed); err != nil {
			t.Fatal(err)
		}
	})
	if len(cj.Files) != 0 {
		t.Fatalf("review still has comments after --clear: %+v", cj.Files)
	}
	if _, err := os.Stat(session.ReviewPathsFor(path).Review); !os.IsNotExist(err) {
		t.Fatalf("review file still present after --clear (%v)", err)
	}
}

func TestHelperProcess_CommentClear(t *testing.T) {
	if os.Getenv("GO_TEST_HELPER") != "1" {
		return
	}
	runComment([]string{"--output", os.Getenv("GO_TEST_DIR"), "--clear"})
}

// TestRunComment_RangeLine verifies that a range line spec like "10-25" is parsed.
func TestRunComment_RangeLine(t *testing.T) {
	_, cj := runCommentHelper(t, "TestHelperProcess_CommentRange", nil)
	comments := cj.Files["test.go"].Comments
	if len(comments) != 1 || comments[0].StartLine != 10 || comments[0].EndLine != 25 {
		t.Fatalf("written comment = %+v, want lines 10-25", comments)
	}
}

func TestHelperProcess_CommentRange(t *testing.T) {
	if os.Getenv("GO_TEST_HELPER") != "1" {
		return
	}
	runComment([]string{"--output", os.Getenv("GO_TEST_DIR"), "--author", "Bot", "test.go:10-25", "range body"})
}

// TestRunComment_InvalidRange verifies that a bad range like "10-abc" exits with error.
func TestRunComment_InvalidRange(t *testing.T) {
	cmd := exec.Command(os.Args[0], "-test.run=TestHelperProcess_CommentBadRange", "--")
	cmd.Env = append(os.Environ(), "GO_TEST_HELPER=1")
	err := cmd.Run()
	if err == nil {
		t.Fatal("expected non-zero exit for invalid range")
	}
}

func TestHelperProcess_CommentBadRange(t *testing.T) {
	if os.Getenv("GO_TEST_HELPER") != "1" {
		return
	}
	runComment([]string{"file.go:10-abc", "some body"})
}

// TestRunComment_JSONFlag verifies that --json reads from stdin and produces output.
func TestRunComment_JSONFlag(t *testing.T) {
	cmd := exec.Command(os.Args[0], "-test.run=^TestHelperProcess_CommentJSON$", "--")
	cmd.Env = append(os.Environ(), "GO_TEST_HELPER=1")
	cmd.Stdin = strings.NewReader(`[{"file":"main.go","line":1,"body":"test"}]`)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("process exited with error: %v\noutput: %s", err, out)
	}
	if !strings.Contains(string(out), "Added 1 comment") {
		t.Errorf("expected success message, got: %s", out)
	}
}

func TestHelperProcess_CommentJSON(t *testing.T) {
	if os.Getenv("GO_TEST_HELPER") != "1" {
		return
	}
	tmp := t.TempDir()
	runComment([]string{"--json", "--output", tmp, "--author", "TestBot"})
}

// TestRunComment_JSONFlagMixed verifies that --json handles mixed comments and replies.
func TestRunComment_JSONFlagMixed(t *testing.T) {
	// Step 1: Create a comment and capture its ID
	tmp := t.TempDir()
	err := comment.AddCommentToHunkyardJSONScoped("main.go", 1, 1, "comment", "TestBot", tmp, focus.InheritedScope{})
	if err != nil {
		t.Fatalf("setup comment: %v", err)
	}
	hunkyardPath, err := review.ResolveReviewPath(tmp)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(hunkyardPath, "review.json"))
	if err != nil {
		t.Fatalf("read review.json: %v", err)
	}
	var cj session.HunkyardJSON
	json.Unmarshal(data, &cj)
	commentID := cj.Files["main.go"].Comments[0].ID

	// Step 2: Run --json with a new comment + reply to the existing comment
	input := fmt.Sprintf(`[{"file":"main.go","line":5,"body":"another"},{"reply_to":%q,"body":"reply","resolve":true}]`, commentID)
	cmd := exec.Command(os.Args[0], "-test.run=^TestHelperProcess_CommentJSONMix$", "--")
	cmd.Env = append(os.Environ(), "GO_TEST_HELPER=1", "TEST_OUTPUT_DIR="+tmp)
	cmd.Stdin = strings.NewReader(input)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("process exited with error: %v\noutput: %s", err, out)
	}
	if !strings.Contains(string(out), "1 comment") || !strings.Contains(string(out), "1 reply") {
		t.Errorf("expected mixed success message, got: %s", out)
	}
}

func TestHelperProcess_CommentJSONMix(t *testing.T) {
	if os.Getenv("GO_TEST_HELPER") != "1" {
		return
	}
	tmp := os.Getenv("TEST_OUTPUT_DIR")
	if tmp == "" {
		tmp = t.TempDir()
	}
	runComment([]string{"--json", "--output", tmp, "--author", "TestBot"})
}
