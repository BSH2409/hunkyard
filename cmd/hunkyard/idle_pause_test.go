package main

import (
	"testing"
	"time"
)

func TestIdlePauseTiming(t *testing.T) {
	tests := []struct {
		name, testMode, seconds string
		minutes                 int
		idle, every             time.Duration
	}{
		{"minutes from config", "", "", 30, 30 * time.Minute, 30 * time.Second},
		{"disabled", "", "", 0, 0, 30 * time.Second},
		{"seconds ignored outside test mode", "", "3", 30, 30 * time.Minute, 30 * time.Second},
		{"seconds in test mode", "1", "3", 30, 3 * time.Second, time.Second},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("HUNKYARD_TEST_MODE", tt.testMode)
			t.Setenv("HUNKYARD_IDLE_PAUSE_SECONDS", tt.seconds)
			idle, every := idlePauseTiming(tt.minutes)
			if idle != tt.idle || every != tt.every {
				t.Fatalf("idlePauseTiming(%d) = %v, %v; want %v, %v", tt.minutes, idle, every, tt.idle, tt.every)
			}
		})
	}
}
