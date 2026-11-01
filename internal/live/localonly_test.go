package live

import (
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/BSH2409/hunkyard/internal/localonly"
)

func TestLocalProxyRejectsExternalTargets(t *testing.T) {
	if os.Getenv("HUNKYARD_LOCAL_PROXY_TEST") != "1" {
		cmd := exec.Command(os.Args[0], "-test.run=^TestLocalProxyRejectsExternalTargets$", "-test.count=1")
		cmd.Env = append(os.Environ(), "HUNKYARD_LOCAL_PROXY_TEST=1")
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("local proxy check: %s: %v", output, err)
		}
		return
	}
	localonly.Install()
	if _, err := newLiveProxy("https://external.example", 5000, ""); err == nil {
		t.Fatal("remote live proxy target accepted")
	}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte("<html><head></head><body>local</body></html>"))
	}))
	defer upstream.Close()
	proxy, err := newLiveProxy(upstream.URL, 5000, "")
	if err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	proxy.ServeHTTP(response, httptest.NewRequest("GET", "http://localhost/", nil))
	if response.Code != 200 {
		t.Fatalf("local proxy = %d", response.Code)
	}
}

// An app that redirects off-site gets the notice stub, not an error: the
// proxy never follows the redirect, and only the reviewer can open the URL.
func TestLocalProxyTurnsExternalRedirectIntoNotice(t *testing.T) {
	if os.Getenv("HUNKYARD_LOCAL_PROXY_TEST") != "1" {
		cmd := exec.Command(os.Args[0], "-test.run=^TestLocalProxyTurnsExternalRedirectIntoNotice$", "-test.count=1")
		cmd.Env = append(os.Environ(), "HUNKYARD_LOCAL_PROXY_TEST=1")
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("local proxy redirect check: %s: %v", output, err)
		}
		return
	}
	localonly.Install()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "https://accounts.example.com/login", http.StatusFound)
	}))
	defer upstream.Close()
	proxy, err := newLiveProxy(upstream.URL, 5000, "")
	if err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	proxy.ServeHTTP(response, httptest.NewRequest("GET", "http://localhost/login", nil))
	body := response.Body.String()
	if response.Code != http.StatusOK || !strings.Contains(body, `"cross-origin-redirect"`) || !strings.Contains(body, "https://accounts.example.com/login") {
		t.Fatalf("external redirect = %d %q", response.Code, body)
	}
	if response.Header().Get("Location") != "" {
		t.Fatal("the external Location was passed to the frame")
	}

}

// A reviewed page loads what it loads when opened directly (fonts, captcha
// scripts); HUNKYARD_BLOCK_EXTERNAL=1 restores the strict local policy.
func TestLocalProxyPagePolicyDefaultsToTheAppsOwn(t *testing.T) {
	if os.Getenv("HUNKYARD_LOCAL_PROXY_TEST") != "1" {
		for _, block := range []string{"", "1"} {
			cmd := exec.Command(os.Args[0], "-test.run=^TestLocalProxyPagePolicyDefaultsToTheAppsOwn$", "-test.count=1")
			cmd.Env = append(os.Environ(), "HUNKYARD_LOCAL_PROXY_TEST=1", "HUNKYARD_BLOCK_EXTERNAL="+block)
			if output, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("block=%q: %s: %v", block, output, err)
			}
		}
		return
	}
	localonly.Install()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(`<html><head><link rel="stylesheet" href="https://fonts.googleapis.com/css?family=Inter"></head><body>app</body></html>`))
	}))
	defer upstream.Close()
	proxy, err := newLiveProxy(upstream.URL, 5000, "")
	if err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	proxy.ServeHTTP(response, httptest.NewRequest("GET", "http://localhost/", nil))
	csp := response.Header().Get("Content-Security-Policy")
	if os.Getenv("HUNKYARD_BLOCK_EXTERNAL") == "1" {
		if !strings.Contains(csp, "connect-src 'self'") {
			t.Fatalf("HUNKYARD_BLOCK_EXTERNAL=1 page policy = %q, want the strict local policy", csp)
		}
		return
	}
	if csp != "" {
		t.Fatalf("default page policy = %q, want none (the page loads its own resources)", csp)
	}
}
