package session

import (
	"os"
	"strings"
	"time"
)

// activityWriteInterval throttles how often activity reaches the activity
// file, which only serves status tools and the daemon's own restart.
const activityWriteInterval = time.Minute

// SetActivityFile names the file outside activity is reported through: hooks
// for the owning agent session and the headless `hunkyard comment` touch it.
func (s *Session) SetActivityFile(path string) {
	s.activityMu.Lock()
	s.activityPath = path
	s.activityMu.Unlock()
}

// MarkActivity records reviewer or agent activity that keeps the review
// awake: tab interactions, state-changing requests and agent commands.
func (s *Session) MarkActivity(source string) {
	now := time.Now()
	s.activityMu.Lock()
	s.lastActivity = now
	path, written := s.activityPath, s.activityWritten
	write := path != "" && now.Sub(written) >= activityWriteInterval
	if write {
		s.activityWritten = now
	}
	s.activityMu.Unlock()
	if !write {
		return
	}
	// Keep a hook's turn marker (the content); only move the time.
	if err := os.Chtimes(path, now, now); os.IsNotExist(err) {
		_ = os.WriteFile(path, []byte("idle\n"), 0o600)
	}
}

// LastActivity is the latest of in-process activity and the activity file's
// modification time.
func (s *Session) LastActivity() time.Time {
	s.activityMu.Lock()
	last, path := s.lastActivity, s.activityPath
	s.activityMu.Unlock()
	if path != "" {
		if info, err := os.Stat(path); err == nil && info.ModTime().After(last) {
			last = info.ModTime()
		}
	}
	return last
}

// AgentTurnActive reports whether the owning agent is mid-turn: its hooks
// wrote "turn" to the activity file within window. A session that crashed
// mid-turn never writes "idle", so an older marker is ignored.
func (s *Session) AgentTurnActive(now time.Time, window time.Duration) bool {
	s.activityMu.Lock()
	path := s.activityPath
	s.activityMu.Unlock()
	if path == "" {
		return false
	}
	info, err := os.Stat(path)
	if err != nil || now.Sub(info.ModTime()) > window {
		return false
	}
	data, err := os.ReadFile(path)
	return err == nil && strings.TrimSpace(string(data)) == "turn"
}
