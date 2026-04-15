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
# Run with cron (default: every Thursday at midnight)
go run .

# Run once and exit (useful for testing)
go run . --once

# Custom schedule (every day at 9am)
go run . --schedule "0 0 9 * * *"

# Enable Steam scraper
go run . --steam

# Enable Twitch drops for specific platforms
go run . --twitch-platforms "steam,gog,epic"

# Custom country/locale for Epic
go run . --country US --locale en-US
```

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `DISCORD_WEBHOOK_URL` | _(required)_ | Discord webhook URL |
| `EPIC_COUNTRY` | `US` | Epic store country code |
| `EPIC_LOCALE` | `en-US` | Epic store locale |
| `EPIC_INCLUDE_UPCOMING` | `false` | Include upcoming Epic free games |
| `ENABLE_STEAM` | `false` | Enable Steam scraper |
| `TWITCH_PLATFORMS` | _(empty)_ | Comma-separated Twitch drop platforms to include |
| `TWITCH_ITAD_KEY` | _(empty)_ | Optional isthereanydeal.com API key for cross-platform lookups |
| `CHECK_SCHEDULE` | `0 0 0 * * 4` | Cron schedule (Thursdays midnight) |

## Supported Providers

- [x] Epic Games Store (weekly free games)
- [x] Steam (free games excluding Free-to-Play)
- [x] Twitch Drops (configurable per-platform filtering)

## Twitch Drops

Twitch support filters by **distribution platform** (which store you can claim from).

### Supported Platforms

Set `TWITCH_PLATFORMS` to a comma-separated list of platforms:

- **steam** — Steam store
- **gog** — GOG.com
- **epic** — Epic Games Store
- **amazon** — Amazon Games

Example: `TWITCH_PLATFORMS=steam,gog,epic`

### Cross-Platform Lookups

Without an `TWITCH_ITAD_KEY`, all drops are included since platform info isn't available from the Twitch API. To enable filtering, get a free API key from [isthereanydeal.com](https://isthereanydeal.com) and set `TWITCH_ITAD_KEY`.

When configured, the service looks up each game on ITAD to determine which stores it's available on, and only includes drops that match your enabled platforms.