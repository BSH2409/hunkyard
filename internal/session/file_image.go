package session

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// MaxImageDiffBytes caps one side of an image diff; larger images fall back
// to the binary metadata card.
const MaxImageDiffBytes = 10 << 20

var (
	ErrNotImage         = errors.New("not an image")
	ErrImageTooLarge    = errors.New("image too large")
	ErrImageFileUnknown = errors.New("file is not part of the review")
	ErrImageSide        = errors.New("side must be base or head")
)

// isImageExtension reports whether the review shows ext as an image diff.
func isImageExtension(ext string) bool {
	switch strings.ToLower(ext) {
	case ".png", ".jpg", ".jpeg", ".gif", ".webp":
		return true
	}
	return false
}

// ImageBytes returns one side of a changed image: "head" is the reviewed
// version (working tree, or the range head), "base" the version it is
// compared with. A side the file does not have (an added file's base, a
// deleted file's head) is os.ErrNotExist.
func (s *Session) ImageBytes(path, side string) ([]byte, string, error) {
	if side != "base" && side != "head" {
		return nil, "", ErrImageSide
	}
	if !isImageExtension(filepath.Ext(path)) {
		return nil, "", ErrNotImage
	}
	s.mu.RLock()
	f := s.fileByPathLocked(path)
	s.mu.RUnlock()
	if f == nil {
		return nil, "", ErrImageFileUnknown
	}
	data, err := s.imageSide(f, side)
	if err != nil {
		return nil, "", err
	}
	if len(data) > MaxImageDiffBytes {
		return nil, "", ErrImageTooLarge
	}
	mime := http.DetectContentType(data)
	if !strings.HasPrefix(mime, "image/") {
		return nil, "", ErrNotImage
	}
	return data, mime, nil
}

func (s *Session) imageSide(f *FileEntry, side string) ([]byte, error) {
	if err := s.ensureFileLoaded(f); err != nil {
		return nil, err
	}
	s.mu.RLock()
	status, oldPath, content := f.Status, f.OldPath, f.Content
	focus, baseRef, vc, root := s.Focus, s.BaseRef, s.VCS, s.RepoRoot
	s.mu.RUnlock()
	if side == "head" {
		if status == "deleted" {
			return nil, os.ErrNotExist
		}
		return []byte(content), nil
	}
	if status == "added" || status == "untracked" {
		return nil, os.ErrNotExist
	}
	if vc == nil {
		return nil, os.ErrNotExist
	}
	basePath := f.Path
	if oldPath != "" {
		basePath = oldPath
	}
	if focus.Kind == FocusRange {
		data, err := s.readFileAtSHAForFocus(focus, focus.DiffBaseSHA(), basePath)
		if err != nil {
			return nil, err
		}
		if data == nil {
			return nil, os.ErrNotExist
		}
		return data, nil
	}
	if baseRef == "" {
		baseRef = "HEAD"
	}
	text, err := vc.FileContentAtRef(basePath, baseRef, root)
	if err != nil {
		return nil, fmt.Errorf("reading %s at %s: %w", basePath, baseRef, err)
	}
	if text == "" {
		return nil, os.ErrNotExist
	}
	return []byte(text), nil
}
