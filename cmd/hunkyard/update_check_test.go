package main

import (
	"testing"

	"github.com/BSH2409/hunkyard/internal/server"
)

func TestUpdateCheckEnabled(t *testing.T) {
	tests := []struct {
		name     string
		disabled bool
		env      string
		want     bool
	}{
		{"enabled by default", false, "", true},
		{"no_update_check config", true, "", false},
		{"HUNKYARD_NO_UPDATE_CHECK", false, "1", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("HUNKYARD_NO_UPDATE_CHECK", tt.env)
			if got := updateCheckEnabled(&server.DaemonCLIConfig{NoUpdateCheck: tt.disabled}); got != tt.want {
				t.Fatalf("updateCheckEnabled = %v, want %v", got, tt.want)
			}
		})
	}
}
