package steam

import (
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gravityctl/free-games/common"
)

const (
	// searchAPIURL is Steam's paging endpoint for store search. It returns JSON
	// with a total_count plus the rendered result rows, which is more stable than
	// scraping the full search page (no age gates or region interstitials).
	searchAPIURL = "https://store.steampowered.com/search/results/"
	gameAPIURL   = "https://store.steampowered.com/api/appdetails"

	// categoryGames is Steam's "Games" type filter. The store expects the numeric
	// id here; a word like "game" is silently ignored and returns DLC, soundtracks
	// and videos alongside games.
	categoryGames = "998"

	// maxSearchPages bounds paging over the candidate list.
	maxSearchPages = 4
	searchPageSize = 100
)

type Scraper struct {
	client  *http.Client
	country string
	locale  string
	// Endpoints are fields so tests can point them at a local server.
	searchURL  string
	detailsURL string
}

func NewScraper() *Scraper {
	return NewScraperFor("US", "en")
}

// NewScraperFor builds a scraper for a specific store region. The region matters
// because "free to keep" is a per-region price, so discovery and verification
// must ask for the same country.
func NewScraperFor(country, locale string) *Scraper {
	if country == "" {
		country = "US"
	}
	if locale == "" {
		locale = "en"
	}
	return &Scraper{
		client:     &http.Client{Timeout: 30 * time.Second},
		country:    country,
		locale:     locale,
		searchURL:  searchAPIURL,
		detailsURL: gameAPIURL,
	}
}

func (s *Scraper) FetchFreeGames() ([]common.Game, error) {
	candidates, err := s.findCandidates()
	if err != nil {
		return nil, err
	}

	var games []common.Game
	for _, c := range candidates {
		game, err := s.fetchGameDetails(c.AppID, c.Title)
		if err != nil || game == nil {
			continue
		}
		games = append(games, *game)
	}
	return games, nil
}

type searchResult struct {
	AppID int
	Title string
	// PriceFinal is the row's current price in cents; 0 for both free-to-keep
	// promotions and free-to-play games.
	PriceFinal int
	// Discount is the row's discount percentage; free-to-play games have none,
	// so a 100 here is the store's own "free to keep" marker.
	Discount    int
	HasDiscount bool
}

type searchQuery struct {
	params url.Values
	// serverFiltered marks a query whose price filter Steam applied itself, so
	// every row is already a zero-price candidate and no attribute parsing from
	// the row markup is required to keep it.
	serverFiltered bool
	// maxPages caps paging. One page is enough for a price-sorted sweep, where
	// anything free is on the first page by construction.
	maxPages int
}

// findCandidates collects apps that may currently be free to keep.
//
// The primary query is the store's own "free" price filter combined with
// specials, which is exact and returns a handful of rows at most. A broader sweep
// of the cheapest specials is merged in because Steam occasionally omits 100%-off
// apps from the maxprice filter, and missing a giveaway is the failure mode that
// matters most here. Both are only candidate lists — fetchGameDetails makes the
// final call on every one of them.
func (s *Scraper) findCandidates() ([]searchResult, error) {
	queries := []searchQuery{
		{
			params:         url.Values{"maxprice": {"free"}, "specials": {"1"}, "category1": {categoryGames}},
			serverFiltered: true,
			maxPages:       maxSearchPages,
		},
		{
			params:   url.Values{"specials": {"1"}, "category1": {categoryGames}, "sort_by": {"Price_ASC"}},
			maxPages: 1,
		},
	}

	var (
		out      []searchResult
		seen     = make(map[int]bool)
		firstErr error
	)
	for _, q := range queries {
		results, err := s.search(q.params, q.maxPages)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		for _, r := range results {
			if !q.serverFiltered && !isFreeToKeepRow(r) {
				continue
			}
			if seen[r.AppID] {
				continue
			}
			seen[r.AppID] = true
			out = append(out, r)
		}
	}

	if len(out) == 0 && firstErr != nil {
		return nil, firstErr
	}
	return out, nil
}

// isFreeToKeepRow keeps rows that look like a paid game currently priced at zero.
// A free-to-play game also shows a zero price but carries no discount, which is
// what separates the two.
func isFreeToKeepRow(r searchResult) bool {
	if r.PriceFinal != 0 {
		return false
	}
	if r.HasDiscount {
		return r.Discount >= 100
	}
	// No discount block was rendered; let the store API decide.
	return true
}

func (s *Scraper) search(params url.Values, maxPages int) ([]searchResult, error) {
	if maxPages <= 0 {
		maxPages = 1
	}

	var out []searchResult

	for page := 0; page < maxPages; page++ {
		q := url.Values{}
		for k, v := range params {
			q[k] = v
		}
		q.Set("query", "")
		q.Set("start", strconv.Itoa(page*searchPageSize))
		q.Set("count", strconv.Itoa(searchPageSize))
		q.Set("cc", s.country)
		q.Set("l", s.locale)
		q.Set("infinite", "1")

		req, err := http.NewRequest("GET", s.searchURL+"?"+q.Encode(), nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
		req.Header.Set("Accept-Language", "en-US,en;q=0.9")

		resp, err := s.client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("search request failed: %w", err)
		}
		body, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("search returned %d", resp.StatusCode)
		}
		if readErr != nil {
			return nil, readErr
		}

		var payload struct {
			TotalCount  int    `json:"total_count"`
			ResultsHTML string `json:"results_html"`
		}
		if err := json.Unmarshal(body, &payload); err != nil {
			return nil, fmt.Errorf("failed to parse search response: %w", err)
		}

		results := parseSearchResults(payload.ResultsHTML)
		out = append(out, results...)

		if len(results) == 0 || (page+1)*searchPageSize >= payload.TotalCount {
			break
		}
	}

	return out, nil
}

