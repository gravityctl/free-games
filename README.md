# free-games

Scrapes free games from Epic Games Store, Steam, and Twitch Drops and sends Discord webhook notifications.

## Quick Start

```bash
cp .env.example .env
# Edit .env with your DISCORD_WEBHOOK_URL

go mod download
go run .
```

## Usage

```bash
# Run once (all enabled scrapers, no cron)
go run . --once

# Custom schedule (every day at 9am — applies to all providers if not overridden)
go run . --schedule "0 0 9 * * *"

# Enable Steam scraper
go run . --steam
```

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `DISCORD_WEBHOOK_URL` | _(required)_ | Discord webhook URL |
| `EPIC_COUNTRY` | `US` | Epic store country code |
| `EPIC_LOCALE` | `en-US` | Epic store locale |
| `EPIC_INCLUDE_UPCOMING` | `false` | Include upcoming Epic free games |
| `ENABLE_STEAM` | `false` | Enable Steam scraper |
| `ENABLE_TWITCH_DROPS` | `false` | Enable Twitch drops scraper (Minecraft cape drops) |
| `TWITCH_DROPS_PLATFORMS` | _(all)_ | Comma-separated platforms: steam,gog,epic,amazon |
| `TWITCH_ITAD_KEY` | _(empty)_ | Optional isthereanydeal.com API key |
| `EPIC_SCHEDULE` | `0 0 0 * * 4` | Cron schedule for Epic scraper |
| `STEAM_SCHEDULE` | `0 0 9 * * *` | Cron schedule for Steam scraper |
| `TWITCH_DROPS_SCHEDULE` | `0 0 12 * * *` | Cron schedule for Twitch drops |
| `NOTIFICATION_STORE_PATH` | `.free-games-store.json` | Deduplication store path |

## Supported Providers

- [x] Epic Games Store (weekly free games — default: Thursdays at midnight)
- [x] Steam (free games excluding Free-to-Play — default: daily at 9am)
- [x] Twitch Drops (Minecraft cape drops — default: daily at noon)

## Deduplication

The service maintains a notification store (`NOTIFICATION_STORE_PATH`) to avoid sending duplicate Discord notifications. Once a game is notified, it is not notified again unless a new offer window starts.

## Twitch Drops

Twitch support filters by **distribution platform** (which store you can claim from).

### Supported Platforms

- **steam** — Steam store
- **gog** — GOG.com
- **epic** — Epic Games Store
- **amazon** — Amazon Games

Example: `TWITCH_DROPS_PLATFORMS=steam,gog,epic`

### Cross-Platform Lookups

Without `TWITCH_ITAD_KEY`, all drops are included since platform info isn't available from the Twitch API alone. To enable filtering, get a free API key from [isthereanydeal.com](https://isthereanydeal.com) and set `TWITCH_ITAD_KEY`.

## Docker

```bash
# Build and run
docker compose up -d

# View logs
docker compose logs -f

# Stop
docker compose down
```

The notification store (`.free-games-store.json`) is persisted in a named volume and survives restarts.

## Scheduling

Each provider can have its own cron schedule:

```
EPIC_SCHEDULE=0 0 0 * * 4        # Every Thursday at midnight
STEAM_SCHEDULE=0 0 9 * * *        # Daily at 9am
TWITCH_DROPS_SCHEDULE=0 0 12 * * * # Daily at noon
```

Cron format: `second minute hour day-of-month month day-of-week`. Use `0` for seconds if unsure.