package preferences

import (
	"strings"
	"testing"
)

func TestPrivacyReferenceParity(t *testing.T) {
	var fixtures struct {
		Cases []struct {
			Name, Input     string
			Accepted        bool
			Excerpt, Reason *string
			Fingerprint     string
		}
	}
	readContract(t, "privacy.json", &fixtures)
	for _, c := range fixtures.Cases {
		t.Run(c.Name, func(t *testing.T) {
			got := sanitize(c.Input)
			if got.Accepted != c.Accepted || got.Fingerprint != c.Fingerprint || got.Excerpt != value(c.Excerpt) || got.Reason != value(c.Reason) {
				t.Fatal("privacy result differs from reference (input withheld)")
			}
		})
	}
}

func TestPrivacyInvalidUTF8(t *testing.T) {
	got := sanitize("prefer " + string([]byte{0xff}))
	if !got.Accepted || got.Excerpt != "prefer �" {
		t.Fatal("invalid UTF-8 not normalized safely")
	}
	if got := sanitize("password=synthetic-test-value"); got.Accepted || got.Reason != "sensitive_content" || got.Excerpt != "" {
		t.Fatal("credential assignment accepted")
	}
	if got := sanitize("prefer " + strings.Repeat("é", 1200)); len([]rune(got.Excerpt)) != 1000 {
		t.Fatal("excerpt length differs")
	}
}

func value(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
