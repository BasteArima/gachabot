package discord

import (
	"context"
	"fmt"
	"time"
)

// Вспомогательные методы для работы с Redis (Discord)
func (b *Bot) setSuggestState(internalUserID int64, state bool) {
	rCtx := context.Background()
	key := fmt.Sprintf("suggest_state:%d", internalUserID)
	if state {
		b.rdb.Set(rCtx, key, "1", 10*time.Minute)
	} else {
		b.rdb.Del(rCtx, key)
	}
}

func (b *Bot) isSuggesting(internalUserID int64) bool {
	rCtx := context.Background()
	key := fmt.Sprintf("suggest_state:%d", internalUserID)
	val, err := b.rdb.Get(rCtx, key).Result()
	if err != nil {
		return false
	}
	return val == "1"
}
