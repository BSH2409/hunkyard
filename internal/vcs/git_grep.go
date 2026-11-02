package vcs

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"strconv"
)

// GrepHit is one line of a tracked file that mentions a searched word.
type GrepHit struct {
	Path string
	Line int
	Text string
}

// maxGrepHits bounds one search; callers report the overflow as truncated.
const maxGrepHits = 500

// GrepTracked finds whole-word, literal mentions of words in tracked text
// files under dir, skipping excludePath. No match is not an error.
func GrepTracked(ctx context.Context, dir string, words []string, excludePath string) ([]GrepHit, error) {
	if len(words) == 0 {
		return nil, nil
	}
	args := []string{"grep", "-n", "-w", "-F", "-I", "--null", "--no-color"}
	for _, w := range words {
		args = append(args, "-e", w)
	}
	args = append(args, "--", ".")
	if excludePath != "" {
		args = append(args, ":(exclude,literal)"+excludePath)
	}
	out, err := runGit(ctx, dir, args...)
	var exit *exec.ExitError
	if err != nil && errors.As(err, &exit) && exit.ExitCode() == 1 {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var hits []GrepHit
	for _, line := range bytes.Split(out, []byte("\n")) {
		parts := bytes.SplitN(line, []byte{0}, 3)
		if len(parts) != 3 {
			continue
		}
		n, err := strconv.Atoi(string(parts[1]))
		if err != nil {
			continue
		}
		hits = append(hits, GrepHit{Path: string(parts[0]), Line: n, Text: string(bytes.TrimSpace(parts[2]))})
		if len(hits) >= maxGrepHits {
			break
		}
	}
	return hits, nil
}
