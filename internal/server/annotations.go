package server

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/BSH2409/hunkyard/internal/annotate"
	"github.com/BSH2409/hunkyard/internal/session"
)

// annotationsRequest is one tool run imported by `hunkyard annotate`.
type annotationsRequest struct {
	Tool     string                     `json:"tool"`
	Findings []annotate.Finding         `json:"findings"`
	Marks    map[string][]annotate.Mark `json:"marks"`
}

type annotationsResponse struct {
	session.ReplaceResult
	Coverage map[string]session.Coverage `json:"coverage"`
}

// handleAnnotations replaces one tool's findings and marks (POST) or lists
// every tool's marks (GET).
func (s *Server) handleAnnotations(w http.ResponseWriter, r *http.Request) {
	sess := s.session.Load()
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, map[string]any{"tools": sess.ToolMarks()})
	case http.MethodPost:
		r.Body = http.MaxBytesReader(w, r.Body, 10<<20)
		var req annotationsRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			var tooLarge *http.MaxBytesError
			if errors.As(err, &tooLarge) {
				http.Error(w, "annotations too large", http.StatusRequestEntityTooLarge)
				return
			}
			http.Error(w, "invalid annotations", http.StatusBadRequest)
			return
		}
		if req.Tool == "" || len(req.Tool) > 128 {
			http.Error(w, "tool name required", http.StatusBadRequest)
			return
		}
		incoming := make([]session.MachineComment, 0, len(req.Findings))
		for _, f := range req.Findings {
			body := f.Message
			if f.Rule != "" {
				body += " `" + f.Rule + "`"
			}
			incoming = append(incoming, session.MachineComment{Path: f.Path, Line: f.Line, EndLine: f.EndLine, Body: body, Author: req.Tool})
		}
		resp := annotationsResponse{ReplaceResult: sess.ReplaceMachineComments("tool", req.Tool, incoming)}
		if req.Marks != nil {
			marks := make(map[string][]session.MarkRange, len(req.Marks))
			for path, ranges := range req.Marks {
				for _, m := range ranges {
					marks[path] = append(marks[path], session.MarkRange{Start: m.Start, End: m.End, Kind: m.Kind})
				}
			}
			resp.Coverage = sess.SetToolMarks(req.Tool, marks)
		}
		writeJSON(w, resp)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}
