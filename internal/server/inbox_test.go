package server

import (
	"net/http"
	"strings"
	"testing"
)

func TestInboxEndpoints(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	s, sess := newTestServer(t)
	sess.ReviewRound = 2
	sameOrigin := map[string]string{"Sec-Fetch-Site": "same-origin"}
	cases := []struct {
		name, method, target, body string
		header                     map[string]string
		want                       int
		contains                   string
	}{
		{"list", http.MethodGet, "/api/inbox", "", nil, http.StatusOK, "["},
		{"list method", http.MethodPost, "/api/inbox", "", sameOrigin, http.StatusMethodNotAllowed, ""},
		{"state", http.MethodGet, "/api/inbox/state", "", nil, http.StatusOK, `"round":2`},
		{"state method", http.MethodPost, "/api/inbox/state", "", sameOrigin, http.StatusMethodNotAllowed, ""},
		{"resume outside the page", http.MethodPost, "/api/inbox/resume", `{"key":"0123456789ab"}`, nil, http.StatusForbidden, ""},
		{"resume bad key", http.MethodPost, "/api/inbox/resume", `{"key":"../x"}`, sameOrigin, http.StatusBadRequest, ""},
		{"resume bad body", http.MethodPost, "/api/inbox/resume", `{`, sameOrigin, http.StatusBadRequest, ""},
		{"resume not paused", http.MethodPost, "/api/inbox/resume", `{"key":"0123456789ab"}`, sameOrigin, http.StatusConflict, ""},
		{"resume method", http.MethodGet, "/api/inbox/resume", "", nil, http.StatusMethodNotAllowed, ""},
		{"page", http.MethodGet, "/inbox", "", nil, http.StatusOK, "Review inbox"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := serve(s, tc.method, tc.target, tc.body, tc.header)
			if rec.Code != tc.want || !strings.Contains(rec.Body.String(), tc.contains) {
				t.Fatalf("%s = %d %s", tc.name, rec.Code, rec.Body)
			}
		})
	}
}
