package main

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
	"time"

	"github.com/BSH2409/hunkyard/internal/daemon"
	"github.com/BSH2409/hunkyard/internal/session"
)

// A daemon that never answers must not hang `hunkyard story`.
func TestStoryPostGivesUpOnAStuckDaemon(t *testing.T) {
	release := make(chan struct{})
	stuck := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-release }))
	defer stuck.Close()
	defer close(release)
	previous := storyHTTPClient.Timeout
	storyHTTPClient.Timeout = 50 * time.Millisecond
	defer func() { storyHTTPClient.Timeout = previous }()
	u, _ := url.Parse(stuck.URL)
	port, _ := strconv.Atoi(u.Port())
	done := make(chan error, 1)
	go func() {
		done <- postStoryToDaemon(daemon.SessionEntry{Host: u.Hostname(), Port: port}, &session.Story{})
	}()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("posting to a stuck daemon succeeded")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("posting the story hung on a stuck daemon")
	}
}
