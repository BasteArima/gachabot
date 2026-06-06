package repository

// GetUserCount returns the total number of registered players.
func (r *PostgresRepo) GetUserCount() (int, error) {
	var n int
	err := r.db.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&n)
	return n, err
}
