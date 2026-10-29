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
}

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
