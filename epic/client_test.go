package epic

import (
	"testing"
	"time"
)

// now is a fixed point inside the "live" offer windows in the fixture below.
var now = time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)

// fixture mirrors the shape of a real freeGamesPromotions payload. The endpoint is
// a feed of everything queued for Epic's "Free Games" row, so it carries ordinary
// sales alongside giveaways — the cases below are the ones the selection logic has
// to tell apart.
const fixture = `{"data":{"Catalog":{"searchStore":{"elements":[
  {
    "title": "Live Giveaway",
    "status": "ACTIVE",
    "offerType": "BASE_GAME",
    "productSlug": null,
    "urlSlug": "9e9a438c651a49799c3ddd94a0c3f4c0",
    "offerMappings": [{"pageSlug": "live-giveaway-629fc3", "pageType": "productHome"}],
    "catalogNs": {"mappings": [{"pageSlug": "live-giveaway-629fc3", "pageType": "productHome"}]},
    "keyImages": [{"type": "Thumbnail", "url": "thumb.jpg"}, {"type": "OfferImageWide", "url": "wide.jpg"}],
    "seller": {"name": "Some Publisher"},
    "price": {"totalPrice": {"originalPrice": 1999, "discountPrice": 0,
      "fmtPrice": {"originalPrice": "$19.99", "discountPrice": "0"}}},
    "promotions": {"promotionalOffers": [{"promotionalOffers": [
      {"startDate": "2026-08-06T15:00:00.000Z", "endDate": "2026-08-13T15:00:00.000Z",
       "discountSetting": {"discountType": "PERCENTAGE", "discountPercentage": 0}}]}]}
  },
  {
    "title": "Live Half Price Sale",
    "status": "ACTIVE",
    "offerType": "BASE_GAME",
    "productSlug": "half-price",
    "catalogNs": {"mappings": [{"pageSlug": "half-price", "pageType": "productHome"}]},
    "price": {"totalPrice": {"originalPrice": 3999, "discountPrice": 1999,
      "fmtPrice": {"originalPrice": "$39.99", "discountPrice": "$19.99"}}},
    "promotions": {"promotionalOffers": [{"promotionalOffers": [
      {"startDate": "2026-08-06T15:00:00.000Z", "endDate": "2026-08-13T15:00:00.000Z",
       "discountSetting": {"discountType": "PERCENTAGE", "discountPercentage": 50}}]}]}
  },
  {
    "title": "Upcoming Giveaway",
    "status": "ACTIVE",
    "offerType": "BASE_GAME",
    "productSlug": "upcoming-giveaway/home",
    "catalogNs": {"mappings": [{"pageSlug": "upcoming-giveaway", "pageType": "productHome"}]},
    "price": {"totalPrice": {"originalPrice": 2499, "discountPrice": 2499,
      "fmtPrice": {"originalPrice": "$24.99", "discountPrice": "$24.99"}}},
    "promotions": {"upcomingPromotionalOffers": [{"promotionalOffers": [
      {"startDate": "2026-08-13T15:00:00.000Z", "endDate": "2026-08-20T15:00:00.000Z",
       "discountSetting": {"discountType": "PERCENTAGE", "discountPercentage": 0}}]}]}
  },
  {
    "title": "Upcoming Sale Not A Giveaway",
    "status": "ACTIVE",
    "offerType": "ADD_ON",
    "productSlug": null,
    "urlSlug": "upcoming-sale",
    "catalogNs": {"mappings": [{"pageSlug": "upcoming-sale", "pageType": "productHome"}]},
    "price": {"totalPrice": {"originalPrice": 399, "discountPrice": 399,
      "fmtPrice": {"originalPrice": "$3.99", "discountPrice": "$3.99"}}},
    "promotions": {"upcomingPromotionalOffers": [{"promotionalOffers": [
      {"startDate": "2026-08-14T01:00:00.000Z", "endDate": "2026-08-20T23:59:00.000Z",
       "discountSetting": {"discountType": "PERCENTAGE", "discountPercentage": 50}}]}]}
  },
  {
    "title": "Upcoming Beyond Window",
    "status": "ACTIVE",
    "offerType": "BASE_GAME",
    "productSlug": "far-off",
    "price": {"totalPrice": {"originalPrice": 799, "discountPrice": 799, "fmtPrice": {"originalPrice": "$7.99"}}},
    "promotions": {"upcomingPromotionalOffers": [{"promotionalOffers": [
      {"startDate": "2026-09-03T15:00:00.000Z", "endDate": "2026-09-10T15:00:00.000Z",
       "discountSetting": {"discountType": "PERCENTAGE", "discountPercentage": 0}}]}]}
  },
  {
    "title": "Expired Giveaway",
    "status": "ACTIVE",
    "offerType": "BASE_GAME",
    "productSlug": "expired",
    "price": {"totalPrice": {"originalPrice": 1299, "discountPrice": 0, "fmtPrice": {"originalPrice": "$12.99"}}},
    "promotions": {"promotionalOffers": [{"promotionalOffers": [
      {"startDate": "2026-07-30T15:00:00.000Z", "endDate": "2026-08-06T15:00:00.000Z",
       "discountSetting": {"discountType": "PERCENTAGE", "discountPercentage": 0}}]}]}
  },
  {
    "title": "Delisted Giveaway",
    "status": "SUNSET",
    "offerType": "BASE_GAME",
    "productSlug": "delisted",
    "price": {"totalPrice": {"originalPrice": 1299, "discountPrice": 0, "fmtPrice": {"originalPrice": "$12.99"}}},
    "promotions": {"promotionalOffers": [{"promotionalOffers": [
      {"startDate": "2026-08-06T15:00:00.000Z", "endDate": "2026-08-13T15:00:00.000Z",
       "discountSetting": {"discountType": "PERCENTAGE", "discountPercentage": 0}}]}]}
  },
  {
    "title": "Free Bundle",
    "status": "ACTIVE",
    "offerType": "BUNDLE",
    "productSlug": "free-bundle",
    "offerMappings": null,
    "catalogNs": null,
    "categories": [{"path": "bundles"}, {"path": "freegames"}],
    "price": {"totalPrice": {"originalPrice": 2499, "discountPrice": 0, "fmtPrice": {"originalPrice": "$24.99"}}},
    "promotions": {"promotionalOffers": [{"promotionalOffers": [
      {"startDate": "2026-08-06T15:00:00.000Z", "endDate": "2026-08-13T15:00:00.000Z",
       "discountSetting": {"discountType": "PERCENTAGE", "discountPercentage": 0}}]}]}
  },
  {
    "title": "Duplicated Offer",
    "status": "ACTIVE",
    "offerType": "BASE_GAME",
    "productSlug": "duplicated",
    "price": {"totalPrice": {"originalPrice": 999, "discountPrice": 0, "fmtPrice": {"originalPrice": "$9.99"}}},
    "promotions": {"promotionalOffers": [
      {"promotionalOffers": [{"startDate": "2026-08-06T15:00:00.000Z", "endDate": "2026-08-13T15:00:00.000Z",
        "discountSetting": {"discountType": "PERCENTAGE", "discountPercentage": 0}}]},
      {"promotionalOffers": [{"startDate": "2026-08-06T15:00:00.000Z", "endDate": "2026-08-13T15:00:00.000Z",
        "discountSetting": {"discountType": "PERCENTAGE", "discountPercentage": 0}}]}]}
  },
  {
    "title": "No Promotions At All",
    "status": "ACTIVE",
    "offerType": "BASE_GAME",
    "productSlug": "no-promos",
    "price": {"totalPrice": {"originalPrice": 1999, "discountPrice": 1999, "fmtPrice": {"originalPrice": "$19.99"}}},
    "promotions": null
  }
]}}}}`

