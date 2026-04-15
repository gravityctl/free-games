package main

import (
	"flag"
	"log"
	"os"

	"github.com/gravityctl/free-games/discord"
	"github.com/gravityctl/free-games/epic"
	"github.com/joho/godotenv"
	"github.com/robfig/cron/v3"
)

func main() {
	godotenv.Load()

	discordWebhook := flag.String("discord-webhook", envOr("DISCORD_WEBHOOK_URL", ""), "Discord webhook URL")
	country := flag.String("country", envOr("EPIC_COUNTRY", "US"), "Epic store country code")
	locale := flag.String("locale", envOr("EPIC_LOCALE", "en-US"), "Epic store locale")
	cronSchedule := flag.String("schedule", envOr("CHECK_SCHEDULE", "0 0 * * 4"), "Cron schedule (default: every Thursday at midnight)")
	runOnce := flag.Bool("once", false, "Run once and exit (no cron)")
	flag.Parse()

	if *discordWebhook == "" {
		log.Fatal("DISCORD_WEBHOOK_URL is required")
	}

	epicClient := epic.NewClient(*country, *locale)

	runner := func() {
		games, err := epicClient.FetchFreeGames()
		if err != nil {
			log.Printf("Error fetching games: %v", err)
			return
		}
		if len(games) == 0 {
			log.Println("No free games found this week")
			return
		}

		log.Printf("Found %d free game(s): %v", len(games), games)

		if err := discord.Send(*discordWebhook, games); err != nil {
			log.Printf("Error sending Discord notification: %v", err)
		} else {
			log.Printf("Notification sent for %d game(s)", len(games))
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

	log.Printf("free-games service started. Checking %s every %s", *country, *cronSchedule)
	c.Start()

	<-make(chan struct{})
}

func envOr(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}
