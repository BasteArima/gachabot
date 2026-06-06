package repository

import (
	"database/sql"
	"time"

	"gachabot/internal/models"

	"github.com/lib/pq"
)

// --- Spawn audit rows ---

func (r *PostgresRepo) InsertSpawn(waveID int64, platform string, chatID int64, cardID int, expiresAt time.Time) (int64, error) {
	var id int64
	err := r.db.QueryRow(`
		INSERT INTO spawns (wave_id, platform, chat_id, card_id, expires_at)
		VALUES ($1, $2, $3, $4, $5) RETURNING id`,
		waveID, platform, chatID, cardID, expiresAt).Scan(&id)
	return id, err
}

func (r *PostgresRepo) SetSpawnMessageID(spawnID, messageID int64) error {
	_, err := r.db.Exec(`UPDATE spawns SET message_id = $1 WHERE id = $2`, messageID, spawnID)
	return err
}

// ClaimSpawnRow marks the spawn claimed only if still unclaimed; reports whether
// this call won the row (defence in depth alongside the Redis atomic claim).
func (r *PostgresRepo) ClaimSpawnRow(spawnID, userID int64) (bool, error) {
	res, err := r.db.Exec(
		`UPDATE spawns SET claimed_by = $1, claimed_at = NOW() WHERE id = $2 AND claimed_by IS NULL`,
		userID, spawnID)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// --- Cards for spawn pool ---

func (r *PostgresRepo) GetCardByID(id int) (*models.Card, error) {
	var c models.Card
	err := r.db.QueryRow(
		`SELECT id, name, rarity_id, image_url, power_level, set_id FROM cards WHERE id = $1`, id).
		Scan(&c.ID, &c.Name, &c.RarityID, &c.ImageURL, &c.PowerLevel, &c.SetID)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// GetRandomCardByRarityExcluding picks a random card of a rarity, skipping
// excluded ids. Returns (nil, nil) if nothing matches.
func (r *PostgresRepo) GetRandomCardByRarityExcluding(rarityID int, exclude []int64) (*models.Card, error) {
	var c models.Card
	err := r.db.QueryRow(`
		SELECT id, name, rarity_id, image_url, power_level, set_id
		FROM cards
		WHERE rarity_id = $1 AND (cardinality($2::bigint[]) = 0 OR id <> ALL($2))
		ORDER BY RANDOM() LIMIT 1`,
		rarityID, pq.Array(exclude)).
		Scan(&c.ID, &c.Name, &c.RarityID, &c.ImageURL, &c.PowerLevel, &c.SetID)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// GetRandomCardFromList picks a random card from an explicit id list, skipping
// excluded ids. Returns (nil, nil) if nothing matches.
func (r *PostgresRepo) GetRandomCardFromList(ids, exclude []int64) (*models.Card, error) {
	var c models.Card
	err := r.db.QueryRow(`
		SELECT id, name, rarity_id, image_url, power_level, set_id
		FROM cards
		WHERE id = ANY($1) AND (cardinality($2::bigint[]) = 0 OR id <> ALL($2))
		ORDER BY RANDOM() LIMIT 1`,
		pq.Array(ids), pq.Array(exclude)).
		Scan(&c.ID, &c.Name, &c.RarityID, &c.ImageURL, &c.PowerLevel, &c.SetID)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (r *PostgresRepo) UserOwnsCard(userID int64, cardID int) (bool, error) {
	var exists bool
	err := r.db.QueryRow(
		`SELECT EXISTS(SELECT 1 FROM user_inventory WHERE user_id = $1 AND card_id = $2)`,
		userID, cardID).Scan(&exists)
	return exists, err
}

// --- User balance / streak helpers (narrow updates for non-roll grants) ---

// AddBalance atomically adds delta to a user's balance.
func (r *PostgresRepo) AddBalance(internalUserID int64, delta int) error {
	_, err := r.db.Exec(`UPDATE users SET balance = balance + $1 WHERE id = $2`, delta, internalUserID)
	return err
}

// UpdateUserStreak persists only the streak fields (used by claim/boss/lottery,
// which must not touch last_roll_time or premium_rolls).
func (r *PostgresRepo) UpdateUserStreak(internalUserID int64, streakDays int, lastStreakDate time.Time) error {
	_, err := r.db.Exec(
		`UPDATE users SET streak_days = $1, last_streak_date = $2 WHERE id = $3`,
		streakDays, lastStreakDate, internalUserID)
	return err
}
