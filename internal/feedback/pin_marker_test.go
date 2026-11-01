package feedback

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

// The agent is told how the box is drawn; that must be how the page draws it.
func TestPinMarkerMatchesTheFrameMarker(t *testing.T) {
	src, err := os.ReadFile("../../web/agent-pin-marker.js")
	if err != nil {
		t.Fatal(err)
	}
	m := PinMarker
	want := fmt.Sprintf("Object.freeze({ color: '%s', stroke: %d, halo: '%s', halo_width: %d, padding: %d, label: '%s' })", m.Color, m.Stroke, m.Halo, m.HaloWidth, m.Padding, m.Label)
	if !strings.Contains(string(src), "var MARKER_STYLE = "+want+";") {
		t.Errorf("web/agent-pin-marker.js MARKER_STYLE differs from feedback.PinMarker %s", want)
	}
}
