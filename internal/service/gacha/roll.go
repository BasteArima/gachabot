package gacha

import (
	"database/sql"
	"fmt"
	"math"
	"math/rand/v2"
	"time"

	"gachabot/internal/models"
)

func (s *GachaService) RollCard(internalUserID int64) (*models.RollResult, error) {
	userDb, err := s.repo.GetUserByID(internalUserID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	newStreak, streakUpdated := s.calculateStreak(userDb)

	if onCooldown, timeLeft := s.checkCooldown(userDb); onCooldown {
		if streakUpdated {
			userDb.StreakDays = newStreak
			userDb.LastStreakDate = sql.NullTime{Time: s.todayMidnight(), Valid: true}
			_ = s.repo.UpdateUserAfterRoll(userDb)
		}
		return &models.RollResult{
			OnCooldown:       true,
			CooldownTimeLeft: timeLeft,
			StreakDays:       newStreak,
			StreakUpdated:    streakUpdated,
		}, nil
	}

	rarities, err := s.repo.GetRarities()
	if err != nil {
		return nil, fmt.Errorf("failed to get rarities: %w", err)
	}

	userPities, err := s.repo.GetUserPities(internalUserID)
	if err != nil {
		return nil, fmt.Errorf("failed to get pities: %w", err)
	}

	selectedRarity := s.rollRarity(rarities, userPities)

	s.updatePities(userPities, rarities, selectedRarity.ID)
	if err := s.repo.UpdateUserPities(internalUserID, userPities); err != nil {
		return nil, fmt.Errorf("failed to save pities: %w", err)
	}

	card, err := s.repo.GetRandomCard(selectedRarity.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to get card: %w", err)
	}

	bonus := int(math.Sqrt(float64(newStreak)) + 5)
	finalReward := selectedRarity.BaseReward + bonus

	result := &models.RollResult{
		Card:          card,
		RarityName:    selectedRarity.Name,
		Reward:        finalReward,
		StreakDays:    newStreak,
		StreakUpdated: streakUpdated,
	}

	if err := s.processCardDrop(internalUserID, card, selectedRarity, result); err != nil {
		return nil, err
	}

	s.processSetCompletion(internalUserID, card, result)

	userDb.Balance += result.Reward
	userDb.StreakDays = newStreak
	userDb.LastStreakDate = sql.NullTime{Time: s.todayMidnight(), Valid: true}

	if userDb.PremiumRolls > 0 {
		userDb.PremiumRolls--
	} else {
		userDb.LastRollTime = sql.NullTime{Time: time.Now().In(s.loc), Valid: true}
	}

	if err := s.repo.UpdateUserAfterRoll(userDb); err != nil {
		return nil, fmt.Errorf("failed to update user: %w", err)
	}

	return result, nil
}

func (s *GachaService) todayMidnight() time.Time {
	now := time.Now().In(s.loc)
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, s.loc)
}

func (s *GachaService) calculateStreak(userDb *models.User) (int, bool) {
	today := s.todayMidnight()

	if !userDb.LastStreakDate.Valid {
		return 1, true
	}

	dbDate := userDb.LastStreakDate.Time.In(s.loc)
	lastDate := time.Date(dbDate.Year(), dbDate.Month(), dbDate.Day(), 0, 0, 0, 0, s.loc)
	hoursDiff := today.Sub(lastDate).Hours()

	if hoursDiff == 24 {
		return userDb.StreakDays + 1, true
	} else if hoursDiff > 24 {
		return 1, true
	}

	return userDb.StreakDays, false
}

func (s *GachaService) checkCooldown(userDb *models.User) (bool, string) {
	if userDb.PremiumRolls > 0 || !userDb.LastRollTime.Valid {
		return false, ""
	}

	timePassed := time.Since(userDb.LastRollTime.Time)

	bypassCooldown := userDb.TelegramID.Valid && userDb.TelegramID.Int64 == s.adminID

	if timePassed < s.cooldownHours && !bypassCooldown {
		timeLeft := s.cooldownHours - timePassed
		hours := int(timeLeft.Hours())
		minutes := int(timeLeft.Minutes()) % 60

		if hours > 0 {
			return true, fmt.Sprintf("%dh %dm", hours, minutes)
		}
		return true, fmt.Sprintf("%dm", minutes)
	}

	return false, ""
}

func (s *GachaService) rollRarity(rarities []models.Rarity, pities map[int]int) models.Rarity {
	for i := len(rarities) - 1; i >= 0; i-- {
		rarity := rarities[i]
		if rarity.PityThreshold > 0 && pities[rarity.ID] >= rarity.PityThreshold-1 {
			return rarity
		}
	}

	target := rand.Float64() * 100
	var currentSum float64
	for _, v := range rarities {
		currentSum += v.DropChance
		if target <= currentSum {
			return v
		}
	}
	return rarities[0]
}

func (s *GachaService) updatePities(pities map[int]int, rarities []models.Rarity, selectedRarityID int) {
	for _, rarity := range rarities {
		if rarity.PityThreshold > 0 {
			if selectedRarityID >= rarity.ID {
				pities[rarity.ID] = 0
			} else {
				pities[rarity.ID]++
			}
		}
	}
}

func (s *GachaService) processCardDrop(internalUserID int64, card *models.Card, rarity models.Rarity, result *models.RollResult) error {
	if rarity.RequiresFragments {
		result.IsFragment = true
		currentFragments, err := s.repo.AddFragment(internalUserID, card.ID)
		if err != nil {
			return fmt.Errorf("failed to add fragment: %w", err)
		}
		result.FragmentsCount = currentFragments

		if rarity.FragmentsRequired > 0 && currentFragments >= rarity.FragmentsRequired {
			result.CardAssembled = true
			_ = s.repo.ClearFragments(internalUserID, card.ID)
			_ = s.repo.AddCardToInventory(internalUserID, card.ID)
		}
	} else {
		_ = s.repo.AddCardToInventory(internalUserID, card.ID)
	}
	return nil
}

func (s *GachaService) processSetCompletion(userID int64, card *models.Card, result *models.RollResult) {
	if card.SetID == nil || (result != nil && result.IsFragment && !result.CardAssembled) {
		return
	}

	setID := *card.SetID
	isComplete, _ := s.repo.CheckSetCompletion(userID, setID)
	if !isComplete {
		return
	}

	claimed, _ := s.repo.IsSetRewardClaimed(userID, setID)
	if claimed {
		return
	}

	setInfo, err := s.repo.GetCardSetByID(setID)
	if err != nil || setInfo == nil {
		return
	}

	_ = s.repo.MarkSetCompleted(userID, setID)

	if result != nil {
		result.Reward += setInfo.RewardPoints
		result.CompletedSetName = setInfo.Name
		result.CompletedSetReward = setInfo.RewardPoints
	} else {
		userDb, _ := s.repo.GetUserByID(userID)
		userDb.Balance += setInfo.RewardPoints
		_ = s.repo.UpdateUserAfterRoll(userDb)
	}
}
