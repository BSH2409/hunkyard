package session

import (
	"fmt"
	"io/fs"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"time"

	"github.com/BSH2409/hunkyard/internal/browser"
	"github.com/BSH2409/hunkyard/internal/config"
	"github.com/BSH2409/hunkyard/internal/daemon"
)

var startDaemonForConnect = daemon.StartDaemon

func mustGetwd() string {
	cwd, err := os.Getwd()
	if err != nil {
		return "."
	}
	return cwd
}

// connectOrStartDaemon finds an alive session or starts a new daemon.
// Returns the session entry, whether we started a new daemon, and any error.
// When quiet is true, connect/start status lines and integration tips are omitted.
func connectOrStartDaemon(key string, args []string, noOpen bool, openCmd string, quiet bool) (daemon.SessionEntry, bool, error) {
	entry, alive := daemon.FindAliveSession(key)
	if alive {
		if !quiet {
			fmt.Fprintf(os.Stderr, "Connected to hunkyard daemon at %s (session %s)\n", entry.BaseURL(), key)
		}
		if !noOpen && !daemon.DaemonHasBrowser(entry) {
			go browser.OpenBrowserWithCommand(entry.BaseURL(), openCmd)
		}
		return entry, false, nil
	}

	var err error
	entry, err = startDaemonForConnect(key, args)
	if err != nil {
		return daemon.SessionEntry{}, false, err
	}
	if !quiet {
		fmt.Fprintf(os.Stderr, "Started hunkyard daemon at %s (session %s, PID %d)\n", entry.BaseURL(), key, entry.PID)
		HintMissingIntegrations()
	}
	return entry, true, nil
}

// HintMissingIntegrations prints a suggestion when AI tools are detected but
// no hunkyard integration is installed. Skipped when any integration already exists
// or when HUNKYARD_NO_INTEGRATION_CHECK is set.
func HintMissingIntegrations() {
	if os.Getenv("HUNKYARD_NO_INTEGRATION_CHECK") != "" {
		return
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}
	hintMissingIntegrationsFor(mustGetwd(), home)
}

func hintMissingIntegrationsFor(cwd, home string) {
	if InstalledAgentsFn != nil && len(InstalledAgentsFn(cwd, home)) > 0 {
		return
	}
	if CheckMissingIntegrationsFn == nil || PrintMissingHintsFn == nil {
		return
	}
	if missing := CheckMissingIntegrationsFn(cwd, home); len(missing) > 0 {
		PrintMissingHintsFn(missing)
	}
}

func installDaemonSignalHandler(pid int) {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, daemon.TerminationSignals()...)
	go func() {
		<-sigCh
		if proc, err := os.FindProcess(pid); err == nil {
			_ = daemon.TerminateProcess(proc)
		}
		os.Exit(0)
	}()
}

// terminateDaemonProcess, waitForDaemonExit and approvalSignalGrace are seams
// for tests.
var (
	terminateDaemonProcess = daemon.TerminateProcess
	waitForDaemonExit      = daemon.WaitOrKill
	approvalSignalGrace    = 500 * time.Millisecond
)

// stopDaemonOnApproval stops the daemon once the review is approved and runs
// cleanup_on_approve.
func stopDaemonOnApproval(approved bool, entry daemon.SessionEntry, key string, cleanup bool) {
	if !approved {
		return
	}
	switch daemon.RequestShutdown(entry) {
	case daemon.ShutdownWrongDaemon:
		return
	case daemon.ShutdownUnavailable:
		file, err := daemon.ReadSessionFile(key)
		switch {
		case err != nil:
			// The daemon removed its session file, so it is already shutting
			// down for another client.
		case file.PID != entry.PID:
			// A newer daemon has the key; this one is not ours to signal or
			// clean up after.
			return
		default:
			// An older daemon without /api/shutdown gets the signal it always
			// got. On Windows that can be a hard kill, so its approved
			// /api/finish response gets a moment to reach the browser first.
			if runtime.GOOS == "windows" {
				time.Sleep(approvalSignalGrace)
			}
			if proc, err := os.FindProcess(entry.PID); err == nil {
				_ = terminateDaemonProcess(proc)
			}
		}
	}
	// A stopping daemon can still write the review, and a daemon started
	// meanwhile under the same key has loaded it, so the review (or, with
	// cleanup off, its pin screenshots) goes only once the daemon has exited
	// and nobody took the key over.
	if !waitForDaemonExit(entry.PID) {
		return
	}
	daemon.UnlessSessionTakenOver(key, entry.PID, func() {
		cleanupOnApproval(approved, entry.ReviewPath, cleanup)
	})
}

