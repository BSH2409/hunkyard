package server

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"sort"
	"time"

	"github.com/BSH2409/hunkyard/internal/feedback"
	"github.com/BSH2409/hunkyard/internal/preferences"
	"github.com/BSH2409/hunkyard/internal/session"
	"github.com/BSH2409/hunkyard/internal/strictjson"
)

func (s *Server) feedbackStore() (*feedback.Store, error) {
	dir := s.reviewPath
	if dir == "" {
		dir = filepath.Dir(s.session.Load().HunkyardJSONPath())
	}
	return feedback.Open(dir)
}

type publishRequest struct {
	RequestID      string   `json:"request_id"`
	Disposition    string   `json:"disposition"`
	Summary        string   `json:"summary"`
	SkillIDs       []string `json:"skill_ids"`
	DisplayedFiles []struct {
		Path      string `json:"path"`
		ContentID string `json:"content_id"`
		ViewID    string `json:"view_id"`
	} `json:"displayed_files,omitempty"`
}

//nolint:gocyclo // The HTTP boundary validates the complete publication or bounded waiter before acting.
func (s *Server) handlePublish(w http.ResponseWriter, r *http.Request) {
	s.reviewMu.Lock()
	defer s.reviewMu.Unlock()
	var request publishRequest
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 128<<10))
	if err != nil || strictjson.Decode(data, &request) != nil || request.RequestID == "" {
		http.Error(w, "invalid review publication", 400)
		return
	}
	switch request.Disposition {
	case "comment", "approve", "request_changes":
	default:
		http.Error(w, "choose a review disposition", 400)
		return
	}
	store, err := s.feedbackStore()
	if err != nil {
		http.Error(w, "cannot open feedback store", 500)
		return
	}
	catalog, err := store.Skills()
	if err != nil {
		http.Error(w, "cannot read host skill catalog", 500)
		return
	}
	if _, err := feedback.ValidateSelection(catalog, request.SkillIDs); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	sess := s.session.Load()
	snapshot, err := sess.CaptureReviewSnapshot()
	if err != nil {
		http.Error(w, fmt.Sprintf("cannot capture review: %v", err), http.StatusConflict)
		return
	}
	files := map[string]session.ReviewSnapshotFile{}
	for _, file := range snapshot.Files {
		files[file.Path] = file
	}
	seen := map[string]bool{}
	for _, displayed := range request.DisplayedFiles {
		file, exists := files[displayed.Path]
		if !exists || seen[displayed.Path] || displayed.ContentID != file.ContentID || displayed.ViewID != snapshot.ViewID() {
			http.Error(w, "review changed; reload before publishing", http.StatusConflict)
			return
		}
		seen[displayed.Path] = true
	}
	dir := s.reviewPath
	if dir == "" {
		dir = filepath.Dir(sess.HunkyardJSONPath())
	}
	if err := session.SaveReviewSnapshot(dir, snapshot); err != nil {
		http.Error(w, "cannot persist review snapshot", 500)
		return
	}
	batch := feedback.Batch{Version: 1, ReviewID: s.ownerReviewID, SnapshotID: snapshot.ID, Round: snapshot.Round, Disposition: request.Disposition, Summary: request.Summary, SkillIDs: request.SkillIDs, Comments: []feedback.Comment{}}
	appendComment := func(path string, c session.Comment) {
		file, exists := files[path]
		parent := feedback.Comment{ID: c.ID, Body: c.Body, Path: path, OldPath: file.OldPath, Scope: c.Scope, Side: c.Side, StartLine: c.StartLine, EndLine: c.EndLine, Quote: c.Quote, Anchor: c.Anchor, HeadSHA: c.HeadSHA, FocusKey: c.FocusKey, ContentID: c.ContentID, ViewID: c.ViewID, Outdated: path != "" && (!exists || (c.ContentID != "" && c.ContentID != file.ContentID))}
		if c.AuthorKind != "agent" {
			batch.Comments = append(batch.Comments, parent)
		}
		for _, reply := range c.Replies {
			// Unmarked legacy replies have no reliable human provenance.
			if reply.AuthorKind != "human" {
				continue
			}
			child := parent
			child.ID = "reply:" + c.ID + ":" + reply.ID
			child.ParentID = c.ID
			child.Body = reply.Body
			batch.Comments = append(batch.Comments, child)
		}
	}
	for _, c := range sess.GetReviewComments() {
		appendComment("", c)
	}
	paths := []string{}
	comments := sess.GetVisibleComments()
	for path := range comments {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		for _, c := range comments[path] {
			appendComment(path, c)
		}
	}
	if request.Summary != "" {
		batch.Comments = append(batch.Comments, feedback.Comment{ID: "review-summary", Body: request.Summary, Scope: "review"})
	}
	if err := sess.SyncWriteFiles(); err != nil {
		http.Error(w, "cannot persist review comments", 500)
		return
	}
	published, err := store.Publish(request.RequestID, batch)
	if err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	sess.SetAwaitingFirstReview(false)
	sess.SetWaitingForAgent(true)
	prompt := fmt.Sprintf("Review published locally as %s. Return to the launching CLI to process it.", published.ID)
	response := map[string]any{"status": "published", "approved": request.Disposition == "approve", "prompt": prompt, "batch": published, "session_id": s.ownerReviewID, "stats": s.buildSessionStats(sess)}
	writeJSONComplete(w, response)
	encoded, _ := json.Marshal(response)
	sess.Notify(SSEEvent{Type: "finish", Content: string(encoded)})
}

