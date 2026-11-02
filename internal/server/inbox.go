package server

import (
	"encoding/json"
	"net/http"

	"github.com/BSH2409/hunkyard/internal/daemon"
	"github.com/BSH2409/hunkyard/internal/session"
)

// handleInbox handles GET /api/inbox: every review on this machine. It needs
// no review session, so a paused review's tab can still reach it.
func (s *Server) handleInbox(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	entries, err := session.ListInbox(r.Context())
	if err != nil {
		http.Error(w, "cannot list reviews", http.StatusInternalServerError)
		return
	}
	if entries == nil {
		entries = []session.InboxEntry{}
	}
	writeJSON(w, entries)
}

// handleInboxState handles GET /api/inbox/state: this daemon's round, open
// comments and whether the agent is working.
func (s *Server) handleInboxState(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, s.session.Load().InboxState())
}

// handleInboxResume handles POST /api/inbox/resume {"key"}: restart a paused
// review from the inbox page, the same way `hunkyard --session KEY` does.
func (s *Server) handleInboxResume(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if r.Header.Get("Sec-Fetch-Site") != "same-origin" {
		http.Error(w, "reviews are resumed from the inbox page", http.StatusForbidden)
		return
	}
	var req struct {
		Key string `json:"key"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&req); err != nil || !daemon.ValidSessionKey(req.Key) {
		http.Error(w, "invalid review id", http.StatusBadRequest)
		return
	}
	url, err := session.ResumePausedReview(req.Key)
	if err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	writeJSON(w, map[string]string{"url": url})
}
