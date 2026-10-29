package live

import (
	"github.com/BSH2409/hunkyard/internal/comment"
	"github.com/BSH2409/hunkyard/internal/config"
	"github.com/BSH2409/hunkyard/internal/github"
	"github.com/BSH2409/hunkyard/internal/review"
	"github.com/BSH2409/hunkyard/internal/server"
	"github.com/BSH2409/hunkyard/internal/session"
)

type (
	Config           = config.Config
	HunkyardJSON     = session.HunkyardJSON
	HunkyardJSONFile = session.HunkyardJSONFile
	Session          = session.Session
	FileEntry        = session.FileEntry
	Comment          = session.Comment
	DOMAnchor        = session.DOMAnchor
	SSEEvent         = session.SSEEvent
)

var (
	looksLikeLiveArgs      = LooksLikeLiveArgs
	saveHunkyardJSON       = review.SaveHunkyardJSON
	loadHunkyardJSON       = review.LoadHunkyardJSON
	appendReply            = comment.AppendReply
	checkGitHubSyncAllowed = session.CheckGitHubSyncAllowed
	checkCommentCLIAllowed = comment.CheckCommentCLIAllowed
	carryForwardComment    = session.CarryForwardComment
	NewServer              = server.NewServer
	frontendFS             = server.FrontendFS
)

type GhComment = github.GhComment

var mergeGHComments = github.MergeGHComments
