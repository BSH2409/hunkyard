package preferences

import (
	"fmt"
	"io"
	"os"
	"slices"
	"time"
)

// ReviewNotice is published review feedback waiting for the host session
// that launched the review.
type ReviewNotice struct {
	ReviewID    string
	Pending     int
	BatchID     string
	Disposition string
}

// reviewNotices and reviewWaiter connect hooks to Hunkyard review sessions.
// The review packages import this one, so cmd/hunkyard wires them in; nil
// means no review integration (tests, library use).
var (
	reviewNotices func(hostKind, hostSessionID string) []ReviewNotice
	reviewWaiter  func(reviewID string, timeout time.Duration, accept func(batchID string) bool, active func() bool) (batchID, disposition string, ok bool)
)

// SetReviewIntegration wires review lookups into the preference hooks.
func SetReviewIntegration(notices func(hostKind, hostSessionID string) []ReviewNotice, waiter func(reviewID string, timeout time.Duration, accept func(string) bool, active func() bool) (string, string, bool)) {
	reviewNotices, reviewWaiter = notices, waiter
}

func (s *Store) ownedReviewNotices(sessionID string) []ReviewNotice {
	if reviewNotices == nil || s.hostKind == "" || sessionID == "" {
		return nil
	}
	return reviewNotices(s.hostKind, sessionID)
}

const reviewWakeTimeout = 3600

// claimBatch records that the agent was told about batchID and reports
// whether this caller was first, so the Stop block and any number of wake
// waiters deliver each publication once.
func (s *Store) claimBatch(session, batchID string) bool {
	claimed := false
	_ = s.updateSession(session, func(state *sessionState) {
		if !slices.Contains(state.NotifiedBatchIDs, batchID) {
			state.NotifiedBatchIDs = append(state.NotifiedBatchIDs, batchID)
			claimed = true
		}
	})
	return claimed
}

// reviewWakeHook runs in the background after Claude stops. It waits for the
// reviewer to publish and exits 2 so Claude wakes even when idle. A newer
// waiter supersedes older ones, and each batch is delivered once.
func (s *Store) reviewWakeHook(input io.Reader, errorOutput io.Writer) int {
	payload := readHookPayload(input)
	if payload == nil || reviewWaiter == nil || payload["stop_hook_active"] == true || os.Getenv("LEARN_PREFERENCES_MAINTENANCE") == "1" {
		return 0
	}
	session := textField(payload["session_id"])
	notices := s.ownedReviewNotices(session)
	if len(notices) == 0 {
		return 0
	}
	token := randomID()
	if err := s.updateSession(session, func(state *sessionState) { state.WakeToken = token }); err != nil {
		return 0
	}
	accept := func(batchID string) bool { return !slices.Contains(s.loadSession(session).NotifiedBatchIDs, batchID) }
	active := func() bool { return s.loadSession(session).WakeToken == token }
	review := notices[0].ReviewID
	batchID, disposition, ok := reviewWaiter(review, (reviewWakeTimeout-30)*time.Second, accept, active)
	if !ok || !s.claimBatch(session, batchID) {
		return 0
	}
	fmt.Fprintf(errorOutput, "The reviewer published Hunkyard review %s (%s). Run: hunkyard feedback await --session %s --json\n", review, disposition, review)
	return 2
}
