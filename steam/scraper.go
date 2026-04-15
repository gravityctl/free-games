package steam

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gravityctl/free-games/common"
)

const (
	searchURL  = "https://store.steampowered.com/search/?sort_by=Price_Desc&category1=game&hidef2p=1&empty=1"
	gameAPIURL = "https://store.steampowered.com/api/appdetails?appids=%d&cc=US&l=en"
)

type Scraper struct {
	client *http.Client
}

func NewScraper() *Scraper {
	return &Scraper{client: &http.Client{Timeout: 30 * time.Second}}
}

func (s *Scraper) FetchFreeGames() ([]common.Game, error) {
	return s.scrapeFreeGamesFromSearch()
}

type searchResult struct {
	AppID  int
	Title  string
	IsFree bool
}

func (s *Scraper) scrapeFreeGamesFromSearch() ([]common.Game, error) {
	req, err := http.NewRequest("GET", searchURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
	req.Header.Set("Accept-Language", "en-US")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("search request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("search returned %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	results, err := s.parseSearchResults(string(body))
	if err != nil {
		return nil, err
	}

	var games []common.Game
	for _, r := range results {
		if !r.IsFree {
			continue
		}
		game, err := s.fetchGameDetails(r.AppID, r.Title)
		if err != nil {
			continue
		}
		if game != nil {
			games = append(games, *game)
		}
	}

	return games, nil
}

func (s *Scraper) parseSearchResults(html string) ([]searchResult, error) {
	// Match each search result row
	rowRe := regexp.MustCompile(`<a class="search_result_row[^"]*"[^>]*data-ds-appid="(\d+)"[^>]*>[\s\S]*?</a>`)
	matches := rowRe.FindAllStringSubmatch(html, -1)

	var results []searchResult
	for _, m := range matches {
		if len(m) < 2 {
			continue
		}
		appID, _ := strconv.Atoi(m[1])
		rowHTML := m[0]

		// Extract title
		titleRe := regexp.MustCompile(`<div class="title">([^<]+)</div>`)
		titleMatch := titleRe.FindStringSubmatch(rowHTML)
		title := ""
		if len(titleMatch) > 1 {
			title = strings.TrimSpace(titleMatch[1])
		}

		// Extract price - data-price attribute in cents
		priceRe := regexp.MustCompile(`data-price="(\d+)"`)
		priceMatch := priceRe.FindStringSubmatch(rowHTML)
		isFree := false
		if len(priceMatch) > 1 {
			price, _ := strconv.Atoi(priceMatch[1])
			isFree = price == 0
		}

		if appID > 0 && title != "" {
			results = append(results, searchResult{
				AppID:  appID,
				Title:  title,
				IsFree: isFree,
			})
		}
	}

	return results, nil
}

func (s *Scraper) fetchGameDetails(appID int, fallbackTitle string) (*common.Game, error) {
	url := fmt.Sprintf(gameAPIURL, appID)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, nil
	}

	var result struct {
		Success bool `json:"success"`
		Data    struct {
			Type       string `json:"type"`
			Name       string `json:"name"`
			IsFree     bool   `json:"is_free"`
			Price      struct {
				Currency string `json:"currency"`
				Initial  int    `json:"initial"`
				Final    int    `json:"final"`
			} `json:"price_overview"`
			HeaderImage string `json:"header_image"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, nil
	}

	if !result.Success {
		return nil, nil
	}

	// Only include paid games (type=game with non-zero original price that are currently free)
	// Skip F2P games (is_free=true but initial_price=0)
	if result.Data.Type != "game" {
		return nil, nil
	}

	if !result.Data.IsFree {
		return nil, nil
	}

	// If initial price > 0, it's a paid game temporarily free
	if result.Data.Price.Initial > 0 {
		title := result.Data.Name
		if title == "" {
			title = fallbackTitle
		}
		return &common.Game{
			Title:    title,
			ImageURL: result.Data.HeaderImage,
			URL:      fmt.Sprintf("https://store.steampowered.com/app/%d/", appID),
			Provider: "steam",
		}, nil
	}

	// is_free=true with no price = F2P, skip
	return nil, nil
}
