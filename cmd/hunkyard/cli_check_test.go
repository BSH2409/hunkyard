package main

import (
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestCheckSuggestsUntrackedCacheOnSlowStatus(t *testing.T) {
	repo := t.TempDir()
	if out, err := exec.Command("git", "-C", repo, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v %s", err, out)
	}
	cached := t.TempDir()
	if out, err := exec.Command("git", "-C", cached, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v %s", err, out)
	}
	if out, err := exec.Command("git", "-C", cached, "config", "core.untrackedCache", "true").CombinedOutput(); err != nil {
		t.Fatalf("git config: %v %s", err, out)
	}
	tests := []struct {
		name     string
		dir      string
		elapsed  time.Duration
		ok       bool
		wantHint bool
	}{
		{"slow status suggests the cache", repo, 800 * time.Millisecond, true, true},
		{"fast status stays quiet", repo, 100 * time.Millisecond, true, false},
		{"cache already on stays quiet", cached, 800 * time.Millisecond, true, false},
		{"outside a repository stays quiet", repo, 0, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			previous := gitStatusTimer
			gitStatusTimer = func(string) (time.Duration, bool) { return tt.elapsed, tt.ok }
			t.Cleanup(func() { gitStatusTimer = previous })
			hint := untrackedCacheHint(tt.dir)
			if got := hint != ""; got != tt.wantHint {
				t.Fatalf("hint = %q, want hint: %v", hint, tt.wantHint)
			}
			if tt.wantHint && !strings.Contains(hint, "git config core.untrackedCache true") {
				t.Fatalf("hint does not name the command: %q", hint)
			}
		})
	}
}
