package server

import "testing"

func TestInstallationSourceForPath(t *testing.T) {
	tests := []struct {
		name   string
		path   string
		goBin  string
		goPath string
		home   string
		want   string
	}{
		{"nix store", "/nix/store/hash-hunkyard-0.20.2/bin/hunkyard", "", "", "/home/me", installationSourceNix},
		{"nix path containing opt/hunkyard", "/nix/store/hash/opt/hunkyard/bin/hunkyard", "", "", "/home/me", installationSourceNix},
		{"manual FHS opt", "/opt/hunkyard/bin/hunkyard", "", "", "/Users/me", installationSourceUnknown},
		{"default Go bin", "/Users/me/go/bin/hunkyard", "", "", "/Users/me", installationSourceGo},
		{"configured Go bin", "/tools/bin/hunkyard", "/tools/bin", "", "/Users/me", installationSourceGo},
		{"GOPATH bin", "/workspace/bin/hunkyard", "", "/workspace", "/Users/me", installationSourceGo},
		{"downloaded binary", "/usr/local/bin/hunkyard", "", "", "/Users/me", installationSourceUnknown},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := installationSourceForPath(tt.path, tt.goBin, tt.goPath, tt.home); got != tt.want {
				t.Errorf("installationSourceForPath() = %q, want %q", got, tt.want)
			}
		})
	}
}
