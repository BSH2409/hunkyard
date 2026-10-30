package session

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/BSH2409/hunkyard/internal/browser"
	"github.com/BSH2409/hunkyard/internal/clicmd"
	"github.com/BSH2409/hunkyard/internal/config"
	"github.com/BSH2409/hunkyard/internal/daemon"
	"github.com/BSH2409/hunkyard/internal/feedback"
	"github.com/BSH2409/hunkyard/internal/localonly"
	"github.com/BSH2409/hunkyard/internal/vcs"
)

var (
	startDaemonForReview     = daemon.StartDaemon
	runReviewClientForReview = daemon.RunReviewClient
)

//nolint:gocyclo // CLI review dispatcher
func RunReview(args []string) error {
	go backgroundCleanup()

	if ResolveServerConfigFn == nil {
		return errors.New("ResolveServerConfigFn not wired")
	}
	sc, err := ResolveServerConfigFn(args)
	if err != nil {
		return err
	}
	if sc == nil {
		return nil // --version
	}

	cwd, _ := daemon.ResolvedCWD()

	var key string
	var entry daemon.SessionEntry
	var alive bool
	weStartedDaemon := false

	if sc.SessionID != "" {
		if !daemon.ValidSessionKey(sc.SessionID) {
			return daemon.InvalidSessionIDError(sc.SessionID)
		}
		key = sc.SessionID
		if localonly.Enabled() {
			if _, err := daemon.LoadSessionOwner(key); err != nil {
				return err
			}
		}
		staleEntry, _ := daemon.ReadSessionFile(key)
		entry, alive = daemon.FindAliveSession(key)
		if alive {
			if !sc.Quiet {
				fmt.Fprintf(os.Stderr, "Connected to hunkyard daemon at %s (session %s)\n", entry.BaseURL(), key)
			}
			if note := movedRangeHeadNote(ProbeDaemonFocusForSession(entry), sc.VCSOverride); note != "" {
				fmt.Fprint(os.Stderr, note)
			}
			if !sc.NoOpen && !daemon.DaemonHasBrowser(entry) {
				go browser.OpenBrowserWithCommand(entry.BaseURL(), sc.OpenCmd)
			}
		} else {
			staleEntry.Host = sc.Host
			entry, err = reconnectDeadSession(key, staleEntry, sc.Quiet, "--port", strconv.Itoa(sc.Port))
			if err != nil {
				return err
			}
			weStartedDaemon = true
		}
	} else {
		branch := ""
		if v := vcs.DetectVCS(sc.VCSOverride); v != nil {
			branch = v.CurrentBranch()
		}
		key = daemon.SessionKey(cwd, branch, FocusKeyArgs(sc))

		entry, alive = daemon.FindAliveSession(key)
		if alive {
			if !sc.Quiet {
				fmt.Fprintf(os.Stderr, "Connected to hunkyard daemon at %s (session %s)\n", entry.BaseURL(), key)
			}
			if !sc.NoOpen && !daemon.DaemonHasBrowser(entry) {
				go browser.OpenBrowserWithCommand(entry.BaseURL(), sc.OpenCmd)
			}
		} else {
			if len(sc.Files) == 0 && sc.Focus == nil && sc.PlanDir == "" && PreflightCheckFn != nil {
				if msg := PreflightCheckFn(sc); msg != "" {
					fmt.Fprint(os.Stderr, msg)
					return clicmd.ExitError{Code: 1, Err: errors.New("exit")}
				}
			}
			entry, err = startDaemonForReview(key, args)
			if err != nil {
				return err
			}
			if !sc.Quiet {
				fmt.Fprintf(os.Stderr, "Started hunkyard daemon at %s (session %s, PID %d)\n", entry.BaseURL(), key, entry.PID)
				if dirs := dirArgs(sc.Files); len(dirs) > 0 {
					fmt.Fprintf(os.Stderr, "\nReviewing %s — changed code uses Git diffs; documents use file view.\n\n", strings.Join(dirs, ", "))
				}
				if !sc.NoIntegrationCheck {
					HintMissingIntegrations()
				}
			}
			weStartedDaemon = true
		}
	}

	if weStartedDaemon && (!localonly.Enabled() || feedback.CurrentIdentity().Kind == "terminal") {
		installDaemonSignalHandler(entry.PID)
	}
	if !weStartedDaemon && sc.SkillsFile != "" {
		if err := daemon.RegisterSkills(entry, key, sc.SkillsFile); err != nil {
			return err
		}
	}

	approved := runReviewClientForReview(entry, key, sc.Quiet)
	stopDaemonOnApproval(approved, entry, key, config.LoadConfig(cwd).CleanupOnApproveEnabled())
	return nil
}

// movedRangeHeadNote explains when a range review's head ref has moved since
// the review pinned its commits. Reconnecting keeps the pinned range, so the
// new commits only appear in a review started from the same --range again.
func movedRangeHeadNote(focus *Focus, vcsOverride string) string {
	if focus == nil || focus.Kind != FocusRange || focus.RangeSpec == "" {
		return ""
	}
	headRef := focus.RangeSpec[strings.LastIndex(focus.RangeSpec, "..")+2:]
	v := vcs.DetectVCS(vcsOverride)
	if v == nil {
		return ""
	}
	root, err := v.RepoRoot()
	if err != nil {
		return ""
	}
	current, err := vcs.ResolveCommitOID(v, headRef, root)
	if err != nil || current == focus.HeadSHA {
		return ""
	}
	return fmt.Sprintf("Note: this review is pinned to %s at %.7s, but %s now points to %.7s. "+
		"Its new commits are not shown. Run `hunkyard --range %s` to review them.\n",
		focus.RangeSpec, focus.HeadSHA, headRef, current, focus.RangeSpec)
}
