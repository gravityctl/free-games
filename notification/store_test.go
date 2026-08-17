package notification

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/gravityctl/free-games/common"
)

func newStore(t *testing.T) *NotificationStore {
	t.Helper()
	s, err := NewNotificationStore(filepath.Join(t.TempDir(), "store.json"))
	if err != nil {
		t.Fatalf("NewNotificationStore: %v", err)
	}
	return s
}

func epicGame(title string, start, end time.Time) common.Game {
	return common.Game{Title: title, Provider: "epic", StartDate: start, EndDate: end}
}

func TestFilterNewSuppressesWithinSameOffer(t *testing.T) {
	s := newStore(t)
	start := time.Now().Add(-2 * 24 * time.Hour)
	end := time.Now().Add(5 * 24 * time.Hour)
	game := epicGame("Beacon Pines", start, end)

	fresh, err := s.FilterNew([]common.Game{game})
	if err != nil || len(fresh) != 1 {
		t.Fatalf("first run: got %d games, err %v; want 1", len(fresh), err)
	}
	if err := s.Record(fresh); err != nil {
		t.Fatalf("Record: %v", err)
	}

	// The same live offer on a later day must not notify again.
	fresh, _ = s.FilterNew([]common.Game{game})
	if len(fresh) != 0 {
		t.Fatalf("second run: got %d games, want 0", len(fresh))
	}
}

// TestFilterNewAllowsNewOfferWindow covers giveaways that come back around: a game
// suppressed forever after its first appearance means missing every later one.
func TestFilterNewAllowsNewOfferWindow(t *testing.T) {
	s := newStore(t)

	oldStart := time.Now().Add(-400 * 24 * time.Hour)
	oldEnd := oldStart.Add(7 * 24 * time.Hour)
	if err := s.Record([]common.Game{epicGame("Beacon Pines", oldStart, oldEnd)}); err != nil {
		t.Fatalf("Record: %v", err)
	}

	newStart := time.Now().Add(-1 * 24 * time.Hour)
	newEnd := time.Now().Add(6 * 24 * time.Hour)
	fresh, _ := s.FilterNew([]common.Game{epicGame("Beacon Pines", newStart, newEnd)})
	if len(fresh) != 1 {
		t.Fatalf("got %d games, want 1 for a new offer window", len(fresh))
	}
}

func TestFilterNewSuppressesUpcomingThenLiveSameOffer(t *testing.T) {
	s := newStore(t)

	// Epic announces a giveaway before it starts, then serves it as live with the
	// same window. That is one giveaway, not two.
	start := time.Now().Add(2 * 24 * time.Hour)
	end := start.Add(7 * 24 * time.Hour)
	if err := s.Record([]common.Game{epicGame("Caravan SandWitch", start, end)}); err != nil {
		t.Fatalf("Record: %v", err)
	}

	fresh, _ := s.FilterNew([]common.Game{epicGame("Caravan SandWitch", start, end)})
	if len(fresh) != 0 {
		t.Fatalf("got %d games, want 0", len(fresh))
	}
}

func TestFilterNewDatelessOffersUseCooldown(t *testing.T) {
	s := newStore(t)
	// Steam offers carry no window, so a cooldown stands in for one.
	game := common.Game{Title: "Some Steam Game", Provider: "steam"}

	if err := s.Record([]common.Game{game}); err != nil {
		t.Fatalf("Record: %v", err)
	}
	if fresh, _ := s.FilterNew([]common.Game{game}); len(fresh) != 0 {
		t.Fatalf("got %d games, want 0 within the cooldown", len(fresh))
	}

	// Backdate the entry past the cooldown.
	s.entries[key("steam", "Some Steam Game")] = sentEntry{SentAt: time.Now().Add(-31 * 24 * time.Hour)}
	if fresh, _ := s.FilterNew([]common.Game{game}); len(fresh) != 1 {
		t.Fatalf("got %d games, want 1 after the cooldown", len(fresh))
	}

	s.SetRenotifyAfter(365 * 24 * time.Hour)
	if fresh, _ := s.FilterNew([]common.Game{game}); len(fresh) != 0 {
		t.Fatalf("got %d games, want 0 with a longer cooldown configured", len(fresh))
	}
}

