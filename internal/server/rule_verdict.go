package server

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/BSH2409/hunkyard/internal/session"
)

// handleRuleVerdict handles PUT /api/comment/{id}/verdict?path=X: the
// reviewer confirms a rule finding (it stays open for the agent) or
// dismisses it (it is resolved and counts against the check).
func (s *Server) handleRuleVerdict(w http.ResponseWriter, r *http.Request, path, commentID string) {
	if r.Method != http.MethodPut {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	// A verdict is the reviewer's judgement and becomes rule evidence: only
	// the review page may give one, never an agent's curl.
	if r.Header.Get("Sec-Fetch-Site") != "same-origin" {
		http.Error(w, "verdicts are given from the review page", http.StatusForbidden)
		return
	}
	var req struct {
		Verdict string `json:"verdict"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4<<10)
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	c, err := s.session.Load().SetRuleVerdict(path, commentID, req.Verdict)
	switch {
	case errors.Is(err, session.ErrInvalidVerdict):
		http.Error(w, err.Error(), http.StatusBadRequest)
	case errors.Is(err, session.ErrCommentNotFound):
		http.Error(w, "Comment not found", http.StatusNotFound)
	case errors.Is(err, session.ErrNotRuleFinding):
		http.Error(w, err.Error(), http.StatusConflict)
	case err != nil:
		http.Error(w, err.Error(), http.StatusInternalServerError)
	default:
		writeJSON(w, c)
	}
}
