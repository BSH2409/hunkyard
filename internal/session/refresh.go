package session

import "fmt"

func (s *Session) RefreshWorkingTree() error {
	s.mu.Lock()
	mode, kind, base := s.Mode, s.Focus.Kind, s.BaseBranchName
	if kind == FocusRange {
		s.mu.Unlock()
		return fmt.Errorf("this review is pinned to commits; switch to the working tree to include staged and unstaged changes")
	}
	if mode != "git" {
		s.rereadFileContents(true)
		s.stale = false
		s.mu.Unlock()
		s.RefreshDiffs()
		s.notify(SSEEvent{Type: "base-changed"})
		return nil
	}
	s.mu.Unlock()
	if base == "" {
		return fmt.Errorf("no base branch to refresh against")
	}
	s.invalidateWorkingTreeSnapshot()
	return s.ChangeBaseBranch(base)
}
