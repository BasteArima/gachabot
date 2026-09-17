package season

import (
	"testing"

	"gachabot/internal/models"
)

func TestDefaultConfigWeighsRarerCardsHigher(t *testing.T) {
	// Deliberately out of order, and with the rare one listed first: the weights
	// must follow drop chance, not the order the rows came back in.
	rarities := []models.Rarity{
		{ID: 7, Name: "Мифическая", DropChance: 0.5},
		{ID: 3, Name: "Обычная", DropChance: 60},
		{ID: 5, Name: "Редкая", DropChance: 10},
	}

	cfg := DefaultConfig(rarities)

	if got := cfg.Weights[3]; got != 1 {
		t.Errorf("самая частая редкость должна стоить 1, а стоит %d", got)
	}
	if cfg.Weights[5] <= cfg.Weights[3] {
		t.Errorf("редкая (%d) должна стоить больше обычной (%d)", cfg.Weights[5], cfg.Weights[3])
	}
	if cfg.Weights[7] <= cfg.Weights[5] {
		t.Errorf("мифическая (%d) должна стоить больше редкой (%d)", cfg.Weights[7], cfg.Weights[5])
	}
}

func TestDefaultConfigHandlesMoreRaritiesThanWeights(t *testing.T) {
	var rarities []models.Rarity
	for i := 0; i < len(fibWeights)+3; i++ {
		rarities = append(rarities, models.Rarity{ID: i + 1, DropChance: float64(100 - i)})
	}

	cfg := DefaultConfig(rarities)

	if len(cfg.Weights) != len(rarities) {
		t.Fatalf("вес должен быть у каждой редкости: %d из %d", len(cfg.Weights), len(rarities))
	}
	for _, r := range rarities {
		if cfg.Weights[r.ID] <= 0 {
			t.Errorf("редкость %d осталась без веса", r.ID)
		}
	}
}

func TestNormalizeRepairsNonsense(t *testing.T) {
	cfg := Config{
		Weights:            map[int]int{1: -5},
		ElitePercent:       0,
		MinActivityPercent: 140,
		PodiumMinPlayers:   0,
	}

	cfg.Normalize()

	if cfg.Weights[1] != 0 {
		t.Errorf("отрицательный вес должен стать нулём, а стал %d", cfg.Weights[1])
	}
	if cfg.ElitePercent != defaultElitePercent {
		t.Errorf("доля Элиты вне диапазона должна вернуться к %d, а стала %d", defaultElitePercent, cfg.ElitePercent)
	}
	if cfg.MinActivityPercent != defaultMinActivityPercent {
		t.Errorf("минимум активности вне диапазона должен вернуться к %d, а стал %d", defaultMinActivityPercent, cfg.MinActivityPercent)
	}
	if cfg.PodiumMinPlayers != defaultPodiumMinPlayers {
		t.Errorf("порог пьедестала должен вернуться к %d, а стал %d", defaultPodiumMinPlayers, cfg.PodiumMinPlayers)
	}
}

// A nil season service is the state the bot runs in before the first season is
// started, and every recorder must stay silent rather than panic there.
func TestRecordingWithoutSeasonDoesNothing(t *testing.T) {
	s := &Service{}

	s.RecordCoins(1, 100)
	s.RecordCard(1, 2, 3)
	s.RecordActivity(1, 5)

	if elapsed, minActive := s.Days(); elapsed != 0 || minActive != 0 {
		t.Errorf("без сезона дни должны быть нулевыми, а вышло %d и %d", elapsed, minActive)
	}
}
