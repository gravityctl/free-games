package epic

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const freeGamesURL = "https://store-site-backend-static.ak.epicgames.com/freeGamesPromotions"

type Client struct {
	country string
	locale  string
}

type Game struct {
	Title       string
	Description string
	ImageURL    string
	URL         string
	Publisher   string
	StartDate   time.Time
	EndDate     time.Time
}

func NewClient(country, locale string) *Client {
	return &Client{country: country, locale: locale}
}

type response struct {
	Data struct {
		Catalog struct {
			SearchStore struct {
				Elements []catalogElement `json:"elements"`
			} `json:"searchStore"`
		} `json:"Catalog"`
	} `json:"data"`
}

type catalogElement struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Seller      struct {
		Name string `json:"name"`
	} `json:"seller"`
	KeyImages []struct {
		Type string `json:"type"`
		URL  string `json:"url"`
	} `json:"keyImages"`
	ProductSlug   string `json:"productSlug"`
	URLSlug       string `json:"urlSlug"`
	CatalogNs     struct {
		Mappings []struct {
			PageSlug string `json:"pageSlug"`
			PageType string `json:"pageType"`
		} `json:"mappings"`
	} `json:"catalogNs"`
	Promotions struct {
		PromotionalOffers []struct {
			PromotionalOffers []struct {
				StartDate string `json:"startDate"`
				EndDate   string `json:"endDate"`
			} `json:"promotionalOffers"`
		} `json:"promotionalOffers"`
		UpcomingPromotionalOffers []struct {
			PromotionalOffers []struct {
				StartDate string `json:"startDate"`
				EndDate   string `json:"endDate"`
			} `json:"promotionalOffers"`
		} `json:"upcomingPromotionalOffers"`
	} `json:"promotions"`
}

func (c *Client) FetchFreeGames() ([]Game, error) {
	url := fmt.Sprintf("%s?locale=%s&country=%s", freeGamesURL, c.locale, c.country)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("non-200 response: %d: %s", resp.StatusCode, string(body))
	}

	var r response
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	var games []Game
	now := time.Now()

	for _, el := range r.Data.Catalog.SearchStore.Elements {
		// Check current promotions
		for _, promo := range el.Promotions.PromotionalOffers {
			for _, offer := range promo.PromotionalOffers {
				start, err := time.Parse(time.RFC3339, offer.StartDate)
				if err != nil {
					continue
				}
				end, err := time.Parse(time.RFC3339, offer.EndDate)
				if err != nil {
					continue
				}

				if now.Before(end) && !now.Before(start) {
					games = append(games, c.buildGame(el, start, end))
				}
			}
		}

		// Check upcoming (starting within 7 days)
		for _, promo := range el.Promotions.UpcomingPromotionalOffers {
			for _, offer := range promo.PromotionalOffers {
				start, err := time.Parse(time.RFC3339, offer.StartDate)
				if err != nil {
					continue
				}
				end, err := time.Parse(time.RFC3339, offer.EndDate)
				if err != nil {
					continue
				}

				if start.After(now) && start.Before(now.AddDate(0, 0, 7)) {
					games = append(games, c.buildGame(el, start, end))
				}
			}
		}
	}

	return games, nil
}

func (c *Client) buildGame(el catalogElement, start, end time.Time) Game {
	var imageURL, storeURL string

	for _, img := range el.KeyImages {
		if img.Type == "featuredMedia" || img.Type == "OfferImageWide" {
			imageURL = img.URL
			break
		}
	}

	if el.ProductSlug != "" {
		storeURL = fmt.Sprintf("https://store.epicgames.com/en-US/p/%s", el.ProductSlug)
	} else if el.URLSlug != "" {
		storeURL = fmt.Sprintf("https://store.epicgames.com/en-US/p/%s", el.URLSlug)
	} else if len(el.CatalogNs.Mappings) > 0 {
		storeURL = fmt.Sprintf("https://store.epicgames.com/en-US/p/%s", el.CatalogNs.Mappings[0].PageSlug)
	}

	return Game{
		Title:       el.Title,
		Description: el.Description,
		ImageURL:    imageURL,
		URL:         storeURL,
		Publisher:   el.Seller.Name,
		StartDate:   start,
		EndDate:     end,
	}
}
