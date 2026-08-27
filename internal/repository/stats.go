package repository

// GetUserCount returns the total number of registered players.
func (r *PostgresRepo) GetUserCount() (int, error) {
	var n int
	err := r.db.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&n)
	return n, err
}

// AdminStats is the counter set shown on the admin dashboard.
type AdminStats struct {
	Users        int `json:"users"`
	UsersActive  int `json:"usersActive"` // rolled in the last 24h
	Cards        int `json:"cards"`
	CardsNoArt   int `json:"cardsNoArt"`
	Rarities     int `json:"rarities"`
	Sets         int `json:"sets"`
	Chats        int `json:"chats"`
	ChatsEnabled int `json:"chatsEnabled"`
}

// GetAdminStats collects the dashboard counters in a single round trip.
func (r *PostgresRepo) GetAdminStats() (AdminStats, error) {
	var s AdminStats
	err := r.db.QueryRow(`
		SELECT
			(SELECT COUNT(*) FROM users),
			(SELECT COUNT(*) FROM users WHERE last_roll_time > NOW() - INTERVAL '24 hours'),
			(SELECT COUNT(*) FROM cards),
			(SELECT COUNT(*) FROM cards WHERE image_url IS NULL OR image_url = ''),
			(SELECT COUNT(*) FROM rarities),
			(SELECT COUNT(*) FROM card_sets),
			(SELECT COUNT(*) FROM chats),
			(SELECT COUNT(*) FROM chats WHERE spawn_enabled)
	`).Scan(&s.Users, &s.UsersActive, &s.Cards, &s.CardsNoArt, &s.Rarities, &s.Sets, &s.Chats, &s.ChatsEnabled)
	return s, err
}
