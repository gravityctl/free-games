package steam

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

// realRowsHTML is a trimmed capture of Steam's search result rows, kept verbatim
// so the parser is tested against the markup the store actually serves.
const realRowsHTML = `
	<a href="https://store.steampowered.com/app/553850/HELLDIVERS_2/?snr=1_7_7_2300_150_1"
	 data-ds-appid="553850" data-ds-itemkey="App_553850" class="search_result_row ds_collapse_flag ">
		<div class="search_capsule"><img src="https://example.invalid/capsule.jpg" ></div>
		<div class="responsive_search_name_combined">
			<div class="search_name ellipsis">
				<span class="title">HELLDIVERS&trade; 2</span>
			</div>
			<div class="search_price_discount_combined responsive_secondrow" data-price-final="2999">
				<div class="search_discount_and_price responsive_secondrow">
					<div class="discount_block search_discount_block" data-price-final="2999" data-bundlediscount="0" data-discount="25" role="link" aria-label="25% off. $39.99 normally, discounted to $29.99"><div class="discount_pct">-25%</div></div>
				</div>
			</div>
		</div>
	</a>
	<a href="https://store.steampowered.com/app/424280/Free_To_Keep_Game/?snr=1_7_7_2300_150_1"
	 data-ds-appid="424280" data-ds-itemkey="App_424280" class="search_result_row ds_collapse_flag ">
		<div class="responsive_search_name_combined">
			<div class="search_name ellipsis">
				<span class="title">Free To Keep Game</span>
			</div>
			<div class="search_price_discount_combined responsive_secondrow" data-price-final="0">
				<div class="discount_block search_discount_block" data-price-final="0" data-bundlediscount="0" data-discount="100"><div class="discount_pct">-100%</div></div>
			</div>
		</div>
	</a>
	<a href="https://store.steampowered.com/app/730/CounterStrike_2/?snr=1_7_7_2300_150_1"
	 data-ds-appid="730" data-ds-itemkey="App_730" class="search_result_row ds_collapse_flag ">
		<div class="responsive_search_name_combined">
			<div class="search_name ellipsis">
				<div class="title">Counter-Strike 2</div>
			</div>
			<div class="search_price_discount_combined responsive_secondrow" data-price-final="0">
				<div class="search_price">Free To Play</div>
			</div>
		</div>
	</a>`

func TestParseSearchResults(t *testing.T) {
	got := parseSearchResults(realRowsHTML)
	if len(got) != 3 {
		t.Fatalf("got %d rows, want 3: %+v", len(got), got)
	}

	want := []searchResult{
		{AppID: 553850, Title: "HELLDIVERS™ 2", PriceFinal: 2999, Discount: 25, HasDiscount: true},
		{AppID: 424280, Title: "Free To Keep Game", PriceFinal: 0, Discount: 100, HasDiscount: true},
		{AppID: 730, Title: "Counter-Strike 2", PriceFinal: 0, Discount: 0, HasDiscount: false},
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("row %d: got %+v, want %+v", i, got[i], w)
		}
	}
}

func TestParseSearchResultsEmpty(t *testing.T) {
	if got := parseSearchResults(""); got != nil {
		t.Errorf("got %+v, want nil", got)
	}
}

func TestIsFreeToKeepRow(t *testing.T) {
	tests := []struct {
		name string
		row  searchResult
		want bool
	}{
		{"full price", searchResult{PriceFinal: 2999, Discount: 25, HasDiscount: true}, false},
		{"hundred percent off", searchResult{PriceFinal: 0, Discount: 100, HasDiscount: true}, true},
		{"free to play has no discount", searchResult{PriceFinal: 0}, true},
		{"partial discount at zero price", searchResult{PriceFinal: 0, Discount: 90, HasDiscount: true}, false},
		{"missing price attribute", searchResult{PriceFinal: -1}, false},
	}
	for _, tc := range tests {
		if got := isFreeToKeepRow(tc.row); got != tc.want {
			t.Errorf("%s: got %v, want %v", tc.name, got, tc.want)
		}
	}
}

