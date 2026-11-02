package server

import (
	"bytes"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"testing"
)

func imageServer(t *testing.T) *Server {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewGray(image.Rect(0, 0, 1, 1))); err != nil {
		t.Fatal(err)
	}
	sess := &Session{Mode: "git", RepoRoot: t.TempDir(), Files: []*FileEntry{
		{Path: "logo.png", Status: "added", FileType: "binary", Content: buf.String()},
		{Path: "notes.txt", Status: "added", FileType: "code", Content: "hello\n"},
	}}
	sess.InitTestChannels()
	s, err := NewServer(sess, frontendFS, "", "test", 0, "")
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestFileImageEndpoint(t *testing.T) {
	s := imageServer(t)
	cases := []struct {
		name, method, target, site string
		want                       int
	}{
		{"head", http.MethodGet, "/api/file/image?path=logo.png&side=head", "same-origin", http.StatusOK},
		{"agent without header", http.MethodGet, "/api/file/image?path=logo.png&side=head", "", http.StatusOK},
		{"added file has no base", http.MethodGet, "/api/file/image?path=logo.png&side=base", "same-origin", http.StatusNotFound},
		{"not an image", http.MethodGet, "/api/file/image?path=notes.txt&side=head", "same-origin", http.StatusUnsupportedMediaType},
		{"unknown file", http.MethodGet, "/api/file/image?path=other.png&side=head", "same-origin", http.StatusNotFound},
		{"bad side", http.MethodGet, "/api/file/image?path=logo.png&side=left", "same-origin", http.StatusBadRequest},
		{"missing path", http.MethodGet, "/api/file/image?side=head", "same-origin", http.StatusBadRequest},
		{"cross-site", http.MethodGet, "/api/file/image?path=logo.png&side=head", "cross-site", http.StatusForbidden},
		{"same-site", http.MethodGet, "/api/file/image?path=logo.png&side=head", "same-site", http.StatusForbidden},
		{"method", http.MethodPost, "/api/file/image?path=logo.png&side=head", "same-origin", http.StatusMethodNotAllowed},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(tc.method, tc.target, nil)
			if tc.site != "" {
				req.Header.Set("Sec-Fetch-Site", tc.site)
			}
			s.ServeHTTP(rec, req)
			if rec.Code != tc.want {
				t.Fatalf("%s = %d %s, want %d", tc.name, rec.Code, rec.Body, tc.want)
			}
			if tc.want == http.StatusOK {
				if rec.Header().Get("Content-Type") != "image/png" || rec.Header().Get("Cache-Control") != "private, no-store" {
					t.Fatalf("headers = %v", rec.Header())
				}
			}
		})
	}
}
