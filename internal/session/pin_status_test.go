package session

import (
	"encoding/json"
	"testing"
)

func TestLegacyDriftedPinLoadsAsOutdated(t *testing.T) {
	var pin Comment
	if err := json.Unmarshal([]byte(`{"id":"p1","dom_anchor":{"pathname":"/","css_selector":"h1"},"drifted":true,"drifted_on_round":2}`), &pin); err != nil {
		t.Fatal(err)
	}
	if pin.PinStatus != PinOutdated || pin.Drifted || pin.DriftedOnRound != 0 {
		t.Errorf("legacy drifted pin = status %q drifted %v round %d, want outdated and no drift", pin.PinStatus, pin.Drifted, pin.DriftedOnRound)
	}

	var line Comment
	if err := json.Unmarshal([]byte(`{"id":"c1","drifted":true,"drifted_on_round":2}`), &line); err != nil {
		t.Fatal(err)
	}
	if !line.Drifted || line.DriftedOnRound != 2 || line.PinStatus != "" {
		t.Errorf("line comment drift changed: %+v", line)
	}
}

func TestReanchorResetsPinStatus(t *testing.T) {
	s := &Session{ReviewRound: 3, Files: []*FileEntry{{Path: "/"}}}
	pin, ok := s.AddLivePin("/", "pay", "r", &DOMAnchor{Pathname: "/", CSSSelector: "#pay", TagChain: []string{"BUTTON"}})
	if !ok {
		t.Fatal("pin not added")
	}
	if _, ok := s.SetPinStatus("/", pin.ID, PinOutdated); !ok {
		t.Fatal("status not set")
	}
	got, ok, _ := s.UpdateCommentWithAnchor("/", pin.ID, "", &DOMAnchor{Pathname: "/", CSSSelector: "#pay-now", TagChain: []string{"BUTTON"}})
	if !ok || got.PinStatus != PinCurrent || got.PinCheckedRound != 3 {
		t.Errorf("re-anchored pin = status %q checked round %d, want current on round 3", got.PinStatus, got.PinCheckedRound)
	}
}

// Legacy line comments get scope "line" on load; live pins have no line.
func TestLoadingLeavesLivePinsWithoutLineScope(t *testing.T) {
	comments := []Comment{{ID: "c1"}, {ID: "p1", DOMAnchor: &DOMAnchor{Pathname: "/", CSSSelector: "#pay"}}}
	defaultEmptyCommentScopes(comments)
	if comments[0].Scope != "line" || comments[1].Scope != "" {
		t.Errorf("scopes after load = %q, %q; want line for the legacy comment and none for the pin", comments[0].Scope, comments[1].Scope)
	}
}
