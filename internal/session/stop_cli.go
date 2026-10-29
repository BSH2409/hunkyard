package session

import (
	"errors"
	"fmt"
	"os"

	"github.com/BSH2409/hunkyard/internal/clicmd"
	"github.com/BSH2409/hunkyard/internal/daemon"
	"github.com/BSH2409/hunkyard/internal/localonly"
	"github.com/BSH2409/hunkyard/internal/vcs"
)

var stopDaemonByKey = daemon.StopDaemon

//nolint:gocyclo // The stop dispatcher verifies ownership before acting on daemon processes.
func RunStop(args []string) error {
	all := false
	var fileArgs []string
	sessionID := ""
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--session" {
			if sessionID != "" || i+1 >= len(args) {
				return fmt.Errorf("provide one --session ID")
			}
			i++
			sessionID = args[i]
			continue
		}
		if arg == "--all" {
			all = true
		} else {
			fileArgs = append(fileArgs, arg)
		}
	}

	if sessionID != "" {
		if all || len(fileArgs) > 0 || !daemon.ValidSessionKey(sessionID) {
			return fmt.Errorf("use --session ID without file arguments or --all")
		}
		if localonly.Enabled() {
			if _, err := daemon.LoadSessionOwner(sessionID); err != nil {
				return err
			}
		}
		if err := stopDaemonByKey(sessionID); err != nil {
			return err
		}
		fmt.Println("Daemon stopped.")
		return nil
	}
	cwd, _ := daemon.ResolvedCWD()

	if all {
		if err := stopAllDaemons(cwd); err != nil {
			return err
		}
		fmt.Println("All daemons stopped.")
		return nil
	}

	branch := ""
	if vcs := vcs.DetectVCS(""); vcs != nil {
		branch = vcs.CurrentBranch()
	}

	// If file args were given, use the exact key (user knows which session).
	if len(fileArgs) > 0 {
		key := daemon.SessionKey(cwd, branch, fileArgs)
		if err := stopDaemonByKey(key); err != nil {
			return err
		}
		fmt.Println("Daemon stopped.")
		return nil
	}

	// No file args: try exact key first (git-mode session with no args).
	key := daemon.SessionKey(cwd, branch, nil)
	if entry, _ := daemon.ReadSessionFile(key); entry.PID > 0 {
		if err := stopDaemonByKey(key); err != nil {
			return err
		}
		fmt.Println("Daemon stopped.")
		return nil
	}

	// Exact key not found — fall back to scanning by cwd + branch.
	_, foundKey, matchCount := daemon.FindSessionForCWDBranch(cwd, branch)
	if matchCount > 1 {
		fmt.Fprintf(os.Stderr, "Error: multiple daemons running on branch %q. Use 'hunkyard stop --all' or specify file args.\n", branch)
		return clicmd.ExitError{Code: 1, Err: errors.New("exit")}
	}
	if matchCount == 0 {
		fmt.Fprintln(os.Stderr, "Error: no running daemon found for current directory and branch.")
		return clicmd.ExitError{Code: 1, Err: errors.New("exit")}
	}

	if err := stopDaemonByKey(foundKey); err != nil {
		return err
	}
	fmt.Println("Daemon stopped.")
	return nil
}

func stopAllDaemons(cwd string) error {
	_, keys := daemon.ListAllSessions()
	if len(keys) == 0 {
		daemon.StopAllDaemonsForCWD(cwd)
		return nil
	}
	var stopErrs []error
	for _, key := range keys {
		if err := stopDaemonByKey(key); err != nil {
			stopErrs = append(stopErrs, fmt.Errorf("%s: %w", key, err))
		}
	}
	return errors.Join(stopErrs...)
}
