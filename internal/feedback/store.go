package feedback

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"

	"github.com/BSH2409/hunkyard/internal/strictjson"
)

type Store struct {
	root                       string
	beforePreferenceCheckpoint func() error
}
type publication struct {
	RequestID string `json:"request_id"`
	InputHash string `json:"input_hash"`
	Batch     Batch  `json:"batch"`
	Acked     bool   `json:"acked"`
}
type journal struct {
	Version      int                             `json:"version"`
	Publications []publication                   `json:"publications"`
	Preferences  map[string]preferenceCheckpoint `json:"preferences,omitempty"`
}

func Open(reviewDir string) (*Store, error) {
	if reviewDir == "" {
		return nil, fmt.Errorf("missing review directory")
	}
	root := filepath.Join(reviewDir, "feedback")
	if err := privateDirectory(root); err != nil {
		return nil, err
	}
	return &Store{root: root}, nil
}

func (s *Store) transaction(update func(*journal) error) error {
	return withLock(filepath.Join(s.root, ".lock"), func() error {
		state, err := s.read()
		if err != nil {
			return err
		}
		before, _ := json.Marshal(state)
		if err := update(&state); err != nil {
			return err
		}
		after, err := json.Marshal(state)
		if err != nil {
			return err
		}
		if reflect.DeepEqual(before, after) {
			return nil
		}
		if len(after) > 64<<20 {
			return fmt.Errorf("feedback journal exceeds limit")
		}
		return atomicFile(filepath.Join(s.root, "state.json"), after)
	})
}

//nolint:gocyclo // Journal validation and publication preserve one atomic state boundary.
func (s *Store) read() (journal, error) {
	state := journal{Version: 1, Publications: []publication{}}
	name := filepath.Join(s.root, "state.json")
	info, err := os.Lstat(name)
	if os.IsNotExist(err) {
		return state, nil
	}
	if err != nil {
		return state, err
	}
	if !info.Mode().IsRegular() || info.Size() > 64<<20 {
		return state, fmt.Errorf("invalid feedback journal")
	}
	data, err := os.ReadFile(name)
	if err != nil {
		return state, err
	}
	if strictjson.Decode(data, &state) != nil || state.Version != 1 {
		return state, fmt.Errorf("invalid feedback journal")
	}
	requests, ids := map[string]bool{}, map[string]bool{}
	for _, p := range state.Publications {
		batch := p.Batch
		id := batch.ID
		batch.ID = ""
		if validateBatch(batch) != nil || p.RequestID == "" || p.InputHash == "" || requests[p.RequestID] || ids[id] || id != "batch_"+digest(struct {
			RequestID string
			Batch     Batch
		}{p.RequestID, batch}) {
			return state, fmt.Errorf("invalid feedback publication")
		}
		requests[p.RequestID] = true
		ids[id] = true
	}
	expected := map[string]Comment{}
	for _, p := range state.Publications {
		for _, c := range p.Batch.Comments {
			expected[checkpointKey(p.Batch.ID, c)] = c
		}
	}
	for key, checkpoint := range state.Preferences {
		c, exists := expected[key]
		if !exists || checkpoint.Outcome.CommentID != c.ID || checkpoint.Outcome.Revision != c.Revision || !snapshotIDPattern.MatchString("snapshot_"+checkpoint.DecisionHash) || (!completedPreference(checkpoint.Outcome.Status) && checkpoint.Outcome.Status != "failed") {
			return state, fmt.Errorf("invalid preference checkpoint")
		}
	}
	return state, nil
}

//nolint:gocyclo // Journal validation and publication preserve one atomic state boundary.
func (s *Store) Publish(requestID string, batch Batch) (Batch, error) {
	if requestID == "" || len(requestID) > 128 {
		return Batch{}, fmt.Errorf("invalid publication request identity")
	}
	if err := validateBatch(batch); err != nil {
		return Batch{}, err
	}
	if batch.ID != "" || len(batch.PublishedSkills) != 0 {
		return Batch{}, fmt.Errorf("publication identity is assigned locally")
	}
	for _, c := range batch.Comments {
		if c.Revision != 0 {
			return Batch{}, fmt.Errorf("comment revisions are assigned locally")
		}
	}
	catalog, err := s.Skills()
	if err != nil {
		return Batch{}, err
	}
	selected, err := ValidateSelection(catalog, batch.SkillIDs)
	if err != nil {
		return Batch{}, err
	}
	batch.SkillIDs = []string{}
	for _, ref := range selected {
		batch.SkillIDs = append(batch.SkillIDs, ref.ID)
	}
	batch.PublishedSkills = selected
	inputHash := digest(batch)
	var published Batch
	err = s.transaction(func(state *journal) error {
		for _, p := range state.Publications {
			if p.RequestID == requestID {
				if p.InputHash != inputHash {
					return fmt.Errorf("publication request was already used for different feedback")
				}
				published = p.Batch
				return nil
			}
		}
		latest := map[string]Comment{}
		for _, p := range state.Publications {
			for _, c := range p.Batch.Comments {
				latest[c.ID] = c
			}
		}
		comments := []Comment{}
		for _, c := range batch.Comments {
			previous, exists := latest[c.ID]
			if exists && revisionHash(previous) == revisionHash(c) {
				continue
			}
			c.Revision = previous.Revision + 1
			comments = append(comments, c)
		}
		batch.Comments = comments
		batch.ID = "batch_" + digest(struct {
			RequestID string
			Batch     Batch
		}{requestID, batch})
		state.Publications = append(state.Publications, publication{RequestID: requestID, InputHash: inputHash, Batch: batch})
		published = batch
		return nil
	})
	return published, err
}

func (s *Store) Pending() ([]Batch, error) {
	batches := []Batch{}
	err := s.transaction(func(state *journal) error {
		for _, p := range state.Publications {
			if !p.Acked {
				batches = append(batches, p.Batch)
			}
		}
		return nil
	})
	return batches, err
}

func (s *Store) Ack(batchID string) error {
	if !batchIDPattern.MatchString(batchID) {
		return fmt.Errorf("invalid batch identity")
	}
	return s.transaction(func(state *journal) error {
		for i := range state.Publications {
			if state.Publications[i].Batch.ID == batchID {
				for _, c := range state.Publications[i].Batch.Comments {
					if !completedPreference(state.Preferences[checkpointKey(batchID, c)].Outcome.Status) {
						return fmt.Errorf("classify every human revision with feedback learn before acknowledging")
					}
				}
				state.Publications[i].Acked = true
				return nil
			}
		}
		return fmt.Errorf("feedback batch not found")
	})
}

func (s *Store) HasPublications() (bool, error) {
	published := false
	err := s.transaction(func(state *journal) error { published = len(state.Publications) > 0; return nil })
	return published, err
}

// LatestSnapshotID returns the snapshot of the most recent publication, or ""
// before the first one.
func (s *Store) LatestSnapshotID() (string, error) {
	id := ""
	err := s.transaction(func(state *journal) error {
		if n := len(state.Publications); n > 0 {
			id = state.Publications[n-1].Batch.SnapshotID
		}
		return nil
	})
	return id, err
}
