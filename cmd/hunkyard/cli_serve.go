package main

import (
	"context"
	"errors"
	"fmt"
	"github.com/BSH2409/hunkyard/internal/localonly"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/BSH2409/hunkyard/internal/feedback"
	"github.com/BSH2409/hunkyard/internal/reviewpath"
	"github.com/BSH2409/hunkyard/internal/server"
	"github.com/BSH2409/hunkyard/internal/session"
)

// liveSessionArgsTag is the leading element of sessionEntry.Args for a
// live daemon: ["live", "<origin>"].
const liveSessionArgsTag = "live"

func serveSessionKey(sc *server.DaemonCLIConfig) string {
	if sc.SessionKeyOverride != "" {
		return sc.SessionKeyOverride
	}
	cwd, _ := resolvedCWD()
	if sc.PlanDir != "" {
		return planSessionKey(cwd, sc.PlanName)
	}
	if sc.LiveOrigin != "" {
		return liveSessionKey(cwd, sc.LiveOrigin)
	}
	if sc.PreviewFile != "" {
		return previewSessionKey(cwd, sc.PreviewFile)
	}
	branch := ""
	if vcs := DetectVCS(sc.VCSOverride); vcs != nil {
		branch = vcs.CurrentBranch()
	}
	return sessionKey(cwd, branch, session.ReviewKeyArgs(sc.Focus, sc.Files))
}

func checkStaleIntegrations(sc *server.DaemonCLIConfig, srv *Server, cwd string) {
	if sc.NoIntegrationCheck || os.Getenv("HUNKYARD_NO_INTEGRATION_CHECK") != "" {
		return
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}
	stale := checkInstalledIntegrations(cwd, home)
	warnings := make([]StaleIntegration, len(stale))
	for i, sf := range stale {
		warnings[i] = StaleIntegration{
			Agent:    sf.agent,
			Location: sf.location,
			Hint:     sf.updateHint(),
			Hash:     sf.hash,
		}
	}
	srv.SetIntegrationWarnings(warnings, checkMissingIntegrations(cwd, home))
}

