package server

import (
	"github.com/BSH2409/hunkyard/internal/config"
	"github.com/BSH2409/hunkyard/internal/github"
	"github.com/BSH2409/hunkyard/internal/session"
	"github.com/BSH2409/hunkyard/internal/vcs"
)

type (
	Session          = session.Session
	RoundSnapshot    = session.RoundSnapshot
	Config           = config.Config
	Focus            = session.Focus
	Comment          = session.Comment
	Reply            = session.Reply
	HunkyardJSON     = session.HunkyardJSON
	HunkyardJSONFile = session.HunkyardJSONFile
	FileEntry        = session.FileEntry
	SSEEvent         = session.SSEEvent
	SessionInfo      = session.SessionInfo
	SessionFileInfo  = session.SessionFileInfo
	PRInfo           = github.PRInfo
	Status           = session.Status
	DiffHunk         = session.DiffHunk
	DOMAnchor        = session.DOMAnchor
	Story            = session.Story
	PRListCache      = github.PRListCache
	GitVCS           = vcs.GitVCS
	CommitInfo       = vcs.CommitInfo
)

const (
	FocusRange         = session.FocusRange
	DiffScopeLayer     = session.DiffScopeLayer
	DiffScopeFullStack = session.DiffScopeFullStack
)

type PRSummary = github.PRSummary

const MaxAttachmentBytes = session.MaxAttachmentBytes

// StaleIntegration describes an out-of-date agent integration file.
type StaleIntegration struct {
	Agent    string
	Location string
	Hint     string
	Hash     string
}
