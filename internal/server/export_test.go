package server

import (
	"github.com/BSH2409/hunkyard/internal/config"
	"github.com/BSH2409/hunkyard/internal/session"
)

var (
	resolveHost             = config.ResolveHost
	randomUUID              = session.RandomUUID
	maxAttachmentBytes      = session.MaxAttachmentBytes
	parseDaemonFlagsForTest = parseDaemonFlags
)
