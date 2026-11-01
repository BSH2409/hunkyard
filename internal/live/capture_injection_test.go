package live

import (
	"strings"
	"testing"
)

func TestInjectionIncludesCaptureScripts(t *testing.T) {
	out, injected := applyHTMLInjections([]byte("<html><head></head><body></body></html>"), 5000)
	if !injected {
		t.Fatal("agent bundle not injected")
	}
	page := string(out)
	last := -1
	for _, name := range []string{"snapdom.min.js", "agent-pin-marker.js", "agent-capture.js", "hunkyard-agent.js"} {
		at := strings.Index(page, "http://localhost:5000/"+name)
		if at < 0 || at < last {
			t.Fatalf("%s missing or out of order in %s", name, page)
		}
		last = at
	}
}
