package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/BSH2409/hunkyard/internal/session"
	"github.com/BSH2409/hunkyard/internal/testutil"
)

func TestInboxCLI(t *testing.T) {
	home := t.TempDir()
	testutil.SetHome(t, home)
	var out bytes.Buffer
	if err := inboxCommand(context.Background(), []string{"--json"}, &out); err != nil || strings.TrimSpace(out.String()) != "[]" {
		t.Fatalf("empty inbox = %q %v", out.String(), err)
	}
	out.Reset()
	if err := inboxCommand(context.Background(), nil, &out); err != nil || !strings.Contains(out.String(), "No reviews") {
		t.Fatalf("empty text = %q %v", out.String(), err)
	}
	folder := filepath.Join(home, ".hunkyard", "reviews", "cccccccccccc")
	if err := os.MkdirAll(folder, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(folder, "review.json"), []byte(`{"cwd":"/work/blogvault","branch":"fix/x","review_round":2,"files":{}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := session.WritePausedMarker(folder, session.PausedMarker{PausedAt: time.Now(), Port: 5050}); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if err := inboxCommand(context.Background(), nil, &out); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"paused", "fix/x", "blogvault", "round 2", "hunkyard --session cccccccccccc"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("inbox text lacks %q:\n%s", want, out.String())
		}
	}
}
