package preferences

import (
	"fmt"
	"strings"

	"github.com/BSH2409/hunkyard/internal/config"
)

// doctor checks the whole preference setup: store integrity, hooks on each
// host, maintenance configuration and its last run, and the cron job.
func (s *Store) doctor() map[string]any {
	checks := []map[string]any{}
	add := func(name, status, detail string) {
		checks = append(checks, map[string]any{"name": name, "status": status, "detail": detail})
	}
	if audit := s.audit(false); audit.Status == "success" {
		add("store", "ok", "audit passed")
	} else {
		add("store", "fail", "audit errors: "+strings.Join(audit.Errors, ", "))
	}
	for _, host := range []string{"claude", "codex"} {
		missing := s.missingHookEvents(host)
		if len(missing) == 0 {
			add(host+"_hooks", "ok", "all events installed")
		} else {
			add(host+"_hooks", "warn", fmt.Sprintf("missing or stale: %s; run: hunkyard preferences install-hook --host %s", strings.Join(missing, ", "), host))
		}
	}
	add("codex_trust", "warn", "confirm the Hunkyard commands are trusted in Codex's /hooks view")
	maintenance := config.PreferencesMaintenance{}
	if global, _, err := config.LoadConfigFile(config.GlobalConfigPath()); err == nil {
		maintenance = global.PreferencesMaintenance.Resolved()
	}
	run := s.lastMaintenanceRun()
	switch {
	case maintenance.Host == "":
		add("maintenance", "warn", "automatic maintenance is off; set preferences_maintenance.host in ~/.hunkyard.config.json")
	case !maintenance.Supported():
		add("maintenance", "fail", fmt.Sprintf("unknown preferences_maintenance.host %q; use claude or codex", maintenance.Host))
	case run.Outcome == "failed":
		add("maintenance", "fail", fmt.Sprintf("%s %s last run failed: %s at %s", maintenance.Host, maintenance.Model, run.ErrorClass, run.At))
	default:
		add("maintenance", "ok", fmt.Sprintf("%s %s (%s effort)", maintenance.Host, maintenance.Model, maintenance.Effort))
	}
	if status, err := s.status(); err == nil && status["cron_installed"] == true {
		add("cron", "ok", "hourly maintenance check installed")
	} else {
		add("cron", "warn", "run: hunkyard preferences install-cron")
	}
	overall := "ok"
	for _, check := range checks {
		if check["status"] != "ok" && check["name"] != "codex_trust" {
			overall = "attention"
		}
	}
	return map[string]any{"status": overall, "checks": checks}
}

// missingHookEvents lists events whose current Hunkyard command is absent
// from a host's hook configuration.
func (s *Store) missingHookEvents(host string) []string {
	target, err := s.hookPath(host)
	if err != nil {
		return []string{"configuration path"}
	}
	hooks := record(readJSONFile(target)["hooks"])
	var missing []string
	for _, event := range hookEventsFor(host) {
		want, err := s.hookCommand(host, event.handlerName())
		if err != nil {
			return []string{"hook command"}
		}
		found := false
		groups, _ := mapArray(hooks[event.name])
		for _, group := range groups {
			entries, _ := mapArray(group["hooks"])
			for _, entry := range entries {
				found = found || textField(entry["command"]) == want
			}
		}
		if !found {
			missing = append(missing, event.handlerName())
		}
	}
	return missing
}
