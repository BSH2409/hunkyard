package session

import "fmt"

// CLIReviewConfig holds parsed review CLI flags for RunReview.
type CLIReviewConfig struct {
	Port               int
	Host               string
	Files              []string
	Focus              *Focus
	OutputDir          string
	PlanDir            string
	NoOpen             bool
	OpenCmd            string
	Quiet              bool
	NoIntegrationCheck bool
	VCSOverride        string
	BaseBranch         string
	IgnorePatterns     []string
	SkillsFile         string
	SessionID          string
}

var (
	ResolveServerConfigFn func(args []string) (*CLIReviewConfig, error)
	PreflightCheckFn      func(sc *CLIReviewConfig) string
)

// FocusKeyArgs returns daemon session key file args for PR/range focus.
func FocusKeyArgs(sc *CLIReviewConfig) []string {
	if sc == nil {
		return nil
	}
	return ReviewKeyArgs(sc.Focus, sc.Files)
}

// ReviewKeyArgs returns the args that key a review's daemon session. The
// launching client and the daemon must both derive the key from it, or the
// client looks up a session and owner record the daemon never wrote.
// PR-keyed daemons reuse the same review file across head changes;
// range-keyed daemons are unique per (base, head) pair. DiffScope is not part
// of the key, so the picker can toggle scopes within one session.
func ReviewKeyArgs(focus *Focus, files []string) []string {
	if focus == nil || focus.Kind != FocusRange {
		return files
	}
	if focus.ChangeNumber > 0 {
		return []string{PRFocusKey(focus.ChangeNumber, focus.RemoteBaseProject, focus.RemoteHost)}
	}
	if focus.RangeMode == "merge_base" {
		return []string{fmt.Sprintf("range:%s...%s", focus.RequestedBaseSHA, focus.HeadSHA)}
	}
	return []string{fmt.Sprintf("range:%s..%s", focus.BaseSHA, focus.HeadSHA)}
}