// TestIsFreeToKeep covers the distinction the store API draws between a
// free-to-play app and a paid app being given away. is_free is the app's own
// free-to-play flag and stays false during a giveaway, so requiring is_free would
// reject every real "free to keep" promotion.
func TestIsFreeToKeep(t *testing.T) {
	tests := []struct {
		name string
		data appDetails
		want bool
	}{
		{
			name: "paid game discounted to zero",
			data: appDetails{Type: "game", IsFree: false, PriceOverview: &priceOverview{
				Initial: 1999, Final: 0, DiscountPercent: 100, InitialFormatted: "$19.99", FinalFormatted: "Free"}},
			want: true,
		},
		{
			name: "free to play app has no price block",
			data: appDetails{Type: "game", IsFree: true, PriceOverview: nil},
			want: false,
		},
		{
			name: "ordinary discount is not free",
			data: appDetails{Type: "game", PriceOverview: &priceOverview{
				Initial: 1999, Final: 1799, DiscountPercent: 10}},
			want: false,
		},
		{
			name: "zero price with no promotion",
			data: appDetails{Type: "game", PriceOverview: &priceOverview{Initial: 0, Final: 0, DiscountPercent: 0}},
			want: false,
		},
	}
	for _, tc := range tests {
		if got := isFreeToKeep(tc.data); got != tc.want {
			t.Errorf("%s: got %v, want %v", tc.name, got, tc.want)
		}
	}
}

// appDetailsResponses is the store API's reply for each app id in the fixture.
var appDetailsResponses = map[string]string{
	// A paid game at 100% off: the one result that should be reported.
	"424280": `{"424280":{"success":true,"data":{"type":"game","name":"Free To Keep Game","is_free":false,
		"short_description":"A game you can keep.","publishers":["Keepers Ltd"],
		"price_overview":{"currency":"USD","initial":1999,"final":0,"discount_percent":100,
			"initial_formatted":"$19.99","final_formatted":"Free"},
		"header_image":"https://example.invalid/header.jpg"}}}`,
	// Free to play: zero price, but never a giveaway.
	"730": `{"730":{"success":true,"data":{"type":"game","name":"Counter-Strike 2","is_free":true,
		"publishers":["Valve"],"header_image":"https://example.invalid/cs2.jpg"}}}`,
	// Discounted but still paid.
	"553850": `{"553850":{"success":true,"data":{"type":"game","name":"HELLDIVERS 2","is_free":false,
		"price_overview":{"currency":"USD","initial":3999,"final":2999,"discount_percent":25}}}}`,
	// A free DLC is not a game.
	"111111": `{"111111":{"success":true,"data":{"type":"dlc","name":"Some Free DLC","is_free":false,
		"price_overview":{"currency":"USD","initial":499,"final":0,"discount_percent":100}}}}`,
	// Region-locked or delisted apps come back unsuccessful.
	"222222": `{"222222":{"success":false}}`,
}

type rowSpec struct {
	appID       int
	title       string
	price       int
	discount    int
	hasDiscount bool
}

// searchRows renders result rows for the given apps in Steam's markup.
func searchRows(apps ...rowSpec) string {
	out := ""
	for _, a := range apps {
		discount := ""
		if a.hasDiscount {
			discount = fmt.Sprintf(`<div class="discount_block" data-price-final="%d" data-discount="%d"></div>`, a.price, a.discount)
		}
		out += fmt.Sprintf(`<a href="https://store.steampowered.com/app/%d/" data-ds-appid="%d">
			<span class="title">%s</span>
			<div class="search_price_discount_combined" data-price-final="%d">%s</div></a>`,
			a.appID, a.appID, a.title, a.price, discount)
	}
	return out
}

// newTestScraper serves the search and appdetails endpoints locally. searchRowsFor
// maps the value of the maxprice parameter ("free" or "") to the rows returned, so
// a test can model Steam answering the two discovery queries differently.
func newTestScraper(t *testing.T, searchRowsFor map[string]string) *Scraper {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/search":
			if r.URL.Query().Get("cc") != "US" {
				t.Errorf("search missing country: %s", r.URL.RawQuery)
			}
			rows := searchRowsFor[r.URL.Query().Get("maxprice")]
			total := 0
			if rows != "" {
				total = 1
			}
			json.NewEncoder(w).Encode(map[string]any{
				"success": 1, "results_html": rows, "total_count": total,
			})
		case "/appdetails":
			appID := r.URL.Query().Get("appids")
			body, ok := appDetailsResponses[appID]
			if !ok {
				http.Error(w, "not found", http.StatusNotFound)
				return
			}
			w.Write([]byte(body))
		default:
			http.Error(w, "unexpected path "+r.URL.Path, http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)

	s := NewScraperFor("US", "en")
	s.searchURL = srv.URL + "/search"
	s.detailsURL = srv.URL + "/appdetails"
	return s
}

