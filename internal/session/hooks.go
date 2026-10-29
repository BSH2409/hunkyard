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
	if sc == nil || sc.Focus == nil || sc.Focus.Kind != FocusRange {
		if sc == nil {
			return nil
		}
		return sc.Files
	}
	if sc.Focus.ChangeNumber > 0 {
		return []string{PRFocusKey(sc.Focus.ChangeNumber, sc.Focus.RemoteBaseProject, sc.Focus.RemoteHost)}
	}
	if sc.Focus.RangeMode == "merge_base" {
		return []string{fmt.Sprintf("range:%s...%s", sc.Focus.RequestedBaseSHA, sc.Focus.HeadSHA)}
	}
	return []string{fmt.Sprintf("range:%s..%s", sc.Focus.BaseSHA, sc.Focus.HeadSHA)}
}
