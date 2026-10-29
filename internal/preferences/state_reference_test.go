package preferences

import (
	"fmt"
	"github.com/BSH2409/hunkyard/internal/strictjson"
	"os"
	"testing"
	"time"
)

func TestStateReferenceParity(t *testing.T) {
	data, err := os.ReadFile("testdata/contracts/state.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Reference string
		Events    []struct {
			Case, Kind string
			Input      any
			Accepted   bool
			Result     any
			ErrorClass string `json:"error_class"`
		}
	}
	if err := strictjson.Decode(data, &fixture); err != nil {
		t.Fatal(err)
	}
	for i, event := range fixture.Events {
		t.Run(fmt.Sprintf("%s/%s/%d", event.Case, event.Kind, i), func(t *testing.T) {
			var err error
			var result any
			switch event.Kind {
			case "rule":
				err = validateRule(rule(record(event.Input)))
			case "graph":
				var values []map[string]any
				values, err = mapArray(event.Input)
				if err == nil {
					rules := []rule{}
					for _, value := range values {
						rules = append(rules, rule(value))
					}
					err = validateRuleGraph(rules)
				}
			default:
				input := record(event.Input)
				s := testStore(t)
				clock, parseErr := time.Parse(time.RFC3339, textField(input["clock"]))
				if parseErr != nil {
					t.Fatal(parseErr)
				}
				s.config.Clock = func() time.Time { return clock }
				values, parseErr := mapArray(input["rules"])
				if parseErr != nil {
					t.Fatal(parseErr)
				}
				rules := []rule{}
				for _, value := range values {
					rules = append(rules, rule(value))
				}
				c := &catalog{store: s, rules: cloneRules(rules)}
				id := textField(input["rule"])
				if value := record(input["rule"]); value != nil {
					id = textField(value["id"])
				}
				switch event.Kind {
				case "upsert":
					result, err = c.upsertExplicit(record(input["observation"]), textField(input["directive"]), record(input["scope"]), stringValues(input["surfaces"]))
				case "confirm":
					result, err = c.confirm(id, record(input["observation"]))
				case "retire":
					result, err = c.retire(id)
				case "actions":
					actions, actionErr := mapArray(input["actions"])
					observations, observationErr := mapArray(input["observations"])
					if actionErr != nil || observationErr != nil {
						t.Fatal("invalid action fixture")
					}
					err = c.applyActions(actions, observations)
					result = c.rules
				default:
					t.Fatalf("unknown event %s", event.Kind)
				}
			}
			if (err == nil) != event.Accepted {
				t.Fatalf("acceptance differs for %s: %v", event.Kind, err)
			}
			if err == nil && event.Kind != "rule" && event.Kind != "graph" && !sameValue(result, event.Result) {
				t.Fatalf("catalog result differs: actual %v expected %v", result, event.Result)
			}
		})
	}
}
