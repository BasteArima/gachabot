# CLAUDE.md — agent orientation

Read this first. It's the fast path to understanding the project so you don't have
to re-analyze everything. (User-facing docs: `README.md` / `README.ru.md`.)

## What this is

Cross-platform **gacha card bot** (Telegram + Discord) in **Go**, plus a **web app**
(React SPA) that runs as a Telegram Mini App, a Discord Activity, and a plain site.
Shared PostgreSQL + Redis. Goal: a fun, polished activity "for friends" (not
monetization) and a learning project. Maintainer/owner: **BasteArima** (Shamil Baste).
Repo is **public, MIT**. The bot's theme is adult/hentai cards (NSFW art).

## Orientation (where things live)

- `cmd/bot/main.go` — entry point, wires everything (DI by hand).
- `internal/delivery/{telegram,discord,httpapi}` — the three delivery layers.
- `internal/service/{gacha,duel,spawn,suggest,backup}` — business logic.
- `internal/repository/*.go` — raw SQL (PostgreSQL, `lib/pq`). One file per area.
- `internal/migrations/*.sql` — embedded, auto-applied on startup, idempotent, tracked
  in `schema_migrations`. **Add new tables as `000N_*.sql`; never edit `schema.sql`**
  (that's the initial Docker initdb seed).
- `internal/theme/` — content (rarities/sets/cards) as declarative `theme.json`,
  upserted by name. Rarity **names come from the DB/theme**, not hardcoded.
- `internal/i18n/` + `locales/{base,telegram,discord}/{ru,en}.json` — i18n. Platform
  dicts override `base`. Telegram uses HTML (`<b>`), Discord uses markdown (`**`).
- `web/` — **git submodule** → the frontend repo `BasteArima/gacha-nova` (Vite SPA).
- `tools/` — static HTML admin helpers (theme editor, spawn-config editor).
- `docs/deploy.md` — deployment guide. **Design docs live in the maintainer's Obsidian,
  NOT in this repo** (the old `docs/design/` was removed).

## Architecture notes

- Clean layering: delivery → service → repository. Business logic never in delivery.
- Redis: ephemeral state (cooldowns, sessions, spawn plans/claims, link tokens, caches).
- Cross-platform identity: one `users` row with `telegram_id` and/or `discord_id`;
  `/link` merges. Internal `users.id` is the canonical player id everywhere.
- Time zone is fixed **MSK** in gacha/spawn services.

## The web app (read carefully — easy to get wrong)

- `web/` is a **submodule**. To change the frontend: edit in `web/`, then
  `cd web && git add -A && git commit && git push`, then in the parent repo
  `git add web && git commit` (bump the submodule pointer) and push. If you forget the
  bump, prod builds the **old** frontend.
- Built into the Docker image at `/root/web`; the Go server (`httpapi`) serves the SPA
  (SPA fallback to `index.html`) **and** `/api/*` on the same origin (port 8080). No CORS
  in prod. `HTTP_STATIC_DIR=/root/web` is baked by the Dockerfile.
- **`vite build` does NOT type-check.** The `web` build script is `tsc --noEmit && vite build`
  for exactly this reason (a missing `api.*` method once shipped a black screen). Keep it.
- Auth: provider proof → session token (opaque, in Redis) → sent as `Authorization: Bearer`.
  - Telegram Mini App: `initData` (HMAC). Browser: Telegram Login Widget (different HMAC).
  - Discord Activity: Embedded App SDK `authorize` → code → backend exchange (no
    `redirect_uri`). Browser: Discord OAuth2 redirect (with `redirect_uri`).
  - Owner/admin: `users.telegram_id == ADMIN_TELEGRAM_ID` (web admin panel `isAdmin`).
    Discord owner-only commands gate on `ADMIN_DISCORD_ID`.
- Frontend contract is in `web/src/lib/api.ts`. Inventory rarity tabs are fetched from
  `GET /api/rarities` (don't hardcode rarity names).

## Build / test / run

```bash
go build ./...            # backend
go test ./internal/...    # unit tests (spawn scheduling, httpapi auth HMAC, i18n, theme)
go vet ./...
cd web && npm install && npm run build   # frontend (includes tsc typecheck)
```
Can't run the live bot here (needs Postgres/Redis/tokens). Verify via build+vet+tests;
for the frontend, `tools/`-style static previews work, but the authed Hub needs a backend.

## Deploy model

- CI `.github/workflows/docker.yml`: on push to `main`, builds the image and pushes to
  `ghcr.io/bastearima/gachabot`. Fetches **only** the `web` submodule (not `_content_helpers`)
  via `SUBMODULE_PAT` (gacha-nova is private). No deploy-to-server from CI.
- Maintainer deploys via **Portainer** using `docker-compose.prod.yml` (image from GHCR,
  external Postgres volume `gachabot_postgres_data` to preserve the DB). Reverse proxy
  (Nginx Proxy Manager) on a shared `proxy` network forwards the domain → `gachabot_app:8080`.
- `docker-compose.yml` is the local **build** stack (used by README self-host).
- Env: see `.env.example`. Build-time `VITE_*` come from GitHub Secrets (frontend baked
  into the image). Runtime env lives in the Portainer stack.

## Gotchas that already bit us

- Discord slash commands: register **one by one** (`ApplicationCommandCreate`), not
  `BulkOverwrite` — bulk fails (HTTP 400 / 50240) when an Activity Entry Point command exists.
- Telegram `*SendOptions` passed as a separate vararg **overrides** a separate `ParseMode`
  → HTML lost. Always bundle into one `&tele.SendOptions{ParseMode:..., ...}`.
- Spawns default **OFF** (`spawn.DefaultConfig().Enabled == false`); admin enables via
  `/spawn_import`. `/spawnnow` ignores the flag (test command, owner-only).
- Discord Activity: external resources are CSP-blocked; the Telegram SDK script is loaded
  only outside Activities (guard on `frame_id`). Card images load fine in Discord (roll
  proves it). `index.html` has an on-screen error overlay (webviews lack DevTools).
- `pq.Array(nil-slice)` serializes to SQL NULL — guard array filters with `COALESCE(cardinality(...),0)=0`.

## Current state / open items

- Done: core gacha, spawns (3 slices), web foundation (HTTP API, SPA, TG/DS/browser auth,
  inventory, admin panel with spawn-config editor + overview), avatars, leaderboard, roll/craft API.
- Verify on the maintainer's deploy: Discord Activity loads (after the TG-script CSP fix);
  Discord spawn art (testable now via Discord `/spawnnow`, owner-only).
- **Not built yet** (frontend hub shows them as WIP / disabled): World Boss, daily card-guess
  game ("Карта дня" — guess a blurred card over attempts), lottery, trades, favorite card.
  Roll/Craft/Duel/Shop quick-action row is intentionally hidden in the web hub for now.

## Planned next (maintainer's TODO)

- **Move owner tools into the web admin panel**: theme import/export, spawn config (already
  in admin), promo creation, global broadcast, spawn now/plan/reset — currently chat commands.
- Trades (design: anti-abuse first — alt-funneling, % commission sink, FOR UPDATE swap).
- World Boss (lite), daily card-guess, lottery (see maintainer's Obsidian design docs).
- Later: pull the content tooling from the `_content_helpers` submodule into the main repo.

## Working style with this maintainer

- They drive in **slices**; confirm scope, then build a slice, build/vet/test, summarize,
  and tell them exactly what to commit/deploy. Frontend changes always need the
  submodule commit/push + pointer bump.
- Keep memory current in your own memory files (`~/.claude/.../memory/`): the index is
  `MEMORY.md`; the deep notes are `daily-games-design.md` and `universalization-refactor.md`.
