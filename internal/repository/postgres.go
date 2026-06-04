package repository

import (
	"database/sql"
)

type PostgresRepo struct {
	db *sql.DB
}

func NewPostgresRepo(db *sql.DB) *PostgresRepo {
	return &PostgresRepo{db: db}
}

// DB exposes the underlying database handle for tooling that runs raw SQL
// (e.g. the theme seeder invoked from admin commands).
func (r *PostgresRepo) DB() *sql.DB {
	return r.db
}

func (r *PostgresRepo) GetAllActiveChatIDs() ([]int64, error) {
	var chatIDs []int64
	rows, err := r.db.Query("SELECT telegram_id FROM users WHERE telegram_id IS NOT NULL")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err == nil {
			chatIDs = append(chatIDs, id)
		}
	}

	return chatIDs, nil
}
