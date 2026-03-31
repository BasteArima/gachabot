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

	CraftCost int

	StreakDays    int
	StreakUpdated bool
}

// RollCard делает всю "грязную работу", принимая только ВНУТРЕННИЙ ID
func (s *GachaService) RollCard(internalUserID int64) (*RollResult, error) {
	userDb, err := s.repo.GetUserByID(internalUserID)
	if err != nil {
		return nil, fmt.Errorf("ошибка получения юзера: %w", err)
	}

	// 1. СЧИТАЕМ СТРИК ДО ПРОВЕРКИ КУЛДАУНА
	loc := time.FixedZone("MSK", 3*60*60)
	now := time.Now().In(loc)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)

	newStreak := userDb.StreakDays
	streakUpdated := false

	if userDb.LastStreakDate.Valid {
		dbDate := userDb.LastStreakDate.Time
		lastDate := time.Date(dbDate.Year(), dbDate.Month(), dbDate.Day(), 0, 0, 0, 0, loc)
		hoursDiff := today.Sub(lastDate).Hours()

		if hoursDiff == 24 { // Если разница ровно 1 сутки
			newStreak++
			streakUpdated = true
		} else if hoursDiff > 24 { // Если прошло больше 1 суток
			newStreak = 1
			streakUpdated = true
		}
	} else {
		newStreak = 1
		streakUpdated = true
	}

	// Если стрик обновился, сразу фиксируем его
	if streakUpdated {
		userDb.StreakDays = newStreak
		userDb.LastStreakDate = sql.NullTime{Time: today, Valid: true}
	}

	// 2. Проверка кулдауна и использования Premium крутки
	usedPremium := false

	if userDb.PremiumRolls > 0 {
		usedPremium = true
	} else if userDb.LastRollTime.Valid {
		timePassed := time.Since(userDb.LastRollTime.Time)
		cooldown := 3 * time.Hour

		// Проверка на админа для сброса КД
		bypassCooldown := false
		if userDb.TelegramID.Valid && userDb.TelegramID.Int64 == adminId {
			bypassCooldown = true
		}

		if timePassed < cooldown && !bypassCooldown {
			// !!! МЫ В ОТКАТЕ !!!
			// Но если стрик сегодня обновился, обязательно сохраняем его в базу!
			if streakUpdated {
				_ = s.repo.UpdateUserAfterRoll(userDb)
			}

			timeLeft := cooldown - timePassed
			hours := int(timeLeft.Hours())
			minutes := int(timeLeft.Minutes()) % 60

			var timeStr string
			if hours > 0 {
				timeStr = fmt.Sprintf("%dч %dм", hours, minutes)
			} else {
				timeStr = fmt.Sprintf("%dм", minutes)
			}
			return &RollResult{
				OnCooldown:       true,
				CooldownTimeLeft: timeStr,
				StreakDays:       newStreak,
				StreakUpdated:    streakUpdated,
			}, nil
		}
	}

	// 3. Крутим рандом (Гача)
	rarities, err := s.repo.GetRarities()
	if err != nil {
		return nil, fmt.Errorf("ошибка получения редкостей: %w", err)
	}

	// Получаем текущие счетчики юзера
	userPities, err := s.repo.GetUserPities(internalUserID)
	if err != nil {
		return nil, fmt.Errorf("ошибка получения гарантов: %w", err)
	}

	var selectedRarityID, selectedRarityReward int
	var rarityName string
	var isPityHit bool

	// 3.1 ПРОВЕРЯЕМ ГАРАНТЫ
	for i := len(rarities) - 1; i >= 0; i-- {
		rarity := rarities[i]
		if rarity.PityThreshold > 0 {
			currentCount := userPities[rarity.ID]
			if currentCount >= rarity.PityThreshold-1 {
				selectedRarityID = rarity.ID
				rarityName = rarity.Name
				selectedRarityReward = rarity.BaseReward
				isPityHit = true
				break // Нашли сработавший гарант, прерываем цикл!
			}
		}
	}

	// 3.2 ЕСЛИ ГАРАНТ НЕ СРАБОТАЛ - крутим обычную рулетку
	if !isPityHit {
		target := rand.Float64() * 100
		var currentSum float64
		for _, v := range rarities {
			currentSum += v.DropChance
			if target <= currentSum {
				selectedRarityID = v.ID
				rarityName = v.Name
				selectedRarityReward = v.BaseReward
				break
			}
		}
	}

	// 3.3 ОБНОВЛЯЕМ СЧЕТЧИК ГАРАНТОВ ПОСЛЕ ВЫПАДЕНИЯ
	for _, rarity := range rarities {
		if rarity.PityThreshold > 0 {
			if selectedRarityID >= rarity.ID {
				userPities[rarity.ID] = 0
			} else {
				userPities[rarity.ID]++
			}
		}
	}

	if err := s.repo.UpdateUserPities(internalUserID, userPities); err != nil {
		return nil, fmt.Errorf("ошибка сохранения гарантов: %w", err)
	}

	// 4. Достаем саму карту
	card, err := s.repo.GetRandomCard(selectedRarityID)
	if err != nil {
		return nil, fmt.Errorf("ошибка получения карты: %w", err)
	}

	// 5. Считаем награду
	bonus := int(math.Sqrt(float64(newStreak)) + 5)
	finalReward := selectedRarityReward + bonus

	// 6. Формируем базовый ответ
	result := &RollResult{
		Card:          card,
		RarityName:    rarityName,
		Reward:        finalReward,
		StreakDays:    newStreak,
		StreakUpdated: streakUpdated,
	}

	// 7. Логика выдачи (Осколки или Карта)
	if card.RarityID == 6 {
		result.IsFragment = true
		currentFragments, err := s.repo.AddFragment(internalUserID, card.ID)
		if err != nil {
			return nil, fmt.Errorf("ошибка добавления осколка: %w", err)
		}
		result.FragmentsCount = currentFragments

		if currentFragments >= 10 {
			result.CardAssembled = true
			_ = s.repo.ClearFragments(internalUserID, card.ID)
			_ = s.repo.AddCardToInventory(internalUserID, card.ID)
		}
	} else {
		_ = s.repo.AddCardToInventory(internalUserID, card.ID)
	}

	// 8. Сохраняем прогресс юзера
	userDb.Balance += finalReward
	userDb.StreakDays = newStreak

	if usedPremium {
		userDb.PremiumRolls--
	} else {
		userDb.LastRollTime = sql.NullTime{Time: now, Valid: true}
	}

	userDb.LastStreakDate = sql.NullTime{Time: today, Valid: true}

	if err := s.repo.UpdateUserAfterRoll(userDb); err != nil {
		return nil, fmt.Errorf("ошибка обновления юзера: %w", err)
	}

	return result, nil
}

