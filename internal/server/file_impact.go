package server

import (
	"errors"
	"net/http"

	"github.com/BSH2409/hunkyard/internal/session"
)

// handleFileImpact handles GET /api/file/impact?path=X: callers and tests
// of the symbols the file's diff changes.
func (s *Server) handleFileImpact(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	path := r.URL.Query().Get("path")
	if path == "" {
		http.Error(w, "path query parameter required", http.StatusBadRequest)
		return
	}
	result, err := s.session.Load().FileImpact(r.Context(), path)
	switch {
	case errors.Is(err, session.ErrImpactUnavailable):
		http.Error(w, err.Error(), http.StatusNotFound)
	case err != nil:
		http.Error(w, "Cannot compute impact", http.StatusInternalServerError)
	default:
		writeJSON(w, result)
	}
}
