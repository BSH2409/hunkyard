package preferences

import (
	"bufio"
	"fmt"
	"github.com/BSH2409/hunkyard/internal/strictjson"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"
)

type CaptureInput struct {
	Feedback, Directive, Scope              string
	Surfaces                                []string
	CWD, RepoKey, Source, SessionID, TurnID string
	Explicit                                bool
	Interrupted                             bool
	Context                                 NeighboringContext
	CodeContext                             *CodeContext
}

type CaptureResult struct{ Status, Action, ID, Reason, RuleID string }

var durableLanguage = regexp.MustCompile(`(?i)\b(?:always|never|prefer|avoid|from now on|every time|in the future)\b`)
var correctionLanguage = regexp.MustCompile(`(?i)(?:^\s*no[,!. ]|\b(?:again|actually|instead|stop|don't|do not|wrong|too much)\b)`)
var taskLocalLanguage = regexp.MustCompile(`(?i)\b(?:rename|change|move|replace|delete|add|remove)\s+(?:this|the)\s+(?:local\s+)?(?:variable|identifier|method|function|file|class|module)\b`)
var surfacePatterns = func() map[string]*regexp.Regexp {
	patterns := map[string]*regexp.Regexp{}
	for _, surface := range surfaces {
		pattern := `(?i)\b` + surface + `\b`
		if surface == "wordpress" {
			pattern = `(?i)\bword[- ]?press\b`
		}
		patterns[surface] = regexp.MustCompile(pattern)
	}
	return patterns
}()

func analyzeSignals(feedback string) (observationSignals, []string, bool) {
	feedback = normalize(feedback)
	signals := observationSignals{Correction: correctionLanguage.MatchString(feedback), DurableLanguage: durableLanguage.MatchString(feedback), LikelyTaskLocal: taskLocalLanguage.MatchString(feedback)}
	detected := []string{}
	for _, surface := range surfaces {
		if surfacePatterns[surface].MatchString(feedback) {
			detected = append(detected, surface)
		}
	}
	capture := (signals.DurableLanguage || signals.Correction) && !(signals.LikelyTaskLocal && !signals.DurableLanguage)
	return signals, detected, capture
}

