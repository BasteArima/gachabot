package spawn

import (
	"testing"
	"time"

	"gachabot/internal/models"
)

var testLoc = time.FixedZone("MSK", 3*60*60)

func day(y int, m time.Month, d int) time.Time {
	return time.Date(y, m, d, 12, 0, 0, 0, testLoc)
}

func TestPlanDayFixedTimes(t *testing.T) {
	cfg := Config{Schedule: Schedule{Default: DaySchedule{
		Mode:  ModeFixedTimes,
		Times: []string{"18:00", "09:30", "garbage", "23:15"},
	}}}
	times := cfg.planDay(day(2026, time.June, 6), testLoc)

	if len(times) != 3 {
		t.Fatalf("expected 3 valid times, got %d", len(times))
	}
	// Must be sorted ascending.
	for i := 1; i < len(times); i++ {
		if times[i].Before(times[i-1]) {
			t.Fatalf("times not sorted: %v", times)
		}
	}
	if h, m := times[0].Hour(), times[0].Minute(); h != 9 || m != 30 {
		t.Fatalf("earliest should be 09:30, got %02d:%02d", h, m)
	}
}

func TestPlanDayRandomPerDayWithinWindow(t *testing.T) {
	cfg := Config{Schedule: Schedule{Default: DaySchedule{
		Mode:   ModeRandomPerDay,
		Count:  5,
		Window: []string{"10:00", "20:00"},
	}}}
	d := day(2026, time.June, 6)
	times := cfg.planDay(d, testLoc)

	if len(times) != 5 {
		t.Fatalf("expected 5 times, got %d", len(times))
	}
	start := time.Date(2026, time.June, 6, 10, 0, 0, 0, testLoc)
	end := time.Date(2026, time.June, 6, 20, 0, 0, 0, testLoc)
	for _, ts := range times {
		if ts.Before(start) || ts.After(end) {
			t.Fatalf("time %v outside window [%v,%v]", ts, start, end)
		}
	}
}

func TestDayWindowCrossesMidnight(t *testing.T) {
	start, end := dayWindow(day(2026, time.June, 6), []string{"22:00", "02:00"}, testLoc)
	if !end.After(start) {
		t.Fatalf("end %v should be after start %v", end, start)
	}
	if end.Day() != 7 {
		t.Fatalf("end should roll to next day, got %v", end)
	}
}

func TestScheduleForWeekdayFallback(t *testing.T) {
	cfg := Config{Schedule: Schedule{
		Default:    DaySchedule{Mode: ModeRandomPerDay, Count: 3},
		PerWeekday: map[string]DaySchedule{"saturday": {Mode: ModeFixedTimes, Count: 9}},
	}}
	// 2026-06-06 is a Saturday.
	if ds := cfg.scheduleForWeekday(day(2026, time.June, 6).Weekday()); ds.Count != 9 {
		t.Fatalf("saturday override not applied, got count %d", ds.Count)
	}
	// 2026-06-08 is a Monday -> default.
	if ds := cfg.scheduleForWeekday(day(2026, time.June, 8).Weekday()); ds.Count != 3 {
		t.Fatalf("monday should use default, got count %d", ds.Count)
	}
}

func TestPickRarityWeighted(t *testing.T) {
	rarities := []models.Rarity{
		{ID: 1, Name: "Common", DropChance: 90},
		{ID: 2, Name: "Rare", DropChance: 10},
		{ID: 3, Name: "Mythic", DropChance: 1},
	}
	// Curated weights: only Rare eligible -> must always pick it.
	for i := 0; i < 50; i++ {
		if got := pickRarity(rarities, map[string]float64{"Rare": 5}); got != 2 {
			t.Fatalf("expected Rare(2), got %d", got)
		}
	}
	// Empty weights -> uses drop_chance; with all weight on existing rarities,
	// result is always a valid rarity id.
	for i := 0; i < 50; i++ {
		got := pickRarity(rarities, nil)
		if got != 1 && got != 2 && got != 3 {
			t.Fatalf("unexpected rarity id %d", got)
		}
	}
}

func TestParseConfigValidation(t *testing.T) {
	bad := []byte(`{"claim_window_seconds": 2, "reward_coins": {"min":1,"max":5}}`)
	if _, err := ParseConfig(bad); err == nil {
		t.Fatal("expected error for too-short claim window")
	}
	good := []byte(`{"claim_window_seconds": 90, "reward_coins": {"min":10,"max":50}, "pool": {"mode":"weighted_rarity"}}`)
	if _, err := ParseConfig(good); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