// TestFilterNewDoesNotRecord is the failed-delivery case: filtering must not mark
// anything as sent, or a webhook error loses the notification permanently.
func TestFilterNewDoesNotRecord(t *testing.T) {
	s := newStore(t)
	game := epicGame("Beacon Pines", time.Now().Add(-time.Hour), time.Now().Add(time.Hour))

	for i := 0; i < 3; i++ {
		fresh, _ := s.FilterNew([]common.Game{game})
		if len(fresh) != 1 {
			t.Fatalf("attempt %d: got %d games, want 1 until delivery is recorded", i, len(fresh))
		}
	}
}

func TestStorePersistsAcrossReload(t *testing.T) {
	path := filepath.Join(t.TempDir(), "store.json")
	s, err := NewNotificationStore(path)
	if err != nil {
		t.Fatalf("NewNotificationStore: %v", err)
	}

	start := time.Now().Add(-time.Hour)
	end := time.Now().Add(48 * time.Hour)
	game := epicGame("Beacon Pines", start, end)
	if err := s.Record([]common.Game{game}); err != nil {
		t.Fatalf("Record: %v", err)
	}

	reloaded, err := NewNotificationStore(path)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if !reloaded.HasSent("epic", "Beacon Pines") {
		t.Error("HasSent: entry did not survive reload")
	}
	if fresh, _ := reloaded.FilterNew([]common.Game{game}); len(fresh) != 0 {
		t.Errorf("got %d games after reload, want 0", len(fresh))
	}
}

// TestFilterNewLegacyZeroDates covers store files written before offer windows
// were recorded, where dates were serialised as the zero time.
func TestFilterNewLegacyZeroDates(t *testing.T) {
	s := newStore(t)
	s.entries[key("epic", "Old Game")] = sentEntry{
		SentAt:    time.Now().Add(-2 * 24 * time.Hour),
		StartDate: "0001-01-01T00:00:00Z",
		EndDate:   "0001-01-01T00:00:00Z",
	}

	// Still inside the cooldown, so no repeat notification.
	game := common.Game{Title: "Old Game", Provider: "epic"}
	if fresh, _ := s.FilterNew([]common.Game{game}); len(fresh) != 0 {
		t.Fatalf("got %d games, want 0", len(fresh))
	}

	// A legacy entry records no offer window, so the cooldown still governs even
	// when the incoming game has dates. This keeps an upgrade from re-announcing
	// everything already in the store.
	live := epicGame("Old Game", time.Now().Add(-time.Hour), time.Now().Add(time.Hour))
	if fresh, _ := s.FilterNew([]common.Game{live}); len(fresh) != 0 {
		t.Fatalf("got %d games, want 0 while the cooldown is unexpired", len(fresh))
	}

	// Once the cooldown lapses, the game is announced again.
	s.entries[key("epic", "Old Game")] = sentEntry{
		SentAt:    time.Now().Add(-60 * 24 * time.Hour),
		StartDate: "0001-01-01T00:00:00Z",
		EndDate:   "0001-01-01T00:00:00Z",
	}
	if fresh, _ := s.FilterNew([]common.Game{live}); len(fresh) != 1 {
		t.Fatalf("got %d games, want 1 after the cooldown", len(fresh))
	}
}

func TestPruneDropsAncientEntries(t *testing.T) {
	s := newStore(t)
	s.entries[key("epic", "Ancient")] = sentEntry{SentAt: time.Now().Add(-400 * 24 * time.Hour)}
	s.entries[key("epic", "Recent")] = sentEntry{SentAt: time.Now().Add(-2 * 24 * time.Hour)}

	if err := s.Record([]common.Game{epicGame("Trigger", time.Now(), time.Now().Add(time.Hour))}); err != nil {
		t.Fatalf("Record: %v", err)
	}

	if s.HasSent("epic", "Ancient") {
		t.Error("ancient entry was not pruned")
	}
	if !s.HasSent("epic", "Recent") {
		t.Error("recent entry was pruned")
	}
}
