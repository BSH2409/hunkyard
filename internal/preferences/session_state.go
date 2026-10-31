package preferences

import (
	"encoding/json"
	"time"
)

// sessionState is what the hooks remember about one host session.
type sessionState struct {
	InjectedRuleIDs []string `json:"injected_rule_ids"`
	CommitRequested bool     `json:"commit_requested"`
	StopBlockedTurn string   `json:"stop_blocked_turn,omitempty"`
	TurnDiffDigest  string   `json:"turn_diff_digest,omitempty"`
	TurnAddedLines  []string `json:"turn_added_lines,omitempty"`
	UpdatedAt       string   `json:"updated_at"`
}

func (s *Store) sessionPath(sessionID string) string {
	return s.dataPath("sessions", digest(sessionID)+".json")
}

func (s *Store) loadSession(sessionID string) sessionState {
	var state sessionState
	data, err := regularFile(s.sessionPath(sessionID))
	if err != nil || json.Unmarshal(data, &state) != nil {
		return sessionState{}
	}
	return state
}

func (s *Store) updateSession(sessionID string, fn func(*sessionState)) error {
	if err := ensureDirectory(s.dataPath("sessions")); err != nil {
		return err
	}
	name := s.sessionPath(sessionID)
	return withFileLock(name+".lock", false, func() error {
		state := s.loadSession(sessionID)
		fn(&state)
		state.InjectedRuleIDs = sortedUnique(state.InjectedRuleIDs)
		state.UpdatedAt = s.config.Clock().UTC().Format(time.RFC3339)
		encoded, err := json.Marshal(state)
		if err != nil {
			return err
		}
		return atomicReplace(name, encoded)
	})
}
