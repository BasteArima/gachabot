package repository

import (
	"database/sql"
	"encoding/json"

	"gachabot/internal/models"

	"github.com/lib/pq"
)

// Finished seasons: the medals, and the numbers as they stood when the medals
// were awarded.

// FinishSeason writes the results and closes the season in one transaction.
// Half of this would be worse than none: results without a closed season would
// be handed out again on the next click, and a closed season without results
// would leave everyone's trophy shelf empty with no way to rebuild it.
func (r *PostgresRepo) FinishSeason(seasonID int, results []models.SeasonResult) error {
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	// Refuse to award a season twice: the guard is the WHERE, not a prior read,
	// so two clicks racing each other cannot both win.
	res, err := tx.Exec(`UPDATE seasons SET finished_at = NOW() WHERE id = $1 AND finished_at IS NULL`, seasonID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}

	stmt, err := tx.Prepare(`
		INSERT INTO season_results (season_id, user_id, place, points, tier, detail)
		VALUES ($1, $2, $3, $4, $5, $6)`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, x := range results {
		if _, err := stmt.Exec(seasonID, x.UserID, x.Place, x.Points, x.Tier, []byte(x.Detail)); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// TrophiesOf returns a player's medals, newest season first.
func (r *PostgresRepo) TrophiesOf(userID int64) ([]models.SeasonResult, error) {
	rows, err := r.db.Query(`
		SELECT sr.season_id, s.number, s.title, s.started_at, s.finished_at,
		       sr.user_id, sr.place, sr.points, sr.tier, sr.detail
		FROM season_results sr
		JOIN seasons s ON s.id = sr.season_id
		WHERE sr.user_id = $1
		ORDER BY s.number DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []models.SeasonResult
	for rows.Next() {
		var x models.SeasonResult
		var detail []byte
		if err := rows.Scan(&x.SeasonID, &x.SeasonNumber, &x.SeasonTitle, &x.StartedAt, &x.FinishedAt,
			&x.UserID, &x.Place, &x.Points, &x.Tier, &detail); err != nil {
			return nil, err
		}
		x.Detail = json.RawMessage(detail)
		out = append(out, x)
	}
	return out, rows.Err()
}

// SeasonBadgeCounts returns, for each of the given players, how many medals of
// each tier they hold. Which one goes next to the nickname is a decision for the
// service layer, which knows the order of the tiers.
func (r *PostgresRepo) SeasonBadgeCounts(userIDs []int64) (map[int64]map[string]int, error) {
	out := map[int64]map[string]int{}
	if len(userIDs) == 0 {
		return out, nil
	}
	rows, err := r.db.Query(`
		SELECT user_id, tier, COUNT(*)
		FROM season_results
		WHERE user_id = ANY($1)
		GROUP BY user_id, tier`, pq.Array(userIDs))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var uid int64
		var tier string
		var n int
		if err := rows.Scan(&uid, &tier, &n); err != nil {
			return nil, err
		}
		if out[uid] == nil {
			out[uid] = map[string]int{}
		}
		out[uid][tier] = n
	}
	return out, rows.Err()
}

// ReigningChampion is whoever won the most recently finished season — the one
// who wears the crown until someone takes it. Returns 0 when no season has
// finished yet.
func (r *PostgresRepo) ReigningChampion() (int64, error) {
	var uid int64
	err := r.db.QueryRow(`
		SELECT sr.user_id
		FROM season_results sr
		JOIN seasons s ON s.id = sr.season_id
		WHERE sr.tier = $1 AND s.finished_at IS NOT NULL
		ORDER BY s.number DESC
		LIMIT 1`, "champion").Scan(&uid)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return uid, err
}

// SeasonsWithResults lists the finished seasons a shelf needs to show gaps:
// a season a player sat out is still part of their history.
func (r *PostgresRepo) FinishedSeasons() ([]models.Season, error) {
	rows, err := r.db.Query(`SELECT ` + seasonColumns + ` FROM seasons WHERE finished_at IS NOT NULL ORDER BY number DESC`)
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
