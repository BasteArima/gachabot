-- Seasons: a running scoreboard that starts over, next to the all-time tops.
--
-- The existing tops (balance, streak, unique cards) accumulate forever, so they
-- cannot be reset without wiping what players own. A season instead counts what
-- happened *during* it, in three categories that survive a reset:
--
--   coins_earned — payouts only. Spending is not a penalty, and transfers
--                  (duels, promos, admin grants) are not earnings.
--   card_points  — every card obtained, weighted by rarity. Not "new uniques":
--                  someone holding 300 of 314 cards can barely gain any, and
--                  would lose to a newcomer for whom every roll is new.
--   active_days  — days with at least one action, rather than the current
--                  streak, which one missed day erases along with the month
--                  behind it.
--
-- Only one season runs at a time; the partial unique index below makes that a
-- database guarantee rather than a convention. Seasons are finished by hand
-- (ends_at is a target shown as a countdown, not a deadline that fires), so a
-- season can be extended, or its counters wiped and restarted, without racing
-- a scheduler.
CREATE TABLE IF NOT EXISTS seasons (
    id          SERIAL PRIMARY KEY,
    number      INTEGER     NOT NULL UNIQUE,
    title       TEXT        NOT NULL DEFAULT '',
    started_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    ends_at     TIMESTAMPTZ,
    finished_at TIMESTAMPTZ,
    -- Rarity weights, medal boundaries and the activity minimum, so scoring
    -- rules stay pinned to the season they were played under.
    config      JSONB       NOT NULL DEFAULT '{}'::jsonb
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_seasons_single_active
    ON seasons ((finished_at IS NULL)) WHERE finished_at IS NULL;

CREATE TABLE IF NOT EXISTS season_stats (
    season_id         INTEGER     NOT NULL REFERENCES seasons(id) ON DELETE CASCADE,
    user_id           BIGINT      NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    coins_earned      BIGINT      NOT NULL DEFAULT 0,
    card_points       BIGINT      NOT NULL DEFAULT 0,
    cards_count       INTEGER     NOT NULL DEFAULT 0,
    active_days       INTEGER     NOT NULL DEFAULT 0,
    -- Kept so a second action on the same day does not count twice.
    last_active_day   DATE,
    best_streak       INTEGER     NOT NULL DEFAULT 0,
    -- The season's most valuable drop, for the medal card and the wrap-up.
    best_drop_card_id INTEGER     REFERENCES cards(id) ON DELETE SET NULL,
    best_drop_points  INTEGER     NOT NULL DEFAULT 0,
    best_drop_at      TIMESTAMPTZ,
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (season_id, user_id)
);

CREATE INDEX IF NOT EXISTS idx_season_stats_user ON season_stats(user_id);

-- Written once, when a season is finished and medals are handed out. Separate
-- from season_stats because the counters keep no history: this is the row a
-- trophy shelf reads years later, so it holds the final numbers rather than
-- pointing at data that a later recount could change.
CREATE TABLE IF NOT EXISTS season_results (
    season_id  INTEGER     NOT NULL REFERENCES seasons(id) ON DELETE CASCADE,
    user_id    BIGINT      NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    place      INTEGER     NOT NULL,
    points     INTEGER     NOT NULL,
    -- champion | silver | bronze | elite | participant
    tier       VARCHAR(20) NOT NULL,
    -- Per-category places and values, plus the best drop, as shown on the medal.
    detail     JSONB       NOT NULL DEFAULT '{}'::jsonb,
    awarded_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (season_id, user_id)
);

CREATE INDEX IF NOT EXISTS idx_season_results_user ON season_results(user_id, season_id DESC);
