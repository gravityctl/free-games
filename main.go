package main

import (
	"flag"
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/gravityctl/free-games/common"
	"github.com/gravityctl/free-games/discord"
	"github.com/gravityctl/free-games/epic"
	"github.com/gravityctl/free-games/steam"
	"github.com/gravityctl/free-games/twitch"
	"github.com/joho/godotenv"
	"github.com/robfig/cron/v3"
)

func main() {
	godotenv.Load()

	discordWebhook := flag.String("discord-webhook", envOr("DISCORD_WEBHOOK_URL", ""), "Discord webhook URL")
	country := flag.String("country", envOr("EPIC_COUNTRY", "US"), "Epic store country code")
	locale := flag.String("locale", envOr("EPIC_LOCALE", "en-US"), "Epic store locale")
	includeUpcoming := flag.Bool("include-upcoming", envOrBool("EPIC_INCLUDE_UPCOMING", false), "Include upcoming free games")
	enableSteam := flag.Bool("steam", envOrBool("ENABLE_STEAM", false), "Enable Steam scraper")
	twitchOwnersStr := flag.String("twitch-owners", envOr("TWITCH_OWNERS", ""), "Comma-separated Twitch drop owners to include (empty = all disabled)")
	cronSchedule := flag.String("schedule", envOr("CHECK_SCHEDULE", "0 0 0 * * 4"), "Cron schedule (default: every Thursday at midnight)")
	runOnce := flag.Bool("once", false, "Run once and exit (no cron)")
	flag.Parse()

	if *discordWebhook == "" {
		log.Fatal("DISCORD_WEBHOOK_URL is required")
	}

	// Build Twitch owner lookup map
	var twitchEnabled map[string]bool
	if strings.TrimSpace(*twitchOwnersStr) != "" {
		owners := strings.Split(*twitchOwnersStr, ",")
		twitchEnabled = make(map[string]bool)
		for _, o := range owners {
			twitchEnabled[strings.TrimSpace(o)] = true
		}
	}

	runner := func() {
		var allGames []common.Game

		// Fetch Epic games
		epicClient := epic.NewClient(*country, *locale, *includeUpcoming)
		epicGames, err := epicClient.FetchFreeGames()
		if err != nil {
			log.Printf("Error fetching Epic games: %v", err)
		} else {
			log.Printf("Found %d Epic free game(s)", len(epicGames))
			allGames = append(allGames, epicGames...)
		}

		// Fetch Steam games if enabled
		if *enableSteam {
			steamScraper := steam.NewScraper()
			steamGames, err := steamScraper.FetchFreeGames()
			if err != nil {
				log.Printf("Error fetching Steam games: %v", err)
			} else {
				log.Printf("Found %d Steam free game(s)", len(steamGames))
				allGames = append(allGames, steamGames...)
			}
		}

		// Fetch Twitch drops if owners configured
		if len(twitchEnabled) > 0 {
			twitchClient := twitch.NewClient(twitchEnabled)
			twitchGames, err := twitchClient.FetchDrops()
			if err != nil {
				log.Printf("Error fetching Twitch drops: %v", err)
			} else {
				log.Printf("Found %d Twitch free game(s)", len(twitchGames))
				allGames = append(allGames, twitchGames...)
			}
		}

		if len(allGames) == 0 {
			log.Println("No free games found this week")
			return
		}

		if err := discord.Send(*discordWebhook, allGames); err != nil {
			log.Printf("Error sending Discord notification: %v", err)
		} else {
			log.Printf("Notification sent for %d game(s)", len(allGames))
		}
	}

	if *runOnce {
		runner()
		return
	}

	c := cron.New()
	_, err := c.AddFunc(*cronSchedule, runner)
	if err != nil {
		log.Fatalf("Invalid cron schedule %q: %v", *cronSchedule, err)
	}

	log.Printf("free-games service started. Checking every %s", *cronSchedule)
	c.Start()

	<-make(chan struct{})
}

func envOr(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}

func envOrBool(key string, defaultVal bool) bool {
	if v := os.Getenv(key); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
	}
	return defaultVal
}