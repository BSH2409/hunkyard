package vcs

import "testing"

func TestSplitCommitRange(t *testing.T) {
	tests := []struct {
		name     string
		commit   string
		wantBase string
		wantHead string
		wantOK   bool
	}{
		{"single sha", "abc123", "", "", false},
		{"empty", "", "", "", false},
		{"valid range", "abc123..def456", "abc123", "def456", true},
		{"full hex shas", "0a1b2c3d4e5f60718293a4b5c6d7e8f901234567..76543210fedcba98765432100123456789abcdef", "0a1b2c3d4e5f60718293a4b5c6d7e8f901234567", "76543210fedcba98765432100123456789abcdef", true},
		{"option injection base", "--foo..def456", "", "", false},
		{"option injection head", "abc123..--bar", "", "", false},
		{"empty base", "..def456", "", "", false},
		{"empty head", "abc123..", "", "", false},
		{"both empty", "..", "", "", false},
		{"non-hex base", "zzzz..def456", "", "", false},
		{"non-hex head", "abc123..gggg", "", "", false},
		{"too short base", "ab..def456", "", "", false},
		{"path-like ref", "main..feature", "", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			base, head, ok := SplitCommitRange(tt.commit)
			if base != tt.wantBase || head != tt.wantHead || ok != tt.wantOK {
				t.Errorf("SplitCommitRange(%q) = (%q, %q, %v), want (%q, %q, %v)",
					tt.commit, base, head, ok, tt.wantBase, tt.wantHead, tt.wantOK)
			}
		})
	}
}

func TestIsHexSHA(t *testing.T) {
	tests := []struct {
		in   string
		want bool
	}{
		{"abc1", true},
		{"0123456789abcdefABCDEF0123456789abcdef01", true},
		{"abc", false},   // too short (<4)
		{"", false},      // empty
		{"--foo", false}, // option injection
		{"main", false},  // non-hex
		{"abc123def456abc123def456abc123def456abc12", false}, // 41 chars, too long
	}
	for _, tt := range tests {
		if got := isHexSHA(tt.in); got != tt.want {
			t.Errorf("isHexSHA(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}
