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

// Scraper fetches free games from Steam.
type Scraper struct {
	client *http.Client
}

// NewScraper returns a Steam scraper.
func NewScraper() *Scraper {
	return &Scraper{
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

// FetchFreeGames returns currently-free paid games (excludes Free-to-Play).
func (s *Scraper) FetchFreeGames() ([]common.Game, error) {
	// Try to find free games via the specials page HTML
	games, err := s.scrapeFreeGamesFromSearch()
	if err != nil {
		return nil, err
	}
	return games, nil
}

// searchResult represents a parsed game from search results.
type searchResult struct {
	AppID      int
	Title      string
	IsFree     bool
	OriginalPx int // in cents, 0 means unknown
}

// scrapeFreeGamesFromSearch fetches Steam search and looks for free games.
func (s *Scraper) scrapeFreeGamesFromSearch() ([]common.Game, error) {
	req, err := http.NewRequest("GET", searchURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36")
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

	// Parse game entries. Steam search results are in:
	// <a href="/app/APPID/GameName/..." data-price="PRICE_CENTS">...</a>
	// We look for entries where data-price is "0" (free).
	results, err := s.parseSearchResults(string(body))
	if err != nil {
		return nil, err
	}

	var games []common.Game
	for _, r := range results {
		if !r.IsFree {
			continue
		}
		// Verify it's not F2P by checking the game API
		game, err := s.fetchGameDetails(r.AppID)
		if err != nil {
			continue
		}
		if game != nil {
			games = append(games, *game)
		}
	}

	return games, nil
}

// parseSearchResults extracts game entries from HTML.
func (s *Scraper) parseSearchResults(html string) ([]searchResult, error) {
	// Match: <a class="search_result_row" data-ds-appid="APPID" href="/app/APPID/...">...</a>
	// Inside the row: the price is in data-price attribute or as text like "Free" or "$9.99"
	// Pattern for price: data-price="1234" (cents) or data-price-is-freetype="1"
	var results []searchResult

	// Find all app entries - they have data-ds-appid
	appIDRe := regexp.MustCompile(`data-ds-appid="(\d+)"`)
	rowRe := regexp.MustCompile(`<a class="search_result_row[^"]*"[^>]*data-ds-appid="(\d+)"[^>]*>.*?</a>`, regexp.DotAll)

	matches := rowRe.FindAllStringSubmatch(html, -1)
	for _, m := range matches {
		if len(m) < 2 {
			continue
		}
		appID, _ := strconv.Atoi(m[1])

		// Find the full row HTML
		rowStart := strings.Index(html, `data-ds-appid="`+m[1]+`"`)
		if rowStart == -1 {
			continue
		}
		rowEnd := strings.Index(html[rowStart:], "</a>")
		if rowEnd == -1 {
			continue
		}
		rowHTML := html[rowStart : rowStart+rowEnd+len("</a>")]

		// Extract title
		titleRe := regexp.MustCompile(`<div class="title">([^<]+)</div>`)
		titleMatch := titleRe.FindStringSubmatch(rowHTML)
		title := ""
		if len(titleMatch) > 1 {
			title = strings.TrimSpace(titleMatch[1])
		}

		// Extract price - data-price attribute has it in cents
		priceRe := regexp.MustCompile(`data-price="(\d+)"`)
		priceMatch := priceRe.FindStringSubmatch(rowHTML)
		isFree := false
		var origPx int
		if len(priceMatch) > 1 {
			origPx, _ = strconv.Atoi(priceMatch[1])
			isFree = origPx == 0
		}

		if isFree && appID > 0 && title != "" {
			results = append(results, searchResult{
				AppID:      appID,
				Title:      title,
				IsFree:     isFree,
				OriginalPx: origPx,
			})
		}
	}

	return results, nil
}

// fetchGameDetails checks if a free game is paid-game-freed or actually F2P.
// Returns nil if it's F2P (should be excluded).
func (s *Scraper) fetchGameDetails(appID int) (*common.Game, error) {
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
			Type     string `json:"type"`
			Name     string `json:"name"`
			IsFree   bool   `json:"is_free"`
			Price    struct {
				Currency string `json:"currency"`
				Initial  int    `json:"initial"`
				Final    int    `json:"final"`
			} `json:"price_overview"`
			HeaderImage string `json:"header_image"`
			PCGWS       struct {
				StoreBundleName string `json:"store_bundle_name"`
			} `json:"pcgws_finding_label"`
			FullGame `json:"fullgame"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, nil
	}

	if !result.Success {
		return nil, nil
	}

	// If type is "game" and is_free is true, it's a free game (not F2P).
	// If type is "application" or "video" it's not a game.
	// If it has a non-zero original price, it's a paid game temporarily free.
	if result.Data.Type != "game" {
		return nil // not a game, skip
	}

	// If is_free is true and original price > 0, it's a free game
	// If is_free is true and original price == 0, it's F2P
	if !result.Data.IsFree {
		return nil // not free
	}

	// Check original price - if it has a non-zero initial price, it's a paid game
	// temporarily free (not F2P)
	if result.Data.Price.Initial > 0 {
		return &common.Game{
			Title:       result.Data.Name,
			Description: "", // not available from this API
			ImageURL:    result.Data.HeaderImage,
			URL:         fmt.Sprintf("https://store.steampowered.com/app/%d/", appID),
			Publisher:   "", // not available from this API
			Provider:    "steam",
		}, nil
	}

	// is_free=true but no price = F2P, skip
	return nil
}
