package gacha

import (
	"encoding/json"
	"log"
	"sync/atomic"
	"time"

	"gachabot/internal/models"
	"gachabot/internal/repository"
	"gachabot/internal/service/season"

	"github.com/redis/go-redis/v9"
)

// adminCooldownBypassKey stores the owner's "roll without waiting" switch. It
// lives in bot_settings rather than an env var because it gets flipped for a
// single test and flipped straight back, and a redeploy each way is too slow —
// and because the owner also wants to compete on equal terms between tests.
const adminCooldownBypassKey = "admin_cooldown_bypass"

type GachaService struct {
	repo              *repository.PostgresRepo
	rdb               *redis.Client
	loc               *time.Location
	adminID           int64
	cooldownHours     time.Duration
	duplicatesEnabled bool
	craftEnabled      bool
	// adminBypass is read on every roll and written from the admin panel.
	adminBypass atomic.Bool
	// season counts what players earn while a season runs. Set after
	// construction (the season service is built later in the wiring) and nil
	// when seasons are not in use, so every call site checks it.
	season *season.Service
}

// UseSeason attaches the season scoreboard. Rolls, crafts and spawn rewards
// report to it; bookkeeping failures there never fail the action itself.
func (s *GachaService) UseSeason(sv *season.Service) { s.season = sv }

func NewGachaService(repo *repository.PostgresRepo, rdb *redis.Client, adminID int64, cooldown time.Duration, duplicatesEnabled, craftEnabled bool) *GachaService {
	loc := time.FixedZone("MSK", 3*60*60)
	s := &GachaService{
		repo:              repo,
		rdb:               rdb,
		loc:               loc,
		adminID:           adminID,
		cooldownHours:     cooldown,
		duplicatesEnabled: duplicatesEnabled,
		craftEnabled:      craftEnabled,
	}
	// Default on: that is how the bot has always behaved, so an upgrade changes
	// nothing until the switch is actually thrown.
	s.adminBypass.Store(true)
	raw, err := repo.GetSetting(adminCooldownBypassKey)
	if err != nil {
		log.Printf("[GACHA] не удалось прочитать %s: %v (оставляю обход включённым)", adminCooldownBypassKey, err)
	} else if len(raw) > 0 {
		var on bool
		if json.Unmarshal(raw, &on) == nil {
			s.adminBypass.Store(on)
		}
	}
	return s
}

// AdminCooldownBypass reports whether the owner currently rolls without waiting.
func (s *GachaService) AdminCooldownBypass() bool {
	return s.adminBypass.Load()
}

// SetAdminCooldownBypass persists the switch and applies it immediately — no
// restart, so a test can be run and the owner put back on equal footing.
func (s *GachaService) SetAdminCooldownBypass(on bool) error {
	raw, err := json.Marshal(on)
	if err != nil {
		return err
	}
	if err := s.repo.SetSetting(adminCooldownBypassKey, raw); err != nil {
		return err
	}
	s.adminBypass.Store(on)
	return nil
}

// DuplicatesEnabled reports whether new duplicate cards can drop.
func (s *GachaService) DuplicatesEnabled() bool {
	return s.duplicatesEnabled
}

// CraftEnabled reports whether crafting is available (independent of duplicate drops).
func (s *GachaService) CraftEnabled() bool {
	return s.craftEnabled
}

// ShowDuplicates reports whether duplicate-related UI (owned counts, craft fuel)
// should be shown: either duplicates drop, or crafting (which consumes existing
// duplicates) is on. Used by the delivery layer to pick caption variants.
func (s *GachaService) ShowDuplicates() bool {
	return s.duplicatesEnabled || s.craftEnabled
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
