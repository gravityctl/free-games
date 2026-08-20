package common

import "testing"

func TestDeepLinkURL(t *testing.T) {
	tests := []struct {
		name string
		game Game
		want string
	}{
		{
			name: "epic product page",
			game: Game{Provider: "epic", URL: "https://store.epicgames.com/en-US/p/beacon-pines-629fc3"},
			want: "com.epicgames.launcher://store/p/beacon-pines-629fc3",
		},
		{
			name: "epic product page in another locale",
			game: Game{Provider: "epic", URL: "https://store.epicgames.com/de/p/beacon-pines-629fc3"},
			want: "com.epicgames.launcher://store/p/beacon-pines-629fc3",
		},
		{
			// The launcher has no bundle deep link, so the web page is the honest answer.
			name: "epic bundle falls back to the web page",
			game: Game{Provider: "epic", URL: "https://store.epicgames.com/en-US/bundles/lisa-the-definitive-edition"},
			want: "https://store.epicgames.com/en-US/bundles/lisa-the-definitive-edition",
		},
		{
			name: "steam app",
			game: Game{Provider: "steam", URL: "https://store.steampowered.com/app/424280/"},
			want: "steam://store/424280",
		},
		{
			name: "twitch keeps the web url",
			game: Game{Provider: "twitch", URL: "https://www.twitch.tv/drops/campaigns"},
			want: "https://www.twitch.tv/drops/campaigns",
		},
	}

	for _, tc := range tests {
		if got := tc.game.DeepLinkURL(); got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.name, got, tc.want)
		}
	}
}
