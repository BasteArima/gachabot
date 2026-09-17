package repository

import (
	"database/sql"
	"encoding/json"
	"time"

	"gachabot/internal/models"
)

// Season bookkeeping. See internal/migrations/0007_seasons.sql for why the
// counters live apart from the all-time tops.

const seasonColumns = `id, number, title, started_at, ends_at, finished_at, config`

func scanSeason(row interface{ Scan(...any) error }) (*models.Season, error) {
	var s models.Season
	var cfg []byte
	if err := row.Scan(&s.ID, &s.Number, &s.Title, &s.StartedAt, &s.EndsAt, &s.FinishedAt, &cfg); err != nil {
		return nil, err
	}
	s.Config = json.RawMessage(cfg)
	return &s, nil
}

// ActiveSeason returns the season currently running, or (nil, nil) when none is.
func (r *PostgresRepo) ActiveSeason() (*models.Season, error) {
	s, err := scanSeason(r.db.QueryRow(`SELECT ` + seasonColumns + ` FROM seasons WHERE finished_at IS NULL`))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return s, err
}

func (r *PostgresRepo) SeasonByID(id int) (*models.Season, error) {
	s, err := scanSeason(r.db.QueryRow(`SELECT `+seasonColumns+` FROM seasons WHERE id = $1`, id))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return s, err
}

// ListSeasons returns every season, newest first.
func (r *PostgresRepo) ListSeasons() ([]models.Season, error) {
	rows, err := r.db.Query(`SELECT ` + seasonColumns + ` FROM seasons ORDER BY number DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []models.Season
	for rows.Next() {
		s, err := scanSeason(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *s)
	}
	return out, rows.Err()
}

// CreateSeason opens a new season numbered after the last one. The partial
// unique index refuses a second unfinished season, so a double click cannot
// start two.
func (r *PostgresRepo) CreateSeason(title string, endsAt *time.Time, cfg json.RawMessage) (*models.Season, error) {
	return scanSeason(r.db.QueryRow(`
		INSERT INTO seasons (number, title, ends_at, config)
		VALUES ((SELECT COALESCE(MAX(number), 0) + 1 FROM seasons), $1, $2, $3)
		RETURNING `+seasonColumns, title, endsAt, []byte(cfg)))
}

// UpdateSeason changes the parts the owner may edit while a season runs: its
// name, its target end date, and the scoring rules.
func (r *PostgresRepo) UpdateSeason(id int, title string, endsAt *time.Time, cfg json.RawMessage) error {
	res, err := r.db.Exec(`
		UPDATE seasons SET title = $2, ends_at = $3, config = $4
		WHERE id = $1 AND finished_at IS NULL`, id, title, endsAt, []byte(cfg))
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// RestartSeason throws away everything counted so far and starts the clock
// again. This is the "that was just the warm-up" button: the run-up before the
// season system was finished should not decide medals.
func (r *PostgresRepo) RestartSeason(id int) error {
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.Exec(`DELETE FROM season_stats WHERE season_id = $1`, id); err != nil {
		return err
	}
	res, err := tx.Exec(`UPDATE seasons SET started_at = NOW() WHERE id = $1 AND finished_at IS NULL`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}
	return tx.Commit()
}

// AddSeasonCoins credits coins the game paid out. Callers pass positive amounts
// only: spending is not a penalty in the season standings.
func (r *PostgresRepo) AddSeasonCoins(seasonID int, userID int64, amount int) error {
	_, err := r.db.Exec(`
		INSERT INTO season_stats (season_id, user_id, coins_earned)
		VALUES ($1, $2, $3)
		ON CONFLICT (season_id, user_id) DO UPDATE
		SET coins_earned = season_stats.coins_earned + EXCLUDED.coins_earned,
		    updated_at = NOW()`, seasonID, userID, amount)
	return err
}

// AddSeasonCard records one obtained card worth points, and remembers it as the
// season's best drop when nothing better has landed yet.
func (r *PostgresRepo) AddSeasonCard(seasonID int, userID int64, cardID, points int) error {
	_, err := r.db.Exec(`
		INSERT INTO season_stats (season_id, user_id, card_points, cards_count,
		                          best_drop_card_id, best_drop_points, best_drop_at)
		VALUES ($1, $2, $3, 1, $4, $3, NOW())
		ON CONFLICT (season_id, user_id) DO UPDATE
		SET card_points = season_stats.card_points + EXCLUDED.card_points,
		    cards_count = season_stats.cards_count + 1,
		    best_drop_card_id = CASE WHEN EXCLUDED.best_drop_points > season_stats.best_drop_points
		                             THEN EXCLUDED.best_drop_card_id ELSE season_stats.best_drop_card_id END,
		    best_drop_points  = GREATEST(season_stats.best_drop_points, EXCLUDED.best_drop_points),
		    best_drop_at      = CASE WHEN EXCLUDED.best_drop_points > season_stats.best_drop_points
		                             THEN NOW() ELSE season_stats.best_drop_at END,
		    updated_at = NOW()`, seasonID, userID, points, cardID)
	return err
}

// MarkSeasonActivity counts today once, however many times a player acts, and
// keeps the best streak they reached during the season.
func (r *PostgresRepo) MarkSeasonActivity(seasonID int, userID int64, day time.Time, streak int) error {
	_, err := r.db.Exec(`
		INSERT INTO season_stats (season_id, user_id, active_days, last_active_day, best_streak)
		VALUES ($1, $2, 1, $3, $4)
		ON CONFLICT (season_id, user_id) DO UPDATE
		SET active_days = season_stats.active_days
		        + CASE WHEN season_stats.last_active_day IS DISTINCT FROM EXCLUDED.last_active_day THEN 1 ELSE 0 END,
		    last_active_day = GREATEST(COALESCE(season_stats.last_active_day, EXCLUDED.last_active_day), EXCLUDED.last_active_day),
		    best_streak = GREATEST(season_stats.best_streak, EXCLUDED.best_streak),
		    updated_at = NOW()`, seasonID, userID, day, streak)
	return err
}

// SeasonStandings returns every player who scored anything this season, with the
// names the panel and the chat boards show.
func (r *PostgresRepo) SeasonStandings(seasonID int) ([]models.SeasonStat, error) {
	rows, err := r.db.Query(`
		SELECT st.user_id,
		       COALESCE(NULLIF(u.first_name, ''), NULLIF(u.username, ''), 'id' || st.user_id::text),
		       st.coins_earned, st.card_points, st.cards_count, st.active_days,
		       st.best_streak, st.best_drop_card_id, COALESCE(c.name, ''), st.best_drop_at
		FROM season_stats st
		JOIN users u ON u.id = st.user_id
		LEFT JOIN cards c ON c.id = st.best_drop_card_id
		WHERE st.season_id = $1
		ORDER BY st.card_points DESC, st.coins_earned DESC`, seasonID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []models.SeasonStat
	for rows.Next() {
		var s models.SeasonStat
		if err := rows.Scan(&s.UserID, &s.Name, &s.CoinsEarned, &s.CardPoints, &s.CardsCount,
			&s.ActiveDays, &s.BestStreak, &s.BestDropCardID, &s.BestDropName, &s.BestDropAt); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}
