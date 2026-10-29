package preview

import (
	"github.com/BSH2409/hunkyard/internal/daemon"
	"github.com/BSH2409/hunkyard/internal/server"
	"github.com/BSH2409/hunkyard/internal/session"
)

type (
	Server           = server.Server
	Session          = server.Session
	HunkyardJSON     = session.HunkyardJSON
	HunkyardJSONFile = session.HunkyardJSONFile
	Comment          = session.Comment
	DOMAnchor        = session.DOMAnchor
	FileEntry        = session.FileEntry
	SSEEvent         = session.SSEEvent
)

var (
	frontendFS           = server.FrontendFS
	liveSessionKey       = daemon.LiveSessionKey
	NewServer            = server.NewServer
	previewSessionKey    = PreviewSessionKey
	looksLikePreviewArgs = LooksLikePreviewArgs
)

type serverConfig struct {
	previewFile string
	reviewPath  string
}

func createPreviewSession(sc *serverConfig) (*Session, error) {
	return session.NewPreviewSession(sc.previewFile, sc.reviewPath)
}
