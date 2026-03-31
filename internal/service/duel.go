package service

import (
	"fmt"
	"math/rand/v2"
	"sync"
	"time"

	"gachabot/internal/models"
	"gachabot/internal/repository"
)

type DuelRequest struct {
	ID             string
	ChallengerID   int64
	ChallengerName string
	TargetID       int64
	TargetName     string
	Amount         int
	Timer          *time.Timer
}

type DuelService struct {
	repo  *repository.PostgresRepo
	mu    sync.RWMutex
	duels map[string]*DuelRequest
}

func NewDuelService(repo *repository.PostgresRepo) *DuelService {
	return &DuelService{
		repo:  repo,
		duels: make(map[string]*DuelRequest),
	}
}

func (s *DuelService) CreateDuel(duelID string, challengerID int64, challengerName string, targetID int64, targetName string, amount int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if oldDuel, exists := s.duels[duelID]; exists {
		oldDuel.Timer.Stop()
	}

	req := &DuelRequest{
		ID:             duelID,
		ChallengerID:   challengerID, // Теперь это ВНУТРЕННИЙ ID
		ChallengerName: challengerName,
		TargetID:       targetID, // Теперь это ВНУТРЕННИЙ ID
		TargetName:     targetName,
		Amount:         amount,
	}

	req.Timer = time.AfterFunc(5*time.Minute, func() {
		s.mu.Lock()
		delete(s.duels, duelID)
		s.mu.Unlock()
	})

	s.duels[duelID] = req
}

func (s *DuelService) PopDuel(duelID string) (*DuelRequest, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	duel, exists := s.duels[duelID]
	if exists {
		duel.Timer.Stop()
		delete(s.duels, duelID)
	}
	return duel, exists
}

func (s *DuelService) GetDuel(duelID string) (*DuelRequest, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	duel, exists := s.duels[duelID]
	return duel, exists
}

type DuelResult struct {
	WinnerID         int64
	WinnerName       string
	CardChallenger   *models.Card
	CardTarget       *models.Card
	ChanceChallenger float64
	ChanceTarget     float64
	Roll             float64
	AmountWon        int
}

func (s *DuelService) ExecuteDuel(duel *DuelRequest) (*DuelResult, error) {
	// 1. Проверяем балансы
	// ИЗМЕНЕНО: Используем GetUserByID вместо GetUser
	userA, errA := s.repo.GetUserByID(duel.ChallengerID)
	userB, errB := s.repo.GetUserByID(duel.TargetID)
	if errA != nil || errB != nil {
		return nil, fmt.Errorf("ошибка доступа к данным игроков")
	}
	if userA.Balance < duel.Amount || userB.Balance < duel.Amount {
		return nil, fmt.Errorf("недостаточно очков для боя")
	}

	// 2. Случайные карты
	// GetRandomUserCard уже работает по внутреннему user_id из таблицы user_inventory
	cardA, errA := s.repo.GetRandomUserCard(duel.ChallengerID)
	cardB, errB := s.repo.GetRandomUserCard(duel.TargetID)
	if errA != nil || errB != nil {
		return nil, fmt.Errorf("у одного из бойцов нет карт")
	}

	// 3. Расчет шансов
	totalPower := float64(cardA.PowerLevel + cardB.PowerLevel)
	chanceA := (float64(cardA.PowerLevel) / totalPower) * 100
	chanceB := 100.0 - chanceA

	roll := rand.Float64() * 100
	result := &DuelResult{
		CardChallenger:   cardA,
		CardTarget:       cardB,
		ChanceChallenger: chanceA,
		ChanceTarget:     chanceB,
		Roll:             roll,
		AmountWon:        duel.Amount,
	}

	// 4. Определение победителя и расчеты
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
