package server

import (
	"bytes"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/BSH2409/hunkyard/internal/config"
	"github.com/BSH2409/hunkyard/internal/localonly"
)

// BindPreviewServer listens on host:(apiPort+1), as the live proxy does, and
// serves the previewed HTML file and its sibling assets there. A separate
// origin keeps the previewed page's scripts out of the review page's
// same-origin checks. The server is not started; the caller runs srv.Serve(ln).
func BindPreviewServer(host, previewFile string, apiPort int) (net.Listener, *http.Server, error) {
	abs, err := filepath.Abs(previewFile)
	if err != nil {
		return nil, nil, err
	}
	previewPort := apiPort + 1
	ln, err := net.Listen("tcp", net.JoinHostPort(host, strconv.Itoa(previewPort)))
	if err != nil {
		return nil, nil, fmt.Errorf("preview port %d already in use: %w", previewPort, err)
	}
	srv := &http.Server{
		Handler:     newPreviewContentHandler(host, abs, apiPort),
		ReadTimeout: 15 * time.Second,
		IdleTimeout: 60 * time.Second,
	}
	return ln, srv, nil
}

// newPreviewContentHandler serves /preview-content: the previewed file at the
// root and files beside it below, never anything a symlink leads outside its
// folder. Agent scripts load from the review server at apiPort.
func newPreviewContentHandler(listenHost, previewFile string, apiPort int) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !config.HostAllowed(listenHost, "", r.Host) {
			http.Error(w, "preview: host not allowed", http.StatusForbidden)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if policy := localonly.PagePolicy(); policy != "" {
			w.Header().Set("Content-Security-Policy", policy)
		}
		reqPath, ok := strings.CutPrefix(r.URL.Path, "/preview-content")
		if !ok {
			http.NotFound(w, r)
			return
		}
		if reqPath == "" || reqPath == "/" {
			servePreviewHTML(w, r, previewFile, apiPort)
			return
		}
		baseDir, err := filepath.EvalSymlinks(filepath.Dir(previewFile))
		if err != nil {
			http.NotFound(w, r)
			return
		}
		resolved, err := filepath.EvalSymlinks(filepath.Join(filepath.Dir(previewFile), filepath.Clean("/"+reqPath)))
		if err != nil {
			http.NotFound(w, r)
			return
		}
		if !strings.HasPrefix(resolved, baseDir+string(filepath.Separator)) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		info, err := os.Stat(resolved)
		if err != nil || info.IsDir() {
			http.NotFound(w, r)
			return
		}
		if isPreviewHTMLPath(resolved) {
			servePreviewHTML(w, r, resolved, apiPort)
			return
		}
		http.ServeFile(w, r, resolved)
	})
}

func isPreviewHTMLPath(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return ext == ".html" || ext == ".htm"
}

func servePreviewHTML(w http.ResponseWriter, r *http.Request, filePath string, apiPort int) {
	body, err := os.ReadFile(filePath)
	if err != nil {
		http.Error(w, "failed to read preview file", http.StatusInternalServerError)
		return
	}

	host := requestHost(r.Host)
	if strings.Contains(host, ":") {
		host = "[" + host + "]"
	}
	var sb strings.Builder
	for _, f := range AgentScriptFiles {
		fmt.Fprintf(&sb, `<script src="http://%s:%d/%s"></script>`, host, apiPort, f)
	}
	agentScripts := sb.String()

	// Insert before the last </body>, or append when the file is a fragment
	// with no closing body tag.
	idx := bytes.LastIndex(bytes.ToLower(body), []byte("</body>"))
	if idx >= 0 {
		var out []byte
		out = append(out, body[:idx]...)
		out = append(out, []byte(agentScripts)...)
		out = append(out, body[idx:]...)
		body = out
	} else {
		body = append(body, []byte(agentScripts)...)
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Write(body)
}