func TestFetchFreeGames(t *testing.T) {
	rows := searchRows(
		rowSpec{appID: 424280, title: "Free To Keep Game", price: 0, discount: 100, hasDiscount: true},
		rowSpec{appID: 730, title: "Counter-Strike 2", price: 0},
		rowSpec{appID: 553850, title: "HELLDIVERS 2", price: 2999, discount: 25, hasDiscount: true},
		rowSpec{appID: 111111, title: "Some Free DLC", price: 0, discount: 100, hasDiscount: true},
		rowSpec{appID: 222222, title: "Delisted Thing", price: 0, discount: 100, hasDiscount: true},
	)

	s := newTestScraper(t, map[string]string{"free": rows, "": rows})
	games, err := s.FetchFreeGames()
	if err != nil {
		t.Fatalf("FetchFreeGames: %v", err)
	}

	if len(games) != 1 {
		var titles []string
		for _, g := range games {
			titles = append(titles, g.Title)
		}
		t.Fatalf("got %d games %v, want 1 (Free To Keep Game)", len(games), titles)
	}

	g := games[0]
	if g.Title != "Free To Keep Game" {
		t.Errorf("Title: got %q", g.Title)
	}
	if g.OriginalPrice != "$19.99" {
		t.Errorf("OriginalPrice: got %q, want $19.99", g.OriginalPrice)
	}
	if g.Publisher != "Keepers Ltd" {
		t.Errorf("Publisher: got %q", g.Publisher)
	}
	if want := "https://store.steampowered.com/app/424280/"; g.URL != want {
		t.Errorf("URL: got %q, want %q", g.URL, want)
	}
	if g.Provider != "steam" {
		t.Errorf("Provider: got %q", g.Provider)
	}
	if g.Description == "" {
		t.Error("Description: empty")
	}
}

// TestFetchFreeGamesFallbackSweep covers Steam omitting a giveaway from the
// maxprice=free filter: the broader price-sorted sweep still has to find it.
func TestFetchFreeGamesFallbackSweep(t *testing.T) {
	sweep := searchRows(
		rowSpec{appID: 424280, title: "Free To Keep Game", price: 0, discount: 100, hasDiscount: true},
		rowSpec{appID: 553850, title: "HELLDIVERS 2", price: 2999, discount: 25, hasDiscount: true},
	)

	s := newTestScraper(t, map[string]string{"free": "", "": sweep})
	games, err := s.FetchFreeGames()
	if err != nil {
		t.Fatalf("FetchFreeGames: %v", err)
	}
	if len(games) != 1 || games[0].Title != "Free To Keep Game" {
		t.Fatalf("got %+v, want just Free To Keep Game", games)
	}
}

func TestFetchFreeGamesNoneAvailable(t *testing.T) {
	sweep := searchRows(rowSpec{appID: 553850, title: "HELLDIVERS 2", price: 2999, discount: 25, hasDiscount: true})

	s := newTestScraper(t, map[string]string{"free": "", "": sweep})
	games, err := s.FetchFreeGames()
	if err != nil {
		t.Fatalf("FetchFreeGames: %v", err)
	}
	if len(games) != 0 {
		t.Fatalf("got %+v, want none", games)
	}
}

func TestFetchFreeGamesSearchFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "rate limited", http.StatusTooManyRequests)
	}))
	t.Cleanup(srv.Close)

	s := NewScraperFor("US", "en")
	s.searchURL = srv.URL
	s.detailsURL = srv.URL

	// A failed search must surface as an error rather than an empty result, which
	// would be indistinguishable from "nothing is free this week".
	if _, err := s.FetchFreeGames(); err == nil {
		t.Fatal("got nil error for a failing search")
	}
}
