package repository

// Dashboard aggregates for the admin overview screen. Everything here is derived
// from current state: the bot keeps no per-action event log, so "rolls per day"
// is approximated by last_roll_time activity rather than counted exactly.
type Dashboard struct {
	Players   PlayerStats    `json:"players"`
	Economy   EconomyStats   `json:"economy"`
	Content   ContentStats   `json:"content"`
	Spawns    SpawnStats     `json:"spawns"`
	Rarities  []RarityStat   `json:"rarities"`
	Signups   []DayCount     `json:"signups"`
	TopOwners []TopOwnerStat `json:"topOwners"`
}

type PlayerStats struct {
	Total      int `json:"total"`
	NewToday   int `json:"newToday"`
	New7d      int `json:"new7d"`
	Active24h  int `json:"active24h"`
	Active7d   int `json:"active7d"`
	WithStreak int `json:"withStreak"`
	LinkedBoth int `json:"linkedBoth"`
}

type EconomyStats struct {
	TotalCoins   int64 `json:"totalCoins"`
	MedianCoins  int   `json:"medianCoins"`
	PremiumRolls int   `json:"premiumRolls"`
}

type ContentStats struct {
	Cards      int   `json:"cards"`
	Sets       int   `json:"sets"`
	CopiesHeld int64 `json:"copiesHeld"`
	Duplicates int64 `json:"duplicates"`
	NeverOwned int   `json:"neverOwned"`
}

type SpawnStats struct {
	Total24h   int `json:"total24h"`
	Claimed24h int `json:"claimed24h"`
	Total7d    int `json:"total7d"`
	Claimed7d  int `json:"claimed7d"`
}

type RarityStat struct {
	Name  string `json:"name"`
	Cards int    `json:"cards"`
	Owned int64  `json:"owned"`
}

type DayCount struct {
	Day   string `json:"day"`
	Count int    `json:"count"`
}

type TopOwnerStat struct {
	Name  string `json:"name"`
	Cards int    `json:"cards"`
}

// GetDashboard collects every dashboard figure.
func (r *PostgresRepo) GetDashboard() (Dashboard, error) {
	var d Dashboard

	err := r.db.QueryRow(`
		SELECT
			(SELECT COUNT(*) FROM users),
			(SELECT COUNT(*) FROM users WHERE created_at >= NOW() - INTERVAL '24 hours'),
			(SELECT COUNT(*) FROM users WHERE created_at >= NOW() - INTERVAL '7 days'),
			(SELECT COUNT(*) FROM users WHERE last_roll_time >= NOW() - INTERVAL '24 hours'),
			(SELECT COUNT(*) FROM users WHERE last_roll_time >= NOW() - INTERVAL '7 days'),
			(SELECT COUNT(*) FROM users WHERE streak_days > 0),
			(SELECT COUNT(*) FROM users WHERE telegram_id IS NOT NULL AND discord_id IS NOT NULL)`).
		Scan(&d.Players.Total, &d.Players.NewToday, &d.Players.New7d,
			&d.Players.Active24h, &d.Players.Active7d, &d.Players.WithStreak, &d.Players.LinkedBoth)
	if err != nil {
		return d, err
	}

	err = r.db.QueryRow(`
		SELECT
			COALESCE(SUM(balance), 0),
			COALESCE(PERCENTILE_CONT(0.5) WITHIN GROUP (ORDER BY balance), 0)::int,
			COALESCE(SUM(premium_rolls), 0)
		FROM users`).
		Scan(&d.Economy.TotalCoins, &d.Economy.MedianCoins, &d.Economy.PremiumRolls)
	if err != nil {
		return d, err
	}

	err = r.db.QueryRow(`
		SELECT
			(SELECT COUNT(*) FROM cards),
			(SELECT COUNT(*) FROM card_sets),
			(SELECT COALESCE(SUM(quantity), 0) FROM user_inventory),
			(SELECT COALESCE(SUM(quantity - 1), 0) FROM user_inventory WHERE quantity > 1),
			(SELECT COUNT(*) FROM cards c WHERE NOT EXISTS
				(SELECT 1 FROM user_inventory i WHERE i.card_id = c.id))`).
		Scan(&d.Content.Cards, &d.Content.Sets, &d.Content.CopiesHeld, &d.Content.Duplicates, &d.Content.NeverOwned)
	if err != nil {
		return d, err
	}

	// Spawn history lives in the spawns table (added with chat spawns).
	err = r.db.QueryRow(`
		SELECT
			COUNT(*) FILTER (WHERE spawned_at >= NOW() - INTERVAL '24 hours'),
			COUNT(*) FILTER (WHERE spawned_at >= NOW() - INTERVAL '24 hours' AND claimed_by IS NOT NULL),
			COUNT(*) FILTER (WHERE spawned_at >= NOW() - INTERVAL '7 days'),
			COUNT(*) FILTER (WHERE spawned_at >= NOW() - INTERVAL '7 days' AND claimed_by IS NOT NULL)
		FROM spawns`).
		Scan(&d.Spawns.Total24h, &d.Spawns.Claimed24h, &d.Spawns.Total7d, &d.Spawns.Claimed7d)
	if err != nil {
		return d, err
	}

	if d.Rarities, err = r.rarityStats(); err != nil {
		return d, err
	}
	if d.Signups, err = r.signupSeries(); err != nil {
		return d, err
	}
	if d.TopOwners, err = r.topOwners(); err != nil {
		return d, err
	}
	return d, nil
}

