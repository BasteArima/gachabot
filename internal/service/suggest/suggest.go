package suggest

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gachabot/internal/repository"

	"github.com/redis/go-redis/v9"
)

var ErrInsufficientFunds = errors.New("insufficient_funds")

type SuggestService struct {
	repo *repository.PostgresRepo
	rdb  *redis.Client
}

func NewSuggestService(repo *repository.PostgresRepo, rdb *redis.Client) *SuggestService {
	return &SuggestService{
		repo: repo,
		rdb:  rdb,
	}
}

func (s *SuggestService) SetSuggestState(internalUserID int64, state bool) {
	rCtx := context.Background()
	key := fmt.Sprintf("suggest_state:%d", internalUserID)
	if state {
		s.rdb.Set(rCtx, key, "1", 10*time.Minute)
	} else {
		s.rdb.Del(rCtx, key)
	}
}

func (s *SuggestService) IsSuggesting(internalUserID int64) bool {
	rCtx := context.Background()
	key := fmt.Sprintf("suggest_state:%d", internalUserID)
	val, err := s.rdb.Get(rCtx, key).Result()
	if err != nil {
		return false
	}
	return val == "1"
}

func (s *SuggestService) SubmitSuggestion(internalUserID int64) error {
	userDb, err := s.repo.GetUserByID(internalUserID)
	if err != nil {
		return err
	}

	if userDb.Balance < 1000 {
		return ErrInsufficientFunds
	}

	userDb.Balance -= 1000
	return s.repo.UpdateUserAfterRoll(userDb)
}
