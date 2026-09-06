// Package linkstore holds which Discord account each link secret names.
//
// A link secret is what an install proves its Discord identity with on every call,
// drawn once at link time and stored in that install's settings.
// The store is the manager's one durable fact:
// sessions rebuild from the gateway after a restart, links cannot.
//
// Several links per user stand for several installs.
// A draw past the cap evicts that user's oldest link,
// which is the only revocation there is: relinking often enough ages a stolen secret out.
package linkstore

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"sync"

	"bjoernblessin.de/go-utils/util/assert"
)

// LinksPerUser bounds how many installs one Discord account may hold at once.
// Small: a person has a handful of machines, where an unbounded list grows with every reinstall.
const LinksPerUser = 8

// secretBytes is a link secret's entropy.
// 32, as a group key: the secret is the whole proof of identity, so guessing one is impersonation.
const secretBytes = 32

// Link is what one secret resolves to.
type Link struct {
	UserID string `json:"userId"`
}

// stored is one row of the file: a link and the secret naming it.
// Rows stand in draw order, oldest first, which is what the eviction reads.
type stored struct {
	Secret string `json:"secret"`
	UserID string `json:"userId"`
}

// Store answers and persists the links. Safe for concurrent use.
type Store struct {
	path string

	mu   sync.Mutex
	rows []stored
}

// Open reads the links from path, an absent file being an empty store.
//
// A file that will not read or parse is an Umgebungsfehler and leaves as an error:
// serving with it ignored would cut every linked install loose at once.
func Open(path string) (*Store, error) {
	assert.Assert(path != "", "a store persists to a named file")

	s := &Store{path: path}
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return s, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading the link store %s: %w", path, err)
	}
	if err := json.Unmarshal(raw, &s.rows); err != nil {
		return nil, fmt.Errorf("the link store %s does not parse: %w", path, err)
	}
	return s, nil
}

// Draw links one more install to this user and persists it,
// evicting the user's oldest link where the cap is reached.
//
// A store that cannot persist draws nothing:
// a secret handed out and forgotten on restart would strand that install half-linked.
func (s *Store) Draw(userID string) (secret string, err error) {
	assert.Assert(userID != "", "a link names the user it belongs to")

	raw := make([]byte, secretBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("drawing a link secret: %w", err)
	}
	// URL-safe alphabet: a drawn secret reaches its install through a query string,
	// and a query decode reads '+' as a space (internal/discordapi).
	secret = base64.RawURLEncoding.EncodeToString(raw)

	s.mu.Lock()
	defer s.mu.Unlock()

	held := s.rows
	if oldest, count := s.oldestOf(userID); count >= LinksPerUser {
		s.rows = append(s.rows[:oldest], s.rows[oldest+1:]...)
	}
	s.rows = append(s.rows, stored{Secret: secret, UserID: userID})

	if err := s.persist(); err != nil {
		s.rows = held
		return "", err
	}

	assert.Assert(s.count(userID) <= LinksPerUser,
		"a user's links stay inside the cap", s.count(userID))
	return secret, nil
}

// Resolve answers which user a secret names, ok=false for a secret this store never drew.
func (s *Store) Resolve(secret string) (Link, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, row := range s.rows {
		if row.Secret == secret {
			return Link{UserID: row.UserID}, true
		}
	}
	return Link{}, false
}

// oldestOf finds the first row of this user, and how many rows they hold.
// The index is meaningful only where count is at least one.
func (s *Store) oldestOf(userID string) (oldest, count int) {
	oldest = -1
	for i, row := range s.rows {
		if row.UserID != userID {
			continue
		}
		if oldest < 0 {
			oldest = i
		}
		count++
	}
	return oldest, count
}

// count is how many links this user holds, for the postcondition alone.
func (s *Store) count(userID string) int {
	_, n := s.oldestOf(userID)
	return n
}

// persist writes every row, readable by nobody else: a secret is an identity.
func (s *Store) persist() error {
	encoded, err := json.Marshal(s.rows)
	if err != nil {
		return fmt.Errorf("rendering the link store: %w", err)
	}
	if err := os.WriteFile(s.path, encoded, 0o600); err != nil {
		return fmt.Errorf("writing the link store %s: %w", s.path, err)
	}
	return nil
}
