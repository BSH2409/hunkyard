package daemon

import "github.com/BSH2409/hunkyard/internal/config"

type commonDaemonFlags = CommonDaemonFlags

var (
	atomicWriteFile         = config.AtomicWriteFile
	appendCommonDaemonFlags = AppendCommonDaemonFlags
)
