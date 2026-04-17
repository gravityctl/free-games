package epic

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gravityctl/free-games/common"
)

const freeGamesURL = "https://store-site-backend-static.ak.epicgames.com/freeGamesPromotions"

type Client struct {
	country         string
	locale          string
	includeUpcoming bool
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
	ProductSlug string `json:"productSlug"`
	URLSlug     string `json:"urlSlug"`
	CatalogNs   struct {
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
	Price struct {
		TotalPrice struct {
			FmtPrice struct {
				OriginalPrice  string `json:"originalPrice"`
				DiscountPrice string `json:"discountPrice"`
			} `json:"fmtPrice"`
		} `json:"totalPrice"`
	} `json:"price"`
}

type restResponse struct {
	Data struct {
		Catalog struct {
			SearchStore struct {
				Elements []catalogElement `json:"elements"`
			} `json:"searchStore"`
		} `json:"Catalog"`
	} `json:"data"`
}

func NewClient(country, locale string, includeUpcoming bool) *Client {
	return &Client{country: country, locale: locale, includeUpcoming: includeUpcoming}
}

func (c *Client) FetchFreeGames() ([]common.Game, error) {
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

	var r restResponse
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	var games []common.Game
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
					games = append(games, buildGame(el, start, end))
				}
			}
		}

		// Check upcoming
		if c.includeUpcoming {
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
						games = append(games, buildGame(el, start, end))
					}
				}
			}
		}
	}

	return games, nil
}

func buildGame(el catalogElement, start, end time.Time) common.Game {
	var imageURL, storeURL string

	for _, img := range el.KeyImages {
		if img.Type == "featuredMedia" || img.Type == "OfferImageWide" {
			imageURL = img.URL
			break
		}
	}

	if el.ProductSlug != "" && !strings.HasSuffix(el.ProductSlug, "/home") {
		storeURL = fmt.Sprintf("https://store.epicgames.com/en-US/p/%s", el.ProductSlug)
	} else if len(el.CatalogNs.Mappings) > 0 {
		pageSlug := el.CatalogNs.Mappings[0].PageSlug
		storeURL = fmt.Sprintf("https://store.epicgames.com/en-US/p/%s", pageSlug)
	} else if el.URLSlug != "" {
		storeURL = fmt.Sprintf("https://store.epicgames.com/en-US/p/%s", el.URLSlug)
	}

	return common.Game{
		Title:         el.Title,
		Description:   el.Description,
		ImageURL:      imageURL,
		URL:           storeURL,
		Publisher:     el.Seller.Name,
		OriginalPrice: el.Price.TotalPrice.FmtPrice.OriginalPrice,
		StartDate:     start,
		EndDate:       end,
		Provider:      "epic",
	}
}
