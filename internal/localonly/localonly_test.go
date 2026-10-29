package localonly

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTransportRejectsExternalBeforeNetwork(t *testing.T) {
	client := &http.Client{Transport: NewTransport()}
	for _, address := range []string{"https://example.com/private", "http://localhost.example.com", "http://169.254.169.254/", "http://192.168.1.2/", "http://127.0.0.1@evil.example/", "http://0.0.0.0:5000", "http://[::]:5000"} {
		if response, err := client.Get(address); err == nil {
			response.Body.Close()
			t.Fatalf("external address was accepted: %s", address)
		} else if !strings.Contains(err.Error(), "local-only") {
			t.Fatalf("request reached network: %v", err)
		}
	}
}

func TestTransportAllowsLoopbackAndBlocksRedirect(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/redirect" {
			http.Redirect(w, r, "https://example.com/private", http.StatusFound)
			return
		}
		io.WriteString(w, "local")
	}))
	defer srv.Close()
	t.Setenv("HTTP_PROXY", "http://192.0.2.2:8888")
	client := &http.Client{Transport: NewTransport()}
	response, err := client.Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	data, err := io.ReadAll(response.Body)
	if err != nil || string(data) != "local" {
		t.Fatalf("response = %q, %v", data, err)
	}
	if response, err = client.Get(srv.URL + "/redirect"); err == nil {
		response.Body.Close()
		t.Fatal("external redirect accepted")
	}
}

func TestLocalArguments(t *testing.T) {
	for _, args := range [][]string{{"pr", "12"}, {"review", "--pr", "12"}, {"review", "--remote"}, {"https://example.com"}, {"live", "https://example.com"}, {"review", "--public-url=https://example.com"}, {"review", "--host", "192.0.2.1"}, {"review", "--vcs", "jj"}, {"review", "--vcs=sl"}} {
		if err := ValidateArgs(args); err == nil {
			t.Errorf("accepted external operation: %v", args)
		}
	}
	for _, args := range [][]string{{"review", "--range", "main...HEAD"}, {"review", "--port", "5000"}, {"review", "index.html"}, {"plan", "local.md"}} {
		if err := ValidateArgs(args); err != nil {
			t.Errorf("rejected local operation: %v: %v", args, err)
		}
	}
}

func TestDockerListenerArguments(t *testing.T) {
	for _, command := range []string{"review", "_serve"} {
		for _, host := range []string{"0.0.0.0", "::"} {
			args := []string{command, "--host", host, "--allow-unauthenticated-network"}
			if err := ValidateArgs(args); err != nil {
				t.Errorf("rejected Docker listener %v: %v", args, err)
			}
		}
	}
}

func TestSourceReviewDoesNotExecuteHTMLOrLivePages(t *testing.T) {
	for _, args := range [][]string{{"live", "http://127.0.0.1:3000"}, {"preview", "index.html"}, {"_serve", "--live-origin", "http://127.0.0.1:3000"}, {"_serve", "--preview-file", "index.html"}} {
		if err := ValidateArgs(args); err == nil {
			t.Errorf("accepted executable page review: %v", args)
		}
	}
}

func TestLocalArgumentsPreventAgentSubprocesses(t *testing.T) {
	for _, args := range [][]string{{"story"}, {"story", "--guide", "review.md"}, {"install", "other-agent"}, {"install", "--global", "other-agent"}} {
		if err := ValidateArgs(args); err == nil {
			t.Errorf("accepted potentially outbound subprocess: %v", args)
		}
	}
	for _, args := range [][]string{{"install", "codex"}, {"install", "claude-code"}, {"install", "--global", "preferences"}, {"install", "--global", "all"}, {"preferences", "install-hook", "--host", "codex"}, {"story", "--prep"}, {"story", "--skip-llm"}, {"story", "--story-file", "story.json"}} {
		if err := ValidateArgs(args); err != nil {
			t.Errorf("rejected local operation: %v: %v", args, err)
		}
	}
}
