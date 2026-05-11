package repository

func (r *PostgresRepo) GetUserPities(userID int64) (map[int]int, error) {
	pities := make(map[int]int)
	rows, err := r.db.Query("SELECT rarity_id, counter FROM user_pity WHERE user_id = $1", userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var rarityID, counter int
		if err := rows.Scan(&rarityID, &counter); err != nil {
			return nil, err
		}
		pities[rarityID] = counter
	}
	return pities, nil
}

func (r *PostgresRepo) UpdateUserPities(userID int64, pities map[int]int) error {
	query := `
		INSERT INTO user_pity (user_id, rarity_id, counter) 
		VALUES ($1, $2, $3)
		ON CONFLICT (user_id, rarity_id) 
		DO UPDATE SET counter = EXCLUDED.counter
	`
	for rarityID, counter := range pities {
		if _, err := r.db.Exec(query, userID, rarityID, counter); err != nil {
			return err
		}
	}
	return nil
}
