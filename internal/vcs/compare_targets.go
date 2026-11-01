package vcs

import (
	"fmt"
	"os/exec"
)

// CompareTargets lists revision names the user may pick as the diff comparison
// point: local and remote-tracking git branches.
type CompareTargets struct {
	VCS      string   `json:"vcs"`
	Detected string   `json:"detected"`
	Local    []string `json:"local"`
	Remote   []string `json:"remote"`
}

// CompareTargetsFor returns grouped compare targets for v. When v is nil, returns
// zero values.
func CompareTargetsFor(v VCS, dir string) (CompareTargets, error) {
	if v == nil {
		return CompareTargets{}, nil
	}
	local, err := LocalBranches(dir)
	if err != nil {
		return CompareTargets{}, err
	}
	remote, err := v.RemoteBranches(dir)
	if err != nil {
		return CompareTargets{}, err
	}
	return CompareTargets{
		VCS:      "git",
		Detected: v.DefaultBranch(),
		Local:    local,
		Remote:   remote,
	}, nil
}

// LocalBranches returns local branch short names (git refs/heads).
func LocalBranches(dir string) ([]string, error) {
	cmd := exec.Command("git", "for-each-ref", "--format=%(refname:lstrip=2)", "refs/heads/")
	if dir != "" {
		cmd.Dir = dir
	}
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("for-each-ref heads: %w", err)
	}
	var branches []string
	for _, line := range splitNonEmpty(string(out)) {
		if line != "" && line != "HEAD" {
			branches = append(branches, line)
		}
	}
	return branches, nil
}
