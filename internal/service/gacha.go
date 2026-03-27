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

const adminId int64 = 1

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
}

// RollCard делает всю "грязную работу"
func (s *GachaService) RollCard(userID int64, username, firstName, lastName string) (*RollResult, error) {
	// 1. Создаем или находим юзера
	if err := s.repo.CreateUserIfNotExist(userID, username, firstName, lastName); err != nil {
		return nil, fmt.Errorf("ошибка регистрации: %w", err)
	}

	userDb, err := s.repo.GetUser(userID)
	if err != nil {
		return nil, fmt.Errorf("ошибка получения юзера: %w", err)
	}

	// 2. Проверка кулдауна и использования Premium крутки
	usedPremium := false

	if userDb.PremiumRolls > 0 {
		usedPremium = true
	} else if userDb.LastRollTime.Valid {
		// Обычная логика кулдауна, если нет премиум-круток
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
			return &RollResult{OnCooldown: true, CooldownTimeLeft: timeStr}, nil
		}
	}

	// 3. Считаем стрик (ЖЕСТКАЯ ПРИВЯЗКА К МОСКВЕ)
	// Создаем зону UTC+3 (3 часа * 60 минут * 60 секунд)
	loc := time.FixedZone("MSK", 3*60*60)
	now := time.Now().In(loc) // Текущее время по Москве

	// Начало "сегодняшнего" дня по Москве (00:00:00)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	newStreak := userDb.StreakDays

	if userDb.LastStreakDate.Valid {
		dbDate := userDb.LastStreakDate.Time

		// Собираем вчерашнюю дату из БД в том же самом московском поясе
		lastDate := time.Date(dbDate.Year(), dbDate.Month(), dbDate.Day(), 0, 0, 0, 0, loc)

		// Считаем разницу
		hoursDiff := today.Sub(lastDate).Hours()

		if hoursDiff == 24 { // Если разница ровно 1 сутки
			newStreak++
		} else if hoursDiff > 24 { // Если прошло больше 1 суток
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

	// Получаем текущие счетчики юзера
	userPities, err := s.repo.GetUserPities(userID)
	if err != nil {
		return nil, fmt.Errorf("ошибка получения гарантов: %w", err)
	}

	var selectedRarityID, selectedRarityReward int
	var rarityName string
	var isPityHit bool

	// 4.1 ПРОВЕРЯЕМ ГАРАНТЫ (Идем с конца, от самых крутых к обычным)
	for i := len(rarities) - 1; i >= 0; i-- {
		rarity := rarities[i]
		if rarity.PityThreshold > 0 {
			currentCount := userPities[rarity.ID]
			// Если счетчик равен порог минус 1 (т.к. эта крутка станет решающей)
			if currentCount >= rarity.PityThreshold-1 {
				selectedRarityID = rarity.ID
				rarityName = rarity.Name
				selectedRarityReward = rarity.BaseReward
				isPityHit = true
				break // Нашли сработавший гарант, прерываем цикл!
			}
		}
	}

	// 4.2 ЕСЛИ ГАРАНТ НЕ СРАБОТАЛ - крутим обычную рулетку
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

	// 4.3 ОБНОВЛЯЕМ СЧЕТЧИК ГАРАНТОВ ПОСЛЕ ВЫПАДЕНИЯ
	for _, rarity := range rarities {
		if rarity.PityThreshold > 0 {
			// Если выпавшая редкость круче или равна текущей — сбрасываем её гарант
			if selectedRarityID >= rarity.ID {
				userPities[rarity.ID] = 0
			} else {
				// Иначе (выпал мусор) — увеличиваем счетчик на 1
				userPities[rarity.ID]++
			}
		}
	}

	// Сохраняем обновленные счетчики в БД
	if err := s.repo.UpdateUserPities(userID, userPities); err != nil {
		return nil, fmt.Errorf("ошибка сохранения гарантов: %w", err)
	}

	// 5. Достаем саму карту (как и было)
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

	if usedPremium {
		userDb.PremiumRolls--
		// Если использовали премиум, мы НЕ перезаписываем LastRollTime,
		// чтобы не сбить таймер бесплатной крутки!
	} else {
		userDb.LastRollTime = sql.NullTime{Time: now, Valid: true}
	}

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

	duplicates, _ := s.repo.GetTotalDuplicatesCount(userID)

	return &models.UserProfile{
		Balance:          user.Balance,
		StreakDays:       user.StreakDays,
		UniqueCardsCount: uniqueCards,
		TotalCardsCount:  totalCards,
		DuplicatesCount:  duplicates,
		PremiumRolls:     user.PremiumRolls,
	}, nil
}

// Возвращает карточку по индексу и общее количество уникальных карт
func (s *GachaService) GetUserCardPagination(userID int64, offset int) (*models.UserCardView, int, error) {
	total, err := s.repo.GetUserUniqueCardsCount(userID)
	if err != nil || total == 0 {
		return nil, 0, err // Карт нет
	}

	// Защита от выхода за пределы
	if offset >= total {
		offset = total - 1
	}
	if offset < 0 {
		offset = 0
	}

	card, err := s.repo.GetUserCard(userID, offset)
	if err != nil {
		return nil, 0, err
	}

	return card, total, nil
}

func (s *GachaService) TrackChat(userID int64, chatID int64) {
	_ = s.repo.TrackUserChat(userID, chatID)
}

func (s *GachaService) GetLeaderboard(criteria string, chatID int64) ([]models.LeaderboardEntry, error) {
	return s.repo.GetLeaderboard(criteria, chatID)
}

func (s *GachaService) CraftCard(userID int64) (*RollResult, error) {
	// 1. Получаем все дубликаты юзера
	dupCounts, err := s.repo.GetAvailableCrafts(userID)
	if err != nil {
		return nil, err
	}

	// 2. ПРАЙС-ЛИСТ КРАФТА
	// Ключ: ID исходной редкости, Значение: сколько дубликатов нужно сжечь
	craftCosts := map[int]int{
		1: 5,  // Обычная (1) -> Необычная (2)
		2: 5,  // Необычная (2) -> Редкая (3)
		3: 5,  // Редкая (3) -> Эпическая (4)
		4: 5,  // Эпическая (4) -> Легендарная (5)
		5: 15, // Легендарная (5) -> Мифическая (6) (ОСКОЛОК) - КАСТОМНАЯ ЦЕНА
	}

	var sourceRarityID int
	var cost int

	// Ищем, на что хватает дубликатов. Идем от самых дешевых (1) к дорогим (5).
	// Так игрок сначала будет избавляться от мусора.
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

	// 3. Сжигаем нужное количество дубликатов
	if err := s.repo.ConsumeDuplicates(userID, sourceRarityID, cost); err != nil {
		return nil, err
	}

	// 4. Генерируем награду
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

	// 5. Выдаем карту (или осколки)
	result := &RollResult{
		Card:       card,
		RarityName: rarityName,
		CraftCost:  cost, // Записываем, сколько потратили
	}

	if card.RarityID == 6 {
		result.IsFragment = true
		currentFragments, _ := s.repo.AddFragment(userID, card.ID)
		result.FragmentsCount = currentFragments
		if currentFragments >= 10 {
			result.CardAssembled = true
			_ = s.repo.ClearFragments(userID, card.ID)
			_ = s.repo.AddCardToInventory(userID, card.ID)
		}
	} else {
		_ = s.repo.AddCardToInventory(userID, card.ID)
	}

	return result, nil
}
