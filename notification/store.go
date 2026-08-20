package notification

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gravityctl/free-games/common"
)

// DefaultRenotifyAfter is how long a game stays suppressed when its offer carries
// no end date. Providers that report an offer window are gated on that window
// instead; this only applies to the ones that do not, such as Steam.
const DefaultRenotifyAfter = 30 * 24 * time.Hour

// retention is how long entries are kept before being pruned from the file. It is
// well past DefaultRenotifyAfter, so pruning never resurrects a notification that
// would otherwise still be suppressed.
const retention = 365 * 24 * time.Hour

// NotificationStore tracks sent games to avoid duplicate Discord notifications.
type NotificationStore struct {
	mu            sync.RWMutex
	file          string
	entries       map[string]sentEntry // key: provider|title
	renotifyAfter time.Duration
}

type sentEntry struct {
	SentAt    time.Time `json:"sentAt"`
	StartDate string    `json:"startDate"`
	EndDate   string    `json:"endDate"`
}

// NewNotificationStore loads or creates a store at the given path.
func NewNotificationStore(path string) (*NotificationStore, error) {
	s := &NotificationStore{
		file:          path,
		entries:       make(map[string]sentEntry),
		renotifyAfter: DefaultRenotifyAfter,
	}
	if err := s.load(); err != nil {
		if !os.IsNotExist(err) {
			return nil, err
		}
	}
	return s, nil
}

// SetRenotifyAfter overrides how long a dateless offer stays suppressed.
func (s *NotificationStore) SetRenotifyAfter(d time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if d > 0 {
		s.renotifyAfter = d
	}
}

// key creates a simple deduplication key from provider and title.
// Lowercased and whitespace-trimmed for consistency.
func key(provider, title string) string {
	return strings.ToLower(strings.TrimSpace(provider)) + "|" + strings.ToLower(strings.TrimSpace(title))
}

// HasSent returns true if this game was already notified.
func (s *NotificationStore) HasSent(provider, title string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.entries[key(provider, title)]
	return ok
}

// FilterNew returns the games that still need a notification.
//
// It does not record anything: a game is only marked as sent once delivery has
// actually succeeded, so a failed webhook is retried on the next run instead of
// being silently swallowed. Call Record with the games that were delivered.
func (s *NotificationStore) FilterNew(games []common.Game) ([]common.Game, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	now := time.Now()
	var fresh []common.Game
	for _, g := range games {
		if s.isNew(g, now) {
			fresh = append(fresh, g)
		}
	}
	return fresh, nil
}

// isNew decides whether a game warrants a notification. Beyond "have we seen this
// title", it treats a game as new again when the offer it was last seen under has
// been replaced — giveaways do come back around, and suppressing a title forever
// after its first appearance means missing every later one.
func (s *NotificationStore) isNew(g common.Game, now time.Time) bool {
	prev, ok := s.entries[key(g.Provider, g.Title)]
	if !ok {
		return true
	}

	prevStart := parseTime(prev.StartDate)
	prevEnd := parseTime(prev.EndDate)

	// A different start date means a different giveaway. Timestamps are stored at
	// second precision, so compare at that resolution rather than exactly.
	if !g.StartDate.IsZero() && !prevStart.IsZero() && g.StartDate.Unix() != prevStart.Unix() {
		return true
	}

	// The recorded offer has expired, so this listing is a later one.
	if !prevEnd.IsZero() {
		return now.After(prevEnd)
	}

	// No offer window was ever recorded; fall back to a cooldown.
	return now.Sub(prev.SentAt) > s.renotifyAfter
}

// Record marks games as notified. Call it only after delivery succeeded.
func (s *NotificationStore) Record(games []common.Game) error {
	if len(games) == 0 {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	for _, g := range games {
		s.entries[key(g.Provider, g.Title)] = sentEntry{
			SentAt:    now,
			StartDate: formatTime(g.StartDate),
			EndDate:   formatTime(g.EndDate),
		}
	}
	s.prune(now)
	return s.save()
}

// prune drops entries whose last notification is older than retention.
func (s *NotificationStore) prune(now time.Time) {
	for k, e := range s.entries {
		if !e.SentAt.IsZero() && now.Sub(e.SentAt) > retention {
			delete(s.entries, k)
		}
	}
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

// parseTime reads a stored timestamp, tolerating the empty and zero-value forms
// written by earlier versions of the store.
func parseTime(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}
	}
	if t.IsZero() || t.Year() <= 1 {
		return time.Time{}
	}
	return t
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
	if entries == nil {
		entries = make(map[string]sentEntry)
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
