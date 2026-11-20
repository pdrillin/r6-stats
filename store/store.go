// Package store keeps what the user tells about their matches, which the
// replays do not hold: the kind of each match and the user's team. It is one
// JSON file, rewritten whole on every change: enough for a local application
// with a single user.
package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/pdrillin/stratdata/match"
)

// Kind is what a match was played for. Pracc and official matches are both
// custom games, with the same files: only the user can tell them apart.
type Kind string

const (
	Pracc    Kind = "pracc"    // scrim or training
	Official Kind = "official" // league or tournament match
)

// DefaultKind is the kind of a match the user did not label: a team mostly
// records its praccs.
const DefaultKind = Pracc

// Valid reports whether k is a kind a match can be labelled with.
func (k Kind) Valid() bool { return k == Pracc || k == Official }

// Data is the content of the store.
type Data struct {
	// MyTeam is the team the user plays in, as named in the replays.
	MyTeam string `json:"myTeam"`
	// Kinds holds the kind of the matches the user labelled, by match id.
	Kinds map[string]Kind `json:"kinds"`
	// TeamNames are the names the user gave to teams.
	TeamNames []match.Alias `json:"teamNames,omitempty"`
}

// Store is the user's data, kept in a file.
type Store struct {
	path string
	mu   sync.Mutex
	data Data
}

// DefaultPath is the store of the user: %AppData%\stratdata\data.json on
// Windows, ~/.config/stratdata/data.json on Linux.
func DefaultPath() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	path := filepath.Join(base, "stratdata", "data.json")
	// Data kept under the project's former name moves to the new one.
	old := filepath.Join(base, "r6-stats", "data.json")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		if _, err := os.Stat(old); err == nil {
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err == nil {
				os.Rename(old, path)
			}
		}
	}
	return path, nil
}

// Open reads the store at path. A missing file is an empty store; the file
// is created on the first change.
func Open(path string) (*Store, error) {
	s := &Store{path: path, data: Data{Kinds: map[string]Kind{}}}
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, &s.data); err != nil {
		// Refuse to start over silently: the file holds the user's work.
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if s.data.Kinds == nil {
		s.data.Kinds = map[string]Kind{}
	}
	return s, nil
}

// Memory returns a store that is never written to disk, for tests and for
// running without user data.
func Memory() *Store {
	return &Store{data: Data{Kinds: map[string]Kind{}}}
}

// Path is the file of the store, empty for a store in memory.
func (s *Store) Path() string { return s.path }

// MyTeam returns the user's team, empty when not chosen.
func (s *Store) MyTeam() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.data.MyTeam
}

// SetMyTeam records the user's team; empty clears it.
func (s *Store) SetMyTeam(team string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	old := s.data.MyTeam
	s.data.MyTeam = team
	if err := s.save(); err != nil {
		s.data.MyTeam = old
		return err
	}
	return nil
}

// TeamNames returns the names the user gave to teams.
func (s *Store) TeamNames() []match.Alias {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]match.Alias(nil), s.data.TeamNames...)
}

// SetTeamNames replaces the names the user gave to teams.
func (s *Store) SetTeamNames(names []match.Alias) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	old := s.data.TeamNames
	s.data.TeamNames = names
	if err := s.save(); err != nil {
		s.data.TeamNames = old
		return err
	}
	return nil
}

// Kind returns the kind of a match, DefaultKind when it was not labelled.
func (s *Store) Kind(matchID string) Kind {
	s.mu.Lock()
	defer s.mu.Unlock()
	if k, ok := s.data.Kinds[matchID]; ok {
		return k
	}
	return DefaultKind
}

// SetKind labels a match.
func (s *Store) SetKind(matchID string, k Kind) error {
	if !k.Valid() {
		return fmt.Errorf("unknown match kind %q", k)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	old, had := s.data.Kinds[matchID]
	s.data.Kinds[matchID] = k
	if err := s.save(); err != nil {
		if had {
			s.data.Kinds[matchID] = old
		} else {
			delete(s.data.Kinds, matchID)
		}
		return err
	}
	return nil
}

// save writes the whole store through a temporary file, so the file on disk
// is always either the old or the new content.
func (s *Store) save() error {
	if s.path == "" {
		return nil
	}
	b, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "data.*.tmp")
	if err != nil {
		return err
	}
	if _, err := tmp.Write(append(b, '\n')); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	if err := os.Rename(tmp.Name(), s.path); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return nil
}
