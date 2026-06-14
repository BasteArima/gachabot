package repository

import (
	"gachabot/internal/models"
)

// GetUserInventoryList returns all cards a user owns with display info, optionally
// filtered by rarity name. An empty rarity or "All" means no filter.
func (r *PostgresRepo) GetUserInventoryList(userID int64, rarityName string) ([]models.InventoryCard, error) {
	query := `
		SELECT c.id, c.name, r.name, COALESCE(c.image_url, ''), c.power_level, i.quantity,
		       COALESCE(cs.name, '')
		FROM user_inventory i
		JOIN cards c ON i.card_id = c.id
		JOIN rarities r ON c.rarity_id = r.id
		LEFT JOIN card_sets cs ON c.set_id = cs.id
		WHERE i.user_id = $1 AND ($2 = '' OR $2 = 'All' OR r.name = $2)
		ORDER BY (i.quantity > 1) DESC, c.rarity_id DESC, c.power_level DESC, c.id ASC`

	rows, err := r.db.Query(query, userID, rarityName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var cards []models.InventoryCard
	for rows.Next() {
		var c models.InventoryCard
		if err := rows.Scan(&c.CardID, &c.Name, &c.RarityName, &c.ImageURL, &c.PowerLevel, &c.Quantity, &c.SetName); err != nil {
			return nil, err
		}
		cards = append(cards, c)
	}
	return cards, rows.Err()
}