func titles(t *testing.T, includeUpcoming bool) []string {
	t.Helper()
	games, err := NewClient("US", "en-US", includeUpcoming).parse([]byte(fixture), now)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	out := make([]string, len(games))
	for i, g := range games {
		out[i] = g.Title
	}
	return out
}

func TestParseCurrentOffersOnly(t *testing.T) {
	got := titles(t, false)
	want := []string{"Live Giveaway", "Free Bundle", "Duplicated Offer"}
	assertTitles(t, got, want)
}

func TestParseIncludesUpcomingGiveawaysOnly(t *testing.T) {
	got := titles(t, true)
	want := []string{"Live Giveaway", "Upcoming Giveaway", "Free Bundle", "Duplicated Offer"}
	assertTitles(t, got, want)

	// The regression that matters: a scheduled sale is not a giveaway.
	for _, title := range got {
		if title == "Upcoming Sale Not A Giveaway" || title == "Live Half Price Sale" {
			t.Errorf("discounted (non-free) offer reported as free: %q", title)
		}
	}
}

func assertTitles(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %d games %v, want %d %v", len(got), got, len(want), want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("game %d: got %q, want %q", i, got[i], want[i])
		}
	}
}

func TestParseBuildsGameFields(t *testing.T) {
	games, err := NewClient("US", "en-US", false).parse([]byte(fixture), now)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	byTitle := map[string]int{}
	for i, g := range games {
		byTitle[g.Title] = i
	}

	live := games[byTitle["Live Giveaway"]]
	// The slug must come from the productHome mapping: urlSlug here is a bare
	// 32-char id, which is not a routable store path.
	if want := "https://store.epicgames.com/en-US/p/live-giveaway-629fc3"; live.URL != want {
		t.Errorf("URL: got %q, want %q", live.URL, want)
	}
	if live.ImageURL != "wide.jpg" {
		t.Errorf("ImageURL: got %q, want wide.jpg (wide art preferred)", live.ImageURL)
	}
	if live.OriginalPrice != "$19.99" {
		t.Errorf("OriginalPrice: got %q, want $19.99", live.OriginalPrice)
	}
	if live.Publisher != "Some Publisher" {
		t.Errorf("Publisher: got %q", live.Publisher)
	}
	if live.Provider != "epic" {
		t.Errorf("Provider: got %q", live.Provider)
	}
	if !live.EndDate.Equal(time.Date(2026, 8, 13, 15, 0, 0, 0, time.UTC)) {
		t.Errorf("EndDate: got %v", live.EndDate)
	}

	// Bundles live under a different store path than products.
	bundle := games[byTitle["Free Bundle"]]
	if want := "https://store.epicgames.com/en-US/bundles/free-bundle"; bundle.URL != want {
		t.Errorf("bundle URL: got %q, want %q", bundle.URL, want)
	}
}

