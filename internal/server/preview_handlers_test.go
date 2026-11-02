package server

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestServePreviewHTML_AppendsAgentWhenNoBodyTag(t *testing.T) {
	dir := t.TempDir()
	htmlFile := filepath.Join(dir, "index.html")
	if err := os.WriteFile(htmlFile, []byte("<div>fragment with no body tag</div>"), 0644); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("GET", "/preview-content/", nil)
	req.Host = "localhost:5001"
	w := httptest.NewRecorder()
	newPreviewContentHandler("127.0.0.1", htmlFile, 5000).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "fragment with no body tag") {
		t.Error("response missing original content")
	}
	if !strings.Contains(body, "hunkyard-agent.js") {
		t.Error("agent script not appended when </body> is absent")
	}
	fragIdx := strings.Index(body, "fragment with no body tag")
	agentIdx := strings.Index(body, "hunkyard-agent.js")
	if fragIdx < 0 || agentIdx < 0 || agentIdx < fragIdx {
		t.Error("expected agent scripts appended after fragment content")
	}
}

func TestPreviewContentInjectsAgentFromTheAPIPort(t *testing.T) {
	dir := t.TempDir()
	htmlFile := filepath.Join(dir, "index.html")
	if err := os.WriteFile(htmlFile, []byte("<html><body>x</body></html>"), 0644); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("GET", "/preview-content/", nil)
	req.Host = "localhost:5001"
	w := httptest.NewRecorder()
	newPreviewContentHandler("127.0.0.1", htmlFile, 5000).ServeHTTP(w, req)
	if !strings.Contains(w.Body.String(), `src="http://localhost:5000/hunkyard-agent.js"`) {
		t.Errorf("agent script not loaded from the API port:\n%s", w.Body.String())
	}
}

func TestPreviewContentRejectsSymlinkOutOfItsFolder(t *testing.T) {
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("secret"), 0644); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	htmlFile := filepath.Join(dir, "index.html")
	if err := os.WriteFile(htmlFile, []byte("<html><body>x</body></html>"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(dir, "assets")); err != nil {
		t.Skip("symlinks unavailable:", err)
	}
	req := httptest.NewRequest("GET", "/preview-content/assets/secret.txt", nil)
	req.Host = "localhost:5001"
	w := httptest.NewRecorder()
	newPreviewContentHandler("127.0.0.1", htmlFile, 5000).ServeHTTP(w, req)
	if w.Code != http.StatusForbidden || strings.Contains(w.Body.String(), "secret") {
		t.Fatalf("symlink escape: status %d body %q, want 403", w.Code, w.Body.String())
	}
}

func TestPreviewContentRejectsForeignHost(t *testing.T) {
	dir := t.TempDir()
	htmlFile := filepath.Join(dir, "index.html")
	if err := os.WriteFile(htmlFile, []byte("<html><body>x</body></html>"), 0644); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("GET", "/preview-content/", nil)
	req.Host = "evil.example:5001"
	w := httptest.NewRecorder()
	newPreviewContentHandler("0.0.0.0", htmlFile, 5000).ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("foreign Host status = %d, want 403", w.Code)
	}
}

func TestReviewServerDoesNotServePreviewContent(t *testing.T) {
	dir := t.TempDir()
	htmlFile := filepath.Join(dir, "index.html")
	if err := os.WriteFile(htmlFile, []byte("<html><body>preview page</body></html>"), 0644); err != nil {
		t.Fatal(err)
	}
	sess := &Session{Mode: "files", RepoRoot: dir, ReviewRound: 1, ReviewType: "preview", Origin: htmlFile}
	sess.InitTestChannels()
	s, err := NewServer(sess, frontendFS, "", "test", 0, "")
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	s.ServeHTTP(w, httptest.NewRequest("GET", "/preview-content/", nil))
	if strings.Contains(w.Body.String(), "preview page") {
		t.Fatal("review origin serves the previewed page, so its scripts pass same-origin checks")
	}
}
