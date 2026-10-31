package main

import "testing"

func TestYardHostPrecedence(t *testing.T) {
	tests := []struct{ name, flag, env, global, want string }{
		{"loopback when nothing is set", "", "", "", "127.0.0.1"},
		{"global config host", "", "", "0.0.0.0", "0.0.0.0"},
		{"env over global config", "", "127.0.0.2", "0.0.0.0", "127.0.0.2"},
		{"flag over env", "::1", "127.0.0.2", "0.0.0.0", "::1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("HUNKYARD_HOST", tt.env)
			if got := yardHost(tt.flag, tt.global); got != tt.want {
				t.Fatalf("yardHost = %q, want %q", got, tt.want)
			}
		})
	}
}
