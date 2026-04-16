package discord

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/gravityctl/free-games/common"
)

const userAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36"

type webhookPayload struct {
	Embeds []embed `json:"embeds"`
}

type embed struct {
	Title       string       `json:"title"`
	Description string       `json:"description,omitempty"`
	URL         string       `json:"url,omitempty"`
	Color       int          `json:"color"`
	Image       *embedImage  `json:"image,omitempty"`
	Fields      []embedField `json:"fields,omitempty"`
	Footer      *embedFooter `json:"footer,omitempty"`
}

type embedImage struct {
	URL string `json:"url"`
}

type embedField struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Inline bool   `json:"inline,omitempty"`
}

type embedFooter struct {
	Text string `json:"text"`
}

const (
	colorEpic   = 0x0078f2
	colorSteam  = 0x1b2838
	colorTwitch = 0x9146ff
	maxEmbeds   = 10 // Discord max embeds per message
)

// EmojiFor returns the emoji for a provider, using custom emoji if configured.
func EmojiFor(provider, customEmoji string) string {
	if customEmoji != "" {
		return customEmoji
	}
	switch provider {
	case "epic":
		return "🔶"
	case "steam":
		return "🎲"
	case "twitch":
		return "🟣"
	default:
		return "🎮"
	}
}

// Send delivers a Discord notification for the given games.
// customEmojis maps provider -> emoji string (e.g. "epic" -> "<:epic:123456>")
// redirectBase is optional base URL for desktop-app redirects (e.g. "https://redirect.example.com")
func Send(webhookURL string, games []common.Game, customEmojis map[string]string, redirectBase string) error {
	if len(games) == 0 {
		return nil
	}
	for i := 0; i < len(games); i += maxEmbeds {
		end := i + maxEmbeds
		if end > len(games) {
			end = len(games)
		}
		if err := sendBatch(webhookURL, games[i:end], customEmojis, redirectBase); err != nil {
			return err
		}
	}
	return nil
}

func sendBatch(webhookURL string, games []common.Game, customEmojis map[string]string, redirectBase string) error {
	var embs []embed
	for _, game := range games {
		color := colorEpic
		switch game.Provider {
		case "steam":
			color = colorSteam
		case "twitch":
			color = colorTwitch
		}

		customEmoji := customEmojis[game.Provider]

		e := embed{
			Title:       game.Title,
			Description: truncate(game.Description, 350),
			Color:       color,
			URL:         game.URL,
			Fields: []embedField{
				{Name: "Publisher", Value: game.Publisher, Inline: true},
				{Name: "Provider", Value: EmojiFor(game.Provider, customEmoji) + " " + strings.Title(game.Provider), Inline: true},
			},
			Footer: &embedFooter{Text: "Free Games"},
		}

		// Add "Open in App" link via redirect service
		if redirectBase != "" {
			redirectURL := buildRedirectURL(redirectBase, game)
			if redirectURL != "" {
				e.Fields = append(e.Fields, embedField{Name: "Open in App", Value: "[Open in App](" + redirectURL + ")", Inline: true})
			}
		}

		if !game.StartDate.IsZero() {
			e.Fields = append(e.Fields, embedField{Name: "Start Date", Value: game.StartDate.Format("Jan 2, 2006"), Inline: true})
		}
		if !game.EndDate.IsZero() {
			e.Fields = append(e.Fields, embedField{Name: "End Date", Value: game.EndDate.Format("Jan 2, 2006"), Inline: true})
		}

		if game.ImageURL != "" {
			e.Image = &embedImage{URL: game.ImageURL}
		}

		embs = append(embs, e)
	}

	payload := webhookPayload{Embeds: embs}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	req, err := http.NewRequest("POST", webhookURL, bytes.NewBuffer(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", userAgent)

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 && resp.StatusCode != 204 {
		return fmt.Errorf("webhook returned status %d", resp.StatusCode)
	}

	return nil
}

// buildRedirectURL constructs an HTTPS redirect URL for a game's desktop app.
// Returns empty string if slug extraction fails or redirectBase is not set.
func buildRedirectURL(redirectBase string, game common.Game) string {
	slug := ""
	switch game.Provider {
	case "epic":
		slug = epicSlug(game.URL)
	case "steam":
		slug = steamAppID(game.URL)
	case "twitch":
		slug = twitchSlug(game.URL)
	}
	if slug == "" {
		return ""
	}
	base := strings.TrimSuffix(redirectBase, "/")
	return fmt.Sprintf("%s/%s/%s", base, game.Provider, slug)
}

var epicSlugRe = regexp.MustCompile(`store\.epicgames\.com/en-US/p/([^/\s]+)`)
var steamAppRe = regexp.MustCompile(`store\.steampowered\.com/app/(\d+)`)

func epicSlug(webURL string) string {
	m := epicSlugRe.FindStringSubmatch(webURL)
	if len(m) >= 2 {
		return m[1]
	}
	return ""
}

func steamAppID(webURL string) string {
	m := steamAppRe.FindStringSubmatch(webURL)
	if len(m) >= 2 {
		return m[1]
	}
	return ""
}

func twitchSlug(webURL string) string {
	u, err := url.Parse(webURL)
	if err != nil {
		return ""
	}
	return strings.TrimPrefix(u.Path, "/")
}

func truncate(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max-3]) + "..."
}