-- Chat Spawns, slice 1: chat registry + spawn audit + generic settings store.
-- See docs/design/Chat Spawns.md

-- Registry of chats/channels the bot posts to (spawns, broadcasts, boss announcements).
-- This is "where to write", not "where activity happened" (unlike user_chats).
CREATE TABLE IF NOT EXISTS chats (
    platform       VARCHAR(10) NOT NULL,           -- 'telegram' | 'discord'
    chat_id        BIGINT      NOT NULL,           -- TG: group chat id; DS: main channel id we post into
    guild_id       BIGINT,                         -- DS: guild id (NULL for TG) — one main channel per guild
    title          TEXT,
    spawn_enabled  BOOLEAN     NOT NULL DEFAULT TRUE,
    added_at       TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    PRIMARY KEY (platform, chat_id)
);
CREATE INDEX IF NOT EXISTS idx_chats_guild ON chats(guild_id) WHERE platform = 'discord';

-- Spawn events: audit + active-spawn lookup. The hot claim path lives in Redis,
-- this table keeps history and the authoritative "claimed_by".
CREATE TABLE IF NOT EXISTS spawns (
    id          BIGSERIAL PRIMARY KEY,
    wave_id     BIGINT      NOT NULL,              -- groups spawns of one wave (same card, many chats)
    platform    VARCHAR(10) NOT NULL,
    chat_id     BIGINT      NOT NULL,
    card_id     INTEGER     NOT NULL REFERENCES cards(id) ON DELETE CASCADE,
    message_id  BIGINT,                            -- for editing/expiring the message
    spawned_at  TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    expires_at  TIMESTAMP WITH TIME ZONE NOT NULL,
    claimed_by  BIGINT      REFERENCES users(id) ON DELETE SET NULL,
    claimed_at  TIMESTAMP WITH TIME ZONE
);
CREATE INDEX IF NOT EXISTS idx_spawns_wave ON spawns(wave_id);
CREATE INDEX IF NOT EXISTS idx_spawns_chat_active ON spawns(platform, chat_id) WHERE claimed_by IS NULL;

-- Generic JSONB settings store (spawn_config now; lottery/boss/trade configs later).
CREATE TABLE IF NOT EXISTS bot_settings (
    key        VARCHAR(64) PRIMARY KEY,
    value      JSONB       NOT NULL,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);
