package gacha

import (
	"encoding/json"
	"fmt"
	"gachabot/internal/models"
	"sort"
	"time"
)

func (s *GachaService) RedeemPromo(userID int64, code string) (*models.PromoReward, []models.Card, error) {
	reward, cards, err := s.repo.RedeemPromo(userID, code)
	if err != nil {
		return nil, nil, err
	}

	sort.Slice(cards, func(i, j int) bool {
		if cards[i].RarityID != cards[j].RarityID {
			return cards[i].RarityID > cards[j].RarityID
		}
		return cards[i].PowerLevel > cards[j].PowerLevel
	})

	// Collections from promos
	for _, card := range cards {
		if card.SetID != nil {
			setID := *card.SetID
			isComplete, _ := s.repo.CheckSetCompletion(userID, setID)
			if isComplete {
				claimed, _ := s.repo.IsSetRewardClaimed(userID, setID)
				if !claimed {
					setInfo, _ := s.repo.GetCardSetByID(setID)
					if setInfo != nil {
						_ = s.repo.MarkSetCompleted(userID, setID)

						// Calculating the reward
						userDb, _ := s.repo.GetUserByID(userID)
						userDb.Balance += setInfo.RewardPoints
						_ = s.repo.UpdateUserAfterRoll(userDb)

						// Add points for the set to the overall visual response of the promo code
						if reward != nil {
							reward.Points += setInfo.RewardPoints
							reward.CompletedSets = append(reward.CompletedSets, models.CompletedSetInfo{
								Name:   setInfo.Name,
								Reward: setInfo.RewardPoints,
							})
						}
					}
				}
			}
		}
	}

	return reward, cards, nil
}

// Creating a complex promo code (from JSON)
func (s *GachaService) CreatePromoFromJson(code string, maxUses int, hoursValid int, jsonPayload string) error {
	var reward models.PromoReward
	if err := json.Unmarshal([]byte(jsonPayload), &reward); err != nil {
		return fmt.Errorf("ошибка парсинга JSON: %w", err)
	}

	var usesPtr *int
	if maxUses > 0 {
		usesPtr = &maxUses
	}

	var expiresAt *time.Time
	if hoursValid > 0 {
		t := time.Now().In(s.loc).Add(time.Duration(hoursValid) * time.Hour)
		expiresAt = &t
	}

	return s.repo.CreatePromoCode(code, reward, usesPtr, expiresAt)
}

// Create a simple promo code (points and spins only)
func (s *GachaService) CreateSimplePromo(code string, points int, rolls int, maxUses int) error {
	reward := models.PromoReward{
		Points:       points,
		PremiumRolls: rolls,
	}

	var usesPtr *int
	if maxUses > 0 {
		usesPtr = &maxUses
	}

	// Pass nil as the expiration time (indefinite)
	return s.repo.CreatePromoCode(code, reward, usesPtr, nil)
}
