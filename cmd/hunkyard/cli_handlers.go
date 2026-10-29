package main

import (
	"errors"

	"github.com/BSH2409/hunkyard/internal/clicmd"
	"github.com/BSH2409/hunkyard/internal/comment"
	"github.com/BSH2409/hunkyard/internal/config"
	"github.com/BSH2409/hunkyard/internal/github"
	"github.com/BSH2409/hunkyard/internal/live"
	"github.com/BSH2409/hunkyard/internal/preview"
	"github.com/BSH2409/hunkyard/internal/review"
	"github.com/BSH2409/hunkyard/internal/session"
)

func runConfig(args []string)   { clicmd.Exit(config.RunConfig(args)) }
func runPull(args []string)     { clicmd.Exit(github.RunPull(args)) }
func runPush(args []string)     { clicmd.Exit(github.RunPush(args)) }
func runComment(args []string)  { clicmd.Exit(comment.RunComment(args)) }
func runComments(args []string) { clicmd.Exit(comment.RunComments(args)) }
func runReview(args []string)   { clicmd.Exit(session.RunReview(args)) }
func runResume(args []string)   { clicmd.Exit(session.RunResume(args)) }
func runPlan(args []string)     { clicmd.Exit(session.RunPlan(args)) }
func runStop(args []string)     { clicmd.Exit(session.RunStop(args)) }
func runStatus(args []string)   { clicmd.Exit(session.RunStatus(args)) }
func runCleanup(args []string)  { clicmd.Exit(review.RunCleanup(args)) }

func runLive(args []string)    { live.RunLive(args) }
func runPreview(args []string) { preview.RunPreview(args) }
func runStats(args []string)   { session.RunStats(args) }

func runPlanHook()      { clicmd.Exit(session.RunPlanHook()) }
func runCodexPlanHook() { clicmd.Exit(session.RunCodexPlanHook()) }

func runPR(args []string) {
	if len(args) != 1 {
		clicmd.Exit(errors.New("usage: hunkyard pr <num|url>"))
	}
	clicmd.Exit(session.RunReview([]string{"--pr", args[0]}))
}
