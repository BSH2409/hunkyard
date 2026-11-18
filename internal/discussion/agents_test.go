package discussion

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestFailureAndRedaction(t *testing.T) {
	for diagnostic, status := range map[string]string{"authentication expired": "authentication", "credits exhausted": "quota", "429 rate limit": "rate_limited", "unexpected failure": "unavailable"} {
		if got := Failure(diagnostic); got.Status != status {
			t.Fatalf("%s: %+v", diagnostic, got)
		}
	}
	secret := "password=secret123456789012"
	if strings.Contains(Redact(secret), "secret123") {
		t.Fatal("credential was retained")
	}
	prompt, err := Prompt(map[string]string{"question": secret})
	if err != nil || strings.Contains(prompt, "secret123") {
		t.Fatalf("prompt redaction: %v %s", err, prompt)
	}
}

func TestProviderCLIResponses(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix CLI fixtures")
	}
	dir := t.TempDir()
	claude := "#!/bin/sh\ncat >/dev/null\nprintf '%s' '{\"result\":\"Claude reply\",\"is_error\":false}'\n"
	codex := "#!/bin/sh\nwhile [ $# -gt 0 ]; do\n if [ \"$1\" = --output-last-message ]; then shift; target=$1; fi\n shift\ndone\ncat >/dev/null\nprintf '%s' 'Codex reply' > \"$target\"\n"
	for name, body := range map[string]string{"claude": claude, "codex": codex} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	for _, agent := range []string{"claude", "codex"} {
		result := Run(context.Background(), agent, "a frozen request")
		if result.Status != "replied" || !strings.Contains(strings.ToLower(result.Body), agent) {
			t.Fatalf("%s: %+v", agent, result)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "claude"), []byte("#!/bin/sh\necho 'authentication expired' >&2\nexit 1\n"), 0700); err != nil {
		t.Fatal(err)
	}
	if got := Run(context.Background(), "claude", "question"); got.Status != "authentication" {
		t.Fatalf("authentication failure: %+v", got)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	started := time.Now()
	if got := Run(ctx, "codex", "question"); got.Status != "interrupted" || time.Since(started) > time.Second {
		t.Fatalf("cancelled request: %+v", got)
	}
}

func TestStructuredCredentialsAreRedactedBeforeDelivery(t *testing.T) {
	for _, text := range []string{`{"password":"secret123456789012"}`, `api_key = "secret123456789012"`, `token=secret123456789012`} {
		prompt, err := Prompt(map[string]string{"content": text})
		if err != nil || strings.Contains(prompt, "secret123456789012") {
			t.Fatalf("credential not redacted: %s", prompt)
		}
	}
}

func TestProviderStartsIndependentOfLaunchingHost(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix CLI fixture")
	}
	directory := t.TempDir()
	for _, name := range []string{"CLAUDECODE", "CLAUDE_CODE_ENTRYPOINT", "CLAUDE_SESSION_ID", "CODEX_THREAD_ID", "CODEX_SESSION_ID", "HUNKYARD_OWNER_ID"} {
		t.Setenv(name, "launching-session")
	}
	t.Setenv("CODEX_HOME", directory)
	t.Setenv("CLAUDE_CONFIG_DIR", directory)
	t.Setenv("HUNKYARD_EXPECTED_AUTH_DIRECTORY", directory)
	script := `#!/bin/sh
if [ -n "$CLAUDECODE$CLAUDE_CODE_ENTRYPOINT$CLAUDE_SESSION_ID$CODEX_THREAD_ID$CODEX_SESSION_ID$HUNKYARD_OWNER_ID" ]; then
  echo 'launching host session inherited' >&2
  exit 1
fi
if [ "$CODEX_HOME" != "$HUNKYARD_EXPECTED_AUTH_DIRECTORY" ] || [ "$CLAUDE_CONFIG_DIR" != "$HUNKYARD_EXPECTED_AUTH_DIRECTORY" ]; then
  echo 'authentication configuration lost' >&2
  exit 1
fi
printf '%s\n' "$@" > "$HUNKYARD_EXPECTED_AUTH_DIRECTORY/claude-args"
cat >/dev/null
printf '%s' '{"result":"Independent participant","is_error":false}'
`
	if err := os.WriteFile(filepath.Join(directory, "claude"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", directory+string(os.PathListSeparator)+os.Getenv("PATH"))
	if result := Run(context.Background(), "claude", "Question"); result.Status != "replied" {
		t.Fatalf("participant did not start independently: %+v", result)
	}
	args, err := os.ReadFile(filepath.Join(directory, "claude-args"))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"--print", "--model", "haiku", "--effort", "low", "--output-format", "json",
		"--tools", "", "--strict-mcp-config", "--mcp-config", `{"mcpServers":{}}`,
		"--setting-sources", "user", "--settings", `{"disableAllHooks":true}`, "--safe-mode",
		"--disable-slash-commands", "--no-session-persistence",
	}
	if string(args) != strings.Join(want, "\n")+"\n" {
		t.Errorf("unexpected Claude arguments: %q", args)
	}
}
