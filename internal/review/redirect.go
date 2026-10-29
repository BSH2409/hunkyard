package review

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/BSH2409/hunkyard/internal/session"
)

// PRHeadInfo is the subset of PR metadata redirectReviewPathForPR needs.
type PRHeadInfo struct {
	HeadRefName string
}

// FetchPRHeadInfoFn fetches PR head-branch metadata by number. Wired from github
// at init to avoid an import cycle (review -> github -> comment -> review).
var FetchPRHeadInfoFn func(prNumber int) (*PRHeadInfo, error)

// RedirectReviewPathForPR routes to a review file matching the PR head branch when
// the cwd-resolved file is for a different branch.
func RedirectReviewPathForPR(prNumber int, cwdBranch, cwdHunkyardPath string) (string, session.HunkyardJSON, bool) {
	return redirectReviewPathForPR(prNumber, cwdBranch, cwdHunkyardPath)
}

func redirectReviewPathForPR(prNumber int, cwdBranch, cwdHunkyardPath string) (string, session.HunkyardJSON, bool) {
	if FetchPRHeadInfoFn == nil {
		return "", session.HunkyardJSON{}, false
	}
	info, err := FetchPRHeadInfoFn(prNumber)
	if err != nil || info == nil || info.HeadRefName == "" {
		return "", session.HunkyardJSON{}, false
	}
	if cwdBranch != "" && info.HeadRefName == cwdBranch {
		return "", session.HunkyardJSON{}, false
	}
	altPath, err := findReviewFileByBranch(info.HeadRefName, cwdHunkyardPath)
	if err != nil {
		if errors.Is(err, errReviewFileAmbiguousForBranch) {
			fmt.Fprintf(os.Stderr,
				"Note: multiple review files match branch %q; using cwd-resolved path. Pass --output to disambiguate.\n",
				info.HeadRefName)
		}
		return "", session.HunkyardJSON{}, false
	}
	data, readErr := session.ReadFileShared(altPath)
	if readErr != nil {
		return "", session.HunkyardJSON{}, false
	}
	var altCJ session.HunkyardJSON
	if jsonErr := json.Unmarshal(data, &altCJ); jsonErr != nil {
		return "", session.HunkyardJSON{}, false
	}
	fmt.Fprintf(os.Stderr, "Note: PR #%d targets branch %q; routing to %s (not the cwd-resolved review file)\n",
		prNumber, info.HeadRefName, filepath.Base(altPath))
	return altPath, altCJ, true
}
