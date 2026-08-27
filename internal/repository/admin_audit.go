package repository

import (
	"database/sql"
	"time"
)

// AuditEntry is one recorded admin action.
type AuditEntry struct {
	ID        int64      `json:"id"`
	ActorID   *int64     `json:"actorId"`
	ActorName string     `json:"actorName"`
	Method    string     `json:"method"`
	Path      string     `json:"path"`
	Payload   string     `json:"payload"`
	Status    int        `json:"status"`
	CreatedAt *time.Time `json:"createdAt"`
}

// InsertAudit records an admin action. Audit writes must never break the action
// itself, so callers log and ignore the error.
func (r *PostgresRepo) InsertAudit(actorID int64, method, path, payload string, status int) error {
	var actor any
	if actorID > 0 {
		actor = actorID
	}
	_, err := r.db.Exec(`
		INSERT INTO admin_audit (actor_id, method, path, payload, status)
		VALUES ($1, $2, $3, $4, $5)`,
		actor, method, path, payload, status)
	return err
}

// ListAudit returns the most recent admin actions, newest first.
func (r *PostgresRepo) ListAudit(limit int) ([]AuditEntry, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := r.db.Query(`
		SELECT a.id, a.actor_id, COALESCE(u.first_name, u.username, ''), a.method, a.path,
		       COALESCE(a.payload, ''), a.status, a.created_at
		FROM admin_audit a
		LEFT JOIN users u ON u.id = a.actor_id
		ORDER BY a.created_at DESC, a.id DESC
		LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	entries := make([]AuditEntry, 0)
	for rows.Next() {
		var (
			e       AuditEntry
			actor   sql.NullInt64
			created sql.NullTime
		)
		if err := rows.Scan(&e.ID, &actor, &e.ActorName, &e.Method, &e.Path, &e.Payload, &e.Status, &created); err != nil {
			return nil, err
		}
		if actor.Valid {
			e.ActorID = &actor.Int64
		}
		if created.Valid {
			e.CreatedAt = &created.Time
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}
