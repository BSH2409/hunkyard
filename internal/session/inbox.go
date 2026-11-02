package session

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/BSH2409/hunkyard/internal/daemon"
)

// InboxState is what a running review daemon reports about itself to the
// inbox.
type InboxState struct {
	Round           int    `json:"round"`
	WaitingForAgent bool   `json:"waiting_for_agent"`
	Unresolved      int    `json:"unresolved"`
	LastActivity    string `json:"last_activity,omitempty"`
}

// InboxEntry is one review on this machine as the inbox lists it.
type InboxEntry struct {
	Key          string   `json:"key"`
	Label        string   `json:"label"`
	Repository   string   `json:"repository"`
	Cwd          string   `json:"cwd"`
	Branch       string   `json:"branch,omitempty"`
	Args         []string `json:"args,omitempty"`
	State        string   `json:"state"` // waiting_for_reviewer | agent_working | paused | unreachable
	Round        int      `json:"round"`
	Unresolved   int      `json:"unresolved"`
	URL          string   `json:"url,omitempty"`
	LastActivity string   `json:"last_activity,omitempty"`
	PID          int      `json:"pid,omitempty"`
	Port         int      `json:"port,omitempty"`
}

// InboxState reports this review's round, open comments and whether the
// agent is working on it.
func (s *Session) InboxState() InboxState {
	s.mu.RLock()
	state := InboxState{Round: s.ReviewRound, WaitingForAgent: s.waitingForAgent}
	for _, c := range s.reviewComments {
		if !c.Resolved {
			state.Unresolved++
		}
	}
	for _, f := range s.Files {
		for _, c := range f.Comments {
			if !c.Resolved {
				state.Unresolved++
			}
		}
	}
	s.mu.RUnlock()
	if last := s.LastActivity(); !last.IsZero() {
		state.LastActivity = last.UTC().Format(time.RFC3339)
	}
	return state
}

const inboxProbeTimeout = time.Second

// ListInbox lists every running review daemon on this machine and every
// paused review: live ones first, then by last activity.
func ListInbox(ctx context.Context) ([]InboxEntry, error) {
	sessions, keys := daemon.ListAllSessions()
	entries := make([]InboxEntry, len(sessions))
	running := map[string]bool{}
	var wg sync.WaitGroup
	limit := make(chan struct{}, 8)
	client := &http.Client{Timeout: inboxProbeTimeout}
	for i, entry := range sessions {
		running[keys[i]] = true
		label := entry.Branch
		if len(entry.Args) > 0 {
			label = strings.Join(entry.Args, " ")
		}
		if label == "" {
			label = keys[i]
		}
		entries[i] = InboxEntry{Key: keys[i], Label: label, Repository: repositoryName(entry.CWD), Cwd: entry.CWD, Branch: entry.Branch, Args: entry.Args, URL: entry.BaseURL(), PID: entry.PID, Port: entry.Port}
		wg.Add(1)
		go func(e *InboxEntry, conn string) {
			defer wg.Done()
			limit <- struct{}{}
			defer func() { <-limit }()
			state, err := probeInboxState(ctx, client, conn)
			if err != nil {
				e.State = "unreachable"
				return
			}
			e.State = "waiting_for_reviewer"
			if state.WaitingForAgent {
				e.State = "agent_working"
			}
			e.Round, e.Unresolved, e.LastActivity = state.Round, state.Unresolved, state.LastActivity
		}(&entries[i], entry.ConnURL())
	}
	wg.Wait()

	paused, err := pausedInboxEntries(running)
	if err != nil {
		return entries, err
	}
	entries = append(entries, paused...)
	sort.SliceStable(entries, func(i, j int) bool {
		li, lj := entries[i].State != "paused", entries[j].State != "paused"
		if li != lj {
			return li
		}
		return entries[i].LastActivity > entries[j].LastActivity
	})
	return entries, nil
}

// pausedInboxEntries lists review folders with a paused marker that no
// daemon serves. The marker records the directory and arguments, so a review
// with no comments (and no review.json) is listed too.
func pausedInboxEntries(running map[string]bool) ([]InboxEntry, error) {
	dir, err := daemon.ReviewsDir()
	if err != nil {
		return nil, err
	}
	folders, err := readReviewsDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []InboxEntry
	for _, f := range folders {
		key := f.Name()
		if !f.IsDir() || !daemon.ValidSessionKey(key) || running[key] {
			continue
		}
		folder := filepath.Join(dir, key)
		marker, paused := ReadPausedMarker(folder)
		if !paused {
			continue
		}
		e := InboxEntry{Key: key, State: "paused", Cwd: marker.CWD, Args: marker.CliArgs, LastActivity: marker.PausedAt.UTC().Format(time.RFC3339)}
		if review, ok := readResumableReview(folder, key); ok {
			e.Branch, e.Unresolved, e.Label = review.branch, review.unresolved, review.label()
			if review.cwd != "" {
				e.Cwd = review.cwd
			}
			if cj, err := readHunkyardJSONFromDisk(folder); err == nil {
				e.Round = cj.ReviewRound
			}
		}
		if e.Label == "" {
			e.Label = strings.Join(e.Args, " ")
		}
		if e.Label == "" {
			e.Label = key
		}
		e.Repository = repositoryName(e.Cwd)
		out = append(out, e)
	}
	return out, nil
}

func probeInboxState(ctx context.Context, client *http.Client, base string) (InboxState, error) {
	var state InboxState
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/api/inbox/state", nil)
	if err != nil {
		return state, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return state, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return state, fmt.Errorf("inbox state: %s", resp.Status)
	}
	err = json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(&state)
	return state, err
}

func reviewFolder(key string) string {
	dir, err := daemon.ReviewsDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, key)
}

func repositoryName(cwd string) string {
	if cwd == "" {
		return ""
	}
	return filepath.Base(cwd)
}

// ResumePausedReview restarts a paused review's daemon (on its recorded
// port when free) and returns the review's URL.
func ResumePausedReview(key string) (string, error) {
	if !daemon.ValidSessionKey(key) {
		return "", fmt.Errorf("invalid review id")
	}
	if entry, alive := daemon.FindAliveSession(key); alive {
		return entry.BaseURL(), nil
	}
	if _, paused := ReadPausedMarker(reviewFolder(key)); !paused {
		return "", fmt.Errorf("review %s is not paused", key)
	}
	entry, err := reconnectDeadSession(key, daemon.SessionEntry{}, true)
	if err != nil {
		return "", err
	}
	return entry.BaseURL(), nil
}
