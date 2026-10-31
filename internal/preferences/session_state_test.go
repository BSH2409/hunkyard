package preferences

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSessionStateRoundTrip(t *testing.T) {
	s := testStore(t)
	if err := s.updateSession("session-1", func(state *sessionState) {
		state.InjectedRuleIDs = append(state.InjectedRuleIDs, "rule_a")
		state.CommitRequested = true
	}); err != nil {
		t.Fatal(err)
	}
	got := s.loadSession("session-1")
	if len(got.InjectedRuleIDs) != 1 || got.InjectedRuleIDs[0] != "rule_a" || !got.CommitRequested || got.UpdatedAt == "" {
		t.Fatalf("round trip: %+v", got)
	}
	if other := s.loadSession("session-2"); other.CommitRequested || len(other.InjectedRuleIDs) != 0 {
		t.Fatalf("sessions leak: %+v", other)
	}
}

func TestSessionStateCorruptFileIsEmpty(t *testing.T) {
	s := testStore(t)
	if err := s.updateSession("session-1", func(state *sessionState) { state.CommitRequested = true }); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(s.sessionPath("session-1"), []byte("{broken"), 0600); err != nil {
		t.Fatal(err)
	}
	if got := s.loadSession("session-1"); got.CommitRequested {
		t.Fatalf("corrupt state must read as empty: %+v", got)
	}
}

func TestPruneRetentionDropsOldSessions(t *testing.T) {
	s := testStore(t)
	for _, id := range []string{"old", "new"} {
		if err := s.updateSession(id, func(state *sessionState) { state.CommitRequested = true }); err != nil {
			t.Fatal(err)
		}
	}
	old := s.config.Clock().Add(-8 * 24 * time.Hour)
	if err := os.Chtimes(s.sessionPath("old"), old, old); err != nil {
		t.Fatal(err)
	}
	if err := s.pruneRetention(""); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(s.sessionPath("old")); !os.IsNotExist(err) {
		t.Fatal("old session kept")
	}
	if _, err := os.Stat(s.sessionPath("new")); err != nil {
		t.Fatal("recent session pruned", err)
	}
	if filepath.Dir(s.sessionPath("x")) != s.dataPath("sessions") {
		t.Fatal("session files must live under learn-preferences/sessions")
	}
}
