package twitch

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/gravityctl/free-games/common"
)

const (
	dropsAPI     = "https://twitch-drops-api.sunkwi.com/drops"
	gameSearchURL = "https://api.isthereanydeal.com/v01/search/?key=%s&q=%s&limit=1"
	// Platform identifiers used in ITAD API
	Steam   = "steam"
	GOG     = "gog"
	Epic    = "epic"
	Amazon  = "amazon"
)

// Platform stores known to ITAD
var KnownPlatforms = []string{Steam, GOG, Epic, Amazon}

type Client struct {
	enabledPlatforms map[string]bool
	itadAPIKey      string
	client          *http.Client
}

// NewClient creates a Twitch drops client with platform filtering.
// enabledPlatforms is a map of platform name -> enabled (e.g. {"steam": true, "gog": true})
// itadAPIKey is an optional isthereanydeal.com API key for cross-platform lookups
func NewClient(enabledPlatforms map[string]bool, itadAPIKey string) *Client {
	return &Client{
		enabledPlatforms: enabledPlatforms,
		itadAPIKey:       itadAPIKey,
		client:           &http.Client{Timeout: 30 * time.Second},
	}
}

type APIResponse []Drop

type Drop struct {
	EndAt           string   `json:"endAt"`
	GameBoxArtURL   string   `json:"gameBoxArtURL"`
	GameDisplayName string   `json:"gameDisplayName"`
	GameID          string   `json:"gameId"`
	Rewards         []Reward `json:"rewards"`
	StartAt         string   `json:"startAt"`
}

type Reward struct {
	Allow      *Allow   `json:"allow"`
	Description string   `json:"description"`
	DetailsURL string   `json:"detailsURL"`
	Name       string   `json:"name"`
	Owner      *Owner   `json:"owner"`
}

type Allow struct {
	Channels []Channel `json:"channels"`
	IsEnabled bool     `json:"isEnabled"`
}

type Channel struct {
	DisplayName string `json:"displayName"`
	ID          string `json:"id"`
	Name        string `json:"name"`
}

type Owner struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// FetchDrops fetches active Twitch drops for enabled platforms only.
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
		start, err := time.Parse(time.RFC3339, drop.StartAt)
		if err != nil {
			continue
		}
		end, err := time.Parse(time.RFC3339, drop.EndAt)
		if err != nil {
			continue
		}

		if now.After(end) {
			continue
		}
		if start.Before(now.AddDate(0, 0, -7)) && start.After(now) == false {
			continue
		}

		games = append(games, common.Game{
			Title:       drop.GameDisplayName,
			Description: dropDescription(drop),
			ImageURL:    drop.GameBoxArtURL,
			URL:         dropURL(drop),
			Publisher:   dropOwner(drop),
			StartDate:   start,
			EndDate:     end,
			Provider:    "twitch",
		})
	}

	return games, nil
}

// FetchDropsWithPlatformFilter fetches drops and filters by enabled platforms.
// Uses isthereanydeal.com to determine cross-platform availability.
func (c *Client) FetchDropsWithPlatformFilter() ([]common.Game, error) {
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
		start, err := time.Parse(time.RFC3339, drop.StartAt)
		if err != nil {
			continue
		}
		end, err := time.Parse(time.RFC3339, drop.EndAt)
		if err != nil {
			continue
		}

		if now.After(end) {
			continue
		}
		if start.Before(now.AddDate(0, 0, -7)) && start.After(now) == false {
			continue
		}

		// Filter by platform availability
		if !c.matchesPlatform(drop.GameDisplayName) {
			continue
		}

		games = append(games, common.Game{
			Title:       drop.GameDisplayName,
			Description: dropDescription(drop),
			ImageURL:    drop.GameBoxArtURL,
			URL:         dropURL(drop),
			Publisher:   dropOwner(drop),
			StartDate:   start,
			EndDate:     end,
			Provider:    "twitch",
		})
	}

	return games, nil
}

// matchesPlatform checks if a game is available on any of the enabled platforms.
// Uses isthereanydeal.com API if key is configured.
func (c *Client) matchesPlatform(gameName string) bool {
	if len(c.enabledPlatforms) == 0 {
		return false
	}

	// If no ITAD key, accept all drops (platform info unavailable)
	if c.itadAPIKey == "" {
		return true
	}

	shops, err := c.getGameShops(gameName)
	if err != nil || len(shops) == 0 {
		return true // If lookup fails, include to avoid false negatives
	}

	for _, shop := range shops {
		if c.enabledPlatforms[shop] {
			return true
		}
	}
	return false
}

// getGameShops queries isthereanydeal.com for shops selling a game.
func (c *Client) getGameShops(gameName string) ([]string, error) {
	searchURL := fmt.Sprintf(gameSearchURL, url.QueryEscape(c.itadAPIKey), url.QueryEscape(gameName))
	req, err := http.NewRequest("GET", searchURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ITAD returned %d", resp.StatusCode)
	}

	var result struct {
		Data []struct {
			ID    string `json:"id"`
			Title string `json:"title"`
			Shops []struct {
				ID   string `json:"id"`
				Name string `json:"name"`
			} `json:"shops"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	if len(result.Data) == 0 {
		return nil, nil
	}

	var shops []string
	for _, shop := range result.Data[0].Shops {
		shops = append(shops, shop.ID)
	}
	return shops, nil
}

// getGameID gets the ITAD game ID for a title.
func (c *Client) getGameID(gameName string) (string, error) {
	searchURL := fmt.Sprintf(gameSearchURL, url.QueryEscape(c.itadAPIKey), url.QueryEscape(gameName))
	req, err := http.NewRequest("GET", searchURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("ITAD returned %d", resp.StatusCode)
	}

	var result struct {
		Data []struct {
			ID    string `json:"id"`
			Title string `json:"title"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}

	if len(result.Data) == 0 {
		return "", nil
	}
	return result.Data[0].ID, nil
}

func dropDescription(drop Drop) string {
	if len(drop.Rewards) == 0 {
		return ""
	}
	return drop.Rewards[0].Description
}

func dropURL(drop Drop) string {
	if len(drop.Rewards) == 0 {
		return ""
	}
	return drop.Rewards[0].DetailsURL
}

func dropOwner(drop Drop) string {
	if len(drop.Rewards) == 0 || drop.Rewards[0].Owner == nil {
		return "Twitch Gaming"
	}
	return drop.Rewards[0].Owner.Name
}

// trimWhitespace removes excessive whitespace from a string.
func trimWhitespace(s string) string {
	re := regexp.MustCompile(`\s+`)
	return strings.TrimSpace(re.ReplaceAllString(s, " "))
}