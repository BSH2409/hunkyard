//go:build !windows

package session

import (
	"os"
	"syscall"
	"testing"
	"time"
)

func TestExternalMergePreservesCommentAddedDuringDiskRead(t *testing.T) {
	s := NewTestSession(t)
	s.AddReviewComment("Existing comment", "Reviewer")
	if err := s.SyncWriteFiles(); err != nil {
		t.Fatal(err)
	}
	path := ReviewPathsFor(s.hunkyardJSONPath()).Review
	previous, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(path, 0600); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.Remove(path)
		_ = os.WriteFile(path, previous, 0600)
	})
	s.mu.Lock()
	s.lastHunkyardJSONMtime = time.Time{}
	s.mu.Unlock()
	merged := make(chan bool, 1)
	go func() { merged <- s.mergeExternalHunkyardJSON() }()
	writers := make(chan *os.File, 1)
	go func() {
		writer, _ := os.OpenFile(path, os.O_WRONLY, 0600)
		writers <- writer
	}()
	var writer *os.File
	select {
	case writer = <-writers:
		if writer == nil {
			t.Fatal("could not open the controlled review stream")
		}
	case <-time.After(5 * time.Second):
		if unblock, err := os.OpenFile(path, os.O_RDWR, 0600); err == nil {
			unblock.Close()
		}
		t.Fatal("external merge did not start reading")
	}
	defer writer.Close()
	added := s.AddReviewComment("Added while an older disk read was in flight", "Reviewer")
	s.mu.Lock()
	s.writeTimer.Stop()
	s.writeGen++
	s.mu.Unlock()
	if _, err := writer.Write(previous); err != nil {
		t.Fatal(err)
	}
	writer.Close()
	select {
	case <-merged:
	case <-time.After(5 * time.Second):
		t.Fatal("external merge did not finish")
	}
	comments := s.GetReviewComments()
	if len(comments) != 2 || comments[1].ID != added.ID {
		t.Fatal("the stale disk read discarded a new review comment")
	}
}
