package feedback

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"github.com/BSH2409/hunkyard/internal/strictjson"
)

const OwnerHeader = "X-Hunkyard-Owner"

var ErrUnauthorized = errors.New("review owner verification failed")
var reviewIDPattern = regexp.MustCompile(`^[0-9a-f]{12}$`)

type Identity struct{ Key, Kind string }

type Owner struct {
	Key        string `json:"key"`
	Kind       string `json:"kind"`
	credential string
}

type ownerRecord struct {
	Version    int    `json:"version"`
	Key        string `json:"key"`
	Kind       string `json:"kind"`
	Credential string `json:"credential"`
}

func CurrentIdentity() Identity {
	seed := os.Getenv("HUNKYARD_OWNER_ID")
	if seed == "" {
		if id := os.Getenv("CODEX_THREAD_ID"); id != "" {
			seed = "codex:" + id
		} else if id := os.Getenv("CODEX_SESSION_ID"); id != "" {
			seed = "codex:" + id
		} else if id := os.Getenv("CLAUDE_SESSION_ID"); id != "" {
			seed = "claude:" + id
		}
	}
	return IdentityForSeed(seed)
}

// IdentityForSeed derives the owner identity for a host seed such as
// "claude:SESSION" or "codex:THREAD", so hooks can find the reviews their own
// session launched.
func IdentityForSeed(seed string) Identity {
	if seed == "" {
		return Identity{Key: "standalone", Kind: "terminal"}
	}
	kind := "cli"
	if strings.HasPrefix(seed, "codex:") {
		kind = "codex"
	} else if strings.HasPrefix(seed, "claude:") {
		kind = "claude"
	}
	hash := sha256.Sum256([]byte(seed))
	return Identity{Key: "owner_" + hex.EncodeToString(hash[:]), Kind: kind}
}

func OpenOwner(stateRoot, reviewID string, identity Identity) (*Owner, error) {
	selectedRoot, err := filepath.Abs(stateRoot)
	if err != nil || strings.TrimSpace(stateRoot) == "" || filepath.Dir(selectedRoot) == selectedRoot {
		return nil, fmt.Errorf("invalid private state root")
	}
	stateRoot = selectedRoot
	if !reviewIDPattern.MatchString(reviewID) || identity.Key == "" || identity.Kind == "" {
		return nil, fmt.Errorf("invalid review owner identity")
	}
	if err := privateDirectory(stateRoot); err != nil {
		return nil, err
	}
	root := filepath.Join(stateRoot, "owners")
	if err := privateDirectory(root); err != nil {
		return nil, err
	}
	var owner *Owner
	err = withLock(filepath.Join(root, reviewID+".lock"), func() error {
		existing, err := LoadOwner(stateRoot, reviewID, identity)
		if err == nil {
			owner = existing
			return nil
		}
		if !os.IsNotExist(err) {
			return err
		}
		var secret [32]byte
		if _, err := rand.Read(secret[:]); err != nil {
			return err
		}
		token := base64.RawURLEncoding.EncodeToString(secret[:])
		record := ownerRecord{Version: 1, Key: identity.Key, Kind: identity.Kind, Credential: token}
		encoded, err := json.Marshal(record)
		if err != nil {
			return err
		}
		if err := atomicFile(filepath.Join(root, reviewID+".json"), encoded); err != nil {
			return err
		}
		owner = &Owner{Key: identity.Key, Kind: identity.Kind, credential: token}
		return nil
	})
	return owner, err
}

//nolint:gocyclo // Credential loading checks private-file and session identity invariants together.
func LoadOwner(stateRoot, reviewID string, identity Identity) (*Owner, error) {
	if !reviewIDPattern.MatchString(reviewID) {
		return nil, fmt.Errorf("invalid review identity")
	}
	for _, dir := range []string{stateRoot, filepath.Join(stateRoot, "owners")} {
		info, err := os.Lstat(dir)
		if err != nil {
			return nil, err
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("invalid private owner directory")
		}
	}
	path := filepath.Join(stateRoot, "owners", reviewID+".json")
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > 16*1024 || !privateRecordMode(info.Mode(), runtime.GOOS) {
		return nil, fmt.Errorf("invalid private owner record")
	}
	encoded, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var record ownerRecord
	if err := strictjson.Decode(encoded, &record); err != nil {
		return nil, fmt.Errorf("invalid review owner record")
	}
	token, err := base64.RawURLEncoding.DecodeString(record.Credential)
	if err != nil || len(token) != 32 || record.Version != 1 || record.Key == "" || record.Kind == "" {
		return nil, fmt.Errorf("invalid review owner record")
	}
	if record.Key != identity.Key || record.Kind != identity.Kind {
		return nil, ErrUnauthorized
	}
	return &Owner{Key: record.Key, Kind: record.Kind, credential: record.Credential}, nil
}

func (o *Owner) Credential() string { return o.credential }
func (o *Owner) Verify(credential string) error {
	if o == nil || len(o.credential) != 43 || subtle.ConstantTimeCompare([]byte(credential), []byte(o.credential)) != 1 {
		return ErrUnauthorized
	}
	return nil
}
func (o *Owner) String() string   { return o.Kind + " owner " + o.Key }
func (o *Owner) GoString() string { return o.String() }

func (o *Owner) Proof(nonce, reviewID, generation string) (string, error) {
	bytes, err := base64.RawURLEncoding.DecodeString(nonce)
	if o == nil || len(o.credential) != 43 || err != nil || len(bytes) != 32 || !reviewIDPattern.MatchString(reviewID) || generation == "" || strings.ContainsRune(generation, '\x00') {
		return "", fmt.Errorf("invalid owner challenge")
	}
	mac := hmac.New(sha256.New, []byte(o.credential))
	mac.Write([]byte("hunkyard-owner-v1\x00" + nonce + "\x00" + reviewID + "\x00" + generation))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil)), nil
}
func (o *Owner) VerifyProof(nonce, reviewID, generation, proof string) error {
	expected, err := o.Proof(nonce, reviewID, generation)
	if err != nil || subtle.ConstantTimeCompare([]byte(expected), []byte(proof)) != 1 {
		return ErrUnauthorized
	}
	return nil
}

// privateRecordMode reports whether a credential file is private to its
// owner. Windows reports 0666 for every file and protects the user profile
// with ACLs instead, so permission bits are only checked elsewhere.
func privateRecordMode(mode os.FileMode, goos string) bool {
	return goos == "windows" || mode.Perm()&0o077 == 0
}
