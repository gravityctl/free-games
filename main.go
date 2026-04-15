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
	"github.com/gravityctl/free-games/notification"
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
	cronSchedule := flag.String("schedule", envOr("CHECK_SCHEDULE", ""), "Legacy cron schedule (used if no per-provider schedule set)")
	runOnce := flag.Bool("once", false, "Run all enabled scrapers once and exit (no cron)")
	storePath := flag.String("store", envOr("NOTIFICATION_STORE_PATH", ".free-games-store.json"), "Path to notification deduplication store")
	flag.Parse()

	if *discordWebhook == "" {
		log.Fatal("DISCORD_WEBHOOK_URL is required")
	}

	// Load notification store for deduplication
	notifStore, err := notification.NewNotificationStore(*storePath)
	if err != nil {
		log.Printf("Warning: could not open notification store: %v", err)
	}

	// Build Twitch drops config
	twitchDropsPlatformsStr := envOr("TWITCH_DROPS_PLATFORMS", "")
	twitchDropsEnabled := envOrBool("ENABLE_TWITCH_DROPS", false)
	var twitchEnabled map[string]bool
	if strings.TrimSpace(twitchDropsPlatformsStr) != "" {
		platforms := strings.Split(twitchDropsPlatformsStr, ",")
		twitchEnabled = make(map[string]bool)
		for _, p := range platforms {
			twitchEnabled[strings.TrimSpace(strings.ToLower(p))] = true
		}
	} else if twitchDropsEnabled {
		twitchEnabled = map[string]bool{
			"steam":  true,
			"gog":    true,
			"epic":   true,
			"amazon": true,
		}
	}

	itadKey := os.Getenv("TWITCH_ITAD_KEY")

	runner := func(provider string) func() {
		return func() {
			var allGames []common.Game

			if provider == "epic" {
				epicClient := epic.NewClient(*country, *locale, *includeUpcoming)
				epicGames, err := epicClient.FetchFreeGames()
				if err != nil {
					log.Printf("[epic] error fetching: %v", err)
				} else {
					log.Printf("[epic] found %d free game(s)", len(epicGames))
					allGames = append(allGames, epicGames...)
				}
			}

			if provider == "steam" && *enableSteam {
				steamScraper := steam.NewScraper()
				steamGames, err := steamScraper.FetchFreeGames()
				if err != nil {
					log.Printf("[steam] error fetching: %v", err)
				} else {
					log.Printf("[steam] found %d free game(s)", len(steamGames))
					allGames = append(allGames, steamGames...)
				}
			}

			if provider == "twitch-drops" && len(twitchEnabled) > 0 {
				twitchClient := twitch.NewClient(twitchEnabled, itadKey, true)
				twitchGames, err := twitchClient.FetchDrops()
				if err != nil {
					log.Printf("[twitch-drops] error fetching: %v", err)
				} else {
					log.Printf("[twitch-drops] found %d free game(s)", len(twitchGames))
					allGames = append(allGames, twitchGames...)
				}
			}

			if len(allGames) == 0 {
				log.Printf("[%s] no free games found", provider)
				return
			}

			// Filter out duplicates using notification store (keyed on provider+title)
			if notifStore != nil {
				filtered, err := notifStore.FilterNew(allGames)
				if err != nil {
					log.Printf("[%s] warning: store error: %v", provider, err)
				}
				allGames = filtered
			}

			if len(allGames) == 0 {
				log.Printf("[%s] no new games after deduplication", provider)
				return
			}

			if err := discord.Send(*discordWebhook, allGames); err != nil {
				log.Printf("[%s] error sending Discord notification: %v", provider, err)
			} else {
				log.Printf("[%s] notification sent for %d game(s)", provider, len(allGames))
			}
		}
	}

	if *runOnce {
		log.Println("Running all scrapers once (no cron)...")
		runner("epic")()
		if *enableSteam {
			runner("steam")()
		}
		if len(twitchEnabled) > 0 {
			runner("twitch-drops")()
		}
		return
	}

	c := cron.New()

	// Epic schedule: default Thursday midnight, overrideable via EPIC_SCHEDULE
	epicSchedule := envOr("EPIC_SCHEDULE", "0 0 0 * * 4")
	if *cronSchedule != "" && envOr("EPIC_SCHEDULE", "") == "" {
		epicSchedule = *cronSchedule
	}
	if _, err := c.AddFunc(epicSchedule, runner("epic")); err != nil {
		log.Fatalf("Invalid EPIC_SCHEDULE %q: %v", epicSchedule, err)
	}
	log.Printf("[epic] scheduled: %s", epicSchedule)

	// Steam schedule: default daily at 9am, overrideable via STEAM_SCHEDULE
	if *enableSteam {
		steamSchedule := envOr("STEAM_SCHEDULE", "0 0 9 * * *")
		if _, err := c.AddFunc(steamSchedule, runner("steam")); err != nil {
			log.Fatalf("Invalid STEAM_SCHEDULE %q: %v", steamSchedule, err)
		}
		log.Printf("[steam] scheduled: %s", steamSchedule)
	}

	// Twitch drops schedule: default daily at noon, overrideable via TWITCH_DROPS_SCHEDULE
	if len(twitchEnabled) > 0 {
		tdSchedule := envOr("TWITCH_DROPS_SCHEDULE", "0 0 12 * * *")
		if _, err := c.AddFunc(tdSchedule, runner("twitch-drops")); err != nil {
			log.Fatalf("Invalid TWITCH_DROPS_SCHEDULE %q: %v", tdSchedule, err)
		}
		log.Printf("[twitch-drops] scheduled: %s", tdSchedule)
	}

	log.Println("free-games service started")
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