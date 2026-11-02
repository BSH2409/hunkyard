package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/BSH2409/hunkyard/internal/preferences"
)

func TestRuleVerdictEndpoint(t *testing.T) {
	s, sess := annotationServer(t)
	sess.SetRuleChecker(func(lines []preferences.AddedLine) []preferences.DiffFinding {
		return []preferences.DiffFinding{{RuleID: "rule_x", Directive: "Avoid b.", Path: "server.go", Line: 2, Message: "b added"}}
	})
	sess.RunRuleChecks()
	if rec := postAnnotations(s, `{"tool":"lint","findings":[{"path":"server.go","line":3,"end_line":3,"message":"m"}]}`); rec.Code != 200 {
		t.Fatal(rec.Body.String())
	}
	var ruleID, toolID string
	for _, c := range sess.GetVisibleComments()["server.go"] {
		if c.Source == "rule" {
			ruleID = c.ID
		} else if c.Source == "tool" {
			toolID = c.ID
		}
	}
	cases := []struct {
		name, method, id, body string
		want                   int
	}{
		{"outside the page", http.MethodPut, ruleID, `{"verdict":"dismissed"}`, http.StatusForbidden},
		{"bad verdict", http.MethodPut, ruleID, `{"verdict":"maybe"}`, http.StatusBadRequest},
		{"bad body", http.MethodPut, ruleID, `{`, http.StatusBadRequest},
		{"unknown comment", http.MethodPut, "c_missing", `{"verdict":"dismissed"}`, http.StatusNotFound},
		{"tool comment", http.MethodPut, toolID, `{"verdict":"dismissed"}`, http.StatusConflict},
		{"method", http.MethodPost, ruleID, `{"verdict":"dismissed"}`, http.StatusMethodNotAllowed},
		{"dismiss", http.MethodPut, ruleID, `{"verdict":"dismissed"}`, http.StatusOK},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(tc.method, "/api/comment/"+tc.id+"/verdict?path=server.go", strings.NewReader(tc.body))
			if tc.name != "outside the page" {
				req.Header.Set("Sec-Fetch-Site", "same-origin")
			}
			s.ServeHTTP(rec, req)
			if rec.Code != tc.want {
				t.Fatalf("%s = %d %s, want %d", tc.name, rec.Code, rec.Body, tc.want)
			}
		})
	}
	for _, c := range sess.GetVisibleComments()["server.go"] {
		if c.ID == ruleID && (!c.Resolved || c.Verdict != "dismissed") {
			t.Fatalf("dismissal not stored: %+v", c)
		}
	}
}
