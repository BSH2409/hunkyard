package server

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/BSH2409/hunkyard/internal/feedback"
)

func TestOwnerChallengeAndProtectedReview(t *testing.T) {
	s, _ := newTestServer(t)
	owner, err := feedback.OpenOwner(t.TempDir(), "012345abcdef", feedback.Identity{Key: "test", Kind: "codex"})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.BindOwner(owner, "012345abcdef", "generation-a"); err != nil {
		t.Fatal(err)
	}
	nonce := base64.RawURLEncoding.EncodeToString(make([]byte, 32))
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/owner/challenge", strings.NewReader(`{"nonce":"`+nonce+`"}`)))
	var proof struct {
		Proof string `json:"proof"`
	}
	if rec.Code != 200 || json.Unmarshal(rec.Body.Bytes(), &proof) != nil {
		t.Fatalf("challenge: %d %s", rec.Code, rec.Body)
	}
	if err := owner.VerifyProof(nonce, "012345abcdef", "generation-a", proof.Proof); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(rec.Body.String(), owner.Credential()) {
		t.Fatal("credential exposed")
	}
	rec = httptest.NewRecorder()
	s.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/review-cycle", nil))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("unowned review consumer: %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	s.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/session", nil))
	if !strings.Contains(rec.Body.String(), `"kind":"codex"`) || strings.Contains(rec.Body.String(), owner.Credential()) {
		t.Fatal("public owner identity missing or secret exposed")
	}
	if err := s.BindOwner(&feedback.Owner{}, "012345abcdef", "generation-a"); err == nil {
		t.Fatal("zero owner accepted")
	}
}
