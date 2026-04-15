package twitch

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/gravityctl/free-games/common"
)

const dropsAPI = "https://twitch-drops-api.sunkwi.com/drops"

type Client struct {
	enabledOwners map[string]bool
	client        *http.Client
}

func NewClient(enabledOwners map[string]bool) *Client {
	return &Client{
		enabledOwners: enabledOwners,
		client:        &http.Client{Timeout: 30 * time.Second},
	}
}

type APIResponse []Drop

type Drop struct {
	EndAt          string   `json:"endAt"`
	GameBoxArtURL  string   `json:"gameBoxArtURL"`
	GameDisplayName string `json:"gameDisplayName"`
	GameID         string   `json:"gameId"`
	Rewards        []Reward `json:"rewards"`
	StartAt        string   `json:"startAt"`
}

type Reward struct {
	Allow struct {
		Channels []struct {
			DisplayName string `json:"displayName"`
			ID          string `json:"id"`
			Name        string `json:"name"`
		} `json:"channels"`
		IsEnabled bool `json:"isEnabled"`
	} `json:"allow"`
	Description    string `json:"description"`
	DetailsURL     string `json:"detailsURL"`
	Name           string `json:"name"`
	Owner          *struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"owner"`
}

// FetchDrops fetches active Twitch drops for enabled owners.
func (c *Client) FetchDrops() ([]common.Game, error) {
	resp, err := c.client.Get(dropsAPI)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch drops: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API returned %d: %s", resp.StatusCode, string(body))
	}

	var drops APIResponse
	if err := json.NewDecoder(resp.Body).Decode(&drops); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	var games []common.Game
	now := time.Now()

	for _, drop := range drops {
		// Check if this drop's owner is enabled
		ownerName := ""
		if len(drop.Rewards) > 0 && drop.Rewards[0].Owner != nil {
			ownerName = drop.Rewards[0].Owner.Name
		}
		if ownerName == "" {
			ownerName = drop.GameDisplayName
		}

		if !c.isOwnerEnabled(ownerName) {
			continue
		}

		start, err := time.Parse(time.RFC3339, drop.StartAt)
		if err != nil {
			continue
		}
		end, err := time.Parse(time.RFC3339, drop.EndAt)
		if err != nil {
			continue
		}

		// Only include if active or upcoming within 7 days
		if now.After(end) {
			continue
		}
		if start.Before(now.AddDate(0, 0, -7)) && start.After(now) == false {
			continue // Started more than 7 days ago and already active
		}

		games = append(games, common.Game{
			Title:       drop.GameDisplayName,
			Description: drop.Rewards[0].Description,
			ImageURL:    drop.GameBoxArtURL,
			URL:         drop.Rewards[0].DetailsURL,
			Publisher:   ownerName,
			StartDate:   start,
			EndDate:     end,
			Provider:    "twitch",
		})
	}

	return games, nil
}

func (c *Client) isOwnerEnabled(owner string) bool {
	if len(c.enabledOwners) == 0 {
		return false
	}
	return c.enabledOwners[owner]
}
