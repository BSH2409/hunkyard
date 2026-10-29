package session

import (
	"testing"
)

func TestReconcilePendingRemoteDeletesDoesNotResurrectDrained(t *testing.T) {
	old := RemoteRef{Forge: "github", CommentID: 1}
	fresh := RemoteRef{Forge: "github", CommentID: 2}
	got := reconcilePendingRemoteDeletes(
		[]RemoteRef{old, fresh}, nil,
		map[RemoteRef]struct{}{old: {}},
	)
	if len(got) != 1 || got[0] != fresh {
		t.Fatalf("reconciled = %+v, want only fresh", got)
	}
}
