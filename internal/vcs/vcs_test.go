package vcs

import "testing"

// TestDetectVCSGitOnly pins git as the only backend: Sapling and Jujutsu
// overrides were removed in v1.0.3 and now detect nothing.
func TestDetectVCSGitOnly(t *testing.T) {
	dir := InitTestRepo(t)
	t.Chdir(dir)
	tests := []struct {
		override string
		wantGit  bool
	}{
		{"", true},
		{"git", true},
		{"sl", false},
		{"sapling", false},
		{"jj", false},
		{"jujutsu", false},
	}
	for _, tt := range tests {
		t.Run(tt.override, func(t *testing.T) {
			v := DetectVCS(tt.override)
			if tt.wantGit {
				if v == nil || v.Name() != "git" {
					t.Fatalf("DetectVCS(%q) = %v, want git", tt.override, v)
				}
				return
			}
			if v != nil {
				t.Fatalf("DetectVCS(%q) = %s, want nil", tt.override, v.Name())
			}
		})
	}
}
