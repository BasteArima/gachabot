// Package theme describes a bot content theme (rarities, sets, cards) as a single
// declarative JSON file, and applies it to / reads it back from the database.
//
// A theme is the single source of truth for content. Cards reference rarities and
// sets by name (not by numeric id), so a theme file is portable and human-editable.
package theme

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
)

type Theme struct {
	Rarities []Rarity `json:"rarities"`
	Sets     []Set    `json:"sets"`
	Cards    []Card   `json:"cards"`
}

type Rarity struct {
	Name              string  `json:"name"`
	DropChance        float64 `json:"drop_chance"`
	BaseReward        int     `json:"base_reward"`
	PityThreshold     int     `json:"pity_threshold"`
	CraftCost         int     `json:"craft_cost"`
	RequiresFragments bool    `json:"requires_fragments"`
	FragmentsRequired int     `json:"fragments_required"`
}

type Set struct {
	Name         string `json:"name"`
	BuffType     string `json:"buff_type"`
	BuffValue    int    `json:"buff_value"`
	RewardPoints int    `json:"reward_points"`
}

type Card struct {
	Name       string `json:"name"`
	Rarity     string `json:"rarity"` // rarity name, resolved to rarity_id on import
	PowerLevel int    `json:"power_level"`
	ImageURL   string `json:"image_url"`
	Set        string `json:"set,omitempty"` // set name (optional), resolved to set_id
}

// Load reads and parses a theme file.
func Load(path string) (*Theme, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read theme file: %w", err)
	}
	return Parse(data)
}

// Parse decodes a theme from raw JSON bytes, rejecting unknown fields.
func Parse(data []byte) (*Theme, error) {
	var t Theme
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&t); err != nil {
		return nil, fmt.Errorf("failed to parse theme JSON: %w", err)
	}
	return &t, nil
}

// Save writes the theme as pretty-printed JSON.
func (t *Theme) Save(path string) error {
	data, err := json.MarshalIndent(t, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}

// Validate checks referential integrity and balance. It returns a list of
// non-fatal warnings and a fatal error (nil if the theme is safe to apply).
func (t *Theme) Validate() (warnings []string, err error) {
	rarityNames := map[string]bool{}
	for i, r := range t.Rarities {
		if r.Name == "" {
			return warnings, fmt.Errorf("rarities[%d]: name is empty", i)
		}
		if rarityNames[r.Name] {
			return warnings, fmt.Errorf("duplicate rarity name %q", r.Name)
		}
		rarityNames[r.Name] = true
		if r.RequiresFragments && r.FragmentsRequired <= 0 {
			warnings = append(warnings, fmt.Sprintf("rarity %q requires fragments but fragments_required is %d", r.Name, r.FragmentsRequired))
		}
		if r.DropChance < 0 {
			return warnings, fmt.Errorf("rarity %q has negative drop_chance", r.Name)
		}
	}

	var dropSum float64
	for _, r := range t.Rarities {
		dropSum += r.DropChance
	}
	if len(t.Rarities) > 0 && (dropSum < 99.0 || dropSum > 101.0) {
		warnings = append(warnings, fmt.Sprintf("drop_chance of all rarities sums to %.2f, expected ~100", dropSum))
	}

	setNames := map[string]bool{}
	for i, s := range t.Sets {
		if s.Name == "" {
			return warnings, fmt.Errorf("sets[%d]: name is empty", i)
		}
		if setNames[s.Name] {
			return warnings, fmt.Errorf("duplicate set name %q", s.Name)
		}
		setNames[s.Name] = true
	}

	cardNames := map[string]bool{}
	for i, c := range t.Cards {
		if c.Name == "" {
			return warnings, fmt.Errorf("cards[%d]: name is empty", i)
		}
		if cardNames[c.Name] {
			return warnings, fmt.Errorf("duplicate card name %q (card identity is its name)", c.Name)
		}
		cardNames[c.Name] = true
		if !rarityNames[c.Rarity] {
			return warnings, fmt.Errorf("card %q references unknown rarity %q", c.Name, c.Rarity)
		}
		if c.Set != "" && !setNames[c.Set] {
			return warnings, fmt.Errorf("card %q references unknown set %q", c.Name, c.Set)
		}
		if c.PowerLevel < 0 {
			return warnings, fmt.Errorf("card %q has negative power_level", c.Name)
		}
		if c.ImageURL == "" {
			warnings = append(warnings, fmt.Sprintf("card %q has empty image_url", c.Name))
		}
	}

	return warnings, nil
}
