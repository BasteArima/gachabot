package spawn

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// CurrentConfigJSON returns the active config (stored or default) as pretty JSON,
// for /spawn_export.
func (s *SpawnService) CurrentConfigJSON() ([]byte, error) {
	return json.MarshalIndent(s.loadConfig(), "", "  ")
}

// SaveConfigJSON validates and persists a new spawn config.
func (s *SpawnService) SaveConfigJSON(data []byte) (Config, error) {
	cfg, err := ParseConfig(data)
	if err != nil {
		return Config{}, err
	}
	norm, err := json.Marshal(cfg)
	if err != nil {
		return Config{}, err
	}
	if err := s.repo.SetSetting(settingKey, norm); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// Summarize renders a short human-readable description of a config (RU), used in
// the import dry-run preview.
func Summarize(cfg Config) string {
	var b strings.Builder
	yn := func(v bool) string {
		if v {
			return "да"
		}
		return "нет"
	}
	fmt.Fprintf(&b, "Включено: <b>%s</b>\n", yn(cfg.Enabled))
	fmt.Fprintf(&b, "Окно ловли: <b>%d сек</b>\n", cfg.ClaimWindowSeconds)
	fmt.Fprintf(&b, "Награда: <b>%d–%d</b> монет\n", cfg.RewardCoins.Min, cfg.RewardCoins.Max)

	switch cfg.Pool.Mode {
	case PoolCardList:
		fmt.Fprintf(&b, "Пул: список из <b>%d</b> карт\n", len(cfg.Pool.CardIDs))
	default:
		if len(cfg.Pool.RarityWeights) == 0 {
			b.WriteString("Пул: по редкостям (веса = дроп-шансы)\n")
		} else {
			fmt.Fprintf(&b, "Пул: по редкостям (%s)\n", weightsString(cfg.Pool.RarityWeights))
		}
	}
	if len(cfg.Pool.ExcludeCardIDs) > 0 {
		fmt.Fprintf(&b, "Исключено карт: <b>%d</b>\n", len(cfg.Pool.ExcludeCardIDs))
	}

	tz := cfg.Schedule.Timezone
	if tz == "" {
		tz = "MSK"
	}
	fmt.Fprintf(&b, "\n<b>Расписание</b> (зона %s):\n", tz)
	fmt.Fprintf(&b, "• будни (default): %s\n", daySummary(cfg.Schedule.Default))
	days := make([]string, 0, len(cfg.Schedule.PerWeekday))
	for d := range cfg.Schedule.PerWeekday {
		days = append(days, d)
	}
	sort.Strings(days)
	for _, d := range days {
		fmt.Fprintf(&b, "• %s: %s\n", d, daySummary(cfg.Schedule.PerWeekday[d]))
	}
	return b.String()
}

func daySummary(ds DaySchedule) string {
	switch ds.Mode {
	case ModeFixedTimes:
		return fmt.Sprintf("точные часы [%s]", strings.Join(ds.Times, ", "))
	case ModeInterval:
		w := "весь день"
		if len(ds.Window) == 2 {
			w = ds.Window[0] + "–" + ds.Window[1]
		}
		return fmt.Sprintf("каждые %.1f ч в окне %s", ds.IntervalHours, w)
	default:
		w := "весь день"
		if len(ds.Window) == 2 {
			w = ds.Window[0] + "–" + ds.Window[1]
		}
		return fmt.Sprintf("%d случайных в окне %s", ds.Count, w)
	}
}

func weightsString(weights map[string]float64) string {
	keys := make([]string, 0, len(weights))
	for k := range weights {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s:%g", k, weights[k]))
	}
	return strings.Join(parts, ", ")
}
