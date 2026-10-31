package server

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/BSH2409/hunkyard/internal/preferences"
)

// handleYard serves the Preference Yard API. It needs no review session, so
// it works inside a review and under `hunkyard yard`. Changes require
// Sec-Fetch-Site: same-origin, which stops a page on another site from
// changing rules through the reviewer's browser. It does not authenticate:
// any local client can send that header.
//
//nolint:gocyclo // One case per Yard endpoint keeps the small API in one place.
func (s *Server) handleYard(w http.ResponseWriter, r *http.Request) {
	store, err := preferences.Open(preferences.Config{})
	if err != nil {
		http.Error(w, "cannot open preference store", http.StatusInternalServerError)
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/api/yard/")
	if r.Method != http.MethodGet && r.Header.Get("Sec-Fetch-Site") != "same-origin" {
		http.Error(w, "changes are only accepted from the Yard page", http.StatusForbidden)
		return
	}
	var result any
	switch {
	case r.Method == http.MethodGet && path == "rules":
		result, err = store.YardRules()
	case r.Method == http.MethodGet && path == "persona":
		result, err = store.YardPersona()
	case r.Method == http.MethodGet && path == "pipeline":
		result, err = store.YardPipeline()
	case r.Method == http.MethodGet && path == "loads":
		result, err = store.YardLoads()
	case r.Method == http.MethodGet && path == "health":
		result = store.YardHealth()
	case r.Method == http.MethodGet && path == "activity":
		limit, convErr := strconv.Atoi(r.URL.Query().Get("limit"))
		if convErr != nil || limit <= 0 || limit > 500 {
			limit = 100
		}
		result, err = store.Activity(limit)
	case r.Method == http.MethodPost && path == "rules":
		var body struct {
			Directive, Scope, RepoKey, Feedback string
			Surfaces                            []string
		}
		if decodeErr := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64*1024)).Decode(&body); decodeErr != nil {
			http.Error(w, "invalid rule", http.StatusBadRequest)
			return
		}
		result, err = store.YardCreateRule(body.Directive, body.Scope, body.RepoKey, body.Surfaces, body.Feedback)
	case r.Method == http.MethodPost && path == "maintenance":
		result, err = store.YardRunMaintenance()
	case r.Method == http.MethodPost && strings.HasPrefix(path, "rules/"):
		id, action, _ := strings.Cut(strings.TrimPrefix(path, "rules/"), "/")
		switch action {
		case "retire":
			err = store.YardRetire(id)
		case "promote":
			err = store.YardPromote(id)
		default:
			http.NotFound(w, r)
			return
		}
		result = map[string]any{"status": "success"}
	default:
		http.NotFound(w, r)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	writeJSON(w, result)
}
