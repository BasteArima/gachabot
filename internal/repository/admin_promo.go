package repository

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"gachabot/internal/models"
)

// AdminPromo is a promo code row for the admin list.
type AdminPromo struct {
	Code        string             `json:"code"`
	Reward      models.PromoReward `json:"reward"`
	MaxUses     *int               `json:"maxUses"`
	CurrentUses int                `json:"currentUses"`
	ExpiresAt   *time.Time         `json:"expiresAt"`
	CreatedAt   *time.Time         `json:"createdAt"`
}

// ListPromoCodes returns every promo code, newest first.
func (r *PostgresRepo) ListPromoCodes() ([]AdminPromo, error) {
	rows, err := r.db.Query(`
		SELECT code, reward_json, max_uses, COALESCE(current_uses, 0), expires_at, created_at
		FROM promocodes ORDER BY created_at DESC NULLS LAST, code ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	promos := make([]AdminPromo, 0)
	for rows.Next() {
		var (
			p         AdminPromo
			rewardRaw []byte
			maxUses   sql.NullInt64
			expires   sql.NullTime
			created   sql.NullTime
		)
		if err := rows.Scan(&p.Code, &rewardRaw, &maxUses, &p.CurrentUses, &expires, &created); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(rewardRaw, &p.Reward)
		if maxUses.Valid {
			v := int(maxUses.Int64)
			p.MaxUses = &v
		}
		if expires.Valid {
			p.ExpiresAt = &expires.Time
		}
		if created.Valid {
			p.CreatedAt = &created.Time
		}
		promos = append(promos, p)
	}
	return promos, rows.Err()
}

// DeletePromoCode removes a code. Redemption history in promocode_usages is
// cascaded away with it, so a re-created code with the same name is usable again
// by everyone — recreate under a new name if that matters.
func (r *PostgresRepo) DeletePromoCode(code string) error {
	code = strings.ToUpper(strings.TrimSpace(code))
	res, err := r.db.Exec(`DELETE FROM promocodes WHERE code = $1`, code)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("промокод %q не найден", code)
	}
	return nil
}

// PromoExists reports whether a code is already taken (create should not silently
// overwrite an active code from the web UI).
func (r *PostgresRepo) PromoExists(code string) (bool, error) {
	var exists bool
	err := r.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM promocodes WHERE code = $1)`,
		strings.ToUpper(strings.TrimSpace(code))).Scan(&exists)
	return exists, err
}
