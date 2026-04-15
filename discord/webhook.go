package discord

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
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
// Custom emojis are specified as Discord native format <:name:id> or plain unicode.
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
// customEmojis is a map of provider -> emoji string (e.g. "epic" -> "<:epic:123456>")
func Send(webhookURL string, games []common.Game, customEmojis ...map[string]string) error {
	if len(games) == 0 {
		return nil
	}

	// Send in batches of maxEmbeds (Discord limit)
	for i := 0; i < len(games); i += maxEmbeds {
		end := i + maxEmbeds
		if end > len(games) {
			end = len(games)
		}
		batch := games[i:end]

		if err := sendBatch(webhookURL, batch, customEmojis...); err != nil {
			return err
		}
	}
	return nil
}

func sendBatch(webhookURL string, games []common.Game, customEmojis ...map[string]string) error {
	var embs []embed
	for _, game := range games {
		color := colorEpic
		switch game.Provider {
		case "steam":
			color = colorSteam
		case "twitch":
			color = colorTwitch
		}

		customEmoji := ""
		if len(customEmojis) > 0 && customEmojis[0] != nil {
			customEmoji = customEmojis[0][game.Provider]
		}

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

func truncate(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max-3]) + "..."
}