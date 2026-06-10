package artguess

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

const settingKey = "artguess_config"

// Config is the Art Guess configuration, stored as JSON in
// bot_settings['artguess_config'] (see docs/design/Art Guess.md). Unlike spawns,
// the game is enabled by default — the daily puzzle is harmless and the web hub
// gates visibility on Enabled anyway.
type Config struct {
	Enabled         bool        `json:"enabled"`
	MaxAttempts     int         `json:"max_attempts"`
	RewardBase      int         `json:"reward_base"`       // coins for solving at all
	RewardPerStep   int         `json:"reward_per_step"`   // extra coins per unused attempt
	GrantStreak     bool        `json:"grant_streak"`      // solving extends the daily streak
	EpochDate       string      `json:"epoch_date"`        // "YYYY-MM-DD" (MSK) that is puzzle #1
	ChatPostEnabled bool        `json:"chat_post_enabled"` // post the live scoreboard + daily summary to chats
	MorningPing     MorningPing `json:"morning_ping"`
	Pool            Pool        `json:"pool"`
}

// MorningPing controls the daily "card of the day is available" scoreboard the
// bot posts to registered chats.
type MorningPing struct {
	Enabled bool   `json:"enabled"`
	Time    string `json:"time"` // "HH:MM" MSK
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
		Enabled:         true,
		MaxAttempts:     6,
		RewardBase:      50,
		RewardPerStep:   10,
		GrantStreak:     true,
		EpochDate:       "2026-06-10",
		ChatPostEnabled: true,
		MorningPing:     MorningPing{Enabled: true, Time: "10:00"},
		Pool:            Pool{ExcludeMythic: false},
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
	if c.MorningPing.Time != "" {
		if _, _, ok := parseClock(c.MorningPing.Time); !ok {
			return fmt.Errorf("morning_ping.time must be HH:MM")
		}
	}
	return nil
}

// parseClock parses a "HH:MM" string into hour and minute.
func parseClock(hhmm string) (int, int, bool) {
	parts := strings.SplitN(hhmm, ":", 2)
	if len(parts) != 2 {
		return 0, 0, false
	}
	h, err1 := strconv.Atoi(strings.TrimSpace(parts[0]))
	m, err2 := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err1 != nil || err2 != nil || h < 0 || h > 23 || m < 0 || m > 59 {
		return 0, 0, false
	}
	return h, m, true
}
