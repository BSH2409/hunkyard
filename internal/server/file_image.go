package server

import (
	"errors"
	"net/http"
	"os"
	"strconv"

	"github.com/BSH2409/hunkyard/internal/session"
)

// handleFileImage handles GET /api/file/image?path=X&side=base|head: one
// side of a changed image, for the review page's image diff. Like pin
// screenshots, it is never served to another site.
func (s *Server) handleFileImage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if site := r.Header.Get("Sec-Fetch-Site"); site == "cross-site" || site == "same-site" {
		http.Error(w, "Images are not served to other sites", http.StatusForbidden)
		return
	}
	path := r.URL.Query().Get("path")
	if path == "" {
		http.Error(w, "path query parameter required", http.StatusBadRequest)
		return
	}
	data, mime, err := s.session.Load().ImageBytes(path, r.URL.Query().Get("side"))
	switch {
	case errors.Is(err, session.ErrImageSide):
		http.Error(w, err.Error(), http.StatusBadRequest)
	case errors.Is(err, session.ErrImageFileUnknown), errors.Is(err, os.ErrNotExist):
		http.Error(w, "Image not found", http.StatusNotFound)
	case errors.Is(err, session.ErrImageTooLarge):
		http.Error(w, err.Error(), http.StatusRequestEntityTooLarge)
	case errors.Is(err, session.ErrNotImage):
		http.Error(w, err.Error(), http.StatusUnsupportedMediaType)
	case err != nil:
		http.Error(w, "Cannot read image", http.StatusInternalServerError)
	default:
		w.Header().Set("Content-Type", mime)
		w.Header().Set("Content-Length", strconv.Itoa(len(data)))
		w.Header().Set("Cache-Control", "private, no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		_, _ = w.Write(data)
	}
}
