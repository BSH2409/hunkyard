package review

import (
	"github.com/BSH2409/hunkyard/internal/daemon"
	"github.com/BSH2409/hunkyard/internal/session"
	"github.com/BSH2409/hunkyard/internal/vcs"
)

type (
	HunkyardJSON     = session.HunkyardJSON
	HunkyardJSONFile = session.HunkyardJSONFile
	Comment          = session.Comment
	RoundSnapshot    = session.RoundSnapshot
	SessionEntry     = daemon.SessionEntry
)

var (
	ResolvedCWD = daemon.ResolvedCWD
	SessionKey  = daemon.SessionKey
	DetectVCS   = vcs.DetectVCS
)
