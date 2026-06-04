-- Migration 0001: data-driven fragment behaviour for rarities.
--
-- Replaces the hardcoded "rarity 6 = mythic = fragments" and the magic number 10
-- in the Go code with configurable per-rarity columns.
--
-- Idempotent (ADD COLUMN IF NOT EXISTS + value-setting UPDATEs), so it is safe even
-- if the columns were already added manually. The migration runner wraps this file
-- in a single transaction — do NOT add BEGIN/COMMIT here.
--
-- Backfill assumes the production data this code shipped with:
--     id = 6  -> Mythic  (rolls drop as fragments, 10 to assemble)
--     id = 4  -> Epic    (used by the "buy 100 rolls" promo fragment bonus)
-- If your rarity IDs differ, adjust the UPDATE statements below.

ALTER TABLE rarities ADD COLUMN IF NOT EXISTS requires_fragments BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE rarities ADD COLUMN IF NOT EXISTS fragments_required INTEGER NOT NULL DEFAULT 0;

-- Mythic: a normal roll yields a fragment; 10 fragments assemble the card.
UPDATE rarities SET requires_fragments = TRUE, fragments_required = 10 WHERE id = 6;

-- Epic: not fragment-based on a normal roll, but the "buy 100 rolls" promo grants
-- Epic fragments, so it still needs an assembly threshold.
UPDATE rarities SET fragments_required = 10 WHERE id = 4;
