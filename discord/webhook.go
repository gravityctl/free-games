package discord

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/gravityctl/free-games/epic"
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
	Timestamp   string       `json:"timestamp,omitempty"`
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

// accent color (Epic blue-ish)
const embedColor = 0x0078f2

// Send posts a Discord webhook notification for the given free games.
func Send(webhookURL string, games []epic.Game) error {
	if len(games) == 0 {
		return nil
	}

	var embeds []embed
	for _, game := range games {
		e := embed{
			Title:       "🎮 " + game.Title,
			Description: truncate(game.Description, 350),
			Color:       embedColor,
			Fields: []embedField{
				{Name: "Publisher", Value: game.Publisher, Inline: true},
				{Name: "Start Date", Value: game.StartDate.Format("Jan 2, 2006"), Inline: true},
				{Name: "End Date", Value: game.EndDate.Format("Jan 2, 2006"), Inline: true},
			},
			Footer: &embedFooter{Text: "Epic Games Store • Free Games"},
		}

		if game.ImageURL != "" {
			e.Image = &embedImage{URL: game.ImageURL}
		}

		embeds = append(embeds, e)
	}

	payload := webhookPayload{Embeds: embeds}
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
