package server

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/BSH2409/hunkyard/internal/config"
	"github.com/BSH2409/hunkyard/internal/discussion"
	"github.com/BSH2409/hunkyard/internal/session"
	"github.com/BSH2409/hunkyard/internal/strictjson"
)

type discussionResponse struct {
	Agent   string `json:"agent"`
	Status  string `json:"status"`
	Detail  string `json:"detail,omitempty"`
	ReplyID string `json:"reply_id,omitempty"`
}
type discussionRequest struct {
	ID         string               `json:"id"`
	CommentID  string               `json:"comment_id"`
	MessageID  string               `json:"message_id,omitempty"`
	Recipients []string             `json:"recipients"`
	SnapshotID string               `json:"snapshot_id"`
	Generation string               `json:"generation"`
	CreatedAt  string               `json:"created_at"`
	Responses  []discussionResponse `json:"responses"`
}
type discussionState struct {
	Participants []string            `json:"participants"`
	Requests     []discussionRequest `json:"requests"`
}

var discussionID = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,128}$`)

func (s *Server) discussionPath() string {
	directory := s.reviewPath
	if directory == "" {
		directory = filepath.Dir(s.session.Load().HunkyardJSONPath())
	}
	return filepath.Join(directory, "discussion.json")
}

func (s *Server) readDiscussion() (discussionState, error) {
	state := discussionState{Participants: []string{}, Requests: []discussionRequest{}}
	raw, err := os.ReadFile(s.discussionPath())
	if errors.Is(err, os.ErrNotExist) {
		return state, nil
	}
	if err != nil {
		return state, err
	}
	if len(raw) > 4<<20 || strictjson.Decode(raw, &state) != nil {
		return state, fmt.Errorf("invalid discussion state")
	}
	for i := range state.Requests {
		if state.Requests[i].Generation == s.ownerGeneration {
			continue
		}
		for j := range state.Requests[i].Responses {
			response := &state.Requests[i].Responses[j]
			if response.Status == "thinking" {
				response.Status = "interrupted"
				response.Detail = "The review server restarted. Send again to retry."
			}
		}
	}
	return state, nil
}
func (s *Server) writeDiscussion(state discussionState) error {
	if err := os.MkdirAll(filepath.Dir(s.discussionPath()), 0700); err != nil {
		return err
	}
	raw, err := json.Marshal(state)
	if err != nil {
		return err
	}
	if len(raw) > 4<<20 {
		return fmt.Errorf("discussion state exceeds its storage limit")
	}
	return config.AtomicWriteFile(s.discussionPath(), raw, 0600)
}

func validParticipants(agents []string) bool {
	return len(agents) <= 2 && (len(agents) != 2 || agents[0] != agents[1]) && slices.IndexFunc(agents, func(agent string) bool { return agent != "claude" && agent != "codex" }) < 0
}

func (s *Server) handleDiscussion(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodPut && r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if r.Method != http.MethodGet && r.Header.Get("Sec-Fetch-Site") != "same-origin" {
		http.Error(w, "Open the review page to manage the discussion", http.StatusForbidden)
		return
	}
	s.discussionMu.Lock()
	defer s.discussionMu.Unlock()
	state, err := s.readDiscussion()
	if err != nil {
		http.Error(w, "Cannot read discussion state", http.StatusInternalServerError)
		return
	}
	switch r.Method {
	case http.MethodGet:
		available := map[string]bool{}
		for _, agent := range []string{"claude", "codex"} {
			_, err := exec.LookPath(agent)
			available[agent] = err == nil
		}
		writeJSON(w, map[string]any{"participants": state.Participants, "requests": state.Requests, "available": available})
	case http.MethodPut:
		var request struct {
			Participants []string `json:"participants"`
		}
		raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 4096))
		if err != nil || strictjson.Decode(raw, &request) != nil || !validParticipants(request.Participants) {
			http.Error(w, "Choose Claude, Codex, or both", http.StatusBadRequest)
			return
		}
		state.Participants = append([]string{}, request.Participants...)
		if err := s.writeDiscussion(state); err != nil {
			http.Error(w, "Cannot save participants", http.StatusInternalServerError)
			return
		}
		writeJSON(w, state)
		s.session.Load().Notify(SSEEvent{Type: "discussion-changed"})
	case http.MethodPost:
		s.startDiscussion(w, r, &state)
	}
}

//nolint:gocyclo // Validate the complete frozen request before scheduling a paid CLI call.
func (s *Server) startDiscussion(w http.ResponseWriter, r *http.Request, state *discussionState) {
	var input struct {
		RequestID  string   `json:"request_id"`
		CommentID  string   `json:"comment_id"`
		MessageID  string   `json:"message_id,omitempty"`
		Recipients []string `json:"recipients"`
	}
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 4096))
	if err != nil || strictjson.Decode(raw, &input) != nil || !discussionID.MatchString(input.RequestID) || input.CommentID == "" || len(input.Recipients) == 0 || !validParticipants(input.Recipients) {
		http.Error(w, "Invalid discussion request", http.StatusBadRequest)
		return
	}
	for _, agent := range input.Recipients {
		if !slices.Contains(state.Participants, agent) {
			http.Error(w, "Invite the participant before sending", http.StatusConflict)
			return
		}
	}
	for _, request := range state.Requests {
		if request.ID == input.RequestID {
			if request.CommentID != input.CommentID || request.MessageID != input.MessageID || !slices.Equal(request.Recipients, input.Recipients) {
				http.Error(w, "Request identity already used", http.StatusConflict)
				return
			}
			writeJSON(w, request)
			return
		}
	}
	if len(state.Requests) >= 1000 {
		http.Error(w, "The discussion request limit has been reached", http.StatusConflict)
		return
	}
	active := 0
	for _, request := range state.Requests {
		for _, response := range request.Responses {
			if response.Status == "thinking" {
				active++
			}
		}
	}
	if active+len(input.Recipients) > 4 {
		http.Error(w, "Wait for the current participants to finish", http.StatusConflict)
		return
	}
	sess := s.session.Load()
	comment, found := sess.FindReviewCommentByID(input.CommentID)
	if !found || comment.Scope != "review" {
		http.Error(w, "Discussion message not found", http.StatusNotFound)
		return
	}
	question := comment.Body
	if input.MessageID != "" {
		found := false
		for _, reply := range comment.Replies {
			if reply.ID == input.MessageID && reply.AuthorKind != "agent" {
				question = reply.Body
				found = true
				break
			}
		}
		if !found {
			http.Error(w, "Discussion reply not found", http.StatusNotFound)
			return
		}
	}
	snapshot, err := sess.CaptureReviewSnapshot()
	if err != nil {
		http.Error(w, "Refresh the review before asking participants", http.StatusConflict)
		return
	}
	files := map[string]any{}
	omitted := []string{}
	remaining := 64 << 10
	for _, file := range snapshot.Files {
		evidence := map[string]any{"content": string(file.Content), "base_content": string(file.BaseContent), "diff_hunks": file.DiffHunks}
		encoded, _ := json.Marshal(evidence)
		lower := strings.ToLower(filepath.Base(file.Path))
		if strings.HasPrefix(lower, ".env") || strings.Contains(lower, "credential") || strings.HasSuffix(lower, ".pem") || strings.Contains(string(encoded), "PRIVATE KEY") {
			omitted = append(omitted, file.Path+": sensitive file")
			continue
		}
		if len(encoded) > remaining {
			omitted = append(omitted, file.Path+": context limit")
			continue
		}
		files[file.Path] = evidence
		remaining -= len(encoded)
	}
	conversation := sess.GetReviewComments()
	if len(conversation) > 30 {
		conversation = conversation[len(conversation)-30:]
	}
	prompt, err := discussion.Prompt(map[string]any{"snapshot_id": snapshot.ID, "focus": snapshot.Focus, "question": question, "participants": input.Recipients, "conversation": conversation, "files": files, "omitted": omitted})
	if err != nil || len(prompt) > 256<<10 {
		http.Error(w, "Discussion context exceeds the message limit", http.StatusBadRequest)
		return
	}
	request := discussionRequest{ID: input.RequestID, CommentID: input.CommentID, MessageID: input.MessageID, Recipients: input.Recipients, SnapshotID: snapshot.ID, Generation: s.ownerGeneration, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano), Responses: []discussionResponse{}}
	for _, agent := range input.Recipients {
		request.Responses = append(request.Responses, discussionResponse{Agent: agent, Status: "thinking"})
	}
	state.Requests = append(state.Requests, request)
	if err := s.writeDiscussion(*state); err != nil {
		http.Error(w, "Cannot persist discussion request", http.StatusInternalServerError)
		return
	}
	for _, agent := range input.Recipients {
		s.bgWG.Add(1)
		go func() { defer s.bgWG.Done(); s.runDiscussion(request, agent, prompt, sess) }()
	}
	w.WriteHeader(http.StatusAccepted)
	writeJSON(w, request)
	sess.Notify(SSEEvent{Type: "discussion-changed"})
}

func (s *Server) runDiscussion(request discussionRequest, agent, prompt string, sess *session.Session) {
	author := "Claude"
	role := "Emphasize domain design, naming, and conventions when relevant to the question."
	if agent == "codex" {
		author = "Codex"
		role = "Emphasize behavior, contracts, runtime safety, and test evidence when relevant to the question."
	}
	result := discussion.Run(s.effectiveCtx(), agent, "Your participant name is "+author+". "+role+"\n"+prompt)
	sum := sha256.Sum256([]byte(request.ID + ":" + agent))
	replyID := "rp_" + hex.EncodeToString(sum[:12])
	if result.Status == "replied" {
		if _, ok := sess.AddAgentReviewReply(request.CommentID, replyID, result.Body, author); !ok {
			result = discussion.Result{Status: "interrupted", Body: "The original message was removed before this reply arrived."}
		} else if err := sess.SyncWriteFiles(); err != nil {
			result = discussion.Result{Status: "unavailable", Body: "The reply could not be persisted. Check review storage before retrying."}
		}
	}
	s.discussionMu.Lock()
	defer s.discussionMu.Unlock()
	state, err := s.readDiscussion()
	if err != nil {
		log.Printf("discussion %s: cannot read completion state: %v", request.ID, err)
		return
	}
	for i := range state.Requests {
		if state.Requests[i].ID != request.ID {
			continue
		}
		for j := range state.Requests[i].Responses {
			response := &state.Requests[i].Responses[j]
			if response.Agent != agent {
				continue
			}
			response.Status = result.Status
			if result.Status == "replied" {
				response.ReplyID = replyID
			} else {
				response.Detail = result.Body
			}
		}
	}
	if err := s.writeDiscussion(state); err != nil {
		log.Printf("discussion %s: cannot persist completion state: %v", request.ID, err)
		return
	}
	sess.Notify(SSEEvent{Type: "comments-changed"})
	sess.Notify(SSEEvent{Type: "discussion-changed"})
}
