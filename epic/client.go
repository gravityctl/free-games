package epic

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/gravityctl/free-games/common"
)

const freeGamesURL = "https://store-site-backend-static.ak.epicgames.com/freeGamesPromotions"

// upcomingWindow is how far ahead an upcoming giveaway may start to be reported.
const upcomingWindow = 7 * 24 * time.Hour

type Client struct {
	country         string
	locale          string
	includeUpcoming bool
}

type discountSetting struct {
	DiscountType string `json:"discountType"`
	// DiscountPercentage is the percentage of the original price the buyer still
	// pays, so 0 means the offer is a 100% giveaway and anything above 0 is an
	// ordinary sale.
	DiscountPercentage *int `json:"discountPercentage"`
}

type promotionalOffer struct {
	StartDate       string           `json:"startDate"`
	EndDate         string           `json:"endDate"`
	DiscountSetting *discountSetting `json:"discountSetting"`
}

type promotionGroup struct {
	PromotionalOffers []promotionalOffer `json:"promotionalOffers"`
}

type pageMapping struct {
	PageSlug string `json:"pageSlug"`
	PageType string `json:"pageType"`
}

type catalogElement struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Status      string `json:"status"`
	OfferType   string `json:"offerType"`
	Seller      struct {
		Name string `json:"name"`
	} `json:"seller"`
	KeyImages []struct {
		Type string `json:"type"`
		URL  string `json:"url"`
	} `json:"keyImages"`
	ProductSlug   *string       `json:"productSlug"`
	URLSlug       string        `json:"urlSlug"`
	OfferMappings []pageMapping `json:"offerMappings"`
	CatalogNs     struct {
		Mappings []pageMapping `json:"mappings"`
	} `json:"catalogNs"`
	Categories []struct {
		Path string `json:"path"`
	} `json:"categories"`
	Promotions *struct {
		PromotionalOffers         []promotionGroup `json:"promotionalOffers"`
		UpcomingPromotionalOffers []promotionGroup `json:"upcomingPromotionalOffers"`
	} `json:"promotions"`
	Price struct {
		TotalPrice struct {
			OriginalPrice int `json:"originalPrice"`
			DiscountPrice int `json:"discountPrice"`
			FmtPrice      struct {
				OriginalPrice string `json:"originalPrice"`
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

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	return c.parse(body, time.Now())
}

// parse selects the genuinely free offers from a freeGamesPromotions payload.
//
// The endpoint is a feed of everything Epic has scheduled for its "Free Games"
// row, which includes ordinary sales: an element can carry a promotion whose
// discountPercentage is 50, meaning half price rather than free. Selecting on
// "has an active promotion" therefore reports paid games as free, so every
// offer is checked for a 100% discount before it is emitted.
func (c *Client) parse(body []byte, now time.Time) ([]common.Game, error) {
	var r restResponse
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	var games []common.Game
	seen := make(map[string]bool)

	add := func(el catalogElement, start, end time.Time) {
		game := c.buildGame(el, start, end)
		k := strings.ToLower(game.Title) + "|" + start.UTC().Format(time.RFC3339)
		if seen[k] {
			return
		}
		seen[k] = true
		games = append(games, game)
	}

	for _, el := range r.Data.Catalog.SearchStore.Elements {
		// Epic keeps delisted and region-locked offers in the feed.
		if el.Status != "" && el.Status != "ACTIVE" {
			continue
		}
		if el.Promotions == nil {
			continue
		}

		for _, group := range el.Promotions.PromotionalOffers {
			for _, offer := range group.PromotionalOffers {
				start, end, ok := offerWindow(offer)
				if !ok || now.Before(start) || !now.Before(end) {
					continue
				}
				// A live giveaway has its price already at zero, which stands in
				// for the discount setting when the feed omits it.
				if !isFreeOffer(offer, el.Price.TotalPrice.DiscountPrice == 0) {
					continue
				}
				add(el, start, end)
			}
		}

		if !c.includeUpcoming {
			continue
		}
		for _, group := range el.Promotions.UpcomingPromotionalOffers {
			for _, offer := range group.PromotionalOffers {
				start, end, ok := offerWindow(offer)
				if !ok || !start.After(now) || start.After(now.Add(upcomingWindow)) {
					continue
				}
				// The price fields still show today's price for an offer that has
				// not started, so the discount setting is the only free signal.
				if !isFreeOffer(offer, false) {
					continue
				}
				add(el, start, end)
			}
		}
	}

	return games, nil
}

func offerWindow(offer promotionalOffer) (start, end time.Time, ok bool) {
	start, err := time.Parse(time.RFC3339, offer.StartDate)
	if err != nil {
		return time.Time{}, time.Time{}, false
	}
	end, err = time.Parse(time.RFC3339, offer.EndDate)
	if err != nil {
		return time.Time{}, time.Time{}, false
	}
	return start, end, true
}

// isFreeOffer reports whether a promotional offer is a 100% giveaway rather than
// a sale. Epic expresses this as discountSetting.discountPercentage == 0 (the
// share of the original price still owed). When the setting is missing the
// answer falls back to assumeFree, which callers set only when they have already
// confirmed a zero price by another means.
func isFreeOffer(offer promotionalOffer, assumeFree bool) bool {
	if offer.DiscountSetting == nil || offer.DiscountSetting.DiscountPercentage == nil {
		return assumeFree
	}
	if t := offer.DiscountSetting.DiscountType; t != "" && !strings.EqualFold(t, "PERCENTAGE") {
		return assumeFree
	}
	return *offer.DiscountSetting.DiscountPercentage == 0
}

func (c *Client) buildGame(el catalogElement, start, end time.Time) common.Game {
	originalPrice := el.Price.TotalPrice.FmtPrice.OriginalPrice
	if originalPrice == "" && el.Price.TotalPrice.OriginalPrice > 0 {
		originalPrice = fmt.Sprintf("%.2f", float64(el.Price.TotalPrice.OriginalPrice)/100)
	}

	return common.Game{
		Title:         el.Title,
		Description:   el.Description,
		ImageURL:      pickImage(el),
		URL:           c.storeURL(el),
		Publisher:     el.Seller.Name,
		OriginalPrice: originalPrice,
		StartDate:     start,
		EndDate:       end,
		Provider:      "epic",
	}
}

// imagePreference is ordered widest-first; Discord embeds render wide art best.
var imagePreference = []string{
	"OfferImageWide",
	"DieselStoreFrontWide",
	"featuredMedia",
	"VaultClosed",
	"OfferImageTall",
	"DieselStoreFrontTall",
	"Thumbnail",
}

func pickImage(el catalogElement) string {
	for _, want := range imagePreference {
		for _, img := range el.KeyImages {
			if img.Type == want && img.URL != "" {
				return img.URL
			}
		}
	}
	// Unknown image type is still better than no art at all.
	for _, img := range el.KeyImages {
		if img.URL != "" {
			return img.URL
		}
	}
	return ""
}

// guidSlugRe matches the bare 32-character hex ids Epic puts in urlSlug for some
// offers. Those are not routable store paths, so they must not become a link.
var guidSlugRe = regexp.MustCompile(`^[0-9a-f]{32}$`)

func (c *Client) storeURL(el catalogElement) string {
	slug := productSlug(el)
	if slug == "" {
		return ""
	}
	locale := c.locale
	if locale == "" {
		locale = "en-US"
	}
	// Bundles live under /bundles/, everything else under /p/.
	path := "p"
	if isBundle(el) {
		path = "bundles"
	}
	return fmt.Sprintf("https://store.epicgames.com/%s/%s/%s", locale, path, slug)
}

// productSlug resolves the store page slug for an offer, preferring the mappings
// Epic maintains for product pages over the legacy slug fields.
func productSlug(el catalogElement) string {
	for _, mappings := range [][]pageMapping{el.OfferMappings, el.CatalogNs.Mappings} {
		for _, m := range mappings {
			if m.PageType == "productHome" && m.PageSlug != "" {
				return m.PageSlug
			}
		}
	}

	if el.ProductSlug != nil {
		if slug := strings.TrimSuffix(*el.ProductSlug, "/home"); slug != "" {
			return slug
		}
	}

	// Any remaining mapping beats a raw urlSlug.
	for _, mappings := range [][]pageMapping{el.OfferMappings, el.CatalogNs.Mappings} {
		for _, m := range mappings {
			if m.PageSlug != "" {
				return m.PageSlug
			}
		}
	}

	if el.URLSlug != "" && !guidSlugRe.MatchString(el.URLSlug) {
		return strings.TrimSuffix(el.URLSlug, "/home")
	}
	return ""
}

func isBundle(el catalogElement) bool {
	if strings.EqualFold(el.OfferType, "BUNDLE") {
		return true
	}
	for _, cat := range el.Categories {
		if cat.Path == "bundles" {
			return true
		}
	}
	return false
}
