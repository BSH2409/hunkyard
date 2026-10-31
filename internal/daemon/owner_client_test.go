package daemon

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"sync/atomic"
	"testing"

	"github.com/BSH2409/hunkyard/internal/feedback"
)

func TestOwnerRequestVerifiesServerBeforeCredential(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home) // Windows reads the home directory from it
	t.Setenv("HUNKYARD_OWNER_ID", "codex:synthetic")
	owner, err := feedback.OpenOwner(filepath.Join(os.Getenv("HOME"), ".hunkyard"), "012345abcdef", feedback.CurrentIdentity())
	if err != nil {
		t.Fatal(err)
	}
	var forged atomic.Bool
	var received atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/owner/challenge" {
			if r.Header.Get(feedback.OwnerHeader) != "" {
				t.Error("credential sent before proof")
			}
			var challenge struct {
				Nonce string `json:"nonce"`
			}
			json.NewDecoder(r.Body).Decode(&challenge)
			proof, _ := owner.Proof(challenge.Nonce, "012345abcdef", "generation-a")
			if forged.Load() {
				proof = "forged"
			}
			json.NewEncoder(w).Encode(map[string]string{"proof": proof})
			return
		}
		if owner.Verify(r.Header.Get(feedback.OwnerHeader)) != nil {
			t.Error("missing owner credential")
		}
		received.Add(1)
		w.WriteHeader(204)
	}))
	defer server.Close()
	u, _ := url.Parse(server.URL)
	port, _ := strconv.Atoi(u.Port())
	entry := SessionEntry{Host: u.Hostname(), Port: port, StartedAt: "generation-a"}
	resp, err := OwnerRequest(context.Background(), entry, "012345abcdef", http.MethodPost, "/test", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	forged.Store(true)
	if _, err := OwnerRequest(context.Background(), entry, "012345abcdef", http.MethodPost, "/test", nil); err == nil {
		t.Fatal("forged server received authenticated request")
	}
	if received.Load() != 1 {
		t.Fatalf("credential deliveries: %d", received.Load())
	}
	t.Setenv("HUNKYARD_OWNER_ID", "claude:other")
	if _, err := OwnerRequest(context.Background(), entry, "012345abcdef", http.MethodPost, "/test", nil); err == nil {
		t.Fatal("other owner accepted")
	}
}
