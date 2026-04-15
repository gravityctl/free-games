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

### Available owners
Set `TWITCH_OWNERS` to a comma-separated list of owner names. Current available owners:

- **Bandai/Namco** — Dragon Ball Gekishin Squadra
- **Blizzard** — World of Warcraft, Overwatch
- **Combat Cat Studio** — Wild Assault
- **Cognosphere** — Honkai: Star Rail, Genshin Impact
- **Electronic Arts** — EA Sports College Football 26
- **Embark Studios** — ARC Raiders
- **IGG** — Viking Rise
- **INFOLD PTE. LTD.** — Infinity Nikki
- **KURO GAMES** — Wuthering Waves
- **KRAFTON Inc.** — PUBG: Battlegrounds
- **Level Infinite** — Dragonheir: Silent Gods, Arena Breakout: Infinite
- **Litty Games** — Coin Pusher Live
- **MADFINGER Games** — Gray Zone Warfare
- **Marvel Rivals** — Marvel Rivals
- **NetEase** — NARAKA: BLADEPOINT
- **Netmarble** — Mongil: Star Dive
- **Nimble Neuron Games** — Eternal Return
- **Omeda Studios** — Predecessor
- **OneWay Ticket Studio** — The Midnight Walkers
- **Out of the Park Developments** — Out of the Park Baseball 27
- **Pearl Abyss** — Black Desert
- **Pixile Studios** — Super Animal Royale
- **Qoolandgames** — Soulmask
- **Red Barrels** — The Outlast Trials
- **Riot Games** — League of Legends
- **Roko Game Studios** — Rise Online
- **Sandbox Interactive** — Albion Online
- **Scopely** — MARVEL Strike Force
- **Sharkmob** — Vampire: The Masquerade - Bloodhunt
- **Starry** — Once Human
- **The Pokémon Company** — Pokémon Trading Card Game Live
- **Twitch Gaming** — Dead by Daylight, Don't Starve Together, Shakes and Fidget, Rainbow Six Siege, EVE Online, Mir Korabley, MARVEL Contest of Champions, SMITE 2, Just Chatting, Mir Tankov, Mobile Dungeon, Minecraft, Windrose, Science & Technology, QSMP
- **Ubisoft** — Brawlhalla
- **Vawraek Technology** — The Quinfall
- **ViVa Games** — Kakele Online - MMORPG
- **Wargaming** — World of Tanks, World of Warships, World of Tanks Console
- **Wemade Entertainment** — Legend of YMIR
- **Wolvesville** — Wolvesville
- **Zenimax Online Studios** — The Elder Scrolls Online
- **1047 Games** — SPLITGATE: Arena Reloaded
- **2K Games** — NBA 2K26
- **A PLUS JAPAN Inc.** — Blue Protocol: Star Resonance
- **Artstorm FZE** — Modern Warships
- **Bad Guitar Studio** — FragPunk
- **IO Interactive** — HITMAN World of Assassination

Example: `TWITCH_OWNERS=Wargaming,Riot Games,Blizzard,Electronic Arts`