// Steam renders result rows with the title in either a <span> or a <div>
// depending on the template in use, so both are accepted.
var (
	appIDRe    = regexp.MustCompile(`data-ds-appid="(\d+)"`)
	titleRe    = regexp.MustCompile(`<(?:span|div) class="title">([^<]*)</(?:span|div)>`)
	priceRe    = regexp.MustCompile(`data-price-final="(\d+)"`)
	discountRe = regexp.MustCompile(`data-discount="(\d+)"`)
)

// parseSearchResults extracts appid, title, price and discount from Steam search
// result rows. Rows are delimited by the appid attribute: everything up to the
// next appid belongs to the current row.
func parseSearchResults(rowsHTML string) []searchResult {
	rowsHTML = html.UnescapeString(rowsHTML)

	appMatches := appIDRe.FindAllStringSubmatchIndex(rowsHTML, -1)
	if len(appMatches) == 0 {
		return nil
	}

	var results []searchResult
	for i, idx := range appMatches {
		appID, err := strconv.Atoi(rowsHTML[idx[2]:idx[3]])
		if err != nil || appID == 0 {
			continue
		}

		rowEnd := len(rowsHTML)
		if i+1 < len(appMatches) {
			rowEnd = appMatches[i+1][0]
		}
		row := rowsHTML[idx[0]:rowEnd]

		r := searchResult{AppID: appID, PriceFinal: -1}
		if m := titleRe.FindStringSubmatch(row); len(m) > 1 {
			r.Title = strings.TrimSpace(m[1])
		}
		if m := priceRe.FindStringSubmatch(row); len(m) > 1 {
			r.PriceFinal, _ = strconv.Atoi(m[1])
		}
		if m := discountRe.FindStringSubmatch(row); len(m) > 1 {
			r.Discount, _ = strconv.Atoi(m[1])
			r.HasDiscount = true
		}

		results = append(results, r)
	}

	return results
}

type priceOverview struct {
	Currency         string `json:"currency"`
	Initial          int    `json:"initial"`
	Final            int    `json:"final"`
	DiscountPercent  int    `json:"discount_percent"`
	InitialFormatted string `json:"initial_formatted"`
	FinalFormatted   string `json:"final_formatted"`
}

type appDetails struct {
	Type string `json:"type"`
	Name string `json:"name"`
	// IsFree is the app's free-to-play flag. It is a property of the app, not of
	// any promotion: a paid game given away at 100% off still reports false.
	IsFree bool `json:"is_free"`
	// PriceOverview is absent for free-to-play apps and present, with a final
	// price of 0, for a paid game currently given away.
	PriceOverview *priceOverview `json:"price_overview"`
	Publishers    []string       `json:"publishers"`
	Developers    []string       `json:"developers"`
	ShortDesc     string         `json:"short_description"`
	HeaderImage   string         `json:"header_image"`
}

// fetchGameDetails confirms an app is a paid game currently free to keep and
// builds the notification from Steam's own store data. It returns nil for
// anything that fails the check, including free-to-play apps and DLC.
func (s *Scraper) fetchGameDetails(appID int, fallbackTitle string) (*common.Game, error) {
	q := url.Values{
		"appids": {strconv.Itoa(appID)},
		"cc":     {s.country},
		"l":      {s.locale},
	}
	req, err := http.NewRequest("GET", s.detailsURL+"?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, nil
	}

	// The API replies keyed by appid: {"<appid>": {"success": true, "data": {...}}}
	var result map[string]struct {
		Success bool       `json:"success"`
		Data    appDetails `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, nil
	}

	appResult, ok := result[strconv.Itoa(appID)]
	if !ok || !appResult.Success {
		return nil, nil
	}
	data := appResult.Data

	// Games only — not DLC, demos, soundtracks or videos.
	if data.Type != "game" {
		return nil, nil
	}
	if !isFreeToKeep(data) {
		return nil, nil
	}

	title := data.Name
	if title == "" {
		title = fallbackTitle
	}
	publisher := ""
	if len(data.Publishers) > 0 {
		publisher = data.Publishers[0]
	} else if len(data.Developers) > 0 {
		publisher = data.Developers[0]
	}

	return &common.Game{
		Title:         title,
		Description:   data.ShortDesc,
		ImageURL:      data.HeaderImage,
		URL:           fmt.Sprintf("https://store.steampowered.com/app/%d/", appID),
		Provider:      "steam",
		Publisher:     publisher,
		OriginalPrice: originalPrice(data.PriceOverview),
	}, nil
}

// isFreeToKeep reports whether a paid app is currently discounted to nothing.
//
// A free-to-play app carries is_free=true and no price_overview, so it is never
// a giveaway. A giveaway is the opposite shape: a real price_overview whose final
// price has been discounted to zero.
func isFreeToKeep(data appDetails) bool {
	p := data.PriceOverview
	if p == nil {
		return false
	}
	if p.Final != 0 {
		return false
	}
	// Guard against an app that is simply priced at zero with no promotion.
	return p.DiscountPercent > 0 || p.Initial > 0
}

func originalPrice(p *priceOverview) string {
	if p == nil {
		return ""
	}
	if p.InitialFormatted != "" {
		return p.InitialFormatted
	}
	if p.Initial > 0 {
		return fmt.Sprintf("%.2f %s", float64(p.Initial)/100, p.Currency)
	}
	return ""
}
