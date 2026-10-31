package feedback

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestOwnerIsolation(t *testing.T) {
	root := t.TempDir()
	first := Identity{Key: "codex-session-a", Kind: "codex"}
	second := Identity{Key: "claude-session-b", Kind: "claude"}
	a, err := OpenOwner(root, "012345abcdef", first)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Verify(a.Credential()); err != nil {
		t.Fatal(err)
	}
	if err := a.Verify("wrong"); err == nil {
		t.Fatal("wrong owner credential accepted")
	}
	if _, err := LoadOwner(root, "012345abcdef", second); err == nil {
		t.Fatal("another owner loaded credentials")
	}
	if _, err := OpenOwner(root, "012345abcdef", second); err == nil {
		t.Fatal("another owner replaced the review owner")
	}
	reconnected, err := LoadOwner(root, "012345abcdef", first)
	if err != nil {
		t.Fatal(err)
	}
	if reconnected.Credential() != a.Credential() {
		t.Fatal("reconnect rotated credentials")
	}
}

func TestOwnerCredentialPrivacy(t *testing.T) {
	root := t.TempDir()
	owner, err := OpenOwner(root, "012345abcdef", Identity{Key: "test-owner", Kind: "codex"})
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(owner)
	if err != nil {
		t.Fatal(err)
	}
	if len(owner.Credential()) < 40 {
		t.Fatal("owner credential is too short")
	}
	for _, public := range []string{string(encoded), fmt.Sprint(owner), fmt.Sprintf("%+v", owner), fmt.Sprintf("%#v", owner)} {
		if strings.Contains(public, owner.Credential()) {
			t.Fatal("owner credential leaked through public formatting")
		}
	}
	for name, want := range map[string]os.FileMode{"owners": 0700, "owners/012345abcdef.json": 0600} {
		info, err := os.Stat(filepath.Join(root, filepath.FromSlash(name)))
		if err != nil {
			t.Fatal(err)
		}
		// Windows has no permission bits; ACLs on the profile protect it.
		if runtime.GOOS != "windows" && info.Mode().Perm() != want {
			t.Fatalf("owner artifact permissions: %o, want %o", info.Mode().Perm(), want)
		}
	}
}

func TestOwnerPaths(t *testing.T) {
	for _, id := range []string{"../outside", "", "/tmp/outside", "bad-review"} {
		if _, err := OpenOwner(t.TempDir(), id, Identity{Key: "owner", Kind: "claude"}); err == nil {
			t.Fatal("invalid review identity accepted")
		}
	}
	root := t.TempDir()
	if err := os.Symlink(t.TempDir(), filepath.Join(root, "owners")); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenOwner(root, "012345abcdef", Identity{Key: "owner", Kind: "claude"}); err == nil {
		t.Fatal("symlinked credential directory accepted")
	}
}

func TestOwnerIdentityUsesHostSession(t *testing.T) {
	t.Setenv("HUNKYARD_OWNER_ID", "")
	t.Setenv("CODEX_THREAD_ID", "thread-one")
	t.Setenv("CODEX_SESSION_ID", "")
	t.Setenv("CLAUDE_SESSION_ID", "")
	first := CurrentIdentity()
	t.Setenv("CODEX_THREAD_ID", "thread-two")
	second := CurrentIdentity()
	if first.Kind != "codex" || first.Key == second.Key || strings.Contains(first.Key, "thread-one") {
		t.Fatal("host session identity is missing, shared, or stored raw")
	}
	t.Setenv("HUNKYARD_OWNER_ID", "claude:synthetic-session")
	claude := CurrentIdentity()
	if claude.Kind != "claude" || claude.Key == second.Key || strings.Contains(claude.Key, "synthetic-session") {
		t.Fatal("explicit Claude owner identity was not isolated")
	}
}

func TestOwnerDoesNotChangeParentPermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows has no permission bits to preserve")
	}
	parent := t.TempDir()
	if err := os.Chmod(parent, 0755); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenOwner(filepath.Join(parent, "state"), "012345abcdef", Identity{Key: "test-owner", Kind: "codex"}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(parent)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0755 {
		t.Fatal("creating private state changed unrelated parent permissions")
	}
}

func TestOwnerProofPinsReviewAndDaemonBeforeSendingCredentials(t *testing.T) {
	owner, err := OpenOwner(t.TempDir(), "012345abcdef", Identity{Key: "test-owner", Kind: "codex"})
	if err != nil {
		t.Fatal(err)
	}
	nonce := base64.RawURLEncoding.EncodeToString(make([]byte, 32))
	proof, err := owner.Proof(nonce, "012345abcdef", "generation-a")
	if err != nil {
		t.Fatal(err)
	}
	if proof == owner.Credential() {
		t.Fatal("challenge exposed the owner credential")
	}
	if err := owner.VerifyProof(nonce, "012345abcdef", "generation-a", proof); err != nil {
		t.Fatal(err)
	}
	for _, change := range []struct{ review, generation, proof string }{{"abcdef012345", "generation-a", proof}, {"012345abcdef", "generation-b", proof}, {"012345abcdef", "generation-a", "forged"}} {
		if err := owner.VerifyProof(nonce, change.review, change.generation, change.proof); err == nil {
			t.Fatal("owner proof replay or forgery accepted")
		}
	}
	if _, err := owner.Proof("invalid", "012345abcdef", "generation-a"); err == nil {
		t.Fatal("invalid challenge accepted")
	}
}

func TestOwnerZeroValueCannotAuthenticate(t *testing.T) {
	owner := &Owner{}
	if owner.Verify("") == nil {
		t.Fatal("empty credential authenticated")
	}
	nonce := base64.RawURLEncoding.EncodeToString(make([]byte, 32))
	if _, err := owner.Proof(nonce, "012345abcdef", "generation"); err == nil {
		t.Fatal("zero owner signed challenge")
	}
}
