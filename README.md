<p align="center">
  <img src="https://repository-images.githubusercontent.com/1192848219/a2ce3e7f-f4d1-4a74-85ae-0c50c78cf352" alt="GachaBot Banner" width="100%">
</p>

<h1 align="center">GachaBot 🃏</h1>

<p align="center">
  <i>A cross-platform card-collecting (gacha) bot for Telegram & Discord, with a web Mini App — written in Go.</i>
</p>

<p align="center">
  <b>English</b> · <a href="README.ru.md">Русский</a>
</p>

<p align="center">
  <img src="https://img.shields.io/badge/Go-1.25-00ADD8?style=flat&logo=go" alt="Go">
  <img src="https://img.shields.io/badge/Postgres-15-%23316192.svg?logo=postgresql&logoColor=white" alt="PostgreSQL">
  <img src="https://img.shields.io/badge/Redis-7-DC382D?style=flat&logo=redis" alt="Redis">
  <img src="https://img.shields.io/badge/React-19-61DAFB?style=flat&logo=react&logoColor=black" alt="React">
  <img src="https://img.shields.io/badge/Docker-Ready-2496ED?style=flat&logo=docker" alt="Docker">
  <img src="https://img.shields.io/badge/License-MIT-green.svg" alt="MIT">
</p>

## 📌 About

**GachaBot** is an interactive entertainment bot built around card collecting (gacha), duels and collections. It runs on **Telegram and Discord** at once over a shared database, so players can link their accounts (`/link`) and keep one progression across both platforms. On top of the chat bots there is a **web app** (SPA) that works as a Telegram **Mini App**, a Discord **Activity**, and a standalone site.

Built for reliability, scalability and clean code — also as a learning project for Go, SQL and CI/CD.

## ✨ Key features

- **Cross-platform:** one business core serves three delivery layers — Telegram, Discord and HTTP/web.
- **Clean architecture:** strict `delivery` → `service` → `repository` separation; Redis for ephemeral state.
- **Gameplay:** weighted-random rolls with a Pity system, mythic crafting from fragments, power-based duels with set auras, daily streaks.
- **Chat card spawns:** the bot drops a "wild card" into chats; first to press **Catch** / `/claim` wins it (configurable schedule & pool).
- **Web app (Mini App / Activity / browser):** collection browser, profile, admin panel — served by the Go binary on the same origin as the API.
- **Social/admin:** leaderboards, advanced promo codes (JSON rewards, limits), Telegram Stars purchases, user-generated card submissions with moderation.
- **i18n:** full RU/EN localization with per-user language.
- **Infra:** Docker, GitHub Actions (build → push image), daily DB backups.

## 🏗 Architecture

```text
├── cmd/bot/main.go         # entry point, DI wiring
├── internal/
│   ├── delivery/
│   │   ├── telegram/       # Telegram bot (telebot.v3)
│   │   ├── discord/        # Discord bot (discordgo)
│   │   └── httpapi/        # HTTP API + static SPA (chi)
│   ├── service/            # gacha, duel, spawn, suggest, backup
│   ├── repository/         # PostgreSQL queries (lib/pq)
│   ├── models/             # domain types
│   ├── migrations/         # embedded, auto-applied SQL migrations
│   ├── theme/              # declarative content (rarities/sets/cards)
│   └── i18n/               # localization
├── web/                    # frontend SPA (git submodule → gacha-nova), built into the image
├── locales/                # JSON translation dictionaries
├── tools/                  # admin HTML helpers (theme & spawn config editors)
├── docs/deploy.md          # deployment guide (prebuilt image + reverse proxy)
└── docker-compose.yml      # local build stack (bot, DB, Redis, backups)
```

## 🛠 Stack

- **Backend:** Go · PostgreSQL 15 (`lib/pq`) · Redis 7 (`go-redis/v9`) · `chi` HTTP router
- **Bots:** `telebot.v3` (Telegram) · `discordgo` (Discord)
- **Frontend:** React 19 + Vite + Tailwind v4 (static SPA, served by the Go binary)
- **Deploy:** Docker · GitHub Actions

## 🚀 Self-hosting

The whole thing (bot + API + web) ships as one Docker image. The frontend lives in
the **`web` git submodule**, so clone recursively.

### 1. Clone (with submodules)
```bash
git clone --recursive https://github.com/BasteArima/gachabot.git
cd gachabot
```

### 2. Configure
```bash
cp .env.example .env
# edit .env — see the table below
```

| Variable | Required | Description |
|----------|----------|-------------|
| `TELEGRAM_BOT_TOKEN` | ✅ | from @BotFather |
| `ADMIN_TELEGRAM_ID` | ✅ | your numeric Telegram id (bot owner / admin) |
| `POSTGRES_USER` / `POSTGRES_PASSWORD` / `POSTGRES_DB` | ✅ | database credentials |
| `DISCORD_TOKEN` | – | enables the Discord bot |
| `ADMIN_DISCORD_ID` | – | bot owner's Discord id (owner-only Discord commands) |
| `DISCORD_CLIENT_ID` / `DISCORD_CLIENT_SECRET` / `DISCORD_OAUTH_REDIRECT` | – | Discord web/Activity login |
| `WEB_APP_URL` | – | public HTTPS URL of the web app |
| `REQUIRE_18_PLUS_CONFIRM` / `COOLDOWN_HOURS` / `ENABLE_DUPLICATES` | – | gameplay toggles |
| `VITE_TG_BOT_USERNAME` / `VITE_DISCORD_CLIENT_ID` | – | **build-time** web vars (login buttons) |

### 3. Run
```bash
docker compose up -d --build
```
This builds the image (Go + frontend), starts PostgreSQL (with the initial schema),
Redis, daily backups and the bot. The bot also serves the web app + JSON API on
port **8080** inside the container.

### 4. Expose the web app (optional)
The Mini App / Activity / site needs HTTPS. Put a reverse proxy (e.g. Nginx Proxy
Manager) in front, forwarding your domain to the bot container on `:8080`, then set
`WEB_APP_URL`, the BotFather Mini App URL, and (for Discord) the Activity URL mapping.

> **Advanced / production** (prebuilt image from GHCR + Portainer, DB-safe migration):
> see [docs/deploy.md](docs/deploy.md).

## 🗺 Roadmap

- [x] Shared DB across Telegram & Discord, account linking, i18n
- [x] Streaks, Pity, crafting, duels, sets/auras, promo codes, Stars, UGC submissions
- [x] **Chat card spawns** (catch-the-card)
- [x] **Web app**: HTTP API + SPA (Telegram Mini App, Discord Activity, browser), inventory, admin panel
- [ ] Player-to-player trades
- [ ] Favorite card on profile
- [ ] World Boss · daily card-guess game · lottery
- [ ] Move owner tools into the web admin panel

## 📄 License

[MIT](LICENSE) — feel free to self-host and adapt.

## 👤 Author

**Heather Arima (Shamil Baste)** — Go backend & game developer · [@BasteArima](https://github.com/BasteArima)
