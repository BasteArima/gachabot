<p align="center">
  <img src="https://repository-images.githubusercontent.com/1192848219/a2ce3e7f-f4d1-4a74-85ae-0c50c78cf352" alt="GachaBot Banner" width="100%">
</p>

<h1 align="center">GachaBot 🃏</h1>

<p align="center">
  <i>Кроссплатформенный гача-бот для Telegram и Discord с веб-приложением — на Go.</i>
</p>

<p align="center">
  <a href="README.md">English</a> · <b>Русский</b>
</p>

<p align="center">
  <img src="https://img.shields.io/badge/Go-1.25-00ADD8?style=flat&logo=go" alt="Go">
  <img src="https://img.shields.io/badge/Postgres-15-%23316192.svg?logo=postgresql&logoColor=white" alt="PostgreSQL">
  <img src="https://img.shields.io/badge/Redis-7-DC382D?style=flat&logo=redis" alt="Redis">
  <img src="https://img.shields.io/badge/React-19-61DAFB?style=flat&logo=react&logoColor=black" alt="React">
  <img src="https://img.shields.io/badge/Docker-Ready-2496ED?style=flat&logo=docker" alt="Docker">
  <img src="https://img.shields.io/badge/License-MIT-green.svg" alt="MIT">
</p>

## 📌 О проекте

**GachaBot** — интерактивный развлекательный бот с механикой гачи (коллекционирование карточек), дуэлями и коллекциями. Работает одновременно в **Telegram и Discord** на общей базе данных: игроки линкуют аккаунты (`/link`) и держат единый прогресс на обеих платформах. Поверх чат-ботов есть **веб-приложение** (SPA), которое работает как Telegram **Mini App**, Discord **Activity** и обычный сайт.

Сделан с упором на надёжность, масштабируемость и чистоту кода — и как учебный проект по Go, SQL и CI/CD.

## ✨ Ключевые возможности

- **Кроссплатформенность:** одно бизнес-ядро обслуживает три delivery-слоя — Telegram, Discord и HTTP/веб.
- **Чистая архитектура:** строгое разделение `delivery` → `service` → `repository`; Redis для временных стейтов.
- **Геймплей:** взвешенный рандом с системой Pity (гарант), крафт мификов из осколков, дуэли по силе карт с аурами сетов, ежедневные стрики.
- **Спавны карт в чате:** бот кидает «дикую карту», первый, кто нажал **Поймать** / `/claim`, забирает её (настраиваемое расписание и пул).
- **Веб-приложение (Mini App / Activity / браузер):** просмотр коллекции, профиль, админ-панель — раздаётся самим Go-бинарём на том же домене, что и API.
- **Соц./админ:** лидерборды, продвинутые промокоды (JSON-награды, лимиты), покупки за Telegram Stars, предложка карт от игроков с модерацией.
- **i18n:** полная локализация RU/EN с языком на пользователя.
- **Инфра:** Docker, GitHub Actions (сборка → пуш образа), ежедневные бэкапы БД.

## 🏗 Архитектура

```text
├── cmd/bot/main.go         # точка входа, DI
├── internal/
│   ├── delivery/
│   │   ├── telegram/       # Telegram-бот (telebot.v3)
│   │   ├── discord/        # Discord-бот (discordgo)
│   │   └── httpapi/        # HTTP API + статика SPA (chi)
│   ├── service/            # gacha, duel, spawn, suggest, backup
│   ├── repository/         # запросы к PostgreSQL (lib/pq)
│   ├── models/             # доменные типы
│   ├── migrations/         # встроенные авто-миграции SQL
│   ├── theme/              # декларативный контент (редкости/сеты/карты)
│   └── i18n/               # локализация
├── web/                    # фронт SPA (git-submodule → gacha-nova), вшивается в образ
├── locales/                # словари переводов
├── tools/                  # HTML-инструменты админа (редакторы темы и конфига спавнов)
├── docs/deploy.md          # гайд по деплою (готовый образ + reverse proxy)
└── docker-compose.yml      # локальный сборочный стек (бот, БД, Redis, бэкапы)
```

