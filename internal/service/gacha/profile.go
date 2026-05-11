package gacha

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"gachabot/internal/models"
)

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
	completedSets, totalSets, _ := s.repo.GetUserSetsStats(internalUserID)

	return &models.UserProfile{
		Balance:          user.Balance,
		StreakDays:       user.StreakDays,
		UniqueCardsCount: uniqueCards,
		TotalCardsCount:  totalCards,
		DuplicatesCount:  duplicates,
		PremiumRolls:     user.PremiumRolls,
		CompletedSets:    completedSets,
		TotalSets:        totalSets,
	}, nil
}

func (s *GachaService) GetUserCardPagination(internalUserID int64, offset int) (*models.UserCardView, int, error) {
	total, err := s.repo.GetUserUniqueCardsCount(internalUserID)
	if err != nil || total == 0 {
		return nil, 0, err
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
	ctx := context.Background()

	cacheKey := fmt.Sprintf("top:%s:%d", criteria, chatID)

	cachedData, err := s.rdb.Get(ctx, cacheKey).Result()
	if err == nil {
		var board []models.LeaderboardEntry
		if json.Unmarshal([]byte(cachedData), &board) == nil {
			return board, nil
		}
	}

	board, err := s.repo.GetLeaderboard(criteria, chatID)
	if err != nil {
		return nil, err
	}

	if jsonData, err := json.Marshal(board); err == nil {
		s.rdb.Set(ctx, cacheKey, jsonData, 1*time.Minute)
	}

	return board, nil
}
