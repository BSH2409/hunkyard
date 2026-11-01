package vcs

import "strings"

// TopicChainSHAs returns commit SHAs on the current topic branch, excluding the default branch tip.
func TopicChainSHAs(v VCS, repoRoot string) map[string]bool {
	out := make(map[string]bool)
	if v == nil || v.Name() != "git" {
		return out
	}
	mergeBase, err := RunGitInDir(repoRoot, "merge-base", v.DefaultBranch(), "HEAD")
	if err != nil {
		return out
	}
	baseSHA := strings.TrimSpace(mergeBase)
	if baseSHA == "" {
		return out
	}
	revs, err := RunGitInDir(repoRoot, "rev-list", baseSHA+"..HEAD")
	if err != nil {
		return out
	}
	for _, sha := range splitNonEmpty(revs) {
		out[sha] = true
	}
	return out
}

// CommitSubjectFor returns a short commit subject for sha using the given VCS backend.
func CommitSubjectFor(v VCS, repoRoot, sha string) string {
	if v == nil || v.Name() != "git" {
		return ""
	}
	out, err := RunGitInDir(repoRoot, "log", "-1", "--format=%s", sha)
	if err != nil {
		return ""
	}
	subject := strings.TrimSpace(out)
	if len(subject) > 60 {
		subject = subject[:60] + "\u2026"
	}
	return subject
}
