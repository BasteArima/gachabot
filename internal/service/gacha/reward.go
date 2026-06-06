package gacha

import (
	"fmt"

	"gachabot/internal/models"
)

// ApplyDailyStreak extends (or starts) the user's daily streak if they haven't
// been credited today, persisting only the streak fields. Shared by mechanics
// other than /roll (chat spawns, and later boss/lottery) so they keep the streak
// alive the same way. Returns the current streak and whether it changed.
func (s *GachaService) ApplyDailyStreak(internalUserID int64) (int, bool, error) {
	user, err := s.repo.GetUserByID(internalUserID)
	if err != nil {
		return 0, false, fmt.Errorf("failed to get user: %w", err)
	}
	newStreak, updated := s.calculateStreak(user)
	if updated {
		if err := s.repo.UpdateUserStreak(internalUserID, newStreak, s.todayMidnight()); err != nil {
			return 0, false, fmt.Errorf("failed to update streak: %w", err)
		}
	}
	return newStreak, updated, nil
}

// GrantCardReward awards a specific card plus coins, reusing the same drop
// pipeline as /roll (fragment handling for mythics, set completion). It does NOT
// touch the streak — callers compose ApplyDailyStreak separately.
//
// With duplicates disabled, a non-fragment card the user already owns yields
// coins only (IsDuplicate=true).
func (s *GachaService) GrantCardReward(internalUserID int64, card *models.Card, coins int) (*models.SpawnReward, error) {
	rarities, err := s.repo.GetRarities()
	if err != nil {
		return nil, fmt.Errorf("failed to get rarities: %w", err)
	}
	rarity := findRarity(rarities, card.RarityID)
	if rarity == nil {
		return nil, fmt.Errorf("unknown rarity %d for card %d", card.RarityID, card.ID)
	}

	reward := &models.SpawnReward{Coins: coins}
	rollLike := &models.RollResult{Reward: coins}

	if !s.duplicatesEnabled && !rarity.RequiresFragments {
		owns, _ := s.repo.UserOwnsCard(internalUserID, card.ID)
		reward.IsDuplicate = owns
	}

	if !reward.IsDuplicate {
		if err := s.processCardDrop(internalUserID, card, *rarity, rollLike); err != nil {
			return nil, fmt.Errorf("failed to grant card: %w", err)
		}
		s.processSetCompletion(internalUserID, card, rollLike)
	}

	reward.IsFragment = rollLike.IsFragment
	reward.FragmentsCount = rollLike.FragmentsCount
	reward.CardAssembled = rollLike.CardAssembled
	reward.CompletedSetName = rollLike.CompletedSetName
	reward.CompletedSetReward = rollLike.CompletedSetReward
	reward.Coins = rollLike.Reward // includes any set-completion bonus

	if reward.Coins != 0 {
		if err := s.repo.AddBalance(internalUserID, reward.Coins); err != nil {
			return nil, fmt.Errorf("failed to add balance: %w", err)
		}
	}
	return reward, nil
}
