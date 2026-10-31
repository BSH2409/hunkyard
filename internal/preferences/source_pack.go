package preferences

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

type sourceFile struct{ Path, Digest, Content string }
type sourceSnapshot struct {
	issuer   *Store
	files    map[string]sourceFile
	metadata map[string]any
	root     string
	digest   string
}

var referenceLink = regexp.MustCompile(`!?\[[^\]\n]*\]\(([^)\s]+)(?:\s+["'][^)]*["'])?\)`)
var referencePath = regexp.MustCompile(`^reference/[a-z0-9-]+\.md$`)
var sectionHeading = regexp.MustCompile(`^##\s+`)
var referencesHeading = regexp.MustCompile(`^## References\s*$`)

func (s *Store) sourcePackForGeneration(current *generation) (*sourceSnapshot, error) {
	_, err := os.Lstat(s.dataPath("sources", "code-like-him"))
	if os.IsNotExist(err) && (current == nil || current.SourcePacks["code-like-him"] == nil) {
		return nil, nil
	}
	return s.snapshotSourcePack()
}

//nolint:gocyclo // A snapshot accepts only confined files from the explicit References allowlist.
func (s *Store) snapshotSourcePack() (*sourceSnapshot, error) {
	root, err := filepath.EvalSymlinks(s.dataPath("sources", "code-like-him"))
	if err != nil {
		return nil, err
	}
	read := func(name string) (sourceFile, error) {
		path, err := filepath.EvalSymlinks(filepath.Join(root, filepath.FromSlash(name)))
		if err != nil {
			return sourceFile{}, err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return sourceFile{}, fmt.Errorf("source reference escapes its trusted root")
		}
		info, err := os.Stat(path)
		if err != nil {
			return sourceFile{}, err
		}
		if !info.Mode().IsRegular() {
			return sourceFile{}, fmt.Errorf("source reference must be a regular file")
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return sourceFile{}, err
		}
		return sourceFile{Path: name, Content: string(data), Digest: digest(string(data))}, nil
	}
	main, err := read("SKILL.md")
	if err != nil {
		return nil, err
	}
	paths := map[string]bool{"SKILL.md": true}
	inReferences := false
	foundReferences := false
	for _, line := range strings.Split(main.Content, "\n") {
		if referencesHeading.MatchString(line) {
			inReferences = true
			foundReferences = true
			continue
		}
		if sectionHeading.MatchString(line) {
			inReferences = false
		}
		if !inReferences {
			continue
		}
		for _, link := range referenceLink.FindAllStringSubmatch(line, -1) {
			if !referencePath.MatchString(link[1]) {
				return nil, fmt.Errorf("source reference is not allowlisted: %q", link[1])
			}
			paths[link[1]] = true
		}
	}
	if !foundReferences {
		return nil, fmt.Errorf("source pack needs a References section")
	}
	names := make([]string, 0, len(paths))
	for name := range paths {
		names = append(names, name)
	}
	sort.Strings(names)
	files := map[string]sourceFile{}
	digests := map[string]any{}
	var combined strings.Builder
	for _, name := range names {
		file := main
		if name != "SKILL.md" {
			file, err = read(name)
			if err != nil {
				return nil, err
			}
		}
		files[name] = file
		digests[name] = file.Digest
		combined.WriteString(name + "\x00" + file.Digest)
	}
	checksum := digest(combined.String())
	return &sourceSnapshot{issuer: s, root: root, digest: checksum, files: files, metadata: map[string]any{"code-like-him": map[string]any{
		"digest": checksum, "files": digests, "refreshed_at": s.config.Clock().UTC().Format(time.RFC3339),
	}}}, nil
}

func (s *sourceSnapshot) revalidate() error {
	if s == nil || s.issuer == nil {
		return proposalError("source snapshot lacks authority")
	}
	if s.root == "" {
		return nil
	}
	current, err := s.issuer.snapshotSourcePack()
	if err != nil {
		return err
	}
	if current.root != s.root || current.digest != s.digest {
		return proposalError("source pack changed after capture")
	}
	return nil
}

func (s *sourceSnapshot) validateCitation(citation map[string]any) error {
	if s == nil || s.issuer == nil {
		return proposalError("source citation lacks authority")
	}
	if err := s.revalidate(); err != nil {
		return err
	}
	if err := validateProposalCitation(citation); err != nil {
		return err
	}
	file, exists := s.files[textField(citation["file"])]
	if !exists || file.Digest != citation["digest"] || digest(file.Content) != file.Digest {
		return proposalError("source citation does not match captured contents")
	}
	return nil
}
func (s *sourceSnapshot) validateProposedRule(proposed map[string]any) error {
	if proposed["origin"] != "source_pack" || proposed["source_pack"] != "code-like-him" {
		return proposalError("rule belongs to a different source pack")
	}
	if status, exists := proposed["status"]; exists && status != "active" {
		return proposalError("new source rules must be active")
	}
	if err := s.validateCitation(record(proposed["source_ref"])); err != nil {
		return err
	}
	if proposed["load_tier"] == "core" && (record(proposed["scope"])["kind"] != "global" || len(stringValues(proposed["surfaces"])) < 2) {
		return proposalError("core source rule must be global and cross-cutting")
	}
	if advocatesDiscouragedHabit(textField(proposed["directive"])) {
		return proposalError("source rule advocates a discouraged habit")
	}
	return nil
}

var discouragedHabit = regexp.MustCompile(`(?i)\b(?:vague|uninformative)\s+commit\s+messages?\b`)
var activeController = regexp.MustCompile(`(?i)\b(prefer|use|write|favor|choose|avoid)\b`)
var controllerNegation = regexp.MustCompile(`(?i)(?:\b(?:do|should|must)\s+not|\bdon't|\bnever|\b(?:prefer|choose)\s+not\s+to)\s*$`)
var targetNegation = regexp.MustCompile(`(?i)\bnot\s*$`)
var passiveNegative = regexp.MustCompile(`(?i)^\s*(?:(?:should|must)\s+(?:not|never)\s+be\s+(?:used|written|preferred|favored|chosen)|(?:are|is)\s+not\s+(?:preferred|recommended|favored))\b`)
var passivePositive = regexp.MustCompile(`(?i)^\s*(?:(?:should|must)\s+be\s+(?:used|written|preferred|favored|chosen)|(?:are|is)\s+(?:preferred|recommended|favored))\b`)
var localBoundary = regexp.MustCompile(`(?i)(?:[.;!?]|\bbut\b|\bhowever\b)`)

func advocatesDiscouragedHabit(directive string) bool {
	for _, habit := range discouragedHabit.FindAllStringIndex(directive, -1) {
		prefixes := localBoundary.Split(directive[:habit[0]], -1)
		prefix := prefixes[len(prefixes)-1]
		suffix := localBoundary.Split(directive[habit[1]:], -1)[0]
		if targetNegation.MatchString(prefix) {
			continue
		}
		controllers := activeController.FindAllStringIndex(prefix, -1)
		if len(controllers) > 0 {
			controller := controllers[len(controllers)-1]
			if strings.EqualFold(prefix[controller[0]:controller[1]], "avoid") || controllerNegation.MatchString(prefix[:controller[0]]) || targetNegation.MatchString(prefix[controller[1]:]) {
				continue
			}
			return true
		}
		if !passiveNegative.MatchString(suffix) && passivePositive.MatchString(suffix) {
			return true
		}
	}
	return false
}
