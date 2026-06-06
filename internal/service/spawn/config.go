package spawn

import (
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Schedule modes.
const (
	ModeFixedTimes   = "fixed_times"
	ModeRandomPerDay = "random_per_day"
	ModeInterval     = "interval"
)

// Pool modes.
const (
	PoolWeightedRarity = "weighted_rarity"
	PoolCardList       = "card_list"
)

// Config is the full spawn configuration, stored as JSON in
// bot_settings['spawn_config'] (see docs/design/Chat Spawns.md).
type Config struct {
	Enabled            bool      `json:"enabled"`
	ClaimWindowSeconds int       `json:"claim_window_seconds"`
	RewardCoins        CoinRange `json:"reward_coins"`
	Schedule           Schedule  `json:"schedule"`
	Pool               Pool      `json:"pool"`
}

type CoinRange struct {
	Min int `json:"min"`
	Max int `json:"max"`
}

type Schedule struct {
	Timezone   string                 `json:"timezone"`
	Default    DaySchedule            `json:"default"`
	PerWeekday map[string]DaySchedule `json:"per_weekday"`
}

type DaySchedule struct {
	Mode          string   `json:"mode"`
	Count         int      `json:"count"`
	Window        []string `json:"window"` // ["HH:MM","HH:MM"]
	Times         []string `json:"times"`  // for fixed_times
	IntervalHours float64  `json:"interval_hours"`
}

type Pool struct {
	Mode           string             `json:"mode"`
	RarityWeights  map[string]float64 `json:"rarity_weights"` // by rarity name; empty -> use rarities' drop_chance
	CardIDs        []int64            `json:"card_ids"`
	ExcludeCardIDs []int64            `json:"exclude_card_ids"`
}

// DefaultConfig is used until an admin uploads spawn_config.json. Spawns are OFF
// by default so a fresh deploy never auto-spawns before the admin opts in via
// /spawn_import (the /spawnnow test command ignores this flag). Empty rarity
// weights mean "fall back to each rarity's own drop_chance".
func DefaultConfig() Config {
	return Config{
		Enabled:            false,
		ClaimWindowSeconds: 90,
		RewardCoins:        CoinRange{Min: 20, Max: 60},
		Schedule: Schedule{
			Timezone: "MSK",
			Default:  DaySchedule{Mode: ModeRandomPerDay, Count: 3, Window: []string{"10:00", "23:00"}},
		},
		Pool: Pool{Mode: PoolWeightedRarity},
	}
}

// ParseConfig validates and decodes a spawn config JSON document.
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
	if c.ClaimWindowSeconds < 5 || c.ClaimWindowSeconds > 3600 {
		return fmt.Errorf("claim_window_seconds must be between 5 and 3600")
	}
	if c.RewardCoins.Min < 0 || c.RewardCoins.Max < c.RewardCoins.Min {
		return fmt.Errorf("reward_coins range is invalid")
	}
	switch c.Pool.Mode {
	case PoolWeightedRarity, PoolCardList, "":
	default:
		return fmt.Errorf("unknown pool mode %q", c.Pool.Mode)
	}
	if c.Pool.Mode == PoolCardList && len(c.Pool.CardIDs) == 0 {
		return fmt.Errorf("pool mode card_list requires card_ids")
	}
	return nil
}

// scheduleForWeekday returns the per-weekday schedule, falling back to default.
func (c Config) scheduleForWeekday(wd time.Weekday) DaySchedule {
	if c.Schedule.PerWeekday != nil {
		if ds, ok := c.Schedule.PerWeekday[strings.ToLower(wd.String())]; ok {
			return ds
		}
	}
	return c.Schedule.Default
}

// planDay computes the spawn timestamps for the given calendar day, sorted.
func (c Config) planDay(day time.Time, loc *time.Location) []time.Time {
	ds := c.scheduleForWeekday(day.Weekday())
	var times []time.Time

	switch ds.Mode {
	case ModeFixedTimes:
		for _, t := range ds.Times {
			if at, ok := parseHHMM(day, t, loc); ok {
				times = append(times, at)
			}
		}
	case ModeInterval:
		if ds.IntervalHours <= 0 {
			break
		}
		start, end := dayWindow(day, ds.Window, loc)
		step := time.Duration(ds.IntervalHours * float64(time.Hour))
		for t := start; t.Before(end); t = t.Add(step) {
			times = append(times, t)
		}
	default: // ModeRandomPerDay
		if ds.Count <= 0 {
			break
		}
		start, end := dayWindow(day, ds.Window, loc)
		span := end.Sub(start)
		if span <= 0 {
			break
		}
		for i := 0; i < ds.Count; i++ {
			off := time.Duration(rand.Int64N(int64(span)))
			times = append(times, start.Add(off))
		}
	}

	sort.Slice(times, func(i, j int) bool { return times[i].Before(times[j]) })
	return times
}

// dayWindow resolves a ["HH:MM","HH:MM"] window on the given day. A missing or
// invalid window defaults to the whole day. An end <= start is treated as
// crossing midnight (end moves to the next day).
func dayWindow(day time.Time, window []string, loc *time.Location) (time.Time, time.Time) {
	start := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, loc)
	end := start.Add(24 * time.Hour)
	if len(window) == 2 {
		if s, ok := parseHHMM(day, window[0], loc); ok {
			start = s
		}
		if e, ok := parseHHMM(day, window[1], loc); ok {
			end = e
		}
	}
	if !end.After(start) {
		end = end.Add(24 * time.Hour)
	}
	return start, end
}

func parseHHMM(day time.Time, hhmm string, loc *time.Location) (time.Time, bool) {
	parts := strings.SplitN(hhmm, ":", 2)
	if len(parts) != 2 {
		return time.Time{}, false
	}
	h, err1 := strconv.Atoi(strings.TrimSpace(parts[0]))
	m, err2 := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err1 != nil || err2 != nil || h < 0 || h > 23 || m < 0 || m > 59 {
		return time.Time{}, false
	}
	return time.Date(day.Year(), day.Month(), day.Day(), h, m, 0, 0, loc), true
}
