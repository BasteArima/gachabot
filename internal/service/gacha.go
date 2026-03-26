package service

import (
	"database/sql"
	"fmt"
	"math"
	"math/rand/v2"
	"time"

	"gachabot/internal/models"
	"gachabot/internal/repository"
)

const adminId int64 = 348389728

type GachaService struct {
	repo *repository.PostgresRepo
}

func NewGachaService(repo *repository.PostgresRepo) *GachaService {
	return &GachaService{repo: repo}
}

// RollResult — структура для передачи ответа из сервиса в хэндлер
type RollResult struct {
	OnCooldown       bool
	CooldownTimeLeft string

	Card       *models.Card
	RarityName string
	Reward     int

	IsFragment     bool
	FragmentsCount int
	CardAssembled  bool
}

// RollCard делает всю "грязную работу"
func (s *GachaService) RollCard(userID int64, username string) (*RollResult, error) {
	// 1. Создаем или находим юзера
	if err := s.repo.CreateUserIfNotExist(userID, username); err != nil {
		return nil, fmt.Errorf("ошибка регистрации: %w", err)
	}

	userDb, err := s.repo.GetUser(userID)
	if err != nil {
		return nil, fmt.Errorf("ошибка получения юзера: %w", err)
	}

	// 2. Проверяем кулдаун
	if userDb.LastRollTime.Valid {
		timePassed := time.Since(userDb.LastRollTime.Time)
		cooldown := 3 * time.Hour

		if timePassed < cooldown && userID != adminId {
			timeLeft := cooldown - timePassed
			hours := int(timeLeft.Hours())
			minutes := int(timeLeft.Minutes()) % 60

			var timeStr string
			if hours > 0 {
				timeStr = fmt.Sprintf("%dч %dм", hours, minutes)
			} else {
				timeStr = fmt.Sprintf("%dм", minutes)
			}
			// Возвращаем результат со статусом кулдауна (без ошибки!)
			return &RollResult{OnCooldown: true, CooldownTimeLeft: timeStr}, nil
		}
	}

	// 3. Считаем стрик
	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	newStreak := userDb.StreakDays

	if userDb.LastStreakDate.Valid {
		lastDate := userDb.LastStreakDate.Time
		if today.Sub(lastDate) == 24*time.Hour {
			newStreak++
		} else if today.Sub(lastDate) > 24*time.Hour {
			newStreak = 1
		}
	} else {
		newStreak = 1
	}

	// 4. Крутим рандом (Гача)
	rarities, err := s.repo.GetRarities()
	if err != nil {
		return nil, fmt.Errorf("ошибка получения редкостей: %w", err)
	}

	target := rand.Float64() * 100
	var currentSum float64
	var selectedRarityID, selectedRarityReward int
	var rarityName string

	for _, v := range rarities {
		currentSum += v.DropChance
		if target <= currentSum {
			selectedRarityID = v.ID
			rarityName = v.Name
			selectedRarityReward = v.BaseReward
			break
		}
	}

	card, err := s.repo.GetRandomCard(selectedRarityID)
	if err != nil {
		return nil, fmt.Errorf("ошибка получения карты: %w", err)
	}

	// 5. Считаем награду
	bonus := int(math.Sqrt(float64(newStreak)) + 5)
	finalReward := selectedRarityReward + bonus

	// 6. Формируем базовый ответ
	result := &RollResult{
		Card:       card,
		RarityName: rarityName,
		Reward:     finalReward,
	}

	// 7. Логика выдачи (Осколки или Карта)
	if card.RarityID == 6 {
		result.IsFragment = true
		currentFragments, err := s.repo.AddFragment(userID, card.ID)
		if err != nil {
			return nil, fmt.Errorf("ошибка добавления осколка: %w", err)
		}
		result.FragmentsCount = currentFragments

		if currentFragments >= 10 {
			result.CardAssembled = true
			_ = s.repo.ClearFragments(userID, card.ID)
			_ = s.repo.AddCardToInventory(userID, card.ID)
		}
	} else {
		_ = s.repo.AddCardToInventory(userID, card.ID)
	}

	// 8. Сохраняем прогресс юзера
	userDb.Balance += finalReward
	userDb.StreakDays = newStreak
	userDb.LastRollTime = sql.NullTime{Time: time.Now(), Valid: true}
	userDb.LastStreakDate = sql.NullTime{Time: today, Valid: true}

	if err := s.repo.UpdateUserAfterRoll(userDb); err != nil {
		return nil, fmt.Errorf("ошибка обновления юзера: %w", err)
	}

	return result, nil
}

func (s *GachaService) GetUserProfile(userID int64) (*models.UserProfile, error) {
	// 1. Берем данные юзера (баланс, стрик)
	user, err := s.repo.GetUser(userID)
	if err != nil {
		// Если юзера нет в базе (еще ни разу не играл), вернем пустой профиль
		if err == sql.ErrNoRows {
			return &models.UserProfile{}, nil
		}
		return nil, fmt.Errorf("ошибка получения юзера: %w", err)
	}

	// 2. Берем общее количество карт в игре
	totalCards, err := s.repo.GetTotalCardsCount()
	if err != nil {
		return nil, fmt.Errorf("ошибка подсчета всех карт: %w", err)
	}

	// 3. Берем количество собранных карт юзера
	uniqueCards, err := s.repo.GetUserUniqueCardsCount(userID)
	if err != nil {
		return nil, fmt.Errorf("ошибка подсчета инвентаря: %w", err)
	}

	return &models.UserProfile{
		Balance:          user.Balance,
		StreakDays:       user.StreakDays,
		UniqueCardsCount: uniqueCards,
		TotalCardsCount:  totalCards,
	}, nil
}
