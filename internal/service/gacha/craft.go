package gacha

import (
	"fmt"
	"gachabot/internal/models"
)

func (s *GachaService) CraftCard(internalUserID int64) (*models.RollResult, error) {
	dupCounts, err := s.repo.GetAvailableCrafts(internalUserID)
	if err != nil {
		return nil, err
	}

	rarities, err := s.repo.GetRarities()
	if err != nil {
		return nil, err
	}

	var sourceRarityID int
	var cost int
	var targetRarityID int
	var targetRarityName string

	for i, r := range rarities {
		// If crafting from this rarity is allowed (price > 0)
		if r.CraftCost > 0 {
			if count, exists := dupCounts[r.ID]; exists && count >= r.CraftCost {
				sourceRarityID = r.ID
				cost = r.CraftCost

				// The target rarity is the next rarity in the DB (i+1)
				if i+1 < len(rarities) {
					targetRarityID = rarities[i+1].ID
					targetRarityName = rarities[i+1].Name
				} else {
					// Fallback in case of a poorly configured database
					targetRarityID = r.ID + 1
				}
				break
			}
		}
	}

	if sourceRarityID == 0 {
		return nil, fmt.Errorf("err_not_enough_duplicates")
	}

	if err := s.repo.ConsumeDuplicates(internalUserID, sourceRarityID, cost); err != nil {
		return nil, err
	}

	card, err := s.repo.GetRandomCard(targetRarityID)
	if err != nil {
		return nil, err
	}

	if targetRarityName == "" {
		for _, r := range rarities {
			if r.ID == targetRarityID {
				targetRarityName = r.Name
			}
		}
	}

	result := &models.RollResult{
		Card:       card,
		RarityName: targetRarityName,
		CraftCost:  cost,
	}

	// Distribution logic (shards or map)
	targetRarity := findRarity(rarities, targetRarityID)
	if targetRarity != nil && targetRarity.RequiresFragments {
		result.IsFragment = true
		currentFragments, _ := s.repo.AddFragment(internalUserID, card.ID)
		result.FragmentsCount = currentFragments
		if targetRarity.FragmentsRequired > 0 && currentFragments >= targetRarity.FragmentsRequired {
			result.CardAssembled = true
			_ = s.repo.ClearFragments(internalUserID, card.ID)
			_ = s.repo.AddCardToInventory(internalUserID, card.ID)
		}
	} else {
		_ = s.repo.AddCardToInventory(internalUserID, card.ID)
	}

	// Logic of collecting collections
	if (!result.IsFragment || result.CardAssembled) && card.SetID != nil {
		setID := *card.SetID
		isComplete, _ := s.repo.CheckSetCompletion(internalUserID, setID)
		if isComplete {
			claimed, _ := s.repo.IsSetRewardClaimed(internalUserID, setID)
			if !claimed {
				setInfo, _ := s.repo.GetCardSetByID(setID)
				if setInfo != nil {
					_ = s.repo.MarkSetCompleted(internalUserID, setID)

					userDb, _ := s.repo.GetUserByID(internalUserID)
					userDb.Balance += setInfo.RewardPoints
					_ = s.repo.UpdateUserAfterRoll(userDb)

					result.CompletedSetName = setInfo.Name
					result.CompletedSetReward = setInfo.RewardPoints
				}
			}
		}
	}

	return result, nil
}
