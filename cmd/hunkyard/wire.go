package main

import (
	"github.com/BSH2409/hunkyard/internal/browser"
	"github.com/BSH2409/hunkyard/internal/config"
	"github.com/BSH2409/hunkyard/internal/daemon"
	"github.com/BSH2409/hunkyard/internal/focus"
	"github.com/BSH2409/hunkyard/internal/github"
	"github.com/BSH2409/hunkyard/internal/live"
	"github.com/BSH2409/hunkyard/internal/notify"
	"github.com/BSH2409/hunkyard/internal/preview"
	"github.com/BSH2409/hunkyard/internal/server"
	"github.com/BSH2409/hunkyard/internal/session"
	"github.com/BSH2409/hunkyard/internal/vcs"
)

type (
	Config           = config.Config
	Session          = session.Session
	HunkyardJSON     = session.HunkyardJSON
	HunkyardJSONFile = session.HunkyardJSONFile
	Focus            = session.Focus
	Reply            = session.Reply
	SSEEvent         = session.SSEEvent
	InheritedScope   = focus.InheritedScope
	Server           = server.Server
	StaleIntegration = server.StaleIntegration
	PRInfo           = github.PRInfo
	sessionEntry     = daemon.SessionEntry
)

var (
	DetectVCS    = vcs.DetectVCS
	ResolveFocus = focus.ResolveFocus
	NewServer    = server.NewServer
	mustGetwd    = session.MustGetwd

	resolvedCWD        = daemon.ResolvedCWD
	sessionKey         = daemon.SessionKey
	liveSessionKey     = daemon.LiveSessionKey
	previewSessionKey  = preview.PreviewSessionKey
	planSessionKey     = session.PlanSessionKey
	writeSessionFile   = daemon.WriteSessionFile
	writeDaemonFailure = daemon.WriteDaemonFailure
	removeSessionFile  = daemon.RemoveSessionFile
	reviewFilePath     = daemon.ReviewFilePath
	openReadyPipe      = daemon.OpenReadyPipe
	daemonFatal        = daemon.DaemonFatal
	signalReadiness    = daemon.SignalReadiness
	hostForDisplay     = daemon.HostForDisplay
	advertisedURL      = daemon.AdvertisedURL
	shutdownSignals    = daemon.ShutdownSignals
	openBrowser        = browser.OpenBrowserWithCommand
	notifyRoundReady   = notify.RoundReady

	reviewPathsFor = session.ReviewPathsFor
	detectPRInfo   = github.DetectPRInfo
	CurrentBranch  = vcs.CurrentBranch

	bindProxyServer    = live.BindProxyServer
	recordSessionStats = session.RecordSessionStats
	atomicWriteFile    = session.AtomicWriteFile
)

func init() {
	session.InvalidatePRCache = func(number int, _, _ string) {
		// Drop bare and project-qualified sibling keys for this number —
		// matching hunkyard pull — so a focus switch cannot leave a stale twin.
		github.InvalidatePRCache(number)
	}
	session.PrintVersionFn = printVersion
	session.PrintHelpFn = printHelp
	server.PrintVersionFn = printVersion
	server.PrintHelpFn = printHelp

	session.InstalledAgentsFn = installedAgents
	session.CheckMissingIntegrationsFn = checkMissingIntegrations
	session.PrintMissingHintsFn = printMissingHints
	server.AvailableIntegrationsFn = availableIntegrations
	server.DetectInstalledIntegrationsFn = func(projectDir, homeDir string) []server.IntegrationStatus {
		statuses := detectInstalledIntegrations(projectDir, homeDir)
		out := make([]server.IntegrationStatus, len(statuses))
		for i, st := range statuses {
			out[i] = server.IntegrationStatus{
				Agent: st.Agent, Status: st.Status, Location: st.Location, Hint: st.Hint, Hash: st.Hash,
			}
		}
		return out
	}
	session.ResolveServerConfigFn = func(args []string) (*session.CLIReviewConfig, error) {
		sc, err := server.ResolveDaemonCLIConfig(args)
		if err != nil {
			return nil, err
		}
		if sc == nil {
			return nil, nil
		}
		return &session.CLIReviewConfig{
			Port:               sc.Port,
			Host:               sc.Host,
			Files:              sc.Files,
			Focus:              sc.Focus,
			OutputDir:          sc.OutputDir,
			PlanDir:            sc.PlanDir,
			NoOpen:             sc.NoOpen,
			OpenCmd:            sc.OpenCmd,
			Quiet:              sc.Quiet,
			NoIntegrationCheck: sc.NoIntegrationCheck,
			VCSOverride:        sc.VCSOverride,
			BaseBranch:         sc.BaseBranch,
			IgnorePatterns:     sc.IgnorePatterns,
			SkillsFile:         sc.SkillsFile,
			SessionID:          sc.SessionID,
		}, nil
	}
	session.PreflightCheckFn = func(sc *session.CLIReviewConfig) string {
		return server.PreflightCheck(&server.DaemonCLIConfig{
			VCSOverride:    sc.VCSOverride,
			BaseBranch:     sc.BaseBranch,
			IgnorePatterns: sc.IgnorePatterns,
		})
	}
	// Remote forge resolution is excluded from the local-only build.
}

func wirePRResolveHooks() {
	focus.SetPRResolveHooks(
		func(spec string) (focus.ChangeResolveInfo, error) {
			id, err := github.ParsePRSpec(spec)
			if err != nil {
				return focus.ChangeResolveInfo{}, err
			}
			info, err := github.FetchPR(id)
			if err != nil {
				return focus.ChangeResolveInfo{}, err
			}
			return focus.ChangeResolveInfo{
				URL:               info.URL,
				Number:            info.Number,
				Title:             info.Title,
				BaseRefOid:        info.BaseRefOid,
				HeadRefOid:        info.HeadRefOid,
				BaseRefName:       info.BaseRefName,
				HeadRefName:       info.HeadRefName,
				HeadRepoURL:       info.HeadRepoURL,
				BaseRepoProject:   id.Project, // only URL-derived; bare --pr stays checkout-scoped
				HeadRepoProject:   github.ProjectFromRemoteURL(info.HeadRepoURL),
				HeadRepoHost:      id.Host,
				IsCrossRepository: info.IsCrossRepository,
			}, nil
		},
		func(info focus.ChangeResolveInfo, v vcs.VCS) bool {
			return github.IsStackedPR(&PRInfo{
				URL:               info.URL,
				Number:            info.Number,
				Title:             info.Title,
				BaseRefOid:        info.BaseRefOid,
				HeadRefOid:        info.HeadRefOid,
				BaseRefName:       info.BaseRefName,
				HeadRefName:       info.HeadRefName,
				HeadRepoURL:       info.HeadRepoURL,
				IsCrossRepository: info.IsCrossRepository,
			}, v)
		},
	)
}