func bindListener(host string, port int) (net.Listener, error) {
	addr := net.JoinHostPort(host, strconv.Itoa(port))
	var listener net.Listener
	var err error
	for attempt := 0; attempt < 3; attempt++ {
		listener, err = net.Listen("tcp", addr)
		if err == nil {
			return listener, nil
		}
		if port == 0 {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	return nil, err
}

// resolveServeReviewPath computes the daemon's review folder so that
// srv.reviewPath, the session-registry entry, session.ReviewFilePath, and
// session.hunkyardJSONPath() all agree on one folder.
var serveAbsPath = filepath.Abs

func resolveServeReviewPath(outputDir, planDir, sessionKey string) (string, error) {
	switch {
	case outputDir != "":
		// --output / config output is a hunkyard data root (like ~/.hunkyard).
		return reviewpath.Identity(outputDir, sessionKey)
	case planDir != "":
		abs, err := serveAbsPath(planDir)
		if err != nil {
			return "", err
		}
		return filepath.Join(abs, ".hunkyard"), nil
	default:
		return reviewFilePath(sessionKey)
	}
}

func runServe(args []string) {
	pipe := openReadyPipe()

	sc, err := server.ResolveDaemonCLIConfig(args)
	if err != nil {
		daemonFatal(pipe, "Error: %v", err)
	}
	if sc == nil {
		return
	}
	sc.Quiet = true
	if err := localonly.CheckURL((sessionEntry{Host: sc.Host, Port: sc.Port}).ConnURL()); err != nil {
		daemonFatal(pipe, "Error: %v", err)
	}
	sc.OpenCmd = ""
	if sc.PublicURL != "" {
		if err := localonly.CheckURL(sc.PublicURL); err != nil {
			daemonFatal(pipe, "Error: %v", err)
		}
	}

	listener, err := bindListener(sc.Host, sc.Port)
	if err != nil {
		daemonFatal(pipe, "Error starting server: %v", err)
	}
	addr := listener.Addr().(*net.TCPAddr)

	srv, err := NewServer(nil, frontendFS, sc.Author, version, addr.Port, sc.AgentCmd)
	if err != nil {
		daemonFatal(pipe, "Error creating server: %v", err)
	}
	srv.SetListenHost(sc.Host)
	srv.SetPublicURL(sc.PublicURL)
	if sc.AllowUnauthenticatedNetwork {
		fmt.Fprintln(os.Stderr, "hunkyard: WARNING: unauthenticated network exposure enabled. Anyone who can reach this port can read review files and write comments.")
	}

	cwd, _ := resolvedCWD()
	homeDir := ""
	if home, err := os.UserHomeDir(); err == nil {
		homeDir = home
	}
	key := serveSessionKey(sc)
	branch := ""
	if vcs := DetectVCS(sc.VCSOverride); vcs != nil {
		branch = vcs.CurrentBranch()
	}
	sc.ReviewPath, err = resolveServeReviewPath(sc.OutputDir, sc.PlanDir, key)
	if err != nil {
		daemonFatal(pipe, "Error: resolve review path: %v", err)
	}
	var cliArgs []string
	switch {
	case sc.LiveOrigin != "":
		cliArgs = []string{"live", sc.LiveOrigin}
	case sc.PreviewFile != "":
		cliArgs = []string{"preview", sc.PreviewFile}
	default:
		cliArgs = sc.Files
	}
	sessionArgs := sc.Files
	if sc.LiveOrigin != "" {
		sessionArgs = []string{liveSessionArgsTag, sc.LiveOrigin}
	}
	if sc.PreviewFile != "" {
		sessionArgs = []string{"preview", sc.PreviewFile}
	}
	sessionStartedAt := time.Now().UTC()
	sessionGeneration := sessionStartedAt.Format(time.RFC3339Nano)
	srv.ConfigureDaemon(sc.Cfg, cwd, homeDir, sc.ReviewPath, cliArgs, sessionStartedAt)
	srv.DisableRemoteFeatures()
	owner, err := feedback.OpenOwner(filepath.Join(homeDir, ".hunkyard"), key, feedback.CurrentIdentity())
	if err != nil {
		daemonFatal(pipe, "Error opening review owner: %v", err)
	}
	if err := srv.BindOwner(owner, key, sessionGeneration); err != nil {
		daemonFatal(pipe, "Error binding review owner: %v", err)
	}
	if sc.SkillsFile != "" {
		refs, err := feedback.LoadSkillManifest(sc.SkillsFile)
		if err != nil {
			daemonFatal(pipe, "Error reading host skills: %v", err)
		}
		store, err := feedback.Open(sc.ReviewPath)
		if err != nil {
			daemonFatal(pipe, "Error opening feedback store: %v", err)
		}
		if err := store.RegisterSkills(refs); err != nil {
			daemonFatal(pipe, "Error registering host skills: %v", err)
		}
	}
	if err := writeSessionFile(key, sessionEntry{
		PID:        os.Getpid(),
		Port:       addr.Port,
		Host:       sc.Host,
		PublicURL:  sc.PublicURL,
		CWD:        cwd,
		Args:       sessionArgs,
		Branch:     branch,
		ReviewPath: sc.ReviewPath,
		StartedAt:  sessionGeneration,
	}); err != nil {
		daemonFatal(pipe, "Error writing session file: %v", err)
	}

	var proxyLn net.Listener
	var proxySrv *http.Server
	if sc.LiveOrigin != "" {
		pl, ps, err := bindProxyServer(sc.LiveOrigin, addr.Port, sc.LiveCookie)
		if err != nil {
			daemonFatal(pipe, "Error starting proxy server: %v", err)
		}
		proxyLn = pl
		proxySrv = ps
		go func() {
			if err := proxySrv.Serve(pl); err != nil && !errors.Is(err, http.ErrServerClosed) {
				log.Printf("Proxy server error: %v", err)
				_ = pl.Close()
			}
		}()
	}

	httpServer := &http.Server{
		Handler:     srv,
		ReadTimeout: 15 * time.Second,
		IdleTimeout: 60 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), shutdownSignals()...)
	defer stop()
	// A separate cancel keeps the signal handlers registered when
	// /api/shutdown starts the shutdown; calling stop would unregister them.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	srv.SetShutdownCtx(ctx)
	srv.SetStopFunc(cancel)

	go func() {
		if err := httpServer.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("Server error: %v", err)
			stop()
		}
	}()

	signalReadiness(pipe, addr.Port)

	if !sc.NoOpen {
		openPath := ""
		if sc.LiveOrigin != "" {
			openPath = "/live"
		} else if sc.PreviewFile != "" {
			openPath = "/preview"
		}
		openURL := advertisedURL(sc.PublicURL, sc.Host, addr.Port, openPath)
		go openBrowser(openURL, sc.OpenCmd)
	}

	type sessionResult struct {
		session *Session
		err     error
	}
	ch := make(chan sessionResult, 1)
	go func() {
		s, err := server.CreateSession(sc)
		ch <- sessionResult{s, err}
	}()

	var sess *Session
	var initErr error
	select {
	case res := <-ch:
		sess, initErr = res.session, res.err
	case <-time.After(2 * time.Minute):
		initErr = fmt.Errorf("session initialization timed out after 2 minutes")
	}
	if initErr != nil {
		log.Printf("Error: %v", initErr)
		srv.SetInitErr(initErr)
		if err := writeDaemonFailure(key, sessionGeneration, initErr); err != nil {
			log.Printf("Warning: could not preserve daemon initialization error: %v", err)
		}
		stop()
		<-ctx.Done()
		// Keep the session entry and log until stale-session cleanup. The client
		// may already have received the entry and needs the log to report this
		// fatal initialization error after the server stops.
		shutCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = httpServer.Shutdown(shutCtx)
		return
	}
	server.ApplySessionOverrides(sess, sc)
	switch {
	case sc.LiveOrigin != "":
		sess.CLIArgs = []string{"live", sc.LiveOrigin}
	case sc.PreviewFile != "":
		sess.CLIArgs = []string{"preview", sc.PreviewFile}
	default:
		sess.CLIArgs = sc.Files
	}
	sess.SessionKey = key
	sess.CWD = cwd

	checkStaleIntegrations(sc, srv, cwd)

	if sc.LiveOrigin != "" && proxyLn != nil {
		sess.ProxyPort = proxyLn.Addr().(*net.TCPAddr).Port
		sess.ReviewType = "live"
		sess.Origin = sc.LiveOrigin
	}
	if sess.ReviewType == "live" || sess.ReviewType == "preview" {
		sess.SetLiveRoundStart(func(_, next int) {
			sess.Notify(SSEEvent{Type: "live-round-start", Round: next})
		})
	}
	// Desktop notify is independent of Quiet: the daemon always sets Quiet so
	// status goes through the client pipe, but AFK notifications should still fire.
	if sc.NotifyOnRoundReady {
		reviewURL := advertisedURL(sc.PublicURL, sc.Host, addr.Port, "")
		sess.SetOnRoundReady(func(round int) {
			go notifyRoundReady(round, reviewURL)
		})
	}
	srv.SetSession(sess)

	watchStop := make(chan struct{})
	go sess.Watch(watchStop)

	<-ctx.Done()
	close(watchStop)

	removeSessionFile(key)

	sess.Shutdown()

	var shutWG sync.WaitGroup
	shutWG.Add(1)
	go func() {
		defer shutWG.Done()
		apiCtx, apiCancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer apiCancel()
		_ = httpServer.Shutdown(apiCtx)
	}()
	if proxySrv != nil {
		shutWG.Add(1)
		go func() {
			defer shutWG.Done()
			proxyCtx, proxyCancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer proxyCancel()
			_ = proxySrv.Shutdown(proxyCtx)
		}()
	}
	shutWG.Wait()
	_ = proxyLn

	if !srv.WaitBackground(30 * time.Second) {
		log.Printf("Warning: background goroutines did not drain within 30s; proceeding with shutdown")
	}

	sess.WriteFiles()

	if srv.ShouldRecordStatsOnShutdown() {
		recordSessionStats(sess, srv.Author(), sessionStartedAt)
	}

	if sess.ReviewFilePath != "" {
		fmt.Fprintf(os.Stderr, "Review file: %s\n", reviewPathsFor(sess.ReviewFilePath).Review)
	}
}