func nullableString(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

//nolint:gocyclo // Privacy, identity, and publication guards remain in the same capture transaction.
func (s *Store) prepareObservation(input CaptureInput) (observation, CaptureResult, error) {
	if strings.TrimSpace(input.Feedback) == "" {
		return observation{}, CaptureResult{Status: "no_work", Action: "ignored", Reason: "missing_raw_input"}, nil
	}
	privacy := sanitize(input.Feedback)
	if !privacy.Accepted {
		return observation{}, CaptureResult{Status: "no_work", Action: "ignored", Reason: privacy.Reason}, nil
	}
	repo := input.RepoKey
	if input.Explicit && input.Scope == "global" {
		repo = ""
	} else if repo == "" && input.CWD != "" {
		var err error
		repo, err = repositoryKey(input.CWD)
		if err != nil {
			return observation{}, CaptureResult{}, err
		}
	}
	if repo != "" && !repositoryKeyPattern.MatchString(repo) {
		return observation{}, CaptureResult{Status: "no_work", Action: "ignored", Reason: "invalid_repository_key"}, nil
	}
	signals, detected, capturable := analyzeSignals(input.Feedback)
	source := input.Source
	if source == "" {
		source = "hook"
	}
	// The launching host already classified review candidates; keywords do not add durable intent.
	capturable = capturable || source == "review" && privacy.Excerpt != ""
	// The first prompt after the user interrupts the agent is a correction of
	// what the agent was doing, whatever words it uses.
	if input.Interrupted {
		capturable = true
		signals.Correction = true
	}
	if input.Explicit {
		capturable = true
		source = "explicit"
		signals = observationSignals{Correction: true, DurableLanguage: true}
		detected = []string{}
		for _, surface := range input.Surfaces {
			if slices.Contains(surfaces, surface) {
				detected = append(detected, surface)
			}
		}
	}
	if !capturable {
		return observation{}, CaptureResult{Status: "no_work", Action: "ignored", Reason: "not_capturable"}, nil
	}
	session, turn := "", ""
	if input.SessionID != "" {
		session = digest(input.SessionID)
	}
	if input.TurnID != "" {
		turn = digest(input.TurnID)
	}
	context := sanitizeContext(input.Context)
	components := []string{source, privacy.Fingerprint, session, turn, repo}
	if turn == "" && context != nil {
		encoded, err := canonicalJSON(context)
		if err != nil {
			return observation{}, CaptureResult{}, err
		}
		components = append(components, string(encoded))
	}
	code := sanitizeCodeContext(input.CodeContext)
	entry := observation{CodeContext: code, ID: "obs_" + digest(strings.Join(components, "\x00")), CapturedAt: s.config.Clock().UTC().Format(time.RFC3339), Source: source, RepoKey: nullableString(repo), SessionKey: nullableString(session), TurnKey: nullableString(turn), PromptFingerprint: privacy.Fingerprint, SafeExcerpt: &privacy.Excerpt, Signals: signals, SuggestedSurfaces: detected, Context: context}
	return entry, CaptureResult{Status: "success", Action: "prepared", ID: entry.ID}, nil
}

//nolint:gocyclo // Privacy, identity, and publication guards remain in the same capture transaction.
func (s *Store) appendObservation(entry observation, nonblocking bool) (observation, CaptureResult, error) {
	var stored observation
	result := CaptureResult{Status: "success", Action: "created", ID: entry.ID}
	err := withFileLock(s.dataPath("observations", ".pending.lock"), nonblocking, func() error {
		name := s.dataPath("observations", "pending.jsonl")
		if info, err := os.Lstat(name); err == nil && !info.Mode().IsRegular() {
			return fmt.Errorf("pending observations must be a regular file")
		} else if err != nil && !os.IsNotExist(err) {
			return err
		}
		file, err := os.OpenFile(name, os.O_CREATE|os.O_RDWR|os.O_APPEND, 0600)
		if err != nil {
			return err
		}
		defer file.Close()
		if err := file.Chmod(0600); err != nil {
			return err
		}
		if err := lockFile(file, nonblocking); err != nil {
			return err
		}
		defer unlockFile(file)
		scanner := bufio.NewScanner(file)
		scanner.Buffer(make([]byte, 4096), 1024*1024)
		for scanner.Scan() {
			var existing observation
			if strictjson.Decode(scanner.Bytes(), &existing) == nil && existing.ID == entry.ID {
				stored = existing
				result.Action = "duplicate"
				result.Reason = "duplicate"
				return nil
			}
		}
		if err := scanner.Err(); err != nil {
			return err
		}
		current, err := s.loadCurrent()
		if err != nil {
			return err
		}
		if current != nil && slices.Contains(current.ProcessedIDs, entry.ID) {
			stored = entry
			result.Action = "duplicate"
			result.Reason = "duplicate"
			return nil
		}
		encoded, err := canonicalJSON(entry)
		if err != nil {
			return err
		}
		if _, err := file.Write(append(encoded, '\n')); err != nil {
			return err
		}
		if err := file.Sync(); err != nil {
			return err
		}
		stored = entry
		return nil
	})
	return stored, result, err
}

func (s *Store) Capture(input CaptureInput) (CaptureResult, error) { return s.capture(input, false) }

//nolint:gocyclo // Privacy, identity, and publication guards remain in the same capture transaction.
func (s *Store) capture(input CaptureInput, nonblocking bool) (CaptureResult, error) {
	entry, result, err := s.prepareObservation(input)
	if err != nil || result.Action == "ignored" {
		return result, err
	}
	if !input.Explicit {
		_, result, err = s.appendObservation(entry, nonblocking)
		return result, err
	}
	err = s.withGenerationLock(func() error {
		var err error
		entry, result, err = s.appendObservation(entry, nonblocking)
		if err != nil {
			return err
		}
		current, err := s.loadCurrent()
		if err != nil {
			return err
		}
		catalog, err := s.catalogFromGeneration(current)
		if err != nil {
			return err
		}
		var repo any
		if input.RepoKey != "" {
			repo = input.RepoKey
		} else if input.Scope != "global" && entry.RepoKey != nil {
			repo = *entry.RepoKey
		}
		scope := map[string]any{"kind": input.Scope, "repo_key": repo}
		encoded, err := canonicalJSON(entry)
		if err != nil {
			return err
		}
		var fields map[string]any
		if err := strictjson.Decode(encoded, &fields); err != nil {
			return err
		}
		_, findErr := catalog.findRule(ruleID(normalize(input.Directive), scope, sortedUnique(input.Surfaces)))
		repeated := findErr == nil
		activated, err := catalog.upsertExplicit(fields, input.Directive, scope, input.Surfaces)
		if err != nil {
			return err
		}
		result.RuleID = textField(activated["id"])
		if result.Action == "duplicate" && current != nil && slices.Contains(current.ProcessedIDs, entry.ID) {
			for _, existing := range current.Rules.rules {
				if existing["id"] != activated["id"] {
					continue
				}
				references, err := mapArray(existing["observations"])
				if err != nil {
					return err
				}
				for _, reference := range references {
					if reference["id"] == entry.ID {
						return nil
					}
				}
			}
			return fmt.Errorf("duplicate observation cannot activate a different rule")
		}
		files, manifest := renderPersona(catalog.rules)
		next := generation{Rules: catalog.ruleSet(), ProcessedIDs: []string{entry.ID}, SourcePacks: map[string]any{}, PersonaFiles: files, Manifest: baseManifest(current)}
		if current != nil {
			next.ProcessedIDs = append(append([]string{}, current.ProcessedIDs...), entry.ID)
			next.SourcePacks = current.SourcePacks
		}
		for key, value := range manifest {
			next.Manifest[key] = value
		}
		if _, err := s.commitLocked(next); err != nil {
			return err
		}
		result.Action = "activated"
		result.Reason = ""
		kind := activityLearned
		if repeated {
			kind = activityViolated
		}
		if err := s.recordActivity(ActivityEvent{Kind: kind, RuleID: result.RuleID, RepoKey: textField(repo), Detail: normalize(input.Directive)}); err != nil {
			fmt.Fprintf(os.Stderr, "preferences: activity not recorded: %v\n", err)
		}
		if s.mirrorError != nil {
			result.Reason = "compatibility_repair_needed"
		}
		return nil
	})
	return result, err
}

func baseManifest(current *generation) map[string]any {
	if current == nil {
		return map[string]any{"schema_version": schemaVersion}
	}
	manifest := cloneMap(current.Manifest)
	for _, key := range []string{"files", "generation_id", "persona_files"} {
		delete(manifest, key)
	}
	return manifest
}

func sanitizeCodeContext(code *CodeContext) *CodeContext {
	if code == nil {
		return nil
	}
	clean := *code
	if filepath.IsAbs(clean.Path) || !safeRelative(filepath.ToSlash(clean.Path)) {
		clean.Path = ""
	}
	privacy := sanitize(clean.Excerpt)
	clean.Excerpt = ""
	if privacy.Accepted {
		clean.Excerpt = truncate(privacy.Excerpt, 600)
	}
	return &clean
}
