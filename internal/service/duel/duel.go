package duel

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"time"

	"gachabot/internal/models"
	"gachabot/internal/repository"

	"github.com/redis/go-redis/v9"
)

type DuelService struct {
	repo *repository.PostgresRepo
	rdb  *redis.Client
}

func NewDuelService(repo *repository.PostgresRepo, rdb *redis.Client) *DuelService {
	return &DuelService{
		repo: repo,
		rdb:  rdb,
	}
}

func (s *DuelService) CreateDuel(duelID string, challengerID int64, challengerName string, targetID int64, targetName string, amount int, isFair bool) {
	req := &models.DuelRequest{
		ID:             duelID,
		ChallengerID:   challengerID,
		ChallengerName: challengerName,
		TargetID:       targetID,
		TargetName:     targetName,
		Amount:         amount,
		IsFair:         isFair,
	}

	data, err := json.Marshal(req)
	if err != nil {
		fmt.Println("[DUEL] Failed to marshal duel request:", err)
		return
	}

	ctx := context.Background()
	s.rdb.Set(ctx, "duel:"+duelID, data, 5*time.Minute)
}

func (s *DuelService) PopDuel(duelID string) (*models.DuelRequest, bool) {
	ctx := context.Background()
	key := "duel:" + duelID

	val, err := s.rdb.Get(ctx, key).Result()
	if err != nil {
		return nil, false
	}

	s.rdb.Del(ctx, key)

	var duel models.DuelRequest
	if err := json.Unmarshal([]byte(val), &duel); err != nil {
		return nil, false
	}

	return &duel, true
}

func (s *DuelService) GetDuel(duelID string) (*models.DuelRequest, bool) {
	ctx := context.Background()
	key := "duel:" + duelID

	val, err := s.rdb.Get(ctx, key).Result()
	if err != nil {
		return nil, false
	}

	var duel models.DuelRequest
	if err := json.Unmarshal([]byte(val), &duel); err != nil {
		return nil, false
	}

	return &duel, true
}

func (s *DuelService) ExecuteDuel(duel *models.DuelRequest) (*models.DuelResult, error) {
	userA, errA := s.repo.GetUserByID(duel.ChallengerID)
	userB, errB := s.repo.GetUserByID(duel.TargetID)
	if errA != nil || errB != nil {
		return nil, fmt.Errorf("failed to access player data")
	}
	if userA.Balance < duel.Amount || userB.Balance < duel.Amount {
		return nil, fmt.Errorf("insufficient balance")
	}

	cardA, errA := s.repo.GetRandomUserCard(duel.ChallengerID)
	cardB, errB := s.repo.GetRandomUserCard(duel.TargetID)
	if errA != nil || errB != nil {
		return nil, fmt.Errorf("one of the fighters has no cards")
	}

	result := &models.DuelResult{
		CardChallenger: cardA,
		CardTarget:     cardB,
		AmountWon:      duel.Amount,
		IsFair:         duel.IsFair,
	}

	// Use of auras
	if !duel.IsFair {
		// Attacker's Aura
		if auraC, _ := s.repo.GetActiveUserAura(duel.ChallengerID); auraC != nil {
			result.ChallengerAura = &models.DuelAuraInfo{
				Name:      auraC.Name,
				BuffType:  auraC.BuffType,
				BuffValue: auraC.BuffValue,
			}
			if auraC.BuffType == "power_percent" {
				cardA.PowerLevel += int(float64(cardA.PowerLevel) * (float64(auraC.BuffValue) / 100.0))
			}
		}

		// Defender's Aura
		if auraT, _ := s.repo.GetActiveUserAura(duel.TargetID); auraT != nil {
			result.TargetAura = &models.DuelAuraInfo{
				Name:      auraT.Name,
				BuffType:  auraT.BuffType,
				BuffValue: auraT.BuffValue,
			}
			if auraT.BuffType == "power_percent" {
				cardB.PowerLevel += int(float64(cardB.PowerLevel) * (float64(auraT.BuffValue) / 100.0))
			}
		}
	}

	result.CardChallenger = cardA
	result.CardTarget = cardB

	totalPower := float64(cardA.PowerLevel + cardB.PowerLevel)
	chanceA := (float64(cardA.PowerLevel) / totalPower) * 100
	chanceB := 100.0 - chanceA

	roll := rand.Float64() * 100
	result.ChanceChallenger = chanceA
	result.ChanceTarget = chanceB
	result.Roll = roll

	if roll <= chanceA {
		result.WinnerID = duel.ChallengerID
		result.WinnerName = duel.ChallengerName
		userA.Balance += duel.Amount
		userB.Balance -= duel.Amount
	} else {
		result.WinnerID = duel.TargetID
		result.WinnerName = duel.TargetName
		userA.Balance -= duel.Amount
		userB.Balance += duel.Amount
	}

	_ = s.repo.UpdateUserAfterRoll(userA)
	_ = s.repo.UpdateUserAfterRoll(userB)

	return result, nil
}
