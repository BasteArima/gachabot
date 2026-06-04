package gacha

import (
	"gachabot/internal/models"
	"gachabot/internal/repository"
	"time"

	"github.com/redis/go-redis/v9"
)

type GachaService struct {
	repo              *repository.PostgresRepo
	rdb               *redis.Client
	loc               *time.Location
	adminID           int64
	cooldownHours     time.Duration
	duplicatesEnabled bool
}

func NewGachaService(repo *repository.PostgresRepo, rdb *redis.Client, adminID int64, cooldown time.Duration, duplicatesEnabled bool) *GachaService {
	loc := time.FixedZone("MSK", 3*60*60)
	return &GachaService{
		repo:              repo,
		rdb:               rdb,
		loc:               loc,
		adminID:           adminID,
		cooldownHours:     cooldown,
		duplicatesEnabled: duplicatesEnabled,
	}
}

// DuplicatesEnabled reports whether duplicate cards are allowed (used by the
// delivery layer to hide duplicate-related UI when off).
func (s *GachaService) DuplicatesEnabled() bool {
	return s.duplicatesEnabled
}

// findRarity returns a pointer to the rarity with the given ID, or nil if not found.
func findRarity(rarities []models.Rarity, id int) *models.Rarity {
	for i := range rarities {
		if rarities[i].ID == id {
			return &rarities[i]
		}
	}
	return nil
}

// FragmentsRequiredFor returns how many fragments are needed to assemble a card
// of the given rarity, or 0 if the rarity is unknown / not fragment-based.
func (s *GachaService) FragmentsRequiredFor(rarityID int) int {
	rarities, err := s.repo.GetRarities()
	if err != nil {
		return 0
	}
	if r := findRarity(rarities, rarityID); r != nil {
		return r.FragmentsRequired
	}
	return 0
}
