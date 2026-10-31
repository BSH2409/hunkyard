package session

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"

	"github.com/BSH2409/hunkyard/internal/vcs"
)

type ReviewSnapshotFile struct {
	Path        string         `json:"path"`
	OldPath     string         `json:"old_path,omitempty"`
	Status      string         `json:"status"`
	Content     []byte         `json:"content"`
	BaseContent []byte         `json:"base_content"`
	ContentID   string         `json:"content_id"`
	DiffHunks   []vcs.DiffHunk `json:"diff_hunks"`
}

type ReviewSnapshot struct {
	ID            string               `json:"id,omitempty"`
	Round         int                  `json:"round"`
	Focus         Focus                `json:"focus"`
	BaseRef       string               `json:"base_ref"`
	Files         []ReviewSnapshotFile `json:"files"`
	RawChanges    []byte               `json:"raw_changes,omitempty"`
	StagedChanges []byte               `json:"staged_changes,omitempty"`
}

type CommentContext struct{ ContentID, ViewID string }

func (s *Session) viewIDLocked() string {
	return (ReviewSnapshot{Focus: s.Focus, Round: s.ReviewRound, BaseRef: s.BaseRef}).ViewID()
}

// ViewID binds a browser's displayed files to the snapshot's comparison and round.
func (snapshot ReviewSnapshot) ViewID() string {
	encoded, _ := json.Marshal(struct {
		Focus   Focus
		Round   int
		BaseRef string
	}{snapshot.Focus, snapshot.Round, snapshot.BaseRef})
	return "view_" + computeFileHash(encoded)
}

func (s *Session) matchesCommentContextLocked(file *FileEntry, contexts []CommentContext) bool {
	if len(contexts) == 0 {
		return true
	}
	context := contexts[0]
	return (context.ContentID == "" || context.ContentID == commentContentID(file)) && (context.ViewID == "" || context.ViewID == s.viewIDLocked())
}

func commentContentID(file *FileEntry) string {
	if file.FileHash != "" {
		return file.FileHash
	}
	return fileHash([]byte(file.Content))
}

