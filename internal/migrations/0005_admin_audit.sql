-- Audit trail for admin actions performed through the web panel.
--
-- Until now the only record was the server log, which rotates away and can't be
-- read from the panel. Anything that grants coins/cards, edits content or posts
-- to chats should stay attributable, so every mutating admin request is recorded
-- here (see auditMiddleware in the httpapi layer).
CREATE TABLE IF NOT EXISTS admin_audit (
    id         BIGSERIAL PRIMARY KEY,
    actor_id   BIGINT REFERENCES users(id) ON DELETE SET NULL,
    method     VARCHAR(10) NOT NULL,
    path       TEXT        NOT NULL,
    payload    TEXT,                 -- truncated request body, for context
    status     INTEGER     NOT NULL, -- HTTP status the action returned
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_admin_audit_created ON admin_audit(created_at DESC);
