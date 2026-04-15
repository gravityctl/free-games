package epic

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const graphqlURL = "https://graphql.epicgames.com/graphql"

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

type graphQLResponse struct {
	Data struct {
		Catalog struct {
			SearchStore struct {
				Elements []struct {
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
					URL         string `json:"url"`
					UrlSlug     string `json:"urlSlug"`
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
				} `json:"elements"`
			} `json:"searchStore"`
		} `json:"Catalog"`
	} `json:"data"`
}

func NewClient(country, locale string) *Client {
	return &Client{country: country, locale: locale}
}

func (c *Client) FetchFreeGames() ([]Game, error) {
	query := `query searchStoreQuery($category: String, $count: Int, $country: String!, $locale: String, $freeGame: Boolean, $onSale: Boolean) {
  Catalog {
    searchStore(
      category: $category
      count: $count
      country: $country
      freeGame: $freeGame
      onSale: $onSale
      locale: $locale
    ) {
      elements {
        title
        description
        seller { name }
        keyImages { type url }
        productSlug
        url
        urlSlug
        catalogNs {
          mappings(pageType: "productHome") {
            pageSlug
            pageType
          }
        }
        promotions {
          promotionalOffers {
            promotionalOffers {
              startDate
              endDate
            }
          }
          upcomingPromotionalOffers {
            promotionalOffers {
              startDate
              endDate
            }
          }
        }
      }
    }
  }
}`

	variables := map[string]interface{}{
		"category": "games/edition/base|bundles/games|editors",
		"count":    50,
		"country":  c.country,
		"locale":   c.locale,
		"freeGame": true,
		"onSale":   true,
	}

	body, err := c.doGraphQL(query, variables)
	if err != nil {
		return nil, err
	}

	var resp graphQLResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	var games []Game
	now := time.Now()

	for _, el := range resp.Data.Catalog.SearchStore.Elements {
		// Check current promotional offers
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

				// Only include if currently active (or starting today)
				if now.Before(end) && !now.Before(start) {
					games = append(games, buildGame(el, start, end))
				}
			}
		}

		// Check upcoming offers (only if within ~7 days)
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

				// Only include upcoming games starting within next 7 days
				if start.After(now) && start.Before(now.AddDate(0, 0, 7)) {
					games = append(games, buildGame(el, start, end))
				}
			}
		}
	}

	return games, nil
}

func buildGame(el struct {
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
	URL         string `json:"url"`
	UrlSlug     string `json:"urlSlug"`
	CatalogNs   struct {
		Mappings []struct {
			PageSlug string `json:"pageSlug"`
			PageType string `json:"pageType"`
		} `json:"mappings"`
	} `json:"catalogNs"`
}, start, end time.Time) Game {
	var imageURL, storeURL string

	for _, img := range el.KeyImages {
		if img.Type == "featuredMedia" || img.Type == "OfferImageWide" {
			imageURL = img.URL
			break
		}
	}

	// Build store URL
	if el.ProductSlug != "" {
		storeURL = fmt.Sprintf("https://store.epicgames.com/en-US/p/%s", el.ProductSlug)
	} else if el.UrlSlug != "" {
		storeURL = fmt.Sprintf("https://store.epicgames.com/en-US/p/%s", el.UrlSlug)
	} else if el.CatalogNs.Mappings != nil {
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

func (c *Client) doGraphQL(query string, variables map[string]interface{}) ([]byte, error) {
	payload := map[string]interface{}{
		"query":     query,
		"variables": variables,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", graphqlURL, bytes.NewBuffer(body))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("non-200 response: %d: %s", resp.StatusCode, string(b))
	}

	return io.ReadAll(resp.Body)
}
