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

# Enable Twitch drops for specific owners
go run . --twitch-owners "Wargaming,Riot Games,Blizzard"

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
| `TWITCH_OWNERS` | _(empty)_ | Comma-separated Twitch drop owner names to include |
| `CHECK_SCHEDULE` | `0 0 0 * * 4` | Cron schedule (Thursdays midnight) |

## Supported Providers

- [x] Epic Games Store (weekly free games)
- [x] Steam (free games excluding Free-to-Play)
- [x] Twitch Drops (configurable per-owner filtering)

## Twitch Drops

Twitch support requires specifying which drop owners to include via `TWITCH_OWNERS`. This allows fine-grained control over which games trigger notifications.

Example owners: `Wargaming`, `Riot Games`, `Blizzard`, `Electronic Arts`, `Nimble Neuron Games`, `Madfinger Games`, etc.