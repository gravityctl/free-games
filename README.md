# free-games

Scrapes free games from Epic Games Store (and optionally Steam) and sends Discord webhook notifications.

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
| `CHECK_SCHEDULE` | `0 0 0 * * 4` | Cron schedule (Thursdays midnight) |

## Supported Providers

- [x] Epic Games Store (weekly free games)
- [x] Steam (free games excluding Free-to-Play)
- [ ] Twitch
