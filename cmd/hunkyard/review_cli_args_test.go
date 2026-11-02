package main

import (
	"reflect"
	"testing"

	"github.com/BSH2409/hunkyard/internal/server"
	"github.com/BSH2409/hunkyard/internal/session"
)

func TestReviewCLIArgsRecordThePinnedRange(t *testing.T) {
	sc := &server.DaemonCLIConfig{
		Files: []string{"src"},
		Focus: &session.Focus{Kind: session.FocusRange, BaseSHA: "abc", HeadSHA: "def"},
	}
	if got, want := reviewCLIArgs(sc), []string{"range:abc..def", "src"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("reviewCLIArgs = %v, want %v", got, want)
	}
	live := &server.DaemonCLIConfig{LiveOrigin: "http://localhost:3000"}
	if got, want := reviewCLIArgs(live), []string{"live", "http://localhost:3000"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("live reviewCLIArgs = %v, want %v", got, want)
	}
}
