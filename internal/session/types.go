package session

import "github.com/BSH2409/hunkyard/internal/vcs"

// VCS type aliases for cross-package use within session.
type (
	FileChange  = vcs.FileChange
	DiffHunk    = vcs.DiffHunk
	CommitInfo  = vcs.CommitInfo
	BranchEntry = vcs.BranchEntry
)

// InheritedScope is focus metadata stamped on comments authored via CLI tools.
type InheritedScope struct {
	HeadSHA      string
	BaseSHA      string
	Forge        string
	ChangeNumber int
	DiffScope    string // "layer" | "full_stack" | ""
}

// RemoteRef identifies a GitHub review comment queued for deletion on the
// next push.
type RemoteRef struct {
	Forge        string `json:"forge"`
	ChangeNumber int    `json:"change_number,omitempty"`
	CommentID    int64  `json:"comment_id"`
}
