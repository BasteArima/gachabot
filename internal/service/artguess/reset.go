package artguess

import (
	"context"
	"fmt"
	"math/rand/v2"

	"gachabot/internal/models"
)

// ResetUser clears one user's progress for today (admin/debug helper). The card
// of the day is unchanged, so the player simply gets fresh attempts on it.
func (s *Service) ResetUser(ctx context.Context, uid int64) error {
	return s.rdb.Del(ctx, s.progressKey(uid, dateKey(s.midnight()))).Err()
}

// ResetAll clears every user's progress for today.
func (s *Service) ResetAll(ctx context.Context) error {
	return s.scanDel(ctx, keyProgressPrefix+"*:"+dateKey(s.midnight()))
}

// RerollDailyCard replaces today's card of the day with a fresh one (different
// from the current pick when the pool allows) and clears everyone's progress so
// guess history stays consistent with the new answer. Returns the new card.
func (s *Service) RerollDailyCard(ctx context.Context) (*models.Card, error) {
	cfg := s.Config()
	dateStr := dateKey(s.midnight())
	rmap, err := s.rarityMap()
	if err != nil {
		return nil, err
	}
	ids, err := s.eligibleCardIDs(cfg, rmap)
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return nil, fmt.Errorf("no eligible cards for art guess")
	}

	current, _ := s.rdb.Get(ctx, keyDailyPrefix+dateStr).Int()
	chosen := ids[rand.IntN(len(ids))]
	for len(ids) > 1 && chosen == current {
		chosen = ids[rand.IntN(len(ids))]
	}
	if err := s.rdb.Set(ctx, keyDailyPrefix+dateStr, chosen, dayTTL).Err(); err != nil {
		return nil, err
	}
	if err := s.ResetAll(ctx); err != nil {
		return nil, err
	}
	return s.repo.GetCardByID(chosen)
}

// scanDel deletes every key matching the pattern, iterating with SCAN so it
// never blocks Redis on large keyspaces.
func (s *Service) scanDel(ctx context.Context, pattern string) error {
	var cursor uint64
	for {
		keys, next, err := s.rdb.Scan(ctx, cursor, pattern, 100).Result()
		if err != nil {
			return err
		}
		if len(keys) > 0 {
			if err := s.rdb.Del(ctx, keys...).Err(); err != nil {
				return err
			}
		}
		cursor = next
		if cursor == 0 {
			return nil
		}
	}
}
