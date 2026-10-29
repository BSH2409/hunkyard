package preferences

import (
	"fmt"
	"github.com/BSH2409/hunkyard/internal/strictjson"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

//nolint:gocyclo // The backup validates its complete file manifest before authorizing replacement.
func (s *Store) backupLegacy(plan migrationPlan) (string, error) {
	if err := ensureDirectory(s.dataPath("backups")); err != nil {
		return "", err
	}
	name := s.config.Clock().UTC().Format("20060102T150405Z") + "-legacy-v1"
	target := s.dataPath("backups", name)
	if _, err := os.Lstat(target); err == nil {
		if _, err := verifyLegacyBackup(target, plan); err != nil {
			return "", err
		}
		return target, nil
	} else if !os.IsNotExist(err) {
		return "", err
	}
	stage, err := os.MkdirTemp(s.dataPath("backups"), ".legacy-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(stage)
	files := map[string]any{}
	for _, source := range []struct{ label, root string }{{"global", plan.global.root}, {"project", plan.project.root}} {
		if err := checkDirectoryChain(source.root, source.root); err != nil {
			return "", err
		}
		copyFile := func(name string) error {
			if err := checkDirectoryChain(source.root, filepath.Dir(name)); err != nil {
				return err
			}
			data, err := regularFile(name)
			if err != nil {
				return err
			}
			info, err := os.Lstat(name)
			if err != nil {
				return err
			}
			relative, err := filepath.Rel(source.root, name)
			if err != nil {
				return err
			}
			label := source.label + "/" + filepath.ToSlash(relative)
			dest := filepath.Join(stage, filepath.FromSlash(label))
			if err := ensureDirectory(filepath.Dir(dest)); err != nil {
				return err
			}
			if err := writeNewFile(dest, data); err != nil {
				return err
			}
			files[label] = map[string]any{"digest": digest(string(data)), "size": len(data), "mode": int(info.Mode().Perm())}
			return nil
		}
		if err := copyFile(filepath.Join(source.root, "lessons", "lessons.md")); err != nil {
			return "", err
		}
		for _, dir := range []string{"persona", "reports"} {
			base := filepath.Join(source.root, dir)
			if _, err := os.Lstat(base); os.IsNotExist(err) {
				continue
			} else if err != nil {
				return "", err
			}
			err := filepath.WalkDir(base, func(name string, entry fs.DirEntry, err error) error {
				if err != nil {
					return err
				}
				if entry.Type()&os.ModeSymlink != 0 {
					return fmt.Errorf("legacy backup refuses symlinks")
				}
				if entry.IsDir() {
					return nil
				}
				if !entry.Type().IsRegular() {
					return fmt.Errorf("legacy backup requires regular files")
				}
				return copyFile(name)
			})
			if err != nil {
				return "", err
			}
		}
	}
	manifest := map[string]any{"schema_version": schemaVersion, "files": files}
	encoded, err := canonicalJSON(manifest)
	if err != nil {
		return "", err
	}
	if err := writeNewFile(filepath.Join(stage, "manifest.json"), append(encoded, '\n')); err != nil {
		return "", err
	}
	if _, err := verifyLegacyBackup(stage, plan); err != nil {
		return "", err
	}
	dirs := []string{}
	if err := filepath.WalkDir(stage, func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			dirs = append(dirs, name)
		}
		return nil
	}); err != nil {
		return "", err
	}
	sort.Slice(dirs, func(i, j int) bool { return len(dirs[i]) > len(dirs[j]) })
	for _, dir := range dirs {
		if err := syncDirectory(dir); err != nil {
			return "", err
		}
	}
	if err := os.Rename(stage, target); err != nil {
		return "", err
	}
	if err := syncDirectory(s.dataPath("backups")); err != nil {
		return "", err
	}
	return target, nil
}

//nolint:gocyclo // The backup validates its complete file manifest before authorizing replacement.
func verifyLegacyBackup(root string, plan migrationPlan) (map[string]string, error) {
	if err := checkDirectoryChain(root, root); err != nil {
		return nil, err
	}
	data, err := regularFile(filepath.Join(root, "manifest.json"))
	if err != nil {
		return nil, err
	}
	var document map[string]any
	if err := strictjson.Decode(data, &document); err != nil {
		return nil, err
	}
	if err := exactFields(document, []string{"schema_version", "files"}, nil); err != nil {
		return nil, err
	}
	version, ok := integer(document["schema_version"])
	files := record(document["files"])
	if !ok || version != schemaVersion || files == nil {
		return nil, fmt.Errorf("invalid legacy backup manifest")
	}
	approved := map[string]string{}
	for label, value := range files {
		if !safeRelative(label) || (!strings.HasPrefix(label, "global/") && !strings.HasPrefix(label, "project/")) {
			return nil, fmt.Errorf("invalid legacy backup path")
		}
		metadata := record(value)
		if err := exactFields(metadata, []string{"digest", "mode", "size"}, nil); err != nil {
			return nil, err
		}
		size, sizeOK := integer(metadata["size"])
		mode, modeOK := integer(metadata["mode"])
		if !sizeOK || size < 0 || !modeOK || mode < 0 || mode > 0777 || !sha256Pattern.MatchString(textField(metadata["digest"])) {
			return nil, fmt.Errorf("invalid backup metadata")
		}
		name := filepath.Join(root, filepath.FromSlash(label))
		if err := checkDirectoryChain(root, filepath.Dir(name)); err != nil {
			return nil, err
		}
		data, err := regularFile(name)
		if err != nil {
			return nil, err
		}
		if int64(len(data)) != int64(size) || digest(string(data)) != metadata["digest"] {
			return nil, fmt.Errorf("legacy backup verification failed")
		}
		if strings.HasPrefix(label, "global/persona/") {
			relative := strings.TrimPrefix(label, "global/persona/")
			for _, path := range plan.personaPaths {
				if relative == path {
					approved[relative] = string(data)
				}
			}
		}
	}
	for _, entry := range []struct{ label, digest string }{{"global/lessons/lessons.md", plan.global.digest}, {"project/lessons/lessons.md", plan.project.digest}} {
		if record(files[entry.label])["digest"] != entry.digest {
			return nil, fmt.Errorf("legacy lessons changed before backup")
		}
	}
	return approved, nil
}