func TestParseHonoursLocaleInURL(t *testing.T) {
	games, err := NewClient("DE", "de", false).parse([]byte(fixture), now)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if want := "https://store.epicgames.com/de/p/live-giveaway-629fc3"; games[0].URL != want {
		t.Errorf("URL: got %q, want %q", games[0].URL, want)
	}
}

func TestIsFreeOffer(t *testing.T) {
	pct := func(v int) promotionalOffer {
		return promotionalOffer{
			DiscountSetting: &discountSetting{DiscountType: "PERCENTAGE", DiscountPercentage: &v},
		}
	}

	tests := []struct {
		name       string
		offer      promotionalOffer
		assumeFree bool
		want       bool
	}{
		{"percentage zero is free", pct(0), false, true},
		{"half price is not free", pct(50), true, false},
		{"eighty percent off is not free", pct(20), true, false},
		{"missing setting falls back to price", promotionalOffer{}, true, true},
		{"missing setting without zero price", promotionalOffer{}, false, false},
	}
	for _, tc := range tests {
		if got := isFreeOffer(tc.offer, tc.assumeFree); got != tc.want {
			t.Errorf("%s: got %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestProductSlugPrefersProductHomeMapping(t *testing.T) {
	slug := "base-slug"
	tests := []struct {
		name string
		el   catalogElement
		want string
	}{
		{
			name: "productHome mapping wins over offer mapping",
			el: catalogElement{
				OfferMappings: []pageMapping{{PageSlug: "dlc-offer-page", PageType: "offer"}},
				CatalogNs: struct {
					Mappings []pageMapping `json:"mappings"`
				}{Mappings: []pageMapping{{PageSlug: "product-page", PageType: "productHome"}}},
			},
			want: "product-page",
		},
		{
			name: "home suffix is trimmed from productSlug",
			el:   catalogElement{ProductSlug: strPtr("game-name/home")},
			want: "game-name",
		},
		{
			name: "bare hex id is rejected",
			el:   catalogElement{URLSlug: "9e9a438c651a49799c3ddd94a0c3f4c0"},
			want: "",
		},
		{
			name: "readable urlSlug is accepted as a last resort",
			el:   catalogElement{URLSlug: slug},
			want: slug,
		},
	}
	for _, tc := range tests {
		if got := productSlug(tc.el); got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.name, got, tc.want)
		}
	}
}

func strPtr(s string) *string { return &s }
