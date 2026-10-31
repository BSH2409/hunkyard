package feedback

import (
	"os"
	"testing"
)

func TestOwnerRecordModeIsPrivate(t *testing.T) {
	tests := []struct {
		mode os.FileMode
		goos string
		want bool
	}{
		{0o600, "linux", true},
		{0o644, "linux", false},
		{0o640, "darwin", false},
		{0o666, "windows", true},
	}
	for _, tt := range tests {
		if got := privateRecordMode(tt.mode, tt.goos); got != tt.want {
			t.Errorf("privateRecordMode(%o, %s) = %v, want %v", tt.mode, tt.goos, got, tt.want)
		}
	}
}
