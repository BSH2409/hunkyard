package vcs

import (
	"errors"
	"path/filepath"
	"reflect"
	"testing"
)

type compareTargetsVCS struct {
	*fakeFetchVCS
	defaultBranch string
	remote        []string
	remoteErr     error
}

func (v *compareTargetsVCS) DefaultBranch() string { return v.defaultBranch }
func (v *compareTargetsVCS) RemoteBranches(string) ([]string, error) {
	return v.remote, v.remoteErr
}

func TestCompareTargetsFor_NilVCS(t *testing.T) {
	got, err := CompareTargetsFor(nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if got.VCS != "" || got.Detected != "" || len(got.Local) != 0 || len(got.Remote) != 0 {
		t.Errorf("CompareTargetsFor(nil) = %+v, want zero", got)
	}
}

func TestLocalBranches_GitRepo(t *testing.T) {
	dir := initTestRepo(t)
	gitT(t, dir, "branch", "feature/z")
	gitT(t, dir, "branch", "feature/a")

	got, err := LocalBranches(dir)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"feature/a", "feature/z", "main"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("LocalBranches() = %v, want %v", got, want)
	}
}

func TestCompareTargetsFor_GitRepo(t *testing.T) {
	dir := initTestRepo(t)
	gitT(t, dir, "branch", "topic")
	v := &compareTargetsVCS{
		fakeFetchVCS:  &fakeFetchVCS{name: "git"},
		defaultBranch: "trunk",
		remote:        []string{"origin/topic", "upstream/trunk"},
	}
	got, err := CompareTargetsFor(v, dir)
	if err != nil {
		t.Fatal(err)
	}
	want := CompareTargets{
		VCS:      "git",
		Detected: "trunk",
		Local:    []string{"main", "topic"},
		Remote:   []string{"origin/topic", "upstream/trunk"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("CompareTargetsFor(git) = %+v, want %+v", got, want)
	}
}

func TestCompareTargetsFor_RemoteError(t *testing.T) {
	dir := initTestRepo(t)
	wantErr := errors.New("remote lookup failed")
	v := &compareTargetsVCS{
		fakeFetchVCS: &fakeFetchVCS{name: "git"},
		remoteErr:    wantErr,
	}
	if _, err := CompareTargetsFor(v, dir); !errors.Is(err, wantErr) {
		t.Errorf("CompareTargetsFor(git) error = %v, want %v", err, wantErr)
	}

	if _, err := LocalBranches(filepath.Join(dir, "missing")); err == nil {
		t.Error("LocalBranches(missing dir) = nil error, want command error")
	}
}
