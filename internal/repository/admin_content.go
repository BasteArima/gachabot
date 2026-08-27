package repository

import (
	"database/sql"
	"fmt"

	"gachabot/internal/models"
)

// AdminCard is a card row enriched with the names the admin UI displays.
type AdminCard struct {
	ID         int    `json:"id"`
	Name       string `json:"name"`
	RarityID   int    `json:"rarityId"`
	RarityName string `json:"rarityName"`
	PowerLevel int    `json:"powerLevel"`
	ImageURL   string `json:"imageUrl"`
	SetID      *int   `json:"setId"`
	SetName    string `json:"setName"`
	Owners     int    `json:"owners"` // how many players own at least one copy
}

// CardFilter narrows the admin card list. Zero values mean "no filter".
type CardFilter struct {
	Search   string
	RarityID int
	SetID    int
	NoArt    bool
}

// ListAdminCards returns cards for the admin browser, newest ids last.
func (r *PostgresRepo) ListAdminCards(f CardFilter) ([]AdminCard, error) {
	rows, err := r.db.Query(`
		SELECT c.id, c.name, c.rarity_id, r.name, c.power_level, COALESCE(c.image_url, ''),
		       c.set_id, COALESCE(cs.name, ''),
		       (SELECT COUNT(*) FROM user_inventory ui WHERE ui.card_id = c.id)
		FROM cards c
		JOIN rarities r ON c.rarity_id = r.id
		LEFT JOIN card_sets cs ON c.set_id = cs.id
		WHERE ($1 = '' OR c.name ILIKE '%' || $1 || '%')
		  AND ($2 = 0 OR c.rarity_id = $2)
		  AND ($3 = 0 OR c.set_id = $3)
		  AND (NOT $4 OR c.image_url IS NULL OR c.image_url = '')
		ORDER BY c.rarity_id DESC, c.name ASC`,
		f.Search, f.RarityID, f.SetID, f.NoArt)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	cards := make([]AdminCard, 0)
	for rows.Next() {
		var c AdminCard
		if err := rows.Scan(&c.ID, &c.Name, &c.RarityID, &c.RarityName, &c.PowerLevel,
			&c.ImageURL, &c.SetID, &c.SetName, &c.Owners); err != nil {
			return nil, err
		}
		cards = append(cards, c)
	}
	return cards, rows.Err()
}

// CreateCard inserts a new card. Card identity is its name (the theme importer
// matches on it), so a duplicate name is rejected rather than silently allowed.
func (r *PostgresRepo) CreateCard(name string, rarityID int, imageURL string, power int, setID *int) (int, error) {
	var exists bool
	if err := r.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM cards WHERE name = $1)`, name).Scan(&exists); err != nil {
		return 0, err
	}
	if exists {
		return 0, fmt.Errorf("карта с именем %q уже существует", name)
	}
	var id int
	err := r.db.QueryRow(`
		INSERT INTO cards (name, rarity_id, image_url, power_level, set_id)
		VALUES ($1, $2, $3, $4, $5) RETURNING id`,
		name, rarityID, imageURL, power, setID).Scan(&id)
	return id, err
}

// UpdateCard edits an existing card, keeping names unique across other cards.
func (r *PostgresRepo) UpdateCard(id int, name string, rarityID int, imageURL string, power int, setID *int) error {
	var exists bool
	if err := r.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM cards WHERE name = $1 AND id <> $2)`, name, id).Scan(&exists); err != nil {
		return err
	}
	if exists {
		return fmt.Errorf("другая карта с именем %q уже существует", name)
	}
	res, err := r.db.Exec(`
		UPDATE cards SET name = $2, rarity_id = $3, image_url = $4, power_level = $5, set_id = $6
		WHERE id = $1`, id, name, rarityID, imageURL, power, setID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// UpdateRarity edits a rarity's tuning values. The name is not editable: cards
// and themes reference rarities by name.
func (r *PostgresRepo) UpdateRarity(id int, dropChance float64, baseReward, pityThreshold, craftCost int, requiresFragments bool, fragmentsRequired int) error {
	res, err := r.db.Exec(`
		UPDATE rarities SET drop_chance = $2, base_reward = $3, pity_threshold = $4,
		       craft_cost = $5, requires_fragments = $6, fragments_required = $7
		WHERE id = $1`,
		id, dropChance, baseReward, pityThreshold, craftCost, requiresFragments, fragmentsRequired)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// AdminSet is a set row with its card count.
type AdminSet struct {
	models.CardSet
	CardCount int `json:"cardCount"`
}

func (r *PostgresRepo) ListAdminSets() ([]AdminSet, error) {
	rows, err := r.db.Query(`
		SELECT cs.id, cs.name, cs.buff_type, cs.buff_value, cs.reward_points,
		       (SELECT COUNT(*) FROM cards c WHERE c.set_id = cs.id)
		FROM card_sets cs ORDER BY cs.name ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	sets := make([]AdminSet, 0)
	for rows.Next() {
		var s AdminSet
		if err := rows.Scan(&s.ID, &s.Name, &s.BuffType, &s.BuffValue, &s.RewardPoints, &s.CardCount); err != nil {
			return nil, err
		}
		sets = append(sets, s)
	}
	return sets, rows.Err()
}

func (r *PostgresRepo) CreateSet(name, buffType string, buffValue, rewardPoints int) (int, error) {
	var exists bool
	if err := r.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM card_sets WHERE name = $1)`, name).Scan(&exists); err != nil {
		return 0, err
	}
	if exists {
		return 0, fmt.Errorf("сет с именем %q уже существует", name)
	}
	var id int
	err := r.db.QueryRow(`
		INSERT INTO card_sets (name, buff_type, buff_value, reward_points)
		VALUES ($1, $2, $3, $4) RETURNING id`, name, buffType, buffValue, rewardPoints).Scan(&id)
	return id, err
}

func (r *PostgresRepo) UpdateSet(id int, name, buffType string, buffValue, rewardPoints int) error {
	var exists bool
	if err := r.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM card_sets WHERE name = $1 AND id <> $2)`, name, id).Scan(&exists); err != nil {
		return err
	}
	if exists {
		return fmt.Errorf("другой сет с именем %q уже существует", name)
	}
	res, err := r.db.Exec(`
		UPDATE card_sets SET name = $2, buff_type = $3, buff_value = $4, reward_points = $5
		WHERE id = $1`, id, name, buffType, buffValue, rewardPoints)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// RarityArtFolders maps each rarity to the art folder its cards actually live
// in (…/cards/<Folder>/file.webp). Rarity names in the database are localised
// while the generator's folders and frame files are English, so this is the only
// honest bridge between them: it is read off the content itself. The dominant
// folder wins, so a stray odd url can't flip the answer.
func (r *PostgresRepo) RarityArtFolders() (map[int]string, error) {
	rows, err := r.db.Query(`
		SELECT rarity_id, folder FROM (
			SELECT c.rarity_id,
			       split_part(split_part(c.image_url, '/cards/', 2), '/', 1) AS folder,
			       row_number() OVER (PARTITION BY c.rarity_id ORDER BY count(*) DESC) AS rn
			FROM cards c
			WHERE c.image_url LIKE '%/cards/%'
			GROUP BY c.rarity_id, folder
		) t WHERE rn = 1`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make(map[int]string)
	for rows.Next() {
		var (
			id     int
			folder string
		)
		if err := rows.Scan(&id, &folder); err != nil {
			return nil, err
		}
		if folder != "" {
			out[id] = folder
		}
	}
	return out, rows.Err()
}
