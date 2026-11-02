package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/BSH2409/hunkyard/internal/feedback"
	"github.com/BSH2409/hunkyard/internal/session"
)

// handleFeedbackAsk handles POST /api/feedback/ask: the owning agent asks
// the reviewer a question (owner-verified in ServeHTTP).
func (s *Server) handleFeedbackAsk(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	var req feedback.DecisionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	d, err := s.session.Load().AddDecision(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(d)
}

// handleDecisions handles GET /api/decisions (every request of the review)
// and PUT /api/decisions/{id}, the reviewer's answer. Answers are accepted
// only from the review page itself.
func (s *Server) handleDecisions(w http.ResponseWriter, r *http.Request) {
	sess := s.session.Load()
	id := strings.TrimPrefix(strings.TrimPrefix(r.URL.Path, "/api/decisions"), "/")
	if id == "" {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		decisions := sess.Decisions()
		if decisions == nil {
			decisions = []feedback.DecisionRequest{}
		}
		writeJSON(w, decisions)
		return
	}
	if r.Method != http.MethodPut {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if r.Header.Get("Sec-Fetch-Site") != "same-origin" {
		http.Error(w, "decisions are answered from the review page", http.StatusForbidden)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
	var req struct {
		Chosen  string `json:"chosen"`
		Text    string `json:"text"`
		Dismiss bool   `json:"dismiss"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	d, err := sess.AnswerDecision(id, req.Chosen, req.Text, req.Dismiss)
	switch {
	case errors.Is(err, session.ErrDecisionNotFound):
		http.Error(w, err.Error(), http.StatusNotFound)
	case errors.Is(err, session.ErrDecisionAnswered):
		http.Error(w, err.Error(), http.StatusConflict)
	case err != nil:
		http.Error(w, err.Error(), http.StatusBadRequest)
	default:
		writeJSON(w, d)
	}
}
