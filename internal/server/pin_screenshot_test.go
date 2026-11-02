package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/BSH2409/hunkyard/internal/feedback"
	"github.com/BSH2409/hunkyard/internal/session"
)

var pngBytes = []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR\x00\x00\x00\x01\x00\x00\x00\x01\x08\x06\x00\x00\x00")

func pinServer(t *testing.T) (*Server, *Session) {
	t.Helper()
	s, sess := newTestServer(t)
	s.reviewPath = t.TempDir()
	return s, sess
}

func savedShot(t *testing.T, s *Server) string {
	t.Helper()
	name, err := saveAttachment(s.reviewPath, pngBytes)
	if err != nil {
		t.Fatal(err)
	}
	return name
}

func postPin(s *Server, body string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/file/comments?path=%2F", strings.NewReader(body)))
	return rec
}

func pinBody(text, selector, extra string) string {
	return fmt.Sprintf(`{"body":%q,"dom_anchor":{"pathname":"/","css_selector":%q,"tag_chain":["BUTTON"],"outer_html":"","viewport_width":1280,"viewport_height":800}%s}`, text, selector, extra)
}

func shotJSON(file string) string {
	return fmt.Sprintf(`,"pin_screenshot":{"file":%q,"width":1280,"height":2000,"scale":1,"band_offset_y":0,"marker_rect":{"x":10,"y":20,"width":30,"height":40},"viewport":{"width":1280,"height":800,"scroll_x":0,"scroll_y":100}}`, file)
}

func TestLivePinStoresScreenshot(t *testing.T) {
	s, sess := pinServer(t)
	file := savedShot(t, s)
	rec := postPin(s, pinBody("Pay button", "#pay", shotJSON(file)))
	if rec.Code != http.StatusCreated {
		t.Fatalf("pin with screenshot = %d %s", rec.Code, rec.Body)
	}
	got := sess.GetComments("/")
	if len(got) != 1 || got[0].PinScreenshot == nil {
		t.Fatalf("screenshot not stored: %+v", got)
	}
	want := session.PinScreenshot{File: file, Width: 1280, Height: 2000, Scale: 1, MarkerRect: session.PinRect{X: 10, Y: 20, Width: 30, Height: 40}, Viewport: session.PinViewport{Width: 1280, Height: 800, ScrollY: 100}}
	if *got[0].PinScreenshot != want {
		t.Fatalf("stored %+v, want %+v", *got[0].PinScreenshot, want)
	}
}

func TestLivePinRejectsForeignScreenshot(t *testing.T) {
	s, _ := pinServer(t)
	jpg, err := saveAttachment(s.reviewPath, []byte("\xff\xd8\xff\xe0\x00\x10JFIF\x00"))
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range []string{"../x.png", "pin.png", "a1b2c3d4-0000-4000-8000-000000000000.png", jpg} {
		if rec := postPin(s, pinBody("x", "#pay", shotJSON(file))); rec.Code != http.StatusBadRequest {
			t.Errorf("screenshot %q accepted: %d", file, rec.Code)
		}
	}
	rec := postPin(s, pinBody("x", "#pay", `,"screenshot_status":"failed","screenshot_reason":"blank"`))
	if rec.Code != http.StatusCreated {
		t.Fatalf("failed capture not saved: %d %s", rec.Code, rec.Body)
	}
	var c session.Comment
	_ = json.Unmarshal(rec.Body.Bytes(), &c)
	if c.ScreenshotStatus != "failed" || c.ScreenshotReason != "blank" {
		t.Fatalf("failed capture fields = %q %q", c.ScreenshotStatus, c.ScreenshotReason)
	}
	if rec := postPin(s, pinBody("x", "#pay", `,"screenshot_status":"lost"`)); rec.Code != http.StatusBadRequest {
		t.Fatalf("unknown screenshot status accepted: %d", rec.Code)
	}
}

func TestSameSpotPinMergesIntoExistingPin(t *testing.T) {
	s, sess := pinServer(t)
	first := postPin(s, pinBody("Pay button is misaligned", "#pay", shotJSON(savedShot(t, s))))
	if first.Code != http.StatusCreated {
		t.Fatalf("first pin = %d %s", first.Code, first.Body)
	}
	var original session.Comment
	_ = json.Unmarshal(first.Body.Bytes(), &original)

	second := savedShot(t, s)
	merged := postPin(s, pinBody("Also the label is cut off", "#pay", shotJSON(second)))
	if merged.Code != http.StatusOK {
		t.Fatalf("same-spot pin = %d %s, want 200 (merged)", merged.Code, merged.Body)
	}
	pins := sess.GetComments("/")
	if len(pins) != 1 || pins[0].ID != original.ID || len(pins[0].Replies) != 1 || pins[0].Replies[0].Body != "Also the label is cut off" {
		t.Fatalf("not merged into the existing pin: %+v", pins)
	}
	if pins[0].PinScreenshot.File != original.PinScreenshot.File {
		t.Fatal("the merged pin replaced the original screenshot")
	}
	if _, err := os.Stat(filepath.Join(s.reviewPath, "attachments", second)); !os.IsNotExist(err) {
		t.Fatalf("the duplicate screenshot was left on disk (%v)", err)
	}

	if rec := postPin(s, pinBody("Different element", "#cancel", "")); rec.Code != http.StatusCreated {
		t.Fatalf("different selector = %d, want a new pin", rec.Code)
	}
	if _, ok := sess.SetCommentResolved("/", original.ID, true); !ok {
		t.Fatal("resolve failed")
	}
	if rec := postPin(s, pinBody("Back again", "#pay", "")); rec.Code != http.StatusCreated {
		t.Fatalf("same spot on a resolved pin = %d, want a new pin", rec.Code)
	}
	if got := len(sess.GetComments("/")); got != 3 {
		t.Fatalf("pins on the route = %d, want 3", got)
	}
}

