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

func TestLocalProxyRejectsExternalTargetsAndSetsCSP(t *testing.T) {
	if os.Getenv("HUNKYARD_LOCAL_PROXY_TEST") != "1" {
		cmd := exec.Command(os.Args[0], "-test.run=^TestLocalProxyRejectsExternalTargetsAndSetsCSP$", "-test.count=1")
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
	if response.Code != 200 || !strings.Contains(response.Header().Get("Content-Security-Policy"), "connect-src 'self'") {
		t.Fatal("local preview lacks network boundary")
	}
}
