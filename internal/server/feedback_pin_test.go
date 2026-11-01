package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/BSH2409/hunkyard/internal/feedback"
	"github.com/BSH2409/hunkyard/internal/session"
)

// publishWithOwner binds a synthetic owner and publishes the review.
func publishWithOwner(t *testing.T, s *Server) feedback.Batch {
	t.Helper()
	s.reviewPath = t.TempDir()
	owner, err := feedback.OpenOwner(t.TempDir(), "abcd1234ef01", feedback.Identity{Key: "test", Kind: "claude"})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.BindOwner(owner, "abcd1234ef01", "generation"); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/finish", strings.NewReader(`{"request_id":"pins","disposition":"request_changes"}`)))
	var result struct {
		Batch feedback.Batch `json:"batch"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil || rec.Code != http.StatusOK {
		t.Fatalf("publish: %d %s", rec.Code, rec.Body)
	}
	return result.Batch
}

func TestLivePinPublishesWithRouteAndSelector(t *testing.T) {
	s, sess := newTestServer(t)
	pin, ok := sess.AddLivePin("/settings/billing", "Pay button is misaligned", "reviewer", &session.DOMAnchor{Pathname: "/settings/billing", CSSSelector: "main > form > button.primary", TagChain: []string{"BUTTON"}})
	if !ok {
		t.Fatal("pin not added")
	}
	batch := publishWithOwner(t, s)
	for _, c := range batch.Comments {
		if c.ID != pin.ID {
			continue
		}
		if c.Path != "" || c.Pin == nil || c.Pin.Route != "/settings/billing" || c.Pin.Selector != "main > form > button.primary" {
			t.Fatalf("published pin = path %q pin %+v", c.Path, c.Pin)
		}
		return
	}
	t.Fatalf("pin missing from the batch: %+v", batch.Comments)
}

func TestFeedbackPinBlocks(t *testing.T) {
	s, sess := newTestServer(t)
	s.port = 5123
	sess.ReviewRound = 2
	add := func(route, selector string) session.Comment {
		t.Helper()
		pin, ok := sess.AddLivePin(route, "pin on "+selector, "reviewer", &session.DOMAnchor{Pathname: route, CSSSelector: selector, TagChain: []string{"BUTTON"}})
		if !ok {
			t.Fatal("pin not added")
		}
		return pin
	}
	shot := &session.PinScreenshot{File: "a1b2c3d4-0000-4000-8000-000000000001.png", Width: 1280, Height: 2000, Scale: 0.5, BandOffsetY: 300, MarkerRect: session.PinRect{X: 10, Y: 20, Width: 30, Height: 40}, Viewport: session.PinViewport{Width: 1280, Height: 800, ScrollY: 100}}
	current := add("/", "#pay")
	sess.ReplacePinShot("/", current.ID, session.PinShot{Screenshot: shot})
	sess.SetPinStatus("/", current.ID, session.PinCurrent)
	outdated := add("/", "#total")
	sess.SetPinStatus("/", outdated.ID, session.PinOutdated)
	unloaded := add("/billing", "#card")
	sess.ReviewRound = 1
	sess.SetPinStatus("/billing", unloaded.ID, session.PinCurrent)
	sess.ReviewRound = 2
	failed := add("/", "#help")
	sess.ReplacePinShot("/", failed.ID, session.PinShot{Status: "failed", Reason: "blank"})
	sess.SetPinStatus("/", failed.ID, session.PinCurrent)

	pins := map[string]feedback.Comment{}
	for _, c := range publishWithOwner(t, s).Comments {
		pins[c.ID] = c
	}
	want := feedback.MarkerStyle{Color: "#FF0000", Stroke: 4, Halo: "#FFFFFF", HaloWidth: 2, Padding: 6, Label: "1"}
	for id, tc := range map[string]struct {
		status   string
		outdated bool
	}{current.ID: {"current", false}, outdated.ID: {"outdated", true}, unloaded.ID: {"unchecked", false}, failed.ID: {"current", false}} {
		c, ok := pins[id]
		if !ok || c.Pin == nil {
			t.Fatalf("pin %s missing: %+v", id, c)
		}
		if c.Pin.Status != tc.status || c.Outdated != tc.outdated || c.Pin.Marker != want {
			t.Errorf("pin %s: status %q outdated %v marker %+v", id, c.Pin.Status, c.Outdated, c.Pin.Marker)
		}
	}
	got := pins[current.ID].Pin
	if got.Screenshot == nil || got.Screenshot.URL != "http://localhost:5123/api/attachments/"+shot.File || got.Screenshot.Width != 1280 || got.Screenshot.Scale != 0.5 || got.Screenshot.BandOffsetY != 300 {
		t.Errorf("screenshot = %+v", got.Screenshot)
	}
	if got.MarkerRect != (feedback.Rect{X: 10, Y: 20, Width: 30, Height: 40}) || got.Viewport != (feedback.Viewport{Width: 1280, Height: 800, ScrollY: 100}) {
		t.Errorf("marker rect %+v viewport %+v", got.MarkerRect, got.Viewport)
	}
	if f := pins[failed.ID].Pin; f.Screenshot != nil || f.ScreenshotStatus != "failed" || f.ScreenshotReason != "blank" {
		t.Errorf("failed capture = %+v", f)
	}
}
