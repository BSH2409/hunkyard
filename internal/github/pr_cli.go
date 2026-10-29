package github

import (
	"errors"
	"fmt"
	"os"

	"github.com/BSH2409/hunkyard/internal/clicmd"
	"github.com/BSH2409/hunkyard/internal/session"
)

func RunPR(args []string) error {
	if len(args) != 1 {
		fmt.Fprintln(os.Stderr, "Usage: hunkyard pr <num|url>")
		return clicmd.ExitError{Code: 1, Err: errors.New("exit")}
	}
	return session.RunReview([]string{"--pr", args[0]})
}
