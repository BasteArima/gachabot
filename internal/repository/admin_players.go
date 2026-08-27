package repository

import (
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// AdminPlayer is a player row for the admin search list and detail screen.
type AdminPlayer struct {
	ID           int64      `json:"id"`
	Username     string     `json:"username"`
	FirstName    string     `json:"firstName"`
	LastName     string     `json:"lastName"`
	AvatarURL    string     `json:"avatarUrl"`
	TelegramID   *int64     `json:"telegramId"`
	DiscordID    *int64     `json:"discordId,string"`
	Balance      int        `json:"balance"`
	StreakDays   int        `json:"streakDays"`
	PremiumRolls int        `json:"premiumRolls"`
	UniqueCards  int        `json:"uniqueCards"`
	TotalCopies  int        `json:"totalCopies"`
	Duplicates   int        `json:"duplicates"`
	SetsDone     int        `json:"setsDone"`
	LastRollTime *time.Time `json:"lastRollTime"`
	CreatedAt    *time.Time `json:"createdAt"`
}

const adminPlayerSelect = `
	SELECT u.id, COALESCE(u.username, ''), COALESCE(u.first_name, ''), COALESCE(u.last_name, ''),
	       COALESCE(u.avatar_url, ''), u.telegram_id, u.discord_id,
	       u.balance, u.streak_days, u.premium_rolls,
	       (SELECT COUNT(*) FROM user_inventory i WHERE i.user_id = u.id),
	       (SELECT COALESCE(SUM(i.quantity), 0) FROM user_inventory i WHERE i.user_id = u.id),
	       (SELECT COALESCE(SUM(i.quantity - 1), 0) FROM user_inventory i WHERE i.user_id = u.id AND i.quantity > 1),
	       (SELECT COUNT(*) FROM user_unlocked_sets s WHERE s.user_id = u.id AND s.is_completed),
	       u.last_roll_time, u.created_at
	FROM users u`

func scanAdminPlayer(row interface{ Scan(...any) error }) (AdminPlayer, error) {
	var p AdminPlayer
	var tg, ds sql.NullInt64
	var lastRoll, created sql.NullTime
	err := row.Scan(&p.ID, &p.Username, &p.FirstName, &p.LastName, &p.AvatarURL, &tg, &ds,
		&p.Balance, &p.StreakDays, &p.PremiumRolls,
		&p.UniqueCards, &p.TotalCopies, &p.Duplicates, &p.SetsDone, &lastRoll, &created)
	if err != nil {
		return p, err
	}
	if tg.Valid {
		p.TelegramID = &tg.Int64
	}
	if ds.Valid {
		p.DiscordID = &ds.Int64
	}
	if lastRoll.Valid {
		p.LastRollTime = &lastRoll.Time
	}
	if created.Valid {
		p.CreatedAt = &created.Time
	}
	return p, nil
}

// SearchPlayers finds players by name fragment, or by an exact id when the query
// is numeric (internal id, Telegram id or Discord id — whichever matches).
func (r *PostgresRepo) SearchPlayers(query string, limit int) ([]AdminPlayer, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	query = strings.TrimSpace(query)
	numeric, _ := strconv.ParseInt(query, 10, 64)

	rows, err := r.db.Query(adminPlayerSelect+`
		WHERE ($1 = '' OR u.username ILIKE '%' || $1 || '%' OR u.first_name ILIKE '%' || $1 || '%'
		       OR ($2 <> 0 AND (u.id = $2 OR u.telegram_id = $2 OR u.discord_id = $2)))
		ORDER BY u.balance DESC, u.id ASC
		LIMIT $3`, query, numeric, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	players := make([]AdminPlayer, 0)
	for rows.Next() {
		p, err := scanAdminPlayer(rows)
		if err != nil {
			return nil, err
		}
		players = append(players, p)
	}
	return players, rows.Err()
}

// GetAdminPlayer loads one player's full admin view.
func (r *PostgresRepo) GetAdminPlayer(id int64) (*AdminPlayer, error) {
	p, err := scanAdminPlayer(r.db.QueryRow(adminPlayerSelect+` WHERE u.id = $1`, id))
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// AdjustBalance adds delta (which may be negative) to a player's balance and
// returns the new value. The balance is floored at 0 rather than failing the
// schema's non-negative check, so "take everything" is expressible.
func (r *PostgresRepo) AdjustBalance(id int64, delta int) (int, error) {
	var balance int
	err := r.db.QueryRow(`
		UPDATE users SET balance = GREATEST(0, balance + $2) WHERE id = $1 RETURNING balance`,
		id, delta).Scan(&balance)
	return balance, err
}

// AdjustPremiumRolls adds delta (may be negative) to premium rolls, floored at 0.
func (r *PostgresRepo) AdjustPremiumRolls(id int64, delta int) (int, error) {
	var rolls int
	err := r.db.QueryRow(`
		UPDATE users SET premium_rolls = GREATEST(0, premium_rolls + $2) WHERE id = $1 RETURNING premium_rolls`,
		id, delta).Scan(&rolls)
	return rolls, err
}

// ResetRollCooldown clears the roll timestamp so the player can roll immediately.
func (r *PostgresRepo) ResetRollCooldown(id int64) error {
	res, err := r.db.Exec(`UPDATE users SET last_roll_time = NULL WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// SetStreak overwrites the streak counter (support fixes, not a reward path).
func (r *PostgresRepo) SetStreak(id int64, days int) error {
	if days < 0 {
		return fmt.Errorf("стрик не может быть отрицательным")
	}
	res, err := r.db.Exec(`UPDATE users SET streak_days = $2 WHERE id = $1`, id, days)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// RemoveCardCopy takes one copy of a card away, deleting the row at zero.
func (r *PostgresRepo) RemoveCardCopy(userID int64, cardID int) error {
	var qty int
	err := r.db.QueryRow(`SELECT quantity FROM user_inventory WHERE user_id = $1 AND card_id = $2`,
		userID, cardID).Scan(&qty)
	if err == sql.ErrNoRows {
		return fmt.Errorf("у игрока нет этой карты")
	}
	if err != nil {
		return err
	}
	if qty <= 1 {
		_, err = r.db.Exec(`DELETE FROM user_inventory WHERE user_id = $1 AND card_id = $2`, userID, cardID)
		return err
	}
	_, err = r.db.Exec(`UPDATE user_inventory SET quantity = quantity - 1 WHERE user_id = $1 AND card_id = $2`,
		userID, cardID)
	return err
}
