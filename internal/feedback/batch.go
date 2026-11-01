package feedback

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path"
	"regexp"
	"strings"
)

type Comment struct {
	ParentID  string `json:"parent_id,omitempty"`
	ID        string `json:"id"`
	Revision  int    `json:"revision"`
	Body      string `json:"body"`
	Path      string `json:"path,omitempty"`
	OldPath   string `json:"old_path,omitempty"`
	Scope     string `json:"scope,omitempty"`
	Side      string `json:"side,omitempty"`
	StartLine int    `json:"start_line,omitempty"`
	EndLine   int    `json:"end_line,omitempty"`
	Quote     string `json:"quote,omitempty"`
	Anchor    string `json:"anchor,omitempty"`
	HeadSHA   string `json:"head_sha,omitempty"`
	FocusKey  string `json:"focus_key,omitempty"`
	ContentID string `json:"content_id,omitempty"`
	ViewID    string `json:"view_id,omitempty"`
	Outdated  bool   `json:"outdated,omitempty"`
	Label     string `json:"label,omitempty"`
	Blocking  bool   `json:"blocking,omitempty"`
	// ClaimUnchanged marks a comment the agent replied to without changing
	// the commented code; Hunkyard reopened it.
	ClaimUnchanged bool `json:"claim_unchanged,omitempty"`
	// Pin describes a comment pinned to an element of a live page or HTML
	// preview; its Path is empty because the route is not a file.
	Pin *PinContext `json:"pin,omitempty"`
}

// PinContext is what the agent needs to find a pinned element and see it.
type PinContext struct {
	Route    string `json:"route"`
	Selector string `json:"selector"`
	// Status is "current" (the element looks as it did when pinned),
	// "outdated" (it changed or is gone) or "unchecked" (its route has not
	// been loaded since the last round).
	Status string `json:"status"`
	// Screenshot is the page with the element boxed; when capture failed it
	// is nil and ScreenshotStatus/ScreenshotReason say why.
	Screenshot       *PinShot    `json:"screenshot,omitempty"`
	ScreenshotStatus string      `json:"screenshot_status,omitempty"`
	ScreenshotReason string      `json:"screenshot_reason,omitempty"`
	MarkerRect       Rect        `json:"marker_rect"`
	Marker           MarkerStyle `json:"marker"`
	Viewport         Viewport    `json:"viewport"`
}

// PinShot locates a pin screenshot; URL serves its raw PNG bytes.
type PinShot struct {
	URL         string  `json:"url"`
	Width       int     `json:"width"`
	Height      int     `json:"height"`
	Scale       float64 `json:"scale"`
	BandOffsetY int     `json:"band_offset_y"`
}

// Rect is the pinned element's box in screenshot pixels.
type Rect struct {
	X      int `json:"x"`
	Y      int `json:"y"`
	Width  int `json:"width"`
	Height int `json:"height"`
}

// Viewport is the page's viewport and scroll position when it was captured.
type Viewport struct {
	Width   int `json:"width"`
	Height  int `json:"height"`
	ScrollX int `json:"scroll_x"`
	ScrollY int `json:"scroll_y"`
}

// MarkerStyle is how the box around a pinned element is drawn.
type MarkerStyle struct {
	Color     string `json:"color"`
	Stroke    int    `json:"stroke"`
	Halo      string `json:"halo"`
	HaloWidth int    `json:"halo_width"`
	Padding   int    `json:"padding"`
	Label     string `json:"label"`
}

// PinMarker mirrors MARKER_STYLE in web/agent-pin-marker.js.
var PinMarker = MarkerStyle{Color: "#FF0000", Stroke: 4, Halo: "#FFFFFF", HaloWidth: 2, Padding: 6, Label: "1"}

type Batch struct {
	Version         int        `json:"version"`
	ID              string     `json:"id"`
	ReviewID        string     `json:"review_id"`
	SnapshotID      string     `json:"snapshot_id"`
	Round           int        `json:"round"`
	Disposition     string     `json:"disposition"`
	Summary         string     `json:"summary"`
	Comments        []Comment  `json:"comments"`
	SkillIDs        []string   `json:"skill_ids"`
	PublishedSkills []SkillRef `json:"published_skills,omitempty"`
}

func digest(value any) string {
	encoded, _ := json.Marshal(value)
	hash := sha256.Sum256(encoded)
	return hex.EncodeToString(hash[:])
}

var snapshotIDPattern = regexp.MustCompile(`^snapshot_[0-9a-f]{64}$`)
var batchIDPattern = regexp.MustCompile(`^batch_[0-9a-f]{64}$`)

//nolint:gocyclo // The full publication schema is checked before a batch is trusted.
func validateBatch(batch Batch) error {
	if batch.Version != 1 || !reviewIDPattern.MatchString(batch.ReviewID) || !snapshotIDPattern.MatchString(batch.SnapshotID) || batch.Round < 1 {
		return fmt.Errorf("invalid review batch identity")
	}
	switch batch.Disposition {
	case "comment", "approve", "request_changes":
	default:
		return fmt.Errorf("invalid review disposition")
	}
	if len(batch.Comments) > 5000 || len(batch.SkillIDs) > 100 || len(batch.Summary) > 64*1024 {
		return fmt.Errorf("review batch exceeds limits")
	}
	seen := map[string]bool{}
	for _, c := range batch.Comments {
		if c.ID == "" || len(c.ID) > 256 || seen[c.ID] || c.Body == "" || len(c.Body) > 64*1024 || c.Revision < 0 {
			return fmt.Errorf("invalid human comment")
		}
		seen[c.ID] = true
		if !ValidLabel(c.Label) {
			return fmt.Errorf("invalid comment label")
		}
		for _, name := range []string{c.Path, c.OldPath} {
			if name != "" && (path.IsAbs(name) || path.Clean(name) != name || name == ".." || strings.HasPrefix(name, "../") || strings.ContainsAny(name, "\\\x00")) {
				return fmt.Errorf("invalid comment path")
			}
		}
	}
	return nil
}

// Only a human edit creates new learning evidence; automatic reanchoring does not.
func revisionHash(comment Comment) string { return digest(comment.Body) }

// ValidLabel reports whether label is a review comment label: none, or one
// of the conventional-comment kinds Hunkyard offers.
func ValidLabel(label string) bool {
	switch label {
	case "", "nit", "issue", "suggestion", "praise", "question":
		return true
	}
	return false
}