func (r *PostgresRepo) rarityStats() ([]RarityStat, error) {
	rows, err := r.db.Query(`
		SELECT rr.name, COUNT(DISTINCT c.id),
		       COALESCE(SUM(i.quantity), 0)
		FROM rarities rr
		LEFT JOIN cards c ON c.rarity_id = rr.id
		LEFT JOIN user_inventory i ON i.card_id = c.id
		GROUP BY rr.id, rr.name
		ORDER BY rr.id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	stats := make([]RarityStat, 0)
	for rows.Next() {
		var s RarityStat
		if err := rows.Scan(&s.Name, &s.Cards, &s.Owned); err != nil {
			return nil, err
		}
		stats = append(stats, s)
	}
	return stats, rows.Err()
}

// signupSeries returns registrations per day for the last 14 days, zero-filled.
func (r *PostgresRepo) signupSeries() ([]DayCount, error) {
	rows, err := r.db.Query(`
		SELECT to_char(d.day, 'YYYY-MM-DD'), COALESCE(COUNT(u.id), 0)
		FROM generate_series(CURRENT_DATE - INTERVAL '13 days', CURRENT_DATE, INTERVAL '1 day') AS d(day)
		LEFT JOIN users u ON u.created_at::date = d.day::date
		GROUP BY d.day ORDER BY d.day ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	series := make([]DayCount, 0, 14)
	for rows.Next() {
		var c DayCount
		if err := rows.Scan(&c.Day, &c.Count); err != nil {
			return nil, err
		}
		series = append(series, c)
	}
	return series, rows.Err()
}

func (r *PostgresRepo) topOwners() ([]TopOwnerStat, error) {
	rows, err := r.db.Query(`
		SELECT COALESCE(NULLIF(u.first_name, ''), NULLIF(u.username, ''), 'Игрок ' || u.id),
		       COUNT(i.card_id)
		FROM users u JOIN user_inventory i ON i.user_id = u.id
		GROUP BY u.id, u.first_name, u.username
		ORDER BY COUNT(i.card_id) DESC
		LIMIT 5`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	top := make([]TopOwnerStat, 0, 5)
	for rows.Next() {
		var t TopOwnerStat
		if err := rows.Scan(&t.Name, &t.Cards); err != nil {
			return nil, err
		}
		top = append(top, t)
	}
	return top, rows.Err()
}
