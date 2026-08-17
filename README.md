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
| `STEAM_COUNTRY` | `US` | Steam store country code |
| `STEAM_LOCALE` | `en` | Steam store language |
| `ENABLE_TWITCH_DROPS` | `false` | Enable Twitch drops scraper (Minecraft cape drops) |
| `TWITCH_DROPS_PLATFORMS` | _(all)_ | Comma-separated platforms: steam,gog,epic,amazon |
| `TWITCH_ITAD_KEY` | _(empty)_ | Optional isthereanydeal.com API key |
| `EPIC_SCHEDULE` | `0 0 0 * * 4` | Cron schedule for Epic scraper |
| `STEAM_SCHEDULE` | `0 0 9 * * *` | Cron schedule for Steam scraper |
| `TWITCH_DROPS_SCHEDULE` | `0 0 12 * * *` | Cron schedule for Twitch drops |
| `NOTIFICATION_STORE_PATH` | `.free-games-store.json` | Deduplication store path |
| `NOTIFICATION_RENOTIFY_AFTER` | `720h` | Cooldown for offers with no end date (Steam) |

## Supported Providers

- [x] Epic Games Store (weekly free games — default: Thursdays at midnight)
- [x] Steam (free games excluding Free-to-Play — default: daily at 9am)
- [x] Twitch Drops (Minecraft cape drops — default: daily at noon)

## How free games are identified

Both stores publish paid promotions and giveaways through the same feeds, so each
provider is checked against the signal that actually distinguishes them.

**Epic** — the `freeGamesPromotions` feed is everything queued for Epic's "Free
Games" row, including ordinary sales. An offer counts only when its
`discountSetting.discountPercentage` is `0`, which is Epic's way of saying the
buyer pays nothing; a scheduled 50%-off promotion is skipped. Delisted
(non-`ACTIVE`) offers are skipped too, and store links are built from Epic's
`productHome` page mapping so bundles and opaque offer ids do not produce dead
links.

**Steam** — a game given away at 100% off is still a *paid* game, so the store
API reports `is_free: false` for it (that flag means free-to-play) and returns a
`price_overview` whose `final` price is `0`. Candidates come from the store's own
free-and-discounted search filter, plus a price-sorted sweep of specials as a
safety net, and every candidate is confirmed against `price_overview` before it is
reported. Free-to-play games, DLC and demos are excluded.

## Deduplication

The service maintains a notification store (`NOTIFICATION_STORE_PATH`) to avoid
sending duplicate Discord notifications. A game is notified once per offer window:
it is announced again when it comes back under a new window, which for Epic means
a different start date and for Steam — where offers carry no dates — means after
`NOTIFICATION_RENOTIFY_AFTER` has elapsed.

Games are recorded as sent only after a Discord webhook accepts them, so a failed
delivery is retried on the next run instead of being silently dropped.

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