func TestReanchorReplacesPinScreenshot(t *testing.T) {
	s, sess := pinServer(t)
	oldShot, newShot := savedShot(t, s), savedShot(t, s)
	rec := postPin(s, pinBody("Pay button", "#pay", shotJSON(oldShot)))
	if rec.Code != http.StatusCreated {
		t.Fatalf("pin = %d %s", rec.Code, rec.Body)
	}
	var pin session.Comment
	if err := json.Unmarshal(rec.Body.Bytes(), &pin); err != nil {
		t.Fatal(err)
	}
	put := func(body string) int {
		w := httptest.NewRecorder()
		s.ServeHTTP(w, httptest.NewRequest(http.MethodPut, "/api/comment/"+pin.ID+"?path=%2F", strings.NewReader(body)))
		return w.Code
	}
	anchor := `{"dom_anchor":{"pathname":"/","css_selector":"#pay-now","tag_chain":["BUTTON"],"outer_html":"","viewport_width":1280,"viewport_height":800}`
	if code := put(anchor + shotJSON("../x.png") + `}`); code != http.StatusBadRequest {
		t.Errorf("re-anchor with a foreign screenshot = %d, want 400", code)
	}
	if code := put(anchor + shotJSON(newShot) + `}`); code != http.StatusOK {
		t.Fatalf("re-anchor = %d", code)
	}
	got, _, _ := sess.FindCommentByID(pin.ID, "/")
	if got.DOMAnchor.CSSSelector != "#pay-now" || got.PinScreenshot == nil || got.PinScreenshot.File != newShot {
		t.Errorf("re-anchored pin = %+v / %+v", got.DOMAnchor, got.PinScreenshot)
	}
	dir := session.ReviewPathsFor(s.reviewPath).Attachments
	if _, err := os.Stat(filepath.Join(dir, oldShot)); !os.IsNotExist(err) {
		t.Errorf("old screenshot kept after re-anchor: %v", err)
	}

	if code := put(anchor + `,"screenshot_status":"failed","screenshot_reason":"blank"}`); code != http.StatusOK {
		t.Fatalf("re-anchor with failed capture = %d", code)
	}
	got, _, _ = sess.FindCommentByID(pin.ID, "/")
	if got.PinScreenshot != nil || got.ScreenshotStatus != "failed" || got.ScreenshotReason != "blank" {
		t.Errorf("failed re-capture = %+v status %q reason %q", got.PinScreenshot, got.ScreenshotStatus, got.ScreenshotReason)
	}
	if _, err := os.Stat(filepath.Join(dir, newShot)); !os.IsNotExist(err) {
		t.Errorf("replaced screenshot kept: %v", err)
	}
}

func TestReanchorKeepsAScreenshotAPublishedBatchUses(t *testing.T) {
	s, _ := pinServer(t)
	oldShot, newShot := savedShot(t, s), savedShot(t, s)
	rec := postPin(s, pinBody("Pay button", "#pay", shotJSON(oldShot)))
	var pin session.Comment
	if err := json.Unmarshal(rec.Body.Bytes(), &pin); err != nil {
		t.Fatal(err)
	}
	store, err := s.feedbackStore()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Publish("published", feedback.Batch{Version: 1, ReviewID: "012345abcdef", SnapshotID: "snapshot_" + strings.Repeat("a", 64), Round: 1, Disposition: "comment", Comments: []feedback.Comment{{
		ID: pin.ID, Body: "Pay button", Pin: &feedback.PinContext{Route: "/", Selector: "#pay", Screenshot: &feedback.PinShot{URL: "http://localhost:5000/api/attachments/" + oldShot}},
	}}}); err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	anchor := `{"dom_anchor":{"pathname":"/","css_selector":"#pay-now","tag_chain":["BUTTON"],"outer_html":"","viewport_width":1280,"viewport_height":800}`
	s.ServeHTTP(w, httptest.NewRequest(http.MethodPut, "/api/comment/"+pin.ID+"?path=%2F", strings.NewReader(anchor+shotJSON(newShot)+`}`)))
	if w.Code != http.StatusOK {
		t.Fatalf("re-anchor = %d", w.Code)
	}
	if _, err := os.Stat(filepath.Join(session.ReviewPathsFor(s.reviewPath).Attachments, oldShot)); err != nil {
		t.Fatalf("the screenshot a published batch points at was deleted: %v", err)
	}
}
