package session

import (
	"encoding/json"
	"os"
	"slices"
)

// PinScreenshot is the page image captured when a pin was made, stored as an
// attachment in the review folder, with where the marker sits in it.
type PinScreenshot struct {
	File        string      `json:"file"`
	Width       int         `json:"width"`
	Height      int         `json:"height"`
	Scale       float64     `json:"scale"`
	BandOffsetY int         `json:"band_offset_y"`
	MarkerRect  PinRect     `json:"marker_rect"`
	Viewport    PinViewport `json:"viewport"`
}

// PinRect is a rectangle in screenshot pixels.
type PinRect struct {
	X      int `json:"x"`
	Y      int `json:"y"`
	Width  int `json:"width"`
	Height int `json:"height"`
}

// PinViewport is the frame's viewport and scroll position at capture time.
type PinViewport struct {
	Width   int `json:"width"`
	Height  int `json:"height"`
	ScrollX int `json:"scroll_x"`
	ScrollY int `json:"scroll_y"`
}

// PinShot is what a new pin brings about its screenshot: the image, or why
// there is none.
type PinShot struct {
	Screenshot *PinScreenshot
	Status     string
	Reason     string
}

// samePin reports whether two anchors name exactly the same element.
func samePin(a, b *DOMAnchor) bool {
	return a != nil && b != nil && a.Pathname == b.Pathname && a.CSSSelector == b.CSSSelector && slices.Equal(a.TagChain, b.TagChain)
}

// PlaceLivePin adds a pin, or, when an unresolved pin on exactly the same
// element exists, adds the body as a reply to it (merged is true; the new
// screenshot is not used).
func (s *Session) PlaceLivePin(filePath, body, author string, anchor *DOMAnchor, shot PinShot) (c Comment, merged, ok bool) {
	s.mu.RLock()
	existingID := ""
	if fe := s.fileByPathLocked(filePath); fe != nil {
		for _, existing := range fe.Comments {
			if !existing.Resolved && samePin(existing.DOMAnchor, anchor) {
				existingID = existing.ID
				break
			}
		}
	}
	s.mu.RUnlock()
	if existingID != "" {
		if _, ok := s.AddReply(filePath, existingID, body, author); !ok {
			return Comment{}, false, false
		}
		s.mu.RLock()
		defer s.mu.RUnlock()
		f, i := s.locateFileCommentLocked(filePath, existingID)
		if f == nil {
			return Comment{}, false, false
		}
		return f.Comments[i], true, true
	}
	c, ok = s.AddLivePin(filePath, body, author, anchor)
	if !ok {
		return Comment{}, false, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	f, i := s.locateFileCommentLocked(filePath, c.ID)
	if f == nil {
		return Comment{}, false, false
	}
	f.Comments[i].PinScreenshot = shot.Screenshot
	f.Comments[i].ScreenshotStatus = shot.Status
	f.Comments[i].ScreenshotReason = shot.Reason
	s.scheduleWrite()
	return f.Comments[i], false, true
}

// removePinScreenshots deletes every pin screenshot that the review folder's
// review.json names, leaving the review and pasted images in place.
func removePinScreenshots(reviewPath string) {
	paths := ReviewPathsFor(reviewPath)
	data, err := os.ReadFile(paths.Review)
	if err != nil {
		return
	}
	var review struct {
		Files map[string]struct {
			Comments []Comment `json:"comments"`
		} `json:"files"`
	}
	if json.Unmarshal(data, &review) != nil {
		return
	}
	for _, f := range review.Files {
		for _, c := range f.Comments {
			if c.PinScreenshot == nil {
				continue
			}
			if path, _, err := attachmentPathFor(reviewPath, c.PinScreenshot.File); err == nil {
				_ = os.Remove(path)
			}
		}
	}
}

// Pin statuses the page reports. "unchecked" is not stored: a pin is
// unchecked when PinCheckedRound is behind the review round.
const (
	PinCurrent  = "current"
	PinOutdated = "outdated"
)

// PinFingerprint is what a pinned element looked like: its normalised text
// with 32 characters either side, its size at the viewport width it was
// measured at, and a hash of its normalised HTML.
type PinFingerprint struct {
	Text          string `json:"text"`
	Prefix        string `json:"prefix"`
	Suffix        string `json:"suffix"`
	Width         int    `json:"width"`
	Height        int    `json:"height"`
	ViewportWidth int    `json:"viewport_width"`
	HTMLHash      string `json:"html_hash"`
}

// ReplacePinShot sets a re-anchored pin's screenshot fields and returns the
// screenshot it replaced, so the caller can delete its file.
func (s *Session) ReplacePinShot(filePath, id string, shot PinShot) *PinScreenshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	fe, i := s.locateFileCommentLocked(filePath, id)
	if fe == nil || fe.Comments[i].DOMAnchor == nil {
		return nil
	}
	c := &fe.Comments[i]
	old := c.PinScreenshot
	c.PinScreenshot = shot.Screenshot
	c.ScreenshotStatus = shot.Status
	c.ScreenshotReason = shot.Reason
	s.scheduleWrite()
	return old
}
