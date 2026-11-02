package session

import (
	"os"
	"path/filepath"
	"testing"
)

// A crash between moving review.json into the temp folder and renaming that
// folder into place leaves the review only in the temp folder.
func TestOpeningAReviewRecoversAnInterruptedMigration(t *testing.T) {
	identity := filepath.Join(t.TempDir(), "abcdefabcdef")
	tmp := identity + ".hunkyard-migrate.tmp"
	if err := os.MkdirAll(tmp, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmp, "review.json"), []byte(`{"branch":"main","files":{}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := readHunkyardJSONFromDisk(identity); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(identity, "review.json"))
	if err != nil || string(data) != `{"branch":"main","files":{}}` {
		t.Fatalf("review not recovered: %q %v", data, err)
	}
}

func TestMigrationNeverWipesATempFolderHoldingTheReview(t *testing.T) {
	dir := t.TempDir()
	identity := filepath.Join(dir, "abcdefabcdef")
	tmp := identity + ".hunkyard-migrate.tmp"
	if err := os.MkdirAll(tmp, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmp, "review.json"), []byte(`{"only":"copy"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(identity+".json", []byte(`{"other":"copy"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	_ = ensureReviewFolder(identity)
	for _, p := range []string{filepath.Join(tmp, "review.json"), filepath.Join(identity, "review.json")} {
		if data, err := os.ReadFile(p); err == nil && string(data) == `{"only":"copy"}` {
			return
		}
	}
	t.Fatal("the only copy of the interrupted review was deleted")
}

func TestResumingAReviewRecoversAnInterruptedMigration(t *testing.T) {
	revDir := filepath.Join(t.TempDir(), "abcdefabcdef")
	tmp := revDir + migrateTmpSuffix
	if err := os.MkdirAll(tmp, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmp, "review.json"), []byte(`{"branch":"feature","files":{}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	cj, err := readReconnectReview("abcdefabcdef", revDir)
	if err != nil || cj.Branch != "feature" {
		t.Fatalf("resume did not recover the review: %+v %v", cj, err)
	}
}
