package repository

import (
	"database/sql"
	"gachabot/internal/models"
)

func (r *PostgresRepo) CheckSetCompletion(userID int64, setID int) (bool, error) {
	query := `
		SELECT 
			(SELECT COUNT(id) FROM cards WHERE set_id = $2) as total_cards,
			(SELECT COUNT(DISTINCT c.id) 
			 FROM user_inventory i 
			 JOIN cards c ON i.card_id = c.id 
			 WHERE i.user_id = $1 AND c.set_id = $2) as collected_cards
	`
	var total, collected int
	err := r.db.QueryRow(query, userID, setID).Scan(&total, &collected)
	if err != nil {
		return false, err
	}
	if total == 0 {
		return false, nil
	}
	return total == collected, nil
}

func (r *PostgresRepo) IsSetRewardClaimed(userID int64, setID int) (bool, error) {
	var isCompleted bool
	err := r.db.QueryRow(`SELECT is_completed FROM user_unlocked_sets WHERE user_id = $1 AND set_id = $2`, userID, setID).Scan(&isCompleted)
	if err != nil {
		return false, nil // Если записи нет, значит точно не забирал
	}
	return isCompleted, nil
}

func (r *PostgresRepo) GetCardSetByID(setID int) (*models.CardSet, error) {
	query := `SELECT id, name, buff_type, buff_value, reward_points FROM card_sets WHERE id = $1`
	var cs models.CardSet
	err := r.db.QueryRow(query, setID).Scan(&cs.ID, &cs.Name, &cs.BuffType, &cs.BuffValue, &cs.RewardPoints)
	if err != nil {
		return nil, err
	}
	return &cs, nil
}

func (r *PostgresRepo) MarkSetCompleted(userID int64, setID int) error {
	query := `
		INSERT INTO user_unlocked_sets (user_id, set_id, is_completed)
		VALUES ($1, $2, true)
		ON CONFLICT (user_id, set_id) DO UPDATE SET is_completed = true;
	`
	_, err := r.db.Exec(query, userID, setID)
	return err
}

func (r *PostgresRepo) GetUserSetsStats(userID int64) (int, int, error) {
	query := `
		SELECT 
			(SELECT COUNT(set_id) FROM user_unlocked_sets WHERE user_id = $1 AND is_completed = true) as completed_sets,
			(SELECT COUNT(id) FROM card_sets) as total_sets
	`
	var completed, total int
	err := r.db.QueryRow(query, userID).Scan(&completed, &total)
	if err != nil {
		return 0, 0, err
	}
	return completed, total, nil
}

func (r *PostgresRepo) GetUserSetsProgress(userID int64) ([]models.UserSetProgress, error) {
	// 1) Take all sets and calculate their size (TotalSetCards)
	// 2) Calculate how many cards from the sets the user has (UserSetCards)
	// 3) Combine everything together. If the user has 0 cards, COALESCE will return 0
	query := `
		WITH TotalSetCards AS (
			SELECT set_id, COUNT(id) as total_cards
			FROM cards
			WHERE set_id IS NOT NULL
			GROUP BY set_id
		),
		UserSetCards AS (
			SELECT c.set_id, COUNT(DISTINCT c.id) as collected_cards
			FROM user_inventory i
			JOIN cards c ON i.card_id = c.id
			WHERE i.user_id = $1 AND c.set_id IS NOT NULL
			GROUP BY c.set_id
		)
		SELECT 
			cs.id, cs.name, cs.buff_type, cs.buff_value, cs.reward_points,
			COALESCE(usc.collected_cards, 0) as collected_cards,
			tsc.total_cards,
			COALESCE(uus.is_completed, false) as is_completed,
			COALESCE((u.active_set_id = cs.id), false) as is_active
		FROM TotalSetCards tsc
		JOIN card_sets cs ON cs.id = tsc.set_id
		LEFT JOIN UserSetCards usc ON tsc.set_id = usc.set_id
		LEFT JOIN user_unlocked_sets uus ON uus.user_id = $1 AND uus.set_id = cs.id
		LEFT JOIN users u ON u.id = $1
		ORDER BY cs.id;
	`
	rows, err := r.db.Query(query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var progress []models.UserSetProgress
	for rows.Next() {
		var p models.UserSetProgress
		if err := rows.Scan(&p.SetID, &p.SetName, &p.BuffType, &p.BuffValue, &p.RewardPoints, &p.CollectedCards, &p.TotalCards, &p.IsCompleted, &p.IsActive); err != nil {
			return nil, err
		}
		progress = append(progress, p)
	}
	return progress, nil
}

func (r *PostgresRepo) GetSetCards(userID int64, setID int) ([]models.Card, error) {
	query := `
		SELECT c.id, c.name, c.rarity_id, c.power_level, c.image_url, 
		       (MAX(i.user_id) IS NOT NULL) as is_owned
		FROM cards c
		LEFT JOIN user_inventory i ON c.id = i.card_id AND i.user_id = $1
		WHERE c.set_id = $2
		GROUP BY c.id, c.name, c.rarity_id, c.power_level, c.image_url
		ORDER BY c.rarity_id, c.id;
	`
	rows, err := r.db.Query(query, userID, setID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var cards []models.Card
	for rows.Next() {
		var c models.Card
		var isOwned bool

		if err := rows.Scan(&c.ID, &c.Name, &c.RarityID, &c.PowerLevel, &c.ImageURL, &isOwned); err != nil {
			return nil, err
		}

		if !isOwned {
			c.Name = ""
			c.ImageURL = ""
			c.PowerLevel = 0
		}
		cards = append(cards, c)
	}
	return cards, nil
}

func (r *PostgresRepo) EquipSetAura(userID int64, setID *int) error {
	query := `UPDATE users SET active_set_id = $1 WHERE id = $2`
	_, err := r.db.Exec(query, setID, userID)
	return err
}

func (r *PostgresRepo) GetActiveUserAura(userID int64) (*models.CardSet, error) {
	query := `
		SELECT cs.id, cs.name, cs.buff_type, cs.buff_value, cs.reward_points
		FROM users u
		JOIN card_sets cs ON u.active_set_id = cs.id
		WHERE u.id = $1
	`
	var cs models.CardSet
	err := r.db.QueryRow(query, userID).Scan(&cs.ID, &cs.Name, &cs.BuffType, &cs.BuffValue, &cs.RewardPoints)

	if err == sql.ErrNoRows {
		return nil, nil // У юзера не экипирована аура, это нормально
	}
	if err != nil {
		return nil, err
	}
	return &cs, nil
}
