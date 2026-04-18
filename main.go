package main

import (
	"encoding/json"
	"flag"
	"log"
	"net/http"
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
	serveAddr := flag.String("addr", envOr("ADDR", "0.0.0.0:8080"), "HTTP server address")
	flag.Parse()

	if *discordWebhook == "" {
		log.Fatal("DISCORD_WEBHOOK_URL is required")
	}

	notifStore, err := notification.NewNotificationStore(*storePath)
	if err != nil {
		log.Printf("Warning: could not open notification store: %v", err)
	}

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
		twitchEnabled = map[string]bool{"steam": true, "gog": true, "epic": true, "amazon": true}
	}

	itadKey := os.Getenv("TWITCH_ITAD_KEY")

	customEmojis := make(map[string]string)
	if e := os.Getenv("EPIC_EMOJI"); e != "" {
		customEmojis["epic"] = e
	}
	if e := os.Getenv("STEAM_EMOJI"); e != "" {
		customEmojis["steam"] = e
	}
	if e := os.Getenv("TWITCH_EMOJI"); e != "" {
		customEmojis["twitch"] = e
	}
	redirectBase := os.Getenv("REDIRECT_BASE_URL")

	mux := http.NewServeMux()

	// Redirect: /<provider>/<slug> -> desktop app deep link
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path == "" || !strings.Contains(path, "/") {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		parts := strings.SplitN(path, "/", 2)
		if len(parts) < 2 {
			http.Error(w, "invalid path", http.StatusBadRequest)
			return
		}
		provider, slug := parts[0], parts[1]
		var dest string
		switch provider {
		case "epic":
			dest = "com.epicgames.launcher://store/p/" + slug
		case "steam":
			dest = "steam://store/" + slug
		case "twitch":
			dest = "twitch://stream/" + slug
		default:
			http.Error(w, "unknown provider", http.StatusBadRequest)
			return
		}
		log.Printf("Redirect /%s/%s -> %s", provider, slug, dest)
		http.Redirect(w, r, dest, http.StatusMovedPermanently)
	})

	// GET /games — all providers or filtered by ?provider=epic|steam|twitch_drops
	mux.HandleFunc("/games", func(w http.ResponseWriter, r *http.Request) {
		filter := r.URL.Query().Get("provider")
		games := fetchAllGames(filter, *country, *locale, *includeUpcoming, *enableSteam, twitchEnabled, itadKey)
		writeJSON(w, games)
	})

	// GET /games/epic
	mux.HandleFunc("/games/epic", func(w http.ResponseWriter, r *http.Request) {
		games, _ := epic.NewClient(*country, *locale, *includeUpcoming).FetchFreeGames()
		writeJSON(w, games)
	})

	// GET /games/steam
	mux.HandleFunc("/games/steam", func(w http.ResponseWriter, r *http.Request) {
		if !*enableSteam {
			http.Error(w, "steam not enabled", http.StatusServiceUnavailable)
			return
		}
		games, _ := steam.NewScraper().FetchFreeGames()
		writeJSON(w, games)
	})

	// GET /games/twitch_drops
	mux.HandleFunc("/games/twitch_drops", func(w http.ResponseWriter, r *http.Request) {
		if len(twitchEnabled) == 0 {
			http.Error(w, "twitch drops not enabled", http.StatusServiceUnavailable)
			return
		}
		games, _ := twitch.NewClient(twitchEnabled, itadKey, true).FetchDrops()
		writeJSON(w, games)
	})

	runner := func(provider string) func() {
		return func() {
			games := fetchProviderGames(provider, *country, *locale, *includeUpcoming, *enableSteam, twitchEnabled, itadKey)
			if len(games) == 0 {
				log.Printf("[%s] no free games found", provider)
				return
			}
			if notifStore != nil {
				filtered, err := notifStore.FilterNew(games)
				if err != nil {
					log.Printf("[%s] warning: store error: %v", provider, err)
				}
				games = filtered
			}
			if len(games) == 0 {
				log.Printf("[%s] no new games after deduplication", provider)
				return
			}
			if err := discord.Send(*discordWebhook, games, customEmojis, redirectBase); err != nil {
				log.Printf("[%s] error sending Discord notification: %v", provider, err)
			} else {
				log.Printf("[%s] notification sent for %d game(s)", provider, len(games))
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

	epicSchedule := envOr("EPIC_SCHEDULE", "0 0 0 * * 4")
	if *cronSchedule != "" && envOr("EPIC_SCHEDULE", "") == "" {
		epicSchedule = *cronSchedule
	}
	if _, err := c.AddFunc(epicSchedule, runner("epic")); err != nil {
		log.Fatalf("Invalid EPIC_SCHEDULE %q: %v", epicSchedule, err)
	}
	log.Printf("[epic] scheduled: %s", epicSchedule)

	if *enableSteam {
		steamSchedule := envOr("STEAM_SCHEDULE", "0 0 9 * * *")
		if _, err := c.AddFunc(steamSchedule, runner("steam")); err != nil {
			log.Fatalf("Invalid STEAM_SCHEDULE %q: %v", steamSchedule, err)
		}
		log.Printf("[steam] scheduled: %s", steamSchedule)
	}

	if len(twitchEnabled) > 0 {
		tdSchedule := envOr("TWITCH_DROPS_SCHEDULE", "0 0 12 * * *")
		if _, err := c.AddFunc(tdSchedule, runner("twitch-drops")); err != nil {
			log.Fatalf("Invalid TWITCH_DROPS_SCHEDULE %q: %v", tdSchedule, err)
		}
		log.Printf("[twitch-drops] scheduled: %s", tdSchedule)
	}

	go func() {
		log.Printf("Server listening on %s", *serveAddr)
		log.Fatal(http.ListenAndServe(*serveAddr, mux))
	}()

	c.Start()
	<-make(chan struct{})
}

func fetchProviderGames(provider, country, locale string, includeUpcoming, enableSteam bool, twitchEnabled map[string]bool, itadKey string) []common.Game {
	switch provider {
	case "epic":
		games, _ := epic.NewClient(country, locale, includeUpcoming).FetchFreeGames()
		return games
	case "steam":
		if enableSteam {
			games, _ := steam.NewScraper().FetchFreeGames()
			return games
		}
	case "twitch-drops":
		if len(twitchEnabled) > 0 {
			games, _ := twitch.NewClient(twitchEnabled, itadKey, true).FetchDrops()
			return games
		}
	}
	return nil
}

func fetchAllGames(filter, country, locale string, includeUpcoming, enableSteam bool, twitchEnabled map[string]bool, itadKey string) []common.Game {
	var all []common.Game
	if filter == "" || filter == "epic" {
		if games, _ := epic.NewClient(country, locale, includeUpcoming).FetchFreeGames(); len(games) > 0 {
			all = append(all, games...)
		}
	}
	if (filter == "" || filter == "steam") && enableSteam {
		if games, _ := steam.NewScraper().FetchFreeGames(); len(games) > 0 {
			all = append(all, games...)
		}
	}
	if (filter == "" || filter == "twitch_drops") && len(twitchEnabled) > 0 {
		if games, _ := twitch.NewClient(twitchEnabled, itadKey, true).FetchDrops(); len(games) > 0 {
			all = append(all, games...)
		}
	}
	return all
}

func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	json.NewEncoder(w).Encode(v)
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
