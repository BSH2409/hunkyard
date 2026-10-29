package main

import (
	"github.com/BSH2409/hunkyard/internal/clicmd"
	"github.com/BSH2409/hunkyard/internal/focus"
	"github.com/BSH2409/hunkyard/internal/live"
	"github.com/BSH2409/hunkyard/internal/localonly"
	"github.com/BSH2409/hunkyard/internal/preview"
	"github.com/BSH2409/hunkyard/internal/session"
)

type positionalRoute int

const (
	positionalRouteReview positionalRoute = iota
	positionalRouteChangeReview
	positionalRouteLive
	positionalRoutePreview
)

var (
	runReviewForPositionalCLI  = session.RunReview
	runLiveForPositionalCLI    = runLive
	runPreviewForPositionalCLI = runPreview
)

// routePositionalArgs classifies bare positional hunkyard arguments (no subcommand).
func routePositionalArgs(args []string) positionalRoute {
	if localonly.Enabled() {
		return positionalRouteReview
	}
	if _, ok := changeReviewArgs(args); ok {
		return positionalRouteChangeReview
	}
	if live.LooksLikeLiveArgs(args) {
		return positionalRouteLive
	}
	if preview.LooksLikePreviewArgs(args) {
		return positionalRoutePreview
	}
	return positionalRouteReview
}

// changeReviewArgs rewrites one GitHub PR URL to explicit review argv.
func changeReviewArgs(args []string) ([]string, bool) {
	if len(args) != 1 {
		return nil, false
	}
	if !focus.LooksLikePRURL(args[0]) {
		return nil, false
	}
	return []string{"--pr", args[0]}, true
}

// runPositionalCLI dispatches bare positional arguments from main.
func runPositionalCLI(args []string) {
	switch routePositionalArgs(args) {
	case positionalRouteChangeReview:
		changeArgs, _ := changeReviewArgs(args)
		clicmd.Exit(runReviewForPositionalCLI(changeArgs))
	case positionalRouteLive:
		runLiveForPositionalCLI(args)
	case positionalRoutePreview:
		runPreviewForPositionalCLI(args)
	default:
		clicmd.Exit(runReviewForPositionalCLI(args))
	}
}
