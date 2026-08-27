-- Resync serial id sequences with the data actually in the tables.
--
-- Rows restored from a dump (or inserted with explicit ids) do not advance the
-- backing sequence, so the next INSERT that relies on it collides with an
-- existing id and fails with "duplicate key value violates unique constraint".
-- That breaks adding new cards/sets — both from the admin panel and from
-- /theme_import — until the sequence is moved past max(id).
--
-- setval(..., max(id) + 1, false) makes the *next* generated value max(id) + 1,
-- and leaves an empty table starting at 1. Idempotent: re-running is a no-op
-- once the sequence is already ahead.
DO $$
DECLARE
    t   text;
    seq text;
BEGIN
    FOREACH t IN ARRAY ARRAY['cards', 'card_sets', 'rarities', 'users', 'spawns']
    LOOP
        seq := pg_get_serial_sequence(t, 'id');
        IF seq IS NOT NULL THEN
            EXECUTE format(
                'SELECT setval(%L, COALESCE((SELECT MAX(id) FROM %I), 0) + 1, false)',
                seq, t
            );
        END IF;
    END LOOP;
END $$;
