package server

import (
	"errors"
	"fmt"
	"os"

	"github.com/BSH2409/hunkyard/internal/feedback"
	"github.com/BSH2409/hunkyard/internal/session"
)

// validatePinScreenshot accepts a screenshot that is a PNG attachment of this
// review, or a failed capture with a short reason.
func (s *Server) validatePinScreenshot(shot *session.PinScreenshot, status, reason string) error {
	if status != "" && status != "failed" {
		return errors.New("screenshot_status must be empty or \"failed\"")
	}
	if len(reason) > 200 {
		return errors.New("screenshot_reason too long")
	}
	if shot == nil {
		return nil
	}
	path, mime, err := attachmentPathFor(s.reviewPath, shot.File)
	if err != nil || mime != "image/png" {
		return errors.New("pin_screenshot.file must be a PNG attachment of this review")
	}
	if _, err := os.Stat(path); err != nil {
		return errors.New("pin_screenshot.file must be a PNG attachment of this review")
	}
	return nil
}

// removeAttachment deletes an attachment of this review; a missing file is fine.
func (s *Server) removeAttachment(filename string) {
	if path, _, err := attachmentPathFor(s.reviewPath, filename); err == nil {
		_ = os.Remove(path)
	}
}

// pinContext is what the agent is told about a live pin on this round.
func (s *Server) pinContext(c session.Comment, round int) *feedback.PinContext {
	pin := &feedback.PinContext{
		Route:            c.DOMAnchor.Pathname,
		Selector:         c.DOMAnchor.CSSSelector,
		Status:           c.PinStatus,
		ScreenshotStatus: c.ScreenshotStatus,
		ScreenshotReason: c.ScreenshotReason,
		Marker:           feedback.PinMarker,
	}
	if c.PinStatus == "" || c.PinCheckedRound < round {
		pin.Status = "unchecked"
	}
	if shot := c.PinScreenshot; shot != nil {
		pin.Screenshot = &feedback.PinShot{
			URL:         fmt.Sprintf("http://localhost:%d/api/attachments/%s", s.port, shot.File),
			Width:       shot.Width,
			Height:      shot.Height,
			Scale:       shot.Scale,
			BandOffsetY: shot.BandOffsetY,
		}
		pin.MarkerRect = feedback.Rect(shot.MarkerRect)
		pin.Viewport = feedback.Viewport(shot.Viewport)
	}
	return pin
}
