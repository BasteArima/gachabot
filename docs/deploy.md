# Деплой: образ в GHCR + стек в Portainer

Новая схема: GitHub Actions на каждый push в `main` собирает образ и пушит его в
**GHCR** (`ghcr.io/bastearima/gachabot`). Деплоишь созданием/обновлением
**стека в Portainer**, который тянет этот образ, и сам решаешь, когда репулить.

Старый `deploy.yml` (self-hosted раннер, `docker-compose up --build` на коммитах
`[build]`) удалён — его роль разделена: сборка → CI, деплой → ты, в Portainer.

---

## 1. Переменные окружения: где какая живёт

Их два типа. **Runtime**-переменные вводятся в **UI стека Portainer**.
**Build-time** (`VITE_*`) вшиваются во фронт при сборке образа в CI, поэтому
живут как **Secrets репозитория GitHub** и читаются `.github/workflows/docker.yml`.

### Runtime — задаются в Portainer (Stack → Environment variables)
| Переменная | Где взять |
|------------|-----------|
| `TELEGRAM_BOT_TOKEN` | @BotFather |
| `ADMIN_TELEGRAM_ID` | твой числовой TG id (@userinfobot) |
| `ADMIN_TG_SUGGESTS_GROUP_ID`, `ADMIN_TG_BACKUP_GROUP_ID` | id групп (положительная часть) |
| `ADD_BOT_TO_GROUP_LINK`, `START_BANNER_URL` | твои ссылки |
| `DISCORD_TOKEN` | Discord Dev Portal → приложение → Bot → Token |
| **`DISCORD_CLIENT_ID`** *(новая)* | Dev Portal → OAuth2 → Client ID |
| **`DISCORD_CLIENT_SECRET`** *(новая)* | Dev Portal → OAuth2 → Reset Secret |
| **`DISCORD_OAUTH_REDIRECT`** *(новая)* | `https://<домен>/` — также добавь в Dev Portal → OAuth2 → Redirects |
| **`WEB_APP_URL`** *(новая)* | `https://<домен>` (публичный HTTPS приложения) |
| `POSTGRES_USER` / `POSTGRES_PASSWORD` / `POSTGRES_DB` | креды БД (оставь те же, что сейчас!) |
| `REQUIRE_18_PLUS_CONFIRM`, `COOLDOWN_HOURS`, `ENABLE_DUPLICATES` | твои настройки |
| `BACKUP_TIME_HOUR`, `BACKUP_TIME_MINUTE` | расписание бэкапов |

Зашиты в compose (не задавать): `POSTGRES_HOST=db`, `POSTGRES_PORT=5432`,
`REDIS_ADDR=redis:6379`, `HTTP_PORT=8080`, `DEV_ALLOW_NO_AUTH=false`.

> Старые GitHub-секреты для **runtime**, что использовал `deploy.yml`, для деплоя
> больше не нужны — рантайм-конфиг теперь в Portainer. Можешь оставить или удалить.

### Build-time — задаются как Secrets репозитория (Settings → Secrets and variables → Actions)
| Secret | Значение | Примечание |
|--------|----------|-----------|
| **`VITE_TG_BOT_USERNAME`** | username бота без `@` | браузерный Telegram Login Widget; ещё сделай `/setdomain` в BotFather → твой домен |
| **`VITE_DISCORD_CLIENT_ID`** | то же, что `DISCORD_CLIENT_ID` | client id публичный, вшивать безопасно |
| **`SUBMODULE_PAT`** *(обязателен — `gacha-nova` приватный)* | classic PAT со scope `repo` (или fine-grained: Contents → Read на `gacha-nova`) | CI тянет только сабмодуль `web`; арт-репо `_content_helpers` в образ не идёт |

`VITE_API_BASE_URL` намеренно не задаём → SPA по умолчанию использует `/api`
(тот же домен), что и правильно, раз Go отдаёт и фронт, и API.

---

## 2. Видимость пакета GHCR

В образе **нет секретов** (только публичные client id / username бота), поэтому
проще всего сделать пакет **публичным**: GitHub → профиль → Packages →
`gachabot` → Package settings → Change visibility → Public. Тогда Portainer тянет
его без кредов.

(Если оставляешь приватным: в Portainer добавь Registry с username = твой GitHub
логин, password = PAT со scope `read:packages`, и выбери его на стеке.)

---

## 3. Первая миграция — БЕЗ потери базы

Данные лежат в Docker-томе `gachabot_postgres_data` (префикс `gachabot_` идёт от
имени текущего compose-проекта). Переход с локально собираемого стека на стек с
образом из GHCR **тома не трогает** — БД переживает пересоздание контейнеров и
смену образа. **Потерять данные можно только удалив том или подключив том с
другим именем.** Именно тут подвох нового стека в Portainer: Portainer называет
тома по имени стека, поэтому стек с другим именем создаст новый пустой
`…_postgres_data`.

Две страховки (прод-compose уже использует #2):
1. Назвать стек в Portainer `gachabot` (совпадёт с префиксом тома), **или**
2. Объявить том `external` с явным именем (сделано в `docker-compose.prod.yml`:
   `name: gachabot_postgres_data`).

### Шаги
1. **Сначала бэкап** (у тебя есть DataGrip + ежедневные дампы в `/opt/gachabot/backups`):
   ```bash
   docker exec -t gachabot_db pg_dump -U <POSTGRES_USER> <POSTGRES_DB> > gachabot_$(date +%F).sql
   ```
2. Проверь имя тома:
   ```bash
   docker volume ls | grep postgres   # ожидаем gachabot_postgres_data
   ```
   Если отличается — поправь `name:` в `docker-compose.prod.yml`.
3. Останови текущий локально-собранный стек (тома сохранятся):
   ```bash
   cd <репо-на-сервере> && docker compose down
   ```
4. Запушь коммит → CI соберёт и запушит `ghcr.io/bastearima/gachabot:latest`.
   Сделай пакет публичным (раздел 2).
5. Portainer → Stacks → **Add stack** → имя (любое, БД держит external-том) →
   **Web editor** → вставь `docker-compose.prod.yml` → заполни env → **Deploy**.
6. Глянь логи `gachabot_app`: должно быть `Database schema is up to date` (или
   применённые миграции) — **не** свежая инициализация. Балансы/карты на месте.

### Обновления потом
Portainer → стек → **Pull and redeploy** (перетянет `:latest`). Тома остаются,
БД переживает каждое обновление.

> Только для свежей/пустой БД: прод-compose убирает init-mount `schema.sql` (в
> web-редакторе Portainer нет файлов репо). Для совсем новой БД примени
> `schema.sql` один раз через DataGrip до первого старта; миграции (`0001`,
> `0002`, …) дальше применяются автоматически.
