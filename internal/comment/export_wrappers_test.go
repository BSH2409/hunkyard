package comment

import (
	"testing"

	"github.com/BSH2409/hunkyard/internal/review"
	"github.com/BSH2409/hunkyard/internal/session"
)

func TestParseLineSpecExport(t *testing.T) {
	start, end, err := ParseLineSpec("10-20")
	if err != nil || start != 10 || end != 20 {
		t.Fatalf("ParseLineSpec = %d-%d, %v", start, end, err)
	}
}

func TestAppendCommentScoped_ReviewLevel(t *testing.T) {
	cj := &session.HunkyardJSON{Files: map[string]session.HunkyardJSONFile{}}
	AppendReviewCommentScoped(cj, "review note", "author", session.InheritedScope{})
	if len(cj.ReviewComments) != 1 {
		t.Fatalf("expected review comment, got %+v", cj.ReviewComments)
	}
}

func TestAppendFileCommentScoped(t *testing.T) {
	cj := &session.HunkyardJSON{Files: map[string]session.HunkyardJSONFile{}}
	AppendFileCommentScoped(cj, "a.go", "file note", "author", session.InheritedScope{})
	if len(cj.Files["a.go"].Comments) != 1 {
		t.Fatalf("expected file comment, got %+v", cj.Files)
	}
}

func TestProcessBulkReviewEntry_Scoped(t *testing.T) {
	cj := &session.HunkyardJSON{Files: map[string]session.HunkyardJSONFile{}}
	entry := BulkCommentEntry{Body: "overall", Scope: "review"}
	if err := ProcessBulkReviewEntry(cj, 0, entry, "bot", session.InheritedScope{}); err != nil {
		t.Fatal(err)
	}
	if len(cj.ReviewComments) != 1 {
		t.Fatalf("expected review comment, got %+v", cj.ReviewComments)
	}
}

func TestExportWrappers_PersistComments(t *testing.T) {
	dir := t.TempDir()
	scope := session.InheritedScope{DiffScope: "layer"}
	if err := AddReviewCommentToHunkyardJSONScoped("review body", "bot", dir, scope); err != nil {
		t.Fatal(err)
	}
	if err := AddFileCommentToHunkyardJSONScoped("a.go", "file body", "bot", dir, scope); err != nil {
		t.Fatal(err)
	}
	if err := AddCommentToHunkyardJSONScoped("a.go", 5, 5, "line body", "bot", dir, scope); err != nil {
		t.Fatal(err)
	}
	cj := &session.HunkyardJSON{Files: map[string]session.HunkyardJSONFile{
		"a.go": {Comments: []session.Comment{{ID: "c1", Body: "x"}}},
	}}
	AppendCommentScoped(cj, "b.go", 1, 1, "scoped", "bot", scope)
	if err := BulkAddCommentsToHunkyardJSONScoped([]BulkCommentEntry{{Body: "bulk", Scope: "review"}}, "bot", dir, scope); err != nil {
		t.Fatal(err)
	}
	hunkyardPath, err := review.ResolveReviewPath(dir)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := loadHunkyardJSON(hunkyardPath)
	if err != nil {
		t.Fatal(err)
	}
	var commentID string
	for _, cf := range loaded.Files {
		if len(cf.Comments) > 0 {
			commentID = cf.Comments[0].ID
			break
		}
	}
	if commentID == "" {
		t.Fatal("expected a saved comment to reply to")
	}
	if err := AddReplyToHunkyardJSON(commentID, "reply", "bot", false, dir, ""); err != nil {
		t.Fatal(err)
	}
	if err := CheckCommentCLIAllowed(hunkyardPath, formLineComment); err != nil {
		t.Fatal(err)
	}
}
