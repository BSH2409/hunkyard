package config

import "testing"

func TestIdleSettingGlobalOnlyAndEnvOverride(t *testing.T) {
	ten, one := 10, 1
	merged := mergeConfigs(Config{IdlePauseMinutes: &ten}, Config{IdlePauseMinutes: &one}, ConfigPresence{})
	if merged.IdlePauseMinutes == nil || *merged.IdlePauseMinutes != 10 {
		t.Fatalf("project config changed idle_pause_minutes: %v", merged.IdlePauseMinutes)
	}
	zero, negative := 0, -5
	tests := []struct {
		name string
		set  *int
		env  string
		want int
	}{
		{"default", nil, "", DefaultIdlePauseMinutes},
		{"global value", &ten, "", 10},
		{"env wins", &ten, "5", 5},
		{"zero disables", &zero, "", 0},
		{"negative disables", &negative, "", 0},
		{"env zero disables", &ten, "0", 0},
		{"bad env ignored", &ten, "soon", 10},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("HUNKYARD_IDLE_PAUSE_MINUTES", tt.env)
			if got := (Config{IdlePauseMinutes: tt.set}).IdlePauseMinutesOrDefault(); got != tt.want {
				t.Fatalf("IdlePauseMinutesOrDefault() = %d, want %d", got, tt.want)
			}
		})
	}
}
