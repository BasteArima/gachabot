package repository

import (
	"database/sql"

	"gachabot/internal/models"
)

func (r *PostgresRepo) GetRarities() ([]models.Rarity, error) {
	var rarities []models.Rarity
	rows, err := r.db.Query(`SELECT id, name, drop_chance, base_reward, pity_threshold, COALESCE(craft_cost, 0),
		COALESCE(requires_fragments, false), COALESCE(fragments_required, 0)
		FROM rarities ORDER BY id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var rarity models.Rarity
		if err := rows.Scan(&rarity.ID, &rarity.Name, &rarity.DropChance, &rarity.BaseReward, &rarity.PityThreshold, &rarity.CraftCost,
			&rarity.RequiresFragments, &rarity.FragmentsRequired); err != nil {
			return nil, err
		}
		rarities = append(rarities, rarity)
	}
	return rarities, nil
}

func (r *PostgresRepo) GetRandomCard(rarityID int) (*models.Card, error) {
	query := `SELECT id, name, rarity_id, image_url, power_level, set_id FROM cards WHERE rarity_id = $1 ORDER BY RANDOM() LIMIT 1`
	var c models.Card
	err := r.db.QueryRow(query, rarityID).Scan(&c.ID, &c.Name, &c.RarityID, &c.ImageURL, &c.PowerLevel, &c.SetID)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// GetRandomUnownedCard returns a random card of the given rarity that the user
// does not yet own. Returns (nil, nil) if the user owns every card of that rarity.
func (r *PostgresRepo) GetRandomUnownedCard(userID int64, rarityID int) (*models.Card, error) {
	query := `
		SELECT id, name, rarity_id, image_url, power_level, set_id
		FROM cards
		WHERE rarity_id = $1
		  AND id NOT IN (SELECT card_id FROM user_inventory WHERE user_id = $2)
		ORDER BY RANDOM() LIMIT 1`
	var c models.Card
	err := r.db.QueryRow(query, rarityID, userID).Scan(&c.ID, &c.Name, &c.RarityID, &c.ImageURL, &c.PowerLevel, &c.SetID)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// GetRandomUnownedCardAny returns a random card of any rarity the user does not
// own. Returns (nil, nil) when the user has collected every card.
func (r *PostgresRepo) GetRandomUnownedCardAny(userID int64) (*models.Card, error) {
	query := `
		SELECT id, name, rarity_id, image_url, power_level, set_id
		FROM cards
		WHERE id NOT IN (SELECT card_id FROM user_inventory WHERE user_id = $1)
		ORDER BY RANDOM() LIMIT 1`
	var c models.Card
	err := r.db.QueryRow(query, userID).Scan(&c.ID, &c.Name, &c.RarityID, &c.ImageURL, &c.PowerLevel, &c.SetID)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (r *PostgresRepo) AddCardToInventory(userID int64, cardID int) error {
	query := `
       INSERT INTO user_inventory (user_id, card_id, quantity) 
       VALUES ($1, $2, 1) 
       ON CONFLICT (user_id, card_id) 
       DO UPDATE SET quantity = user_inventory.quantity + 1
    `
	_, err := r.db.Exec(query, userID, cardID)
	return err
}

func (r *PostgresRepo) AddFragment(userID int64, cardID int) (int, error) {
	var currentFragments int
	query := `
       INSERT INTO user_fragments (user_id, card_id, quantity) 
       VALUES ($1, $2, 1) 
       ON CONFLICT (user_id, card_id) 
       DO UPDATE SET quantity = user_fragments.quantity + 1
       RETURNING quantity
    `
	err := r.db.QueryRow(query, userID, cardID).Scan(&currentFragments)
	return currentFragments, err
}

func (r *PostgresRepo) ClearFragments(userID int64, cardID int) error {
	query := "DELETE FROM user_fragments WHERE user_id=$1 AND card_id=$2"
	_, err := r.db.Exec(query, userID, cardID)
	return err
}

func (r *PostgresRepo) GetTotalCardsCount() (int, error) {
	var count int
	err := r.db.QueryRow("SELECT COUNT(*) FROM cards").Scan(&count)
	return count, err
}

func (r *PostgresRepo) GetUserUniqueCardsCount(userID int64) (int, error) {
	var count int
	err := r.db.QueryRow("SELECT COUNT(DISTINCT card_id) FROM user_inventory WHERE user_id = $1", userID).Scan(&count)
	return count, err
}

func (r *PostgresRepo) GetUserCard(userID int64, offset int) (*models.UserCardView, error) {
	query := `
		SELECT c.name, r.name, c.image_url, i.quantity, c.power_level,
		       COALESCE(cs.name, '') as set_name
		FROM user_inventory i
		JOIN cards c ON i.card_id = c.id
		JOIN rarities r ON c.rarity_id = r.id
		LEFT JOIN card_sets cs ON c.set_id = cs.id
		WHERE i.user_id = $1
		ORDER BY c.rarity_id DESC, c.power_level DESC, c.id ASC
		LIMIT 1 OFFSET $2
	`
	var card models.UserCardView
	err := r.db.QueryRow(query, userID, offset).Scan(
		&card.CardName, &card.RarityName, &card.ImageURL,
		&card.Quantity, &card.PowerLevel, &card.SetName,
	)
	if err != nil {
		return nil, err
	}
	return &card, nil
}

func (r *PostgresRepo) GetTotalDuplicatesCount(userID int64) (int, error) {
	var count int
	query := "SELECT COALESCE(SUM(quantity - 1), 0) FROM user_inventory WHERE user_id = $1 AND quantity > 1"
	err := r.db.QueryRow(query, userID).Scan(&count)
	return count, err
}

func (r *PostgresRepo) GetRandomUserCard(userID int64) (*models.Card, error) {
	card := &models.Card{}
	query := `
		SELECT c.id, c.name, c.rarity_id, c.image_url, c.power_level 
		FROM user_inventory ui
		JOIN cards c ON ui.card_id = c.id
		WHERE ui.user_id = $1
		ORDER BY RANDOM() LIMIT 1
	`
	err := r.db.QueryRow(query, userID).Scan(
		&card.ID, &card.Name, &card.RarityID, &card.ImageURL, &card.PowerLevel,
	)
	return card, err
}
