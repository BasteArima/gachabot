package artguess

import (
	"encoding/json"
	"fmt"
	"time"
)

const settingKey = "artguess_config"

// Config is the Art Guess configuration, stored as JSON in
// bot_settings['artguess_config'] (see docs/design/Art Guess.md). Unlike spawns,
// the game is enabled by default — the daily puzzle is harmless and the web hub
// gates visibility on Enabled anyway.
type Config struct {
	Enabled       bool   `json:"enabled"`
	MaxAttempts   int    `json:"max_attempts"`
	RewardBase    int    `json:"reward_base"`     // coins for solving at all
	RewardPerStep int    `json:"reward_per_step"` // extra coins per unused attempt
	GrantStreak   bool   `json:"grant_streak"`    // solving extends the daily streak
	EpochDate     string `json:"epoch_date"`      // "YYYY-MM-DD" (MSK) that is puzzle #1
	Pool          Pool   `json:"pool"`
}

// Pool curates which cards can be the card of the day. Empty CardIDs means "any
// card that has art". ExcludeMythic drops cards whose rarity requires fragments
// (few players own them, harder to recognise).
type Pool struct {
	ExcludeMythic  bool    `json:"exclude_mythic"`
	CardIDs        []int64 `json:"card_ids"`
	ExcludeCardIDs []int64 `json:"exclude_card_ids"`
}

// DefaultConfig is used until an admin saves an artguess_config. Epoch defaults
// to the feature launch day so puzzle numbers start small (#1 on launch day).
func DefaultConfig() Config {
	return Config{
		Enabled:       true,
		MaxAttempts:   6,
		RewardBase:    50,
		RewardPerStep: 10,
		GrantStreak:   true,
		EpochDate:     "2026-06-10",
		Pool:          Pool{ExcludeMythic: false},
	}
}

// ParseConfig validates and decodes an artguess config JSON document.
func ParseConfig(data []byte) (Config, error) {
	var c Config
	if err := json.Unmarshal(data, &c); err != nil {
		return Config{}, fmt.Errorf("invalid JSON: %w", err)
	}
	if err := c.Validate(); err != nil {
		return Config{}, err
	}
	return c, nil
}

func (c Config) Validate() error {
	if c.MaxAttempts < 1 || c.MaxAttempts > 12 {
		return fmt.Errorf("max_attempts must be between 1 and 12")
	}
	if c.RewardBase < 0 || c.RewardPerStep < 0 {
		return fmt.Errorf("reward values must be non-negative")
	}
	if c.EpochDate != "" {
		if _, err := time.Parse("2006-01-02", c.EpochDate); err != nil {
			return fmt.Errorf("epoch_date must be YYYY-MM-DD")
		}
	}
	return nil
}
