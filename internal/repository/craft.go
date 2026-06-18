package repository

func (r *PostgresRepo) GetAvailableCrafts(userID int64) (map[int]int, error) {
	counts := make(map[int]int)
	// SUM(...) over an integer column is bigint, so scan it into int64; ::int would
	// also work but the cast risks overflow errors on huge stacks.
	query := `
       SELECT c.rarity_id, SUM(ui.quantity - 1)
       FROM user_inventory ui
       JOIN cards c ON ui.card_id = c.id
       WHERE ui.user_id = $1 AND ui.quantity > 1
       GROUP BY c.rarity_id
    `
	rows, err := r.db.Query(query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var rarityID int
		var dupCount int64
		if err := rows.Scan(&rarityID, &dupCount); err != nil {
			return nil, err
		}
		counts[rarityID] = int(dupCount)
	}
	return counts, rows.Err()
}

func (r *PostgresRepo) ConsumeDuplicates(userID int64, rarityID int, amount int) error {
	rows, err := r.db.Query(`
		SELECT card_id, quantity FROM user_inventory ui
		JOIN cards c ON ui.card_id = c.id
		WHERE ui.user_id = $1 AND c.rarity_id = $2 AND ui.quantity > 1
	`, userID, rarityID)
	if err != nil {
		return err
	}
	defer rows.Close()

	toBurn := amount
	for rows.Next() && toBurn > 0 {
		var cardID, quantity int
		rows.Scan(&cardID, &quantity)

		canTake := quantity - 1
		if canTake > toBurn {
			canTake = toBurn
		}

		_, _ = r.db.Exec("UPDATE user_inventory SET quantity = quantity - $1 WHERE user_id = $2 AND card_id = $3",
			canTake, userID, cardID)

		toBurn -= canTake
	}
	return nil
}
