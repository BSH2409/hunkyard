package daemon

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/BSH2409/hunkyard/internal/feedback"
	"github.com/BSH2409/hunkyard/internal/localonly"
	"github.com/BSH2409/hunkyard/internal/strictjson"
)

// OwnerRequest verifies the daemon before delivering a private owner credential.
func OwnerRequest(ctx context.Context, entry SessionEntry, key, method, path string, body io.Reader) (*http.Response, error) {
	if !strings.HasPrefix(path, "/") || strings.HasPrefix(path, "//") {
		return nil, fmt.Errorf("invalid local request path")
	}
	if err := localonly.CheckURL(entry.ConnURL()); err != nil {
		return nil, err
	}
	owner, err := LoadSessionOwner(key)
	if err != nil {
		return nil, err
	}

	var nonceBytes [32]byte
	if _, err := rand.Read(nonceBytes[:]); err != nil {
		return nil, err
	}
	nonce := base64.RawURLEncoding.EncodeToString(nonceBytes[:])
	encoded, _ := json.Marshal(map[string]string{"nonce": nonce})
	challengeCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	challenge, err := http.NewRequestWithContext(challengeCtx, http.MethodPost, entry.ConnURL()+"/api/owner/challenge", bytes.NewReader(encoded))
	if err != nil {
		return nil, err
	}
	challenge.Header.Set("Content-Type", "application/json")
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	response, err := client.Do(challenge)
	if err != nil {
		return nil, fmt.Errorf("verifying local review server: %w", err)
	}
	proofBody, err := io.ReadAll(io.LimitReader(response.Body, 4097))
	response.Body.Close()
	var proof struct {
		Proof string `json:"proof"`
	}
	if err != nil || len(proofBody) > 4096 || response.StatusCode != 200 || strictjson.Decode(proofBody, &proof) != nil || owner.VerifyProof(nonce, key, entry.StartedAt, proof.Proof) != nil {
		return nil, fmt.Errorf("local review server identity verification failed")
	}
	request, err := http.NewRequestWithContext(ctx, method, entry.ConnURL()+path, body)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set(feedback.OwnerHeader, owner.Credential())
	return client.Do(request)
}

func LoadSessionOwner(key string) (*feedback.Owner, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	return feedback.LoadOwner(filepath.Join(home, ".hunkyard"), key, feedback.CurrentIdentity())
}