//nolint:gocyclo // The HTTP boundary validates the complete publication or bounded waiter before acting.
func (s *Server) handleFeedbackAwait(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	timeout := 45 * time.Second
	if raw := r.URL.Query().Get("timeout"); raw != "" {
		value, err := time.ParseDuration(raw)
		if err != nil || value <= 0 || value > 45*time.Second {
			http.Error(w, "timeout must be greater than zero and at most 45s", 400)
			return
		}
		timeout = value
	}
	store, err := s.feedbackStore()
	if err != nil {
		http.Error(w, "cannot open feedback store", 500)
		return
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	for {
		batches, err := store.Pending()
		if err != nil {
			http.Error(w, "cannot read feedback queue", 500)
			return
		}
		if len(batches) > 0 {
			catalog, err := store.Skills()
			if err != nil {
				http.Error(w, "cannot read host skill catalog", 500)
				return
			}
			selected := map[string][]feedback.SkillRef{}
			blocked := map[string]string{}
			for _, batch := range batches {
				refs, err := feedback.ValidateSelection(catalog, batch.SkillIDs)
				if err != nil {
					blocked[batch.ID] = err.Error()
					selected[batch.ID] = []feedback.SkillRef{}
					continue
				}
				selected[batch.ID] = refs
			}
			writeJSON(w, map[string]any{"status": "published", "session_id": s.ownerReviewID, "batches": batches, "selected_skills": selected, "blocked_skills": blocked})
			return
		}
		select {
		case <-r.Context().Done():
			return
		case <-timer.C:
			writeJSON(w, map[string]any{"status": "pending", "session_id": s.ownerReviewID, "batches": []feedback.Batch{}})
			return
		case <-ticker.C:
		}
	}
}

func (s *Server) handleFeedbackAck(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var request struct {
		BatchID string `json:"batch_id"`
	}
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1024))
	if err != nil || strictjson.Decode(data, &request) != nil {
		http.Error(w, "invalid acknowledgment", 400)
		return
	}
	store, err := s.feedbackStore()
	if err != nil {
		http.Error(w, "cannot open feedback store", 500)
		return
	}
	if err := store.Ack(request.BatchID); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	writeJSON(w, map[string]string{"status": "acknowledged"})
}

func (s *Server) handleFeedbackRound(w http.ResponseWriter, r *http.Request) {
	s.reviewMu.Lock()
	defer s.reviewMu.Unlock()
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	store, err := s.feedbackStore()
	if err != nil {
		http.Error(w, "cannot open feedback store", 500)
		return
	}
	pending, err := store.Pending()
	if err != nil {
		http.Error(w, "cannot read feedback queue", 500)
		return
	}
	if len(pending) > 0 {
		http.Error(w, "acknowledge published feedback before starting the next round", http.StatusConflict)
		return
	}
	sess := s.session.Load()
	published, err := store.HasPublications()
	if err != nil {
		http.Error(w, "cannot read publication history", 500)
		return
	}
	if published {
		sess.SetAwaitingFirstReview(false)
		sess.SignalRoundComplete()
		sess.SetWaitingForAgent(false)
	}
	writeJSON(w, map[string]string{"status": "reviewing"})
}

func (s *Server) handleSkillCatalog(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	store, err := s.feedbackStore()
	if err != nil {
		http.Error(w, "cannot open feedback store", 500)
		return
	}
	refs, err := store.Skills()
	if err != nil {
		http.Error(w, "cannot read host skill catalog", 500)
		return
	}
	type publicSkill struct {
		ID          string `json:"id"`
		Name        string `json:"name"`
		Description string `json:"description,omitempty"`
	}
	public := []publicSkill{}
	for _, ref := range refs {
		public = append(public, publicSkill{ref.ID, ref.Name, ref.Description})
	}
	writeJSON(w, map[string]any{"skills": public})
}

func (s *Server) handleSkillRegistration(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var request struct {
		Version int                 `json:"version"`
		Skills  []feedback.SkillRef `json:"skills"`
	}
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
	if err != nil || strictjson.Decode(data, &request) != nil || request.Version != 1 {
		http.Error(w, "invalid host skill manifest", 400)
		return
	}
	store, err := s.feedbackStore()
	if err != nil {
		http.Error(w, "cannot open feedback store", 500)
		return
	}
	if err := store.RegisterSkills(request.Skills); err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	writeJSON(w, map[string]string{"status": "registered"})
}

func (s *Server) handleFeedbackLearn(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var request struct {
		BatchID   string                        `json:"batch_id"`
		Decisions []feedback.PreferenceDecision `json:"decisions"`
	}
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 2<<20))
	if err != nil || strictjson.Decode(data, &request) != nil {
		http.Error(w, "invalid preference decisions", 400)
		return
	}
	store, err := s.feedbackStore()
	if err != nil {
		http.Error(w, "cannot open feedback store", 500)
		return
	}
	prefs, err := preferences.Open(preferences.Config{})
	if err != nil {
		http.Error(w, "cannot open preference store", 500)
		return
	}
	sess := s.session.Load()
	sess.RLock()
	root := sess.RepoRoot
	sess.RUnlock()
	report, err := store.Learn(prefs, request.BatchID, request.Decisions, s.owner.Key, root)
	status := http.StatusOK
	message := "classified"
	if err != nil {
		status = http.StatusUnprocessableEntity
		message = err.Error()
	}
	writeJSONStatus(w, status, map[string]any{"status": message, "report": report})
	failed := 0
	for _, outcome := range report.Outcomes {
		if outcome.Status == "failed" {
			failed++
		}
	}
	notification := fmt.Sprintf("Preference classification: %d comments processed, %d need retry.", len(report.Outcomes), failed)
	if err != nil {
		notification = "Preference classification needs attention in the launching CLI."
	}
	sess.Notify(SSEEvent{Type: "preferences-updated", Content: notification})
}
