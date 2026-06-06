package repository

import (
	"database/sql"
	"encoding/json"
)

// GetSetting returns the raw JSON value stored under key, or (nil, nil) if absent.
func (r *PostgresRepo) GetSetting(key string) (json.RawMessage, error) {
	var v []byte
	err := r.db.QueryRow(`SELECT value FROM bot_settings WHERE key = $1`, key).Scan(&v)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return json.RawMessage(v), nil
}

// SetSetting upserts a JSON value under key.
func (r *PostgresRepo) SetSetting(key string, value json.RawMessage) error {
	_, err := r.db.Exec(`
		INSERT INTO bot_settings (key, value, updated_at)
		VALUES ($1, $2, NOW())
		ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = NOW()`,
		key, []byte(value))
	return err
}
