-- Player card suggestions ("предложка").
--
-- Until now a suggestion only charged the player and was forwarded to the admin
-- chat, leaving no record: no author, no status, no way to review later. This
-- table is the queue the admin panel moderates.
--
-- Images: Telegram file URLs embed the bot token and expire, so only the
-- file_id is kept and the backend resolves/streams it on demand. Discord
-- attachments are stored as their CDN url.
CREATE TABLE IF NOT EXISTS suggestions (
    id          BIGSERIAL PRIMARY KEY,
    user_id     BIGINT REFERENCES users(id) ON DELETE SET NULL,
    platform    VARCHAR(10) NOT NULL,
    caption     TEXT        NOT NULL DEFAULT '',
    file_id     TEXT,                 -- Telegram file id
    image_url   TEXT,                 -- Discord attachment url
    status      VARCHAR(20) NOT NULL DEFAULT 'pending', -- pending|approved|rejected
    review_note TEXT,
    card_id     INTEGER REFERENCES cards(id) ON DELETE SET NULL, -- set when approved
    refunded    BOOLEAN     NOT NULL DEFAULT FALSE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    reviewed_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_suggestions_status ON suggestions(status, created_at DESC);
