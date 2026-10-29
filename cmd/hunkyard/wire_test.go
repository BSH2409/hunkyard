package main

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/BSH2409/hunkyard/internal/focus"
	"github.com/BSH2409/hunkyard/internal/github"
	"github.com/BSH2409/hunkyard/internal/session"
	"github.com/BSH2409/hunkyard/internal/testutil"
)

func isolateWireTest(t *testing.T, cwd string) string {
	t.Helper()
	homeDir := t.TempDir()
	testutil.SetHome(t, homeDir)
	if cwd == "" {
		cwd = t.TempDir()
	}
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(cwd); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(origDir) })
	return homeDir
}

func TestWire_ResolveServerConfigPassesSessionID(t *testing.T) {
	dir := t.TempDir()
	homeDir := t.TempDir()
	testutil.SetHome(t, homeDir)

	origDir, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(origDir)

	sc, err := session.ResolveServerConfigFn([]string{"--session", "839f3b4cd5d6", "--no-open"})
	if err != nil {
		t.Fatalf("ResolveServerConfigFn: %v", err)
	}
	if sc.SessionID != "839f3b4cd5d6" {
		t.Errorf("SessionID = %q", sc.SessionID)
	}
}

func TestWirePRResolveHooks_PreservesURLProject(t *testing.T) {
	prevFetch, prevStacked := focus.FetchPRHook, focus.IsStackedPRHook
	t.Cleanup(func() { focus.SetPRResolveHooks(prevFetch, prevStacked) })

	restore := github.SwapFetchPRByNumberForTest(func(n int) (*github.PRInfo, error) {
		if n == 99 {
			return nil, errors.New("fetch boom")
		}
		return &github.PRInfo{
			URL:               "https://github.com/myorg/repo-b/pull/1",
			Number:            n,
			Title:             "Cross-repo",
			BaseRefOid:        "base",
			HeadRefOid:        "head",
			BaseRefName:       "main",
			HeadRefName:       "feat",
			HeadRepoURL:       "https://github.com/fork/repo-b.git",
			IsCrossRepository: true,
		}, nil
	})
	t.Cleanup(restore)

	wirePRResolveHooks()

	info, err := focus.FetchPRHook("https://github.com/myorg/repo-b/pull/1")
	if err != nil {
		t.Fatal(err)
	}
	if info.BaseRepoProject != "myorg/repo-b" {
		t.Errorf("BaseRepoProject = %q", info.BaseRepoProject)
	}
	if info.HeadRepoProject != "fork/repo-b" {
		t.Errorf("HeadRepoProject = %q", info.HeadRepoProject)
	}
	if info.HeadRepoHost != "github.com" {
		t.Errorf("HeadRepoHost = %q", info.HeadRepoHost)
	}

	bare, err := focus.FetchPRHook("7")
	if err != nil {
		t.Fatal(err)
	}
	if bare.BaseRepoProject != "" {
		t.Errorf("bare --pr BaseRepoProject = %q want empty", bare.BaseRepoProject)
	}

	if _, err := focus.FetchPRHook("not-a-pr"); err == nil {
		t.Fatal("expected parse error")
	}
	if _, err := focus.FetchPRHook("99"); err == nil || !strings.Contains(err.Error(), "fetch boom") {
		t.Fatalf("fetch error = %v", err)
	}
	if focus.IsStackedPRHook == nil || focus.IsStackedPRHook(info, nil) {
		t.Fatal("IsStackedPRHook(nil vcs) should be false")
	}
}
