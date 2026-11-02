package live

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type seenRequest struct{ cookie, origin string }

func guardedProxy(t *testing.T, listenHost string) (*httptest.Server, *seenRequest) {
	t.Helper()
	seen := &seenRequest{}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen.cookie, seen.origin = r.Header.Get("Cookie"), r.Header.Get("Origin")
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(upstream.Close)
	handler, err := newGuardedLiveProxy(listenHost, upstream.URL, 9001, "session=configured")
	if err != nil {
		t.Fatal(err)
	}
	ps := httptest.NewServer(handler)
	t.Cleanup(ps.Close)
	return ps, seen
}

func proxyRequest(t *testing.T, ps *httptest.Server, host, fetchSite, origin string) *http.Response {
	t.Helper()
	req, _ := http.NewRequest(http.MethodPost, ps.URL+"/login/callback", strings.NewReader("code=1"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if host != "" {
		req.Host = host
	}
	if fetchSite != "" {
		req.Header.Set("Sec-Fetch-Site", fetchSite)
	}
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	return resp
}

func TestLiveProxyRejectsForeignHost(t *testing.T) {
	ps, _ := guardedProxy(t, "0.0.0.0")
	if resp := proxyRequest(t, ps, "evil.example:3002", "", ""); resp.StatusCode != http.StatusForbidden {
		t.Fatalf("foreign Host status = %d, want 403", resp.StatusCode)
	}
	if resp := proxyRequest(t, ps, "192.168.1.20:3002", "", ""); resp.StatusCode == http.StatusForbidden {
		t.Fatal("IP literal Host rejected on a network bind")
	}
	loop, _ := guardedProxy(t, "127.0.0.1")
	if resp := proxyRequest(t, loop, "192.168.1.20:3002", "", ""); resp.StatusCode != http.StatusForbidden {
		t.Fatalf("non-loopback Host on loopback bind status = %d, want 403", resp.StatusCode)
	}
}

func TestLiveProxyForwardsCrossSiteRequestAsSent(t *testing.T) {
	ps, seen := guardedProxy(t, "127.0.0.1")
	resp := proxyRequest(t, ps, "localhost:3002", "cross-site", "https://accounts.example")
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("cross-site form POST status = %d, want it forwarded", resp.StatusCode)
	}
	if seen.cookie == "session=configured" {
		t.Error("configured cookie injected into a cross-site request")
	}
	if seen.origin != "https://accounts.example" {
		t.Errorf("cross-site Origin = %q, want the browser's", seen.origin)
	}
}

func TestLiveProxyRewritesSameOriginRequest(t *testing.T) {
	ps, seen := guardedProxy(t, "127.0.0.1")
	proxyRequest(t, ps, "localhost:3002", "same-origin", "http://localhost:3002")
	if seen.cookie != "session=configured" {
		t.Errorf("same-origin cookie = %q, want the configured cookie", seen.cookie)
	}
	if seen.origin == "http://localhost:3002" || seen.origin == "" {
		t.Errorf("same-origin Origin = %q, want the upstream origin", seen.origin)
	}
}
