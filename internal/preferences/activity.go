package preferences

import (
	"bufio"
	"bytes"
	"encoding/json"
	"os"
	"time"
)

const (
	activityCaptured             = "captured"
	activityLearned              = "learned"
	activityConfirmed            = "confirmed"
	activityViolated             = "violated"
	activityBlocked              = "blocked"
	activityOverridden           = "overridden"
	activityInterrupted          = "interrupted"
	activityMaintenanceSucceeded = "maintenance_succeeded"
	activityMaintenanceFailed    = "maintenance_failed"
)

const activityRetention = 30 * 24 * time.Hour

// ActivityEvent is one line of the store's activity log, read by the Yard.
type ActivityEvent struct {
	At         string `json:"at"`
	Kind       string `json:"kind"`
	RuleID     string `json:"rule_id,omitempty"`
	RepoKey    string `json:"repo_key,omitempty"`
	SessionKey string `json:"session_key,omitempty"`
	Detail     string `json:"detail,omitempty"`
}

func (s *Store) recordActivity(e ActivityEvent) error {
	if e.At == "" {
		e.At = s.config.Clock().UTC().Format(time.RFC3339)
	}
	if e.Detail != "" {
		privacy := sanitize(e.Detail)
		e.Detail = ""
		if privacy.Accepted {
			e.Detail = truncate(privacy.Excerpt, 240)
		}
	}
	if err := ensureDirectory(s.dataPath()); err != nil {
		return err
	}
	encoded, err := canonicalJSON(e)
	if err != nil {
		return err
	}
	name := s.dataPath("activity.jsonl")
	return withFileLock(name+".lock", false, func() error {
		file, err := os.OpenFile(name, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
		if err != nil {
			return err
		}
		defer file.Close()
		_, err = file.Write(append(encoded, '\n'))
		return err
	})
}

// Activity returns up to limit events, newest first.
func (s *Store) Activity(limit int) ([]ActivityEvent, error) {
	events, err := s.readActivity()
	if err != nil {
		return nil, err
	}
	newest := make([]ActivityEvent, 0, min(limit, len(events)))
	for i := len(events) - 1; i >= 0 && len(newest) < limit; i-- {
		newest = append(newest, events[i])
	}
	return newest, nil
}

func (s *Store) readActivity() ([]ActivityEvent, error) {
	data, err := regularFile(s.dataPath("activity.jsonl"))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var events []ActivityEvent
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 4096), 1024*1024)
	for scanner.Scan() {
		var e ActivityEvent
		if json.Unmarshal(scanner.Bytes(), &e) == nil && e.Kind != "" {
			events = append(events, e)
		}
	}
	return events, scanner.Err()
}

func (s *Store) pruneActivity() error {
	name := s.dataPath("activity.jsonl")
	return withFileLock(name+".lock", false, func() error {
		events, err := s.readActivity()
		if err != nil || events == nil {
			return err
		}
		cutoff := s.config.Clock().Add(-activityRetention)
		var kept bytes.Buffer
		for _, e := range events {
			at, err := time.Parse(time.RFC3339, e.At)
			if err == nil && at.Before(cutoff) {
				continue
			}
			encoded, err := canonicalJSON(e)
			if err != nil {
				return err
			}
			kept.Write(append(encoded, '\n'))
		}
		return atomicReplace(name, kept.Bytes())
	})
}

// proposalActivity turns applied maintenance actions into activity events. A
// confirmation backed by a correction means the user had to repeat a rule the
// agent already had, so it counts as a violation.
func proposalActivity(actions []any, observations []map[string]any) []ActivityEvent {
	corrections := map[string]bool{}
	for _, o := range observations {
		if record(o["signals"])["correction"] == true {
			corrections[textField(o["id"])] = true
		}
	}
	var events []ActivityEvent
	for _, value := range actions {
		action := record(value)
		ruleID := textField(action["rule_id"])
		switch action["type"] {
		case "create_rule":
			events = append(events, ActivityEvent{Kind: activityLearned, RuleID: ruleID})
		case "confirm_rule":
			kind := activityConfirmed
			for _, id := range stringValues(action["observation_ids"]) {
				if corrections[id] {
					kind = activityViolated
				}
			}
			events = append(events, ActivityEvent{Kind: kind, RuleID: ruleID})
		}
	}
	return events
}
