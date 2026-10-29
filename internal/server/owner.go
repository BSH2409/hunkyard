package server

import (
	"fmt"
	"io"
	"net/http"

	"github.com/BSH2409/hunkyard/internal/feedback"
	"github.com/BSH2409/hunkyard/internal/strictjson"
)

// BindOwner is called once before the daemon serves requests.
func (s *Server) BindOwner(owner *feedback.Owner, reviewID, generation string) error {
	if owner == nil || owner.Verify(owner.Credential()) != nil {
		return fmt.Errorf("invalid review owner")
	}
	if _, err := owner.Proof("AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA", reviewID, generation); err != nil {
		return err
	}
	s.owner, s.ownerReviewID, s.ownerGeneration = owner, reviewID, generation
	return nil
}

func (s *Server) handleOwnerChallenge(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.owner == nil {
		http.NotFound(w, r)
		return
	}
	var request struct {
		Nonce string `json:"nonce"`
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1024))
	if err != nil || strictjson.Decode(body, &request) != nil {
		http.Error(w, "invalid owner challenge", 400)
		return
	}
	proof, err := s.owner.Proof(request.Nonce, s.ownerReviewID, s.ownerGeneration)
	if err != nil {
		http.Error(w, "invalid owner challenge", 400)
		return
	}
	writeJSON(w, map[string]string{"proof": proof})
}
