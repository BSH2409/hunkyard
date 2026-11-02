package vcs

import (
	"reflect"
	"testing"
)

// With diff.renames=copies, git reports copies as two-path C records; the
// copy is a new file and the records after it keep their alignment.
func TestNameStatusParsersReadCopyRecords(t *testing.T) {
	want := []FileChange{{Path: "dir/copy.go", Status: "added"}, {Path: "dir/new.go", Status: "modified"}}
	z := []byte("C097\x00dir/new.go\x00dir/copy.go\x00M\x00dir/new.go\x00")
	if got := parseNameStatusZ(z); !reflect.DeepEqual(got, want) {
		t.Errorf("parseNameStatusZ = %+v, want %+v", got, want)
	}
	text := "C097\tdir/new.go\tdir/copy.go\nM\tdir/new.go\n"
	if got := parseNameStatus(text); !reflect.DeepEqual(got, want) {
		t.Errorf("parseNameStatus = %+v, want %+v", got, want)
	}
}
