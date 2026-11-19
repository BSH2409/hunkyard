package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/BSH2409/hunkyard/internal/feedback"
)

func TestFeedbackAwaitReturnsOnPublication(t *testing.T) {
	s, _ := newTestServer(t)
	s.reviewPath = t.TempDir()
	owner, err := feedback.OpenOwner(t.TempDir(), "abcd1234ef01", feedback.Identity{Key: "test", Kind: "claude"})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.BindOwner(owner, "abcd1234ef01", "generation"); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	req := httptest.NewRequest(http.MethodGet, "/api/feedback/await?timeout=45s", nil).WithContext(ctx)
	req.Header.Set(feedback.OwnerHeader, owner.Credential())
	result := httptest.NewRecorder()
	done := make(chan struct{})
	go func() { defer close(done); s.ServeHTTP(result, req) }()
	published := httptest.NewRecorder()
	s.ServeHTTP(published, httptest.NewRequest(http.MethodPost, "/api/finish", strings.NewReader(`{"request_id":"wake-publication","disposition":"comment","summary":"Check the changes"}`)))
	if published.Code != http.StatusOK {
		t.Fatalf("publish: %d %s", published.Code, published.Body)
	}
	select {
	case <-done:
		if result.Code != http.StatusOK || !strings.Contains(result.Body.String(), `"status":"published"`) {
			t.Fatalf("await: %d %s", result.Code, result.Body)
		}
	case <-ctx.Done():
		t.Fatal("publication did not release waiter")
	}
	store, err := s.feedbackStore()
	if err != nil {
		t.Fatal(err)
	}
	batches, err := store.Pending()
	if err != nil || len(batches) != 1 {
		t.Fatalf("pending: %v %v", batches, err)
	}
	excluded := httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/feedback/await?timeout=1ms&exclude="+batches[0].ID, nil)
	req.Header.Set(feedback.OwnerHeader, owner.Credential())
	s.ServeHTTP(excluded, req)
	if !strings.Contains(excluded.Body.String(), `"status":"pending"`) {
		t.Fatalf("excluded batch redelivered: %s", excluded.Body)
	}
}
