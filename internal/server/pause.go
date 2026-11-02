package server

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"github.com/BSH2409/hunkyard/internal/session"
)

// RunIdleMonitor pauses the review once it has been idle for idle: tabs are
// told, a paused marker is written and the daemon stops, freeing its port.
// A zero idle disables pausing. It returns when the daemon stops.
func (s *Server) RunIdleMonitor(ctx context.Context, idle, every time.Duration) {
	if idle <= 0 {
		return
	}
	ticker := time.NewTicker(every)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if s.pauseIfIdle(idle, time.Now()) {
				return
			}
		}
	}
}

// pauseIfIdle pauses when nothing is in flight, no round transition is
// running and the last activity is at least idle before now.
func (s *Server) pauseIfIdle(idle time.Duration, now time.Time) bool {
	sess := s.session.Load()
	if idle <= 0 || sess == nil || s.inFlight.Load() > 0 || sess.RoundBusy() {
		return false
	}
	if now.Sub(sess.LastActivity()) < idle || sess.AgentTurnActive(now, 3*idle) {
		return false
	}
	// A request counts itself before checking pausing, and the pause is set
	// before this re-check, so a request is either seen here or refused.
	s.pausing.Store(true)
	if s.inFlight.Load() > 0 {
		s.pausing.Store(false)
		return false
	}
	payload, _ := json.Marshal(map[string]string{
		"review_id": sess.SessionKey,
		"command":   "hunkyard --session " + sess.SessionKey,
	})
	sess.Notify(session.SSEEvent{Type: "paused", Content: string(payload)})
	if err := session.WritePausedMarker(s.reviewPath, session.PausedMarker{PausedAt: now.UTC(), Port: s.port, Host: s.listenHost, CWD: sess.CWD, CliArgs: sess.CLIArgs}); err != nil {
		log.Printf("Warning: could not record the pause: %v", err)
	} else if err := session.RecordPausedReview(sess.SessionKey, s.reviewPath); err != nil {
		log.Printf("Warning: could not record where the paused review lives: %v", err)
	}
	if s.stopDaemon != nil {
		s.stopDaemon()
	}
	return true
}
