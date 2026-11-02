package session

import (
	"bytes"
	"io"
	"os"
	"testing"

	"github.com/BSH2409/hunkyard/internal/daemon"
	"github.com/BSH2409/hunkyard/internal/diff"
	"github.com/BSH2409/hunkyard/internal/vcs"
)

type (
	sessionEntry      = daemon.SessionEntry
	commonDaemonFlags = PlanDaemonFlags
	DiffLine          = vcs.DiffLine
)

var (
	sessionKey              = daemon.SessionKey
	planSessionKey          = PlanSessionKey
	applyPlanOverrides      = ApplyPlanOverrides
	buildPlanDaemonArgs     = BuildPlanDaemonArgs
	commitAt                = vcs.CommitAtForTest
	newProcessGroup         = daemon.NewProcessGroupForTest
	terminateProcess        = daemon.TerminateProcess
	processExists           = daemon.ProcessExists
	readFileShared          = ReadFileShared
	ResolveDefaultBranchSHA = vcs.ResolveDefaultBranchSHA
	loadSnapshotsFile       = loadSnapshotsFromDisk
	saveSnapshotsFile       = SaveSnapshotsFile
	saveHunkyardJSON        = SaveHunkyardJSON
	LoadHunkyardJSON        = loadHunkyardJSONForTest
	clearReviewFolder       = ClearReviewFolder
	cloneRoundSnapshots     = CloneRoundSnapshots
	resetDefaultBranchOnce  = vcs.ResetDefaultBranchOnceForTest
	ParseUnifiedDiff        = vcs.ParseUnifiedDiff
	fileDiffUnified         = vcs.FileDiffUnifiedForTest
)

func loadHunkyardJSONForTest(hunkyardPath string) (HunkyardJSON, error) {
	if err := ensureReviewFolder(hunkyardPath); err != nil {
		return HunkyardJSON{}, err
	}
	return readHunkyardJSONFromDisk(hunkyardPath)
}

type testServerConfig struct {
	files       []string
	focus       *Focus
	remoteFiles bool
}

func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = w
	fn()
	w.Close()
	os.Stderr = old
	var buf bytes.Buffer
	io.Copy(&buf, r)
	return buf.String()
}

// lineStatsForRound mirrors server.lineStatsForRound for unit tests without
// importing internal/server (import cycle).
func lineStatsForRound(sess *Session, n int) (int, int) {
	if n <= 1 {
		return 0, 0
	}
	var adds, dels int
	for _, byRound := range sess.RoundSnapshots {
		curr, ok := byRound[n]
		if !ok {
			continue
		}
		prev, hasPrev := byRound[n-1]
		if !hasPrev {
			adds += len(diff.SplitLines(curr.Content))
			continue
		}
		entries := diff.ComputeLineDiff(prev.Content, curr.Content)
		for _, e := range entries {
			switch e.Type {
			case "added":
				adds++
			case "removed":
				dels++
			}
		}
	}
	return adds, dels
}