## 🛠 Стек

- **Бэкенд:** Go · PostgreSQL 15 (`lib/pq`) · Redis 7 (`go-redis/v9`) · HTTP-роутер `chi`
- **Боты:** `telebot.v3` (Telegram) · `discordgo` (Discord)
- **Фронт:** React 19 + Vite + Tailwind v4 (статический SPA, раздаётся Go-бинарём)
- **Деплой:** Docker · GitHub Actions

## 🚀 Self-hosting (поднять у себя)

Всё (бот + API + веб) едет одним Docker-образом. Фронт лежит в **git-submodule `web`**,
поэтому клонируй рекурсивно.

### 1. Клонирование (с сабмодулями)
```bash
git clone --recursive https://github.com/BasteArima/gachabot.git
cd gachabot
```

### 2. Конфиг
```bash
cp .env.example .env
# отредактируй .env — см. таблицу ниже
```

| Переменная | Обязательна | Описание |
|------------|-------------|----------|
| `TELEGRAM_BOT_TOKEN` | ✅ | от @BotFather |
| `ADMIN_TELEGRAM_ID` | ✅ | твой числовой Telegram id (владелец/админ бота) |
| `POSTGRES_USER` / `POSTGRES_PASSWORD` / `POSTGRES_DB` | ✅ | креды БД |
| `DISCORD_TOKEN` | – | включает Discord-бота |
| `ADMIN_DISCORD_ID` | – | Discord id владельца (владельческие команды Discord) |
| `DISCORD_CLIENT_ID` / `DISCORD_CLIENT_SECRET` / `DISCORD_OAUTH_REDIRECT` | – | вход через Discord (веб/Activity) |
| `WEB_APP_URL` | – | публичный HTTPS-URL веб-приложения |
| `REQUIRE_18_PLUS_CONFIRM` / `COOLDOWN_HOURS` / `ENABLE_DUPLICATES` | – | геймплейные тумблеры |
| `VITE_TG_BOT_USERNAME` / `VITE_DISCORD_CLIENT_ID` | – | **build-time** переменные фронта (кнопки логина) |

### 3. Запуск
```bash
docker compose up -d --build
```
Соберёт образ (Go + фронт), поднимет PostgreSQL (с начальной схемой), Redis,
ежедневные бэкапы и самого бота. Бот также раздаёт веб-приложение и JSON-API на
порту **8080** внутри контейнера.

### 4. Выставить веб наружу (опционально)
Mini App / Activity / сайту нужен HTTPS. Поставь reverse proxy (например, Nginx
Proxy Manager) и направь домен на контейнер бота `:8080`, затем задай `WEB_APP_URL`,
URL Mini App в BotFather и (для Discord) URL Mapping у Activity.

> **Продвинутый/продакшн** (готовый образ из GHCR + Portainer, безопасная миграция БД):
> см. [docs/deploy.md](docs/deploy.md).

## 🗺 Планы

- [x] Общая БД Telegram/Discord, линковка аккаунтов, i18n
- [x] Стрики, Pity, крафт, дуэли, сеты/ауры, промокоды, Stars, предложка
- [x] **Спавны карт в чате** (лови карту)
- [x] **Веб-приложение**: HTTP API + SPA (Telegram Mini App, Discord Activity, браузер), инвентарь, админка
- [ ] Трейды между игроками
- [ ] Любимая карта в профиле
- [ ] World Boss · ежедневная угадайка карты · лотерея
- [ ] Перенос владельческих инструментов в веб-админку

## 📄 Лицензия

[MIT](LICENSE) — поднимай у себя и адаптируй.

## 👤 Автор

**Heather Arima (Shamil Baste)** — Go backend & game developer · [@BasteArima](https://github.com/BasteArima)
