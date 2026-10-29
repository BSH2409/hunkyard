package github

import (
	"bytes"
	"io"
	"os"
	"testing"

	"github.com/BSH2409/hunkyard/internal/comment"
	"github.com/BSH2409/hunkyard/internal/review"
	"github.com/BSH2409/hunkyard/internal/session"
	"github.com/BSH2409/hunkyard/internal/testutil"
	"github.com/BSH2409/hunkyard/internal/vcs"
)

type (
	BulkCommentEntry = comment.BulkCommentEntry
	bulkCommentEntry = comment.BulkCommentEntry
	Focus            = session.Focus
	DOMAnchor        = session.DOMAnchor
)

const (
	FocusRange         = session.FocusRange
	FocusWorkingTree   = session.FocusWorkingTree
	DiffScopeLayer     = session.DiffScopeLayer
	DiffScopeFullStack = session.DiffScopeFullStack
)

var (
	mustMkdirAll                         = session.MustMkdirAll
	setHome                              = testutil.SetHome
	findReviewFileByCommentID            = review.FindReviewFileByCommentID
	appendReply                          = comment.AppendReply
	appendReviewCommentScoped            = comment.AppendReviewCommentScoped
	appendFileCommentScoped              = comment.AppendFileCommentScoped
	appendCommentScoped                  = comment.AppendCommentScoped
	randomCommentID                      = session.RandomCommentID
	randomReviewCommentID                = session.RandomReviewCommentID
	randomReplyID                        = session.RandomReplyID
	addFileCommentToHunkyardJSONScoped   = comment.AddFileCommentToHunkyardJSONScoped
	addReviewCommentToHunkyardJSONScoped = comment.AddReviewCommentToHunkyardJSONScoped
	addCommentToHunkyardJSONScoped       = comment.AddCommentToHunkyardJSONScoped
	addReplyToHunkyardJSON               = comment.AddReplyToHunkyardJSON
	bulkAddCommentsToHunkyardJSONScoped  = comment.BulkAddCommentsToHunkyardJSONScoped
	resolveReviewPath                    = review.ResolveReviewPath
	clearHunkyardJSON                    = review.ClearHunkyardJSON
	processBulkReviewEntry               = comment.ProcessBulkReviewEntry
	resolvePullScope                     = session.ResolvePullScope
	fetchPRByNumber                      = FetchPRByNumber
	ensureSHAFetched                     = vcs.EnsureSHAFetched
	runPushLive                          = RunPushLive
	randomUUID                           = session.RandomUUID
	reviewPathsFor                       = review.ReviewPathsFor
	parseLineSpec                        = comment.ParseLineSpec
)

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

func withDaemonFocus(t *testing.T, f *Focus) {
	t.Helper()
	restore := session.SetProbeDaemonFocusFnForTest(func() *session.Focus {
		if f == nil {
			return nil
		}
		out := *f
		return &out
	})
	t.Cleanup(restore)
}
