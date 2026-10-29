package preferences

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strings"
)

type operationResult struct {
	Status       string         `json:"status"`
	GenerationID *string        `json:"generation_id"`
	Action       *string        `json:"action"`
	RuleID       *string        `json:"rule_id"`
	Counts       map[string]any `json:"counts"`
}

func randomID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b[:])
}

func (s *Store) build() (operationResult, error) {
	result := operationResult{Status: "no_generation", Counts: map[string]any{}}
	current, err := s.loadCurrent()
	if err != nil || current == nil {
		return result, err
	}
	err = s.withGenerationLock(func() error {
		current, err := s.loadCurrent()
		if err != nil {
			return err
		}
		if current == nil {
			return fmt.Errorf("current generation disappeared")
		}
		files, manifest := renderPersona(current.Rules.rules)
		result.Counts["persona_files"] = len(files)
		result.GenerationID = &current.ID
		if sameValue(files, current.PersonaFiles) && sameValue(manifest["rules"], current.Manifest["rules"]) {
			result.Status = "no_change"
			return nil
		}
		manifest["build"] = map[string]any{"run_id": randomID()}
		id, err := s.commitLocked(generation{Rules: current.Rules, ProcessedIDs: current.ProcessedIDs, SourcePacks: current.SourcePacks, PersonaFiles: files, Manifest: manifest})
		if err == nil {
			result.Status = "success"
			result.GenerationID = &id
		}
		return err
	})
	return result, err
}

func (s *Store) retire(id string) (operationResult, error) {
	result := operationResult{Status: "success", Counts: map[string]any{}}
	err := s.withGenerationLock(func() error {
		current, err := s.loadCurrent()
		if err != nil {
			return err
		}
		if current == nil {
			return fmt.Errorf("cannot retire without a current generation")
		}
		catalog, err := s.catalogFromGeneration(current)
		if err != nil {
			return err
		}
		if _, err := catalog.retire(id); err != nil {
			return err
		}
		files, manifest := renderPersona(catalog.orderedActive())
		manifest["retire"] = map[string]any{"rule_id": id}
		generationID, err := s.commitLocked(generation{Rules: catalog.ruleSet(), ProcessedIDs: current.ProcessedIDs, SourcePacks: current.SourcePacks, PersonaFiles: files, Manifest: manifest})
		if err == nil {
			action := "retired"
			result.GenerationID = &generationID
			result.Action = &action
			result.RuleID = &id
		}
		return err
	})
	return result, err
}

var searchWord = regexp.MustCompile(`[a-z][a-z0-9_-]+`)

//nolint:gocyclo // Search combines canonical surface, lifecycle, and ranked result filters.
func (s *Store) search(query string, filter []string) ([]map[string]any, error) {
	for _, surface := range filter {
		if !slices.Contains(surfaces, surface) {
			return nil, fmt.Errorf("search surfaces must be canonical")
		}
	}
	result := []map[string]any{}
	current, err := s.loadCurrent()
	if err != nil || current == nil {
		return result, err
	}
	terms := sortedUnique(searchWord.FindAllString(strings.ToLower(normalize(query)), -1))
	type match struct {
		rule  rule
		score int
	}
	matches := []match{}
	for _, r := range current.Rules.rules {
		if !slices.Contains([]string{"active", "core", "candidate"}, textField(r["status"])) {
			continue
		}
		if len(filter) > 0 && !slices.ContainsFunc(filter, func(surface string) bool { return slices.Contains(stringValues(r["surfaces"]), surface) }) {
			continue
		}
		words := searchWord.FindAllString(strings.ToLower(textField(r["directive"])), -1)
		score := 0
		for _, term := range terms {
			if slices.Contains(words, term) {
				score++
			}
		}
		if len(terms) == 0 || score > 0 {
			matches = append(matches, match{r, score})
		}
	}
	sort.Slice(matches, func(i, j int) bool {
		a, b := matches[i], matches[j]
		if a.score != b.score {
			return a.score > b.score
		}
		ac, _ := integer(record(a.rule["evidence"])["observation_count"])
		bc, _ := integer(record(b.rule["evidence"])["observation_count"])
		if ac != bc {
			return ac > bc
		}
		return textField(a.rule["id"]) < textField(b.rule["id"])
	})
	for _, match := range matches[:min(len(matches), 8)] {
		r := map[string]any{}
		for _, key := range []string{"id", "directive", "scope", "surfaces", "status"} {
			r[key] = match.rule[key]
		}
		result = append(result, cloneMap(r))
	}
	return result, nil
}
