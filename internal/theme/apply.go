package theme

import (
	"database/sql"
	"fmt"
)

// Options controls how a theme is applied.
type Options struct {
	DryRun bool
	// Reset deletes ALL existing rarities/sets/cards before applying. This
	// cascades to user_inventory, user_fragments, user_pity and unlocked sets —
	// i.e. it wipes player collections. Intended for empty or dev databases only.
	Reset bool
}

// Report summarizes what an import changed (or would change, in dry-run).
type Report struct {
	RaritiesUpserted int
	SetsUpserted     int
	CardsInserted    int
	CardsUpdated     int
	DryRun           bool
	Reset            bool
}

func (r Report) String() string {
	mode := "applied"
	if r.DryRun {
		mode = "would apply (dry-run)"
	}
	return fmt.Sprintf("%s: %d rarities, %d sets, %d new cards, %d updated cards",
		mode, r.RaritiesUpserted, r.SetsUpserted, r.CardsInserted, r.CardsUpdated)
}

// Import applies a theme to the database additively: rows are matched by name and
// inserted or updated. Existing cards keep their id (so user inventories stay
// intact); nothing is deleted. Everything runs in a single transaction — on dry
// run the transaction is rolled back so the database is never touched.
func Import(db *sql.DB, t *Theme, opts Options) (Report, error) {
	rep := Report{DryRun: opts.DryRun, Reset: opts.Reset}

	if _, err := t.Validate(); err != nil {
		return rep, err
	}

	tx, err := db.Begin()
	if err != nil {
		return rep, err
	}
	defer tx.Rollback() // no-op after a successful Commit

	if opts.Reset {
		// Order matters: cards reference rarities (ON DELETE RESTRICT), so cards go first.
		for _, stmt := range []string{`DELETE FROM cards`, `DELETE FROM card_sets`, `DELETE FROM rarities`} {
			if _, err := tx.Exec(stmt); err != nil {
				return rep, fmt.Errorf("reset (%s): %w", stmt, err)
			}
		}
	}

	rarityID := map[string]int{}
	for _, r := range t.Rarities {
		var id int
		err := tx.QueryRow(`
			INSERT INTO rarities (name, drop_chance, base_reward, pity_threshold, craft_cost, requires_fragments, fragments_required)
			VALUES ($1, $2, $3, $4, $5, $6, $7)
			ON CONFLICT (name) DO UPDATE SET
				drop_chance = EXCLUDED.drop_chance,
				base_reward = EXCLUDED.base_reward,
				pity_threshold = EXCLUDED.pity_threshold,
				craft_cost = EXCLUDED.craft_cost,
				requires_fragments = EXCLUDED.requires_fragments,
				fragments_required = EXCLUDED.fragments_required
			RETURNING id`,
			r.Name, r.DropChance, r.BaseReward, r.PityThreshold, r.CraftCost, r.RequiresFragments, r.FragmentsRequired,
		).Scan(&id)
		if err != nil {
			return rep, fmt.Errorf("upsert rarity %q: %w", r.Name, err)
		}
		rarityID[r.Name] = id
		rep.RaritiesUpserted++
	}

	// card_sets has no UNIQUE(name), so match-then-write by name.
	setID := map[string]int{}
	for _, s := range t.Sets {
		var id int
		err := tx.QueryRow(`SELECT id FROM card_sets WHERE name = $1`, s.Name).Scan(&id)
		switch {
		case err == sql.ErrNoRows:
			err = tx.QueryRow(`
				INSERT INTO card_sets (name, buff_type, buff_value, reward_points)
				VALUES ($1, $2, $3, $4) RETURNING id`,
				s.Name, s.BuffType, s.BuffValue, s.RewardPoints,
			).Scan(&id)
		case err == nil:
			_, err = tx.Exec(`
				UPDATE card_sets SET buff_type = $2, buff_value = $3, reward_points = $4 WHERE id = $1`,
				id, s.BuffType, s.BuffValue, s.RewardPoints)
		}
		if err != nil {
			return rep, fmt.Errorf("upsert set %q: %w", s.Name, err)
		}
		setID[s.Name] = id
		rep.SetsUpserted++
	}

	for _, c := range t.Cards {
		var setRef *int
		if c.Set != "" {
			id := setID[c.Set]
			setRef = &id
		}

		var cardID int
		err := tx.QueryRow(`SELECT id FROM cards WHERE name = $1`, c.Name).Scan(&cardID)
		switch {
		case err == sql.ErrNoRows:
			_, err = tx.Exec(`
				INSERT INTO cards (name, rarity_id, image_url, power_level, set_id)
				VALUES ($1, $2, $3, $4, $5)`,
				c.Name, rarityID[c.Rarity], c.ImageURL, c.PowerLevel, setRef)
			rep.CardsInserted++
		case err == nil:
			// Keep the existing id; update its attributes so user_inventory stays valid.
			_, err = tx.Exec(`
				UPDATE cards SET rarity_id = $2, image_url = $3, power_level = $4, set_id = $5 WHERE id = $1`,
				cardID, rarityID[c.Rarity], c.ImageURL, c.PowerLevel, setRef)
			rep.CardsUpdated++
		}
		if err != nil {
			return rep, fmt.Errorf("upsert card %q: %w", c.Name, err)
		}
	}

	if opts.DryRun {
		return rep, nil // deferred Rollback discards everything
	}
	if err := tx.Commit(); err != nil {
		return rep, err
	}
	return rep, nil
}

// Export reads the current content of the database into a Theme.
func Export(db *sql.DB) (*Theme, error) {
	t := &Theme{}

	rows, err := db.Query(`SELECT name, drop_chance, base_reward, pity_threshold, craft_cost,
		requires_fragments, fragments_required FROM rarities ORDER BY id`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var r Rarity
		if err := rows.Scan(&r.Name, &r.DropChance, &r.BaseReward, &r.PityThreshold, &r.CraftCost,
			&r.RequiresFragments, &r.FragmentsRequired); err != nil {
			rows.Close()
			return nil, err
		}
		t.Rarities = append(t.Rarities, r)
	}
	rows.Close()

	rows, err = db.Query(`SELECT name, buff_type, buff_value, reward_points FROM card_sets ORDER BY id`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var s Set
		if err := rows.Scan(&s.Name, &s.BuffType, &s.BuffValue, &s.RewardPoints); err != nil {
			rows.Close()
			return nil, err
		}
		t.Sets = append(t.Sets, s)
	}
	rows.Close()

	rows, err = db.Query(`
		SELECT c.name, r.name, c.power_level, COALESCE(c.image_url, ''), COALESCE(s.name, '')
		FROM cards c
		JOIN rarities r ON c.rarity_id = r.id
		LEFT JOIN card_sets s ON c.set_id = s.id
		ORDER BY c.id`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var c Card
		if err := rows.Scan(&c.Name, &c.Rarity, &c.PowerLevel, &c.ImageURL, &c.Set); err != nil {
			rows.Close()
			return nil, err
		}
		t.Cards = append(t.Cards, c)
	}
	rows.Close()

	return t, nil
}