func (s *Session) snapshotViewLocked() ReviewSnapshot {
	files := make([]ReviewSnapshotFile, 0, len(s.Files))
	for _, file := range s.Files {
		if file == nil || file.Orphaned {
			continue
		}
		hunks := make([]vcs.DiffHunk, len(file.DiffHunks))
		for i, hunk := range file.DiffHunks {
			hunks[i] = hunk
			hunks[i].Lines = append([]vcs.DiffLine{}, hunk.Lines...)
		}
		files = append(files, ReviewSnapshotFile{Path: file.Path, OldPath: file.OldPath, Status: file.Status, Content: []byte(file.Content), DiffHunks: hunks})
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return ReviewSnapshot{Round: s.ReviewRound, Focus: s.Focus, BaseRef: s.BaseRef, Files: files}
}

// CaptureReviewSnapshot freezes the displayed bytes and base content without writing Git objects.
//
//nolint:gocyclo // Snapshot validation binds the complete displayed bytes and Git identities.
func (s *Session) CaptureReviewSnapshot() (ReviewSnapshot, error) {
	var changed []string
	for attempt := 0; attempt < 3; attempt++ {
		s.mu.RLock()
		files := append([]*FileEntry{}, s.Files...)
		s.mu.RUnlock()
		for _, file := range files {
			if err := s.ensureFileLoaded(file); err != nil {
				return ReviewSnapshot{}, err
			}
		}
		s.mu.RLock()
		snapshot := s.snapshotViewLocked()
		vc, root := s.VCS, s.RepoRoot
		s.mu.RUnlock()
		baseline := snapshot
		baseline.Files = append([]ReviewSnapshotFile{}, snapshot.Files...)
		raw, staged, err := snapshotGitMetadata(vc, root, snapshot)
		if err != nil {
			return ReviewSnapshot{}, err
		}
		for i, file := range snapshot.Files {
			sourcePath := file.Path
			if file.OldPath != "" {
				sourcePath = file.OldPath
			}
			if vc != nil {
				var content string
				var err error
				if snapshot.Focus.Kind == FocusRange {
					var bytes []byte
					bytes, err = vc.ReadFileAtSHA(snapshot.Focus.DiffBaseSHA(), sourcePath, root)
					content = string(bytes)
				} else if snapshot.BaseRef != "" {
					content, err = vc.FileContentAtRef(sourcePath, snapshot.BaseRef, root)
				}
				if err != nil {
					return ReviewSnapshot{}, fmt.Errorf("snapshot base content for %s: %w", sourcePath, err)
				}
				snapshot.Files[i].BaseContent = []byte(content)
			}
		}
		s.mu.RLock()
		current := s.snapshotViewLocked()
		s.mu.RUnlock()
		latestRaw, latestStaged, err := snapshotGitMetadata(vc, root, current)
		if err != nil {
			return ReviewSnapshot{}, err
		}
		if !reflect.DeepEqual(current, baseline) || !reflect.DeepEqual(raw, latestRaw) || !reflect.DeepEqual(staged, latestStaged) {
			changed = []string{}
			for i, file := range current.Files {
				if i >= len(baseline.Files) || !reflect.DeepEqual(file, baseline.Files[i]) {
					changed = append(changed, file.Path)
				}
			}
			continue
		}
		snapshot.RawChanges = raw
		snapshot.StagedChanges = staged
		for i, file := range snapshot.Files {
			snapshot.Files[i].ContentID = fileHash(file.Content)
		}
		encoded, err := json.Marshal(snapshot)
		if err != nil {
			return ReviewSnapshot{}, err
		}
		snapshot.ID = "snapshot_" + computeFileHash(encoded)
		return snapshot, nil
	}
	return ReviewSnapshot{}, fmt.Errorf("review changed during three snapshot attempts; reload before publishing (%s)", strings.Join(changed, ", "))
}

func snapshotGitMetadata(vc vcs.VCS, root string, snapshot ReviewSnapshot) ([]byte, []byte, error) {
	if root == "" || vc == nil || vc.Name() != "git" || len(snapshot.Files) == 0 {
		return nil, nil, nil
	}
	if snapshot.Focus.Kind != FocusRange {
		paths := []string{}
		for _, file := range snapshot.Files {
			paths = append(paths, file.Path)
		}
		if err := vcs.CheckWorkingTreeFilters(root, paths...); err != nil {
			return nil, nil, err
		}
	}
	args := []string{"diff", "--raw", "-z", "--no-abbrev", "--no-ext-diff", "--no-textconv"}
	if snapshot.Focus.Kind == FocusRange {
		args = append(args, snapshot.Focus.DiffBaseSHA()+".."+snapshot.Focus.HeadSHA)
	} else if snapshot.BaseRef != "" {
		args = append(args, snapshot.BaseRef)
	}
	args = append(args, "--")
	for _, file := range snapshot.Files {
		args = append(args, file.Path)
		if file.OldPath != "" {
			args = append(args, file.OldPath)
		}
	}
	raw, err := vcs.RunGitInDir(root, args...)
	if err != nil {
		return nil, nil, err
	}
	if snapshot.Focus.Kind == FocusRange {
		return []byte(raw), nil, nil
	}
	stagedArgs := append([]string{"diff", "--cached"}, args[1:]...)
	staged, err := vcs.RunGitInDir(root, stagedArgs...)
	return []byte(raw), []byte(staged), err
}

//nolint:gocyclo // Snapshot validation binds the complete displayed bytes and Git identities.
func SaveReviewSnapshot(reviewDir string, snapshot ReviewSnapshot) error {
	id := snapshot.ID
	snapshot.ID = ""
	material, err := json.Marshal(snapshot)
	if err != nil {
		return err
	}
	if id != "snapshot_"+computeFileHash(material) {
		return fmt.Errorf("invalid review snapshot identity")
	}
	snapshot.ID = id
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		return err
	}
	dir := filepath.Join(reviewDir, "review-snapshots")
	for _, parent := range []string{reviewDir, dir} {
		info, err := os.Lstat(parent)
		if os.IsNotExist(err) && parent == dir {
			if err := os.Mkdir(dir, 0700); err != nil {
				return err
			}
			info, err = os.Lstat(dir)
		}
		if err != nil {
			return err
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("snapshot directory must be a regular directory")
		}
	}
	target := filepath.Join(dir, id+".json")
	if info, err := os.Lstat(target); err == nil {
		if !info.Mode().IsRegular() {
			return fmt.Errorf("snapshot must be a regular file")
		}
		existing, err := os.ReadFile(target)
		if err != nil {
			return err
		}
		if !reflect.DeepEqual(existing, encoded) {
			return fmt.Errorf("immutable review snapshot differs")
		}
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	return AtomicWriteFile(target, encoded, 0600)
}

// LoadReviewSnapshot reads a published snapshot saved by SaveReviewSnapshot.
func LoadReviewSnapshot(reviewDir, id string) (ReviewSnapshot, error) {
	var snapshot ReviewSnapshot
	if !reviewSnapshotIDPattern.MatchString(id) {
		return snapshot, fmt.Errorf("invalid review snapshot identity")
	}
	data, err := os.ReadFile(filepath.Join(reviewDir, "review-snapshots", id+".json"))
	if err != nil {
		return snapshot, err
	}
	err = json.Unmarshal(data, &snapshot)
	return snapshot, err
}

var reviewSnapshotIDPattern = regexp.MustCompile(`^snapshot_[0-9a-f]{64}$`)
