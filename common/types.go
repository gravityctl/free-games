package common

import "time"

// Game represents a free game from any provider.
type Game struct {
	Title       string
	Description string
	ImageURL    string
	URL         string
	Publisher   string
	StartDate   time.Time
	EndDate     time.Time
	Provider    string // "epic" or "steam"
}
