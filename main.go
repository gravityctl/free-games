package main

import (
	"encoding/json"
	"flag"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

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

	discordWebhooks := flag.String("discord-webhook", envOr("DISCORD_WEBHOOK_URL", ""), "Comma-separated Discord webhook URL(s)")
	country := flag.String("country", envOr("EPIC_COUNTRY", "US"), "Epic store country code")
	locale := flag.String("locale", envOr("EPIC_LOCALE", "en-US"), "Epic store locale")
	includeUpcoming := flag.Bool("include-upcoming", envOrBool("EPIC_INCLUDE_UPCOMING", false), "Include upcoming free games")
	enableSteam := flag.Bool("steam", envOrBool("ENABLE_STEAM", false), "Enable Steam scraper")
	steamCountry := flag.String("steam-country", envOr("STEAM_COUNTRY", "US"), "Steam store country code")
	steamLocale := flag.String("steam-locale", envOr("STEAM_LOCALE", "en"), "Steam store language")
	cronSchedule := flag.String("schedule", envOr("CHECK_SCHEDULE", ""), "Legacy cron schedule (used if no per-provider schedule set)")
	runOnce := flag.Bool("once", false, "Run all enabled scrapers once and exit (no cron)")
	storePath := flag.String("store", envOr("NOTIFICATION_STORE_PATH", ".free-games-store.json"), "Path to notification deduplication store")
	serveAddr := flag.String("addr", envOr("ADDR", "0.0.0.0:8080"), "HTTP server address")
	flag.Parse()

	webhookList := parseWebhooks(*discordWebhooks)
	if len(webhookList) == 0 {
		log.Fatal("DISCORD_WEBHOOK_URL is required")
	}

	notifStore, err := notification.NewNotificationStore(*storePath)
	if err != nil {
		log.Printf("Warning: could not open notification store: %v", err)
	}
	if notifStore != nil {
		if d, err := time.ParseDuration(envOr("NOTIFICATION_RENOTIFY_AFTER", "")); err == nil {
			notifStore.SetRenotifyAfter(d)
		}
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

	cfg := providerConfig{
		country:         *country,
		locale:          *locale,
		includeUpcoming: *includeUpcoming,
		enableSteam:     *enableSteam,
		steamCountry:    *steamCountry,
		steamLocale:     *steamLocale,
		twitchEnabled:   twitchEnabled,
		itadKey:         itadKey,
	}

	// GET /games — all providers or filtered by ?provider=epic|steam|twitch_drops
	mux.HandleFunc("/games", func(w http.ResponseWriter, r *http.Request) {
		filter := r.URL.Query().Get("provider")
		writeJSON(w, fetchAllGames(filter, cfg))
	})

	// GET /games/epic
	mux.HandleFunc("/games/epic", func(w http.ResponseWriter, r *http.Request) {
		games, err := fetchProviderGames("epic", cfg)
		if err != nil {
			log.Printf("[epic] fetch failed: %v", err)
		}
		writeJSON(w, games)
	})

	// GET /games/steam
	mux.HandleFunc("/games/steam", func(w http.ResponseWriter, r *http.Request) {
		if !*enableSteam {
			http.Error(w, "steam not enabled", http.StatusServiceUnavailable)
			return
		}
		games, err := fetchProviderGames("steam", cfg)
		if err != nil {
			log.Printf("[steam] fetch failed: %v", err)
		}
		writeJSON(w, games)
	})

	// GET /games/twitch_drops
	mux.HandleFunc("/games/twitch_drops", func(w http.ResponseWriter, r *http.Request) {
		if len(twitchEnabled) == 0 {
			http.Error(w, "twitch drops not enabled", http.StatusServiceUnavailable)
			return
		}
		games, err := fetchProviderGames("twitch-drops", cfg)
		if err != nil {
			log.Printf("[twitch-drops] fetch failed: %v", err)
		}
		writeJSON(w, games)
	})

	runner := func(provider string) func() {
		return func() {
			games, err := fetchProviderGames(provider, cfg)
			// A failed fetch is not the same as "nothing is free" — say so, or a
			// broken scraper looks like a quiet week.
			if err != nil {
				log.Printf("[%s] fetch failed: %v", provider, err)
				return
			}
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
			// Only record games once a webhook has accepted them, so a failed
			// delivery is retried on the next run rather than lost.
			if delivered := discord.SendAll(webhookList, games, customEmojis, redirectBase); delivered > 0 {
				if notifStore != nil {
					if err := notifStore.Record(games); err != nil {
						log.Printf("[%s] warning: could not record sent games: %v", provider, err)
					}
				}
			} else {
				log.Printf("[%s] delivery failed for all webhooks; will retry next run", provider)
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

// providerConfig holds the per-provider settings resolved from flags and env.
type providerConfig struct {
	country         string
	locale          string
	includeUpcoming bool
	enableSteam     bool
	steamCountry    string
	steamLocale     string
	twitchEnabled   map[string]bool
	itadKey         string
}

func fetchProviderGames(provider string, cfg providerConfig) ([]common.Game, error) {
	switch provider {
	case "epic":
		return epic.NewClient(cfg.country, cfg.locale, cfg.includeUpcoming).FetchFreeGames()
	case "steam":
		if cfg.enableSteam {
			return steam.NewScraperFor(cfg.steamCountry, cfg.steamLocale).FetchFreeGames()
		}
	case "twitch-drops":
		if len(cfg.twitchEnabled) > 0 {
			return twitch.NewClient(cfg.twitchEnabled, cfg.itadKey, true).FetchDrops()
		}
	}
	return nil, nil
}

func fetchAllGames(filter string, cfg providerConfig) []common.Game {
	providers := []struct{ filterName, provider string }{
		{"epic", "epic"},
		{"steam", "steam"},
		{"twitch_drops", "twitch-drops"},
	}

	var all []common.Game
	for _, p := range providers {
		if filter != "" && filter != p.filterName {
			continue
		}
		games, err := fetchProviderGames(p.provider, cfg)
		if err != nil {
			log.Printf("[%s] fetch failed: %v", p.provider, err)
			continue
		}
		all = append(all, games...)
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

// parseWebhooks splits a comma-separated string of webhook URLs into a slice.
// Empty strings are filtered out.
func parseWebhooks(raw string) []string {
	var out []string
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}
