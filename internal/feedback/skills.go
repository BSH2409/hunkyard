package feedback

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"

	"github.com/BSH2409/hunkyard/internal/strictjson"
)

type SkillRef struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Path        string `json:"path"`
	Source      string `json:"source"`
	Fingerprint string `json:"fingerprint,omitempty"`
}

type skillManifest struct {
	Version int        `json:"version"`
	Skills  []SkillRef `json:"skills"`
}

var skillIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$`)

func readManifest(path string) ([]SkillRef, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > 1<<20 {
		return nil, fmt.Errorf("invalid skill manifest")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var manifest skillManifest
	if strictjson.Decode(data, &manifest) != nil || manifest.Version != 1 {
		return nil, fmt.Errorf("invalid skill manifest")
	}
	return manifest.Skills, nil
}

func LoadSkillManifest(path string) ([]SkillRef, error) {
	refs, err := readManifest(path)
	if err != nil {
		return nil, err
	}
	return PinSkills(refs)
}

//nolint:gocyclo // Each trusted reference must satisfy the complete path and provenance contract.
func PinSkills(refs []SkillRef) ([]SkillRef, error) {
	if len(refs) > 100 {
		return nil, fmt.Errorf("too many registered skills")
	}
	pinned := make([]SkillRef, 0, len(refs))
	seen := map[string]bool{}
	for _, ref := range refs {
		if !skillIDPattern.MatchString(ref.ID) || seen[ref.ID] || ref.Name == "" || len(ref.Name) > 128 || len(ref.Description) > 2048 || ref.Source == "" || len(ref.Source) > 256 || !filepath.IsAbs(ref.Path) {
			return nil, fmt.Errorf("invalid registered skill")
		}
		path, err := filepath.EvalSymlinks(ref.Path)
		if err != nil {
			return nil, fmt.Errorf("registered skill is unavailable")
		}
		info, err := os.Stat(path)
		if err != nil || !info.Mode().IsRegular() || info.Size() > 1<<20 {
			return nil, fmt.Errorf("invalid registered skill file")
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("registered skill is unreadable")
		}
		fingerprint := digest(data)
		if ref.Fingerprint != "" && ref.Fingerprint != fingerprint {
			return nil, fmt.Errorf("registered skill changed; review its instructions before registering again")
		}
		ref.Path = path
		ref.Fingerprint = fingerprint
		seen[ref.ID] = true
		pinned = append(pinned, ref)
	}
	return pinned, nil
}

func ValidateSelection(catalog []SkillRef, ids []string) ([]SkillRef, error) {
	if len(ids) > 100 {
		return nil, fmt.Errorf("too many selected skills")
	}
	refs := map[string]SkillRef{}
	for _, ref := range catalog {
		if _, exists := refs[ref.ID]; exists {
			return nil, fmt.Errorf("duplicate registered skill identity")
		}
		refs[ref.ID] = ref
	}
	selected := []SkillRef{}
	seen := map[string]bool{}
	for _, id := range ids {
		ref, exists := refs[id]
		if !exists {
			return nil, fmt.Errorf("selected skill is not registered by the launching host")
		}
		if !seen[id] {
			selected = append(selected, ref)
			seen[id] = true
		}
	}
	pinned, err := PinSkills(selected)
	if err != nil {
		return nil, err
	}
	if !reflect.DeepEqual(selected, pinned) {
		return nil, fmt.Errorf("registered skill provenance changed")
	}
	return selected, nil
}

// RegisterSkills accepts only the trusted CLI manifest; browser routes cannot register skills.
func (s *Store) RegisterSkills(refs []SkillRef) error {
	pinned, err := PinSkills(refs)
	if err != nil {
		return err
	}
	return withLock(filepath.Join(s.root, ".lock"), func() error {
		existing, err := readManifest(filepath.Join(s.root, "skills.json"))
		if err == nil {
			identities := append([]SkillRef{}, pinned...)
			originals := append([]SkillRef{}, existing...)
			for i := range identities {
				identities[i].Fingerprint = ""
			}
			for i := range originals {
				originals[i].Fingerprint = ""
			}
			if !reflect.DeepEqual(originals, identities) {
				return fmt.Errorf("this review already has a different host skill manifest")
			}
			if reflect.DeepEqual(existing, pinned) {
				return nil
			}
		}
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		data, err := json.Marshal(skillManifest{Version: 1, Skills: pinned})
		if err != nil {
			return err
		}
		return atomicFile(filepath.Join(s.root, "skills.json"), data)
	})
}

func (s *Store) Skills() ([]SkillRef, error) {
	refs, err := readManifest(filepath.Join(s.root, "skills.json"))
	if os.IsNotExist(err) {
		return []SkillRef{}, nil
	}
	return refs, err
}
