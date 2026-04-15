package notification

import (
	"crypto/sha256"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// NotificationStore tracks sent games to avoid duplicate Discord notifications.
type NotificationStore struct {
	mu      sync.RWMutex
	file    string
	entries map[string]sentEntry // key: provider_title hash
}

type sentEntry struct {
	SentAt    time.Time `json:"sentAt"`
	StartDate string    `json:"startDate"`
	EndDate   string    `json:"endDate"`
}

// NewNotificationStore loads or creates a store at the given path.
func NewNotificationStore(path string) (*NotificationStore, error) {
	s := &NotificationStore{
		file:    path,
		entries: make(map[string]sentEntry),
	}
	if err := s.load(); err != nil {
		if !os.IsNotExist(err) {
			return nil, err
		}
	}
	return s, nil
}

// key generates a deterministic hash for a game entry.
func key(provider, title string) string {
	h := sha256.Sum256([]byte(provider + "|" + title))
	return string(h[:16]) // first 16 bytes as string
}

// HasSent returns true if this game was already notified.
func (s *NotificationStore) HasSent(provider, title string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.entries[key(provider, title)]
	return ok
}

// Record marks a game as sent.
func (s *NotificationStore) Record(provider, title, startDate, endDate string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries[key(provider, title)] = sentEntry{
		SentAt:    time.Now(),
		StartDate: startDate,
		EndDate:   endDate,
	}
	return s.save()
}

// FilterNew returns only games that haven't been notified yet.
// It also updates the store with newly sent games.
func (s *NotificationStore) FilterNew(games []SentGame) ([]SentGame, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var new []SentGame
	for _, g := range games {
		k := key(g.Provider, g.Title)
		if _, exists := s.entries[k]; !exists {
			s.entries[k] = sentEntry{
				SentAt:    time.Now(),
				StartDate: g.StartDate,
				EndDate:   g.EndDate,
			}
			new = append(new, g)
		}
	}
	if len(new) > 0 {
		if err := s.save(); err != nil {
			return new, err
		}
	}
	return new, nil
}

func (s *NotificationStore) load() error {
	data, err := os.ReadFile(s.file)
	if err != nil {
		return err
	}
	var entries map[string]sentEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return err
	}
	s.entries = entries
	return nil
}

func (s *NotificationStore) save() error {
	data, err := json.Marshal(s.entries)
	if err != nil {
		return err
	}
	dir := filepath.Dir(s.file)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	return os.WriteFile(s.file, data, 0644)
}

// Cleanup removes entries older than the given duration.
func (s *NotificationStore) Cleanup(olderThan time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cutoff := time.Now().Add(-olderThan)
	for k, e := range s.entries {
		if e.SentAt.Before(cutoff) {
			delete(s.entries, k)
		}
	}
	return s.save()
}

// sentGame is a lightweight game record for store operations.
type SentGame struct {
	Provider  string
	Title     string
	StartDate string
	EndDate   string
}