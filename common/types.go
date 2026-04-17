package common

import (
	"regexp"
	"time"
)

// Game represents a free game from any provider.
type Game struct {
	Title         string
	Description   string
	ImageURL      string
	URL           string
	Publisher     string
	OriginalPrice string
	StartDate     time.Time
	EndDate       time.Time
	Provider      string // "epic", "steam", or "twitch"
}

// DeepLinkURL returns an app-specific URL to open the game in the provider's desktop app.
// Falls back to the web URL if no app scheme is known.
func (g *Game) DeepLinkURL() string {
	switch g.Provider {
	case "epic":
		return epicDeepLink(g.URL)
	case "steam":
		return steamDeepLink(g.URL)
	default:
		return g.URL
	}
}

var epicSlugRe = regexp.MustCompile(`store\.epicgames\.com/en-US/p/([^/\s]+)`)

func epicDeepLink(webURL string) string {
	m := epicSlugRe.FindStringSubmatch(webURL)
	if len(m) < 2 {
		return webURL
	}
	// Use store/product/{slug} format — the correct Epic launcher deep link
	return "com.epicgames.launcher://store/product/" + m[1]
}

var steamAppRe = regexp.MustCompile(`store\.steampowered\.com/app/(\d+)`)

func steamDeepLink(webURL string) string {
	m := steamAppRe.FindStringSubmatch(webURL)
	if len(m) >= 2 {
		return "steam://store/" + m[1]
	}
	// Try openurl with encoded web URL
	return "steam://openurl/" + webURL
}
