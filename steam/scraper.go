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
	// sort_by=Price_ASC + specials=1: surfaces only promotional "free to keep" games
	// (games on sale, not permanently free F2P). Jon's 5 expected games appear in top 5 results.
	searchURL  = "https://store.steampowered.com/search/?sort_by=Price_ASC&category1=game&specials=1"
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

	results, err := parseSearchResults(string(body))
	if err != nil {
		return nil, err
	}

	var games []common.Game
	for _, r := range results {
		if !r.IsFree {
			continue
		}
		game, err := s.fetchGameDetails(r.AppID, r.Title)
		if err != nil || game == nil {
			continue
		}
		games = append(games, *game)
	}

	return games, nil
}

// parseSearchResults extracts appid, title, and free status from Steam search HTML.
// Steam changed their HTML: titles moved from <div class="title"> to <span class="title">,
// and price attr changed from data-price to data-price-final.
var (
	appIDRe = regexp.MustCompile(`data-ds-appid="(\d+)"`)
	titleRe = regexp.MustCompile(`<span class="title">([^<]+)</span>`)
)

func parseSearchResults(html string) ([]searchResult, error) {
	appMatches := appIDRe.FindAllSubmatchIndex([]byte(html), -1)
	if len(appMatches) == 0 {
		return nil, nil
	}

	var results []searchResult
	for i, idx := range appMatches {
		appID, _ := strconv.Atoi(html[idx[2]:idx[3]])
		if appID == 0 {
			continue
		}

		// Determine row bounds using appid positions
		rowStart := idx[0]
		rowEnd := len(html)
		if i+1 < len(appMatches) {
			rowEnd = appMatches[i+1][0]
		}
		rowHTML := html[rowStart:rowEnd]

		// Extract title
		titleMatch := titleRe.FindStringSubmatch(rowHTML)
		title := ""
		if len(titleMatch) > 1 {
			title = strings.TrimSpace(titleMatch[1])
		}

		// Extract price — Steam uses data-price-final (not data-price)
		// Within the row, the first data-price-final is the game's price
		priceRe := regexp.MustCompile(`data-price-final="(\d+)"`)
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
		return nil, nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, nil
	}

	// Steam API returns {"appid": {"success": true, "data": {...}}}
	// not {"success": true, "data": {...}}
	var result map[string]struct {
		Success bool `json:"success"`
		Data    struct {
			Type        string   `json:"type"`
			Name        string   `json:"name"`
			IsFree      bool     `json:"is_free"`
			Publishers  []string `json:"publishers"`
			Price       struct {
				Currency        string `json:"currency"`
				Initial         int    `json:"initial_price"`
				Final           int    `json:"final_price"`
				DiscountPercent int    `json:"discount_percent"`
				InitialFormatted string `json:"initial_formatted"`
				FinalFormatted  string `json:"final_formatted"`
			} `json:"price_overview"`
			HeaderImage string `json:"header_image"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, nil
	}

	// result is keyed by appid string
	appResult, ok := result[fmt.Sprintf("%d", appID)]
	if !ok {
		return nil, nil
	}
	if !appResult.Success {
		return nil, nil
	}

	// Only include games (not DLC, demos, etc.)
	if appResult.Data.Type != "game" {
		return nil, nil
	}

	// Only include games that were paid and are now free.
	// F2P / permanently free games have discount_percent=0.
	// "Free to keep" / promotional free games have discount_percent=100.
	// initial_price may be null (unmarshal to 0), so check discount_percent instead.
	if !appResult.Data.IsFree || appResult.Data.Price.DiscountPercent == 0 {
		return nil, nil
	}

	title := appResult.Data.Name
	if title == "" {
		title = fallbackTitle
	}
	publisher := ""
	if len(appResult.Data.Publishers) > 0 {
		publisher = appResult.Data.Publishers[0]
	}
	originalPrice := appResult.Data.Price.InitialFormatted
	return &common.Game{
		Title:         title,
		ImageURL:      appResult.Data.HeaderImage,
		URL:           fmt.Sprintf("https://store.steampowered.com/app/%d/", appID),
		Provider:      "steam",
		Publisher:     publisher,
		OriginalPrice: originalPrice,
	}, nil
}