func (s *GachaService) GetUserProfile(internalUserID int64) (*models.UserProfile, error) {
	user, err := s.repo.GetUserByID(internalUserID)
	if err != nil {
		if err == sql.ErrNoRows {
			return &models.UserProfile{}, nil
		}
		return nil, fmt.Errorf("ошибка получения юзера: %w", err)
	}

	totalCards, err := s.repo.GetTotalCardsCount()
	if err != nil {
		return nil, fmt.Errorf("ошибка подсчета всех карт: %w", err)
	}

	uniqueCards, err := s.repo.GetUserUniqueCardsCount(internalUserID)
	if err != nil {
		return nil, fmt.Errorf("ошибка подсчета инвентаря: %w", err)
	}

	duplicates, _ := s.repo.GetTotalDuplicatesCount(internalUserID)

	return &models.UserProfile{
		Balance:          user.Balance,
		StreakDays:       user.StreakDays,
		UniqueCardsCount: uniqueCards,
		TotalCardsCount:  totalCards,
		DuplicatesCount:  duplicates,
		PremiumRolls:     user.PremiumRolls,
	}, nil
}

func (s *GachaService) GetUserCardPagination(internalUserID int64, offset int) (*models.UserCardView, int, error) {
	total, err := s.repo.GetUserUniqueCardsCount(internalUserID)
	if err != nil || total == 0 {
		return nil, 0, err // Карт нет
	}

	if offset >= total {
		offset = total - 1
	}
	if offset < 0 {
		offset = 0
	}

	card, err := s.repo.GetUserCard(internalUserID, offset)
	if err != nil {
		return nil, 0, err
	}

	return card, total, nil
}

func (s *GachaService) TrackChat(internalUserID int64, chatID int64) {
	_ = s.repo.TrackUserChat(internalUserID, chatID)
}

func (s *GachaService) GetLeaderboard(criteria string, chatID int64) ([]models.LeaderboardEntry, error) {
	return s.repo.GetLeaderboard(criteria, chatID)
}

func (s *GachaService) CraftCard(internalUserID int64) (*RollResult, error) {
	dupCounts, err := s.repo.GetAvailableCrafts(internalUserID)
	if err != nil {
		return nil, err
	}

	craftCosts := map[int]int{
		1: 5,  // Обычная -> Необычная
		2: 5,  // Необычная -> Редкая
		3: 5,  // Редкая -> Эпическая
		4: 5,  // Эпическая -> Легендарная
		5: 15, // Легендарная -> Мифическая
	}

	var sourceRarityID int
	var cost int

	for rID := 1; rID <= 5; rID++ {
		required := craftCosts[rID]
		if count, exists := dupCounts[rID]; exists && count >= required {
			sourceRarityID = rID
			cost = required
			break
		}
	}

	if sourceRarityID == 0 {
		return nil, fmt.Errorf("не хватает дубликатов. Нужно 5 обычных или 15 легендарных карт.")
	}

	targetRarityID := sourceRarityID + 1

	if err := s.repo.ConsumeDuplicates(internalUserID, sourceRarityID, cost); err != nil {
		return nil, err
	}

	rarities, _ := s.repo.GetRarities()
	var rarityName string
	for _, r := range rarities {
		if r.ID == targetRarityID {
			rarityName = r.Name
		}
	}

	card, err := s.repo.GetRandomCard(targetRarityID)
	if err != nil {
		return nil, err
	}

	result := &RollResult{
		Card:       card,
		RarityName: rarityName,
		CraftCost:  cost,
	}

	if card.RarityID == 6 {
		result.IsFragment = true
		currentFragments, _ := s.repo.AddFragment(internalUserID, card.ID)
		result.FragmentsCount = currentFragments
		if currentFragments >= 10 {
			result.CardAssembled = true
			_ = s.repo.ClearFragments(internalUserID, card.ID)
			_ = s.repo.AddCardToInventory(internalUserID, card.ID)
		}
	} else {
		_ = s.repo.AddCardToInventory(internalUserID, card.ID)
	}

	return result, nil
}
