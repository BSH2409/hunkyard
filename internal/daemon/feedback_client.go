package daemon

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/BSH2409/hunkyard/internal/feedback"
	"io"
	"net/http"
	"os"
	"time"
)

func runFeedbackClient(entry SessionEntry, key string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Second)
	defer cancel()
	response, err := OwnerRequest(ctx, entry, key, http.MethodPost, "/api/feedback/round", nil)
	if err != nil {
		return err
	}
	response.Body.Close()
	// A reconnect with unpublished handling work returns the queue without advancing a round.
	if response.StatusCode != 200 && response.StatusCode != 409 {
		return fmt.Errorf("cannot begin review round (HTTP %d)", response.StatusCode)
	}
	response, err = OwnerRequest(ctx, entry, key, http.MethodGet, "/api/feedback/await?timeout=45s", nil)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return fmt.Errorf("cannot await published feedback (HTTP %d)", response.StatusCode)
	}
	_, err = io.Copy(os.Stdout, io.LimitReader(response.Body, 32<<20))
	return err
}

func RegisterSkills(entry SessionEntry, key, path string) error {
	refs, err := feedback.LoadSkillManifest(path)
	if err != nil {
		return err
	}
	body, err := json.Marshal(map[string]any{"version": 1, "skills": refs})
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	response, err := OwnerRequest(ctx, entry, key, http.MethodPost, "/api/feedback/skills", bytes.NewReader(body))
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return fmt.Errorf("host skill registration failed (HTTP %d)", response.StatusCode)
	}
	return nil
}