// backgroundCleanup deletes stale reviews other than keep, the review this
// run resumes. Each deletion holds the review's session lock, which a daemon
// start also takes, and rechecks staleness under it.
func backgroundCleanup(keep string) {
	revDir, err := daemon.ReviewsDir()
	if err == nil {
		global, _, _ := config.LoadConfigFile(config.GlobalConfigPath())
		days := global.StaleReviewDaysOrDefault()
		stale := findStaleReviews(revDir, days)
		deleteStaleReviewsSilent(revDir, stale, keep, days)
	}
	daemon.CleanOrphanedSessions()
}

func deleteStaleReviewsSilent(revDir string, stale []staleReview, keep string, days int) {
	sessDir, _ := daemon.SessionsDir()
	cutoff := time.Now().Add(-time.Duration(days) * 24 * time.Hour)
	for _, s := range stale {
		if s.key == keep {
			continue
		}
		_ = daemon.WithSessionLock(s.key, func() {
			if sessDir != "" {
				if _, err := os.Stat(filepath.Join(sessDir, s.key+".json")); err == nil {
					return // a daemon started on it since the scan
				}
			}
			if !stillStale(revDir, s, cutoff) || !removeStaleReviewPath(s.path) {
				return
			}
			if sessDir != "" {
				os.Remove(filepath.Join(sessDir, s.key+".log"))
			}
		})
	}
}

// stillStale repeats the staleness check for one review after the lock is
// held, since a resume may have written it after the scan.
func stillStale(revDir string, s staleReview, cutoff time.Time) bool {
	info, err := os.Lstat(s.path)
	if err != nil {
		return false
	}
	de := fs.FileInfoToDirEntry(info)
	if info.IsDir() {
		_, ok := checkStaleReviewFolder(revDir, de, s.key, cutoff)
		return ok
	}
	_, ok := checkStaleReview(revDir, de, s.key, cutoff)
	return ok
}

func removeStaleReviewPath(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	if info.IsDir() {
		// Published feedback is durable user data; implicit cleanup must preserve it.
		if _, err := os.Lstat(filepath.Join(path, "feedback", "state.json")); err == nil || !os.IsNotExist(err) {
			return false
		}
		return os.RemoveAll(path) == nil
	}
	if err := os.Remove(path); err != nil {
		return false
	}
	_ = os.Remove(path + ".snapshots.json")
	return true
}

// cleanupOnApproval removes an approved review when cleanup_on_approve is on.
// Pin screenshots are deleted on approval either way: they are captures of
// the reviewed app, kept only while the review is open.
func cleanupOnApproval(approved bool, reviewPath string, cleanupEnabled bool) {
	if !approved || reviewPath == "" {
		return
	}
	// First: a folder kept for its published feedback keeps nothing else.
	removePinScreenshots(reviewPath)
	if cleanupEnabled {
		_ = removeStaleReviewPath(reviewPath)
	}
}

func dirArgs(paths []string) []string {
	var dirs []string
	for _, p := range paths {
		if info, err := os.Stat(p); err == nil && info.IsDir() {
			dirs = append(dirs, p)
		}
	}
	return dirs
}
