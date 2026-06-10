package artguess

import (
	"context"
	"log"
	"strconv"
	"time"

	"gachabot/internal/models"
)

// Start launches the scheduler loop: it posts the morning scoreboard and, at the
// MSK midnight rollover, the previous day's summary. No-op for the game itself
// (the card of the day is resolved lazily on request).
func (s *Service) Start() {
	go func() {
		ctx := context.Background()
		s.lastDate = dateKey(s.midnight())
		t := time.NewTicker(30 * time.Second)
		defer t.Stop()
		for range t.C {
			s.tick(ctx)
		}
	}()
	log.Println("[artguess] engine started")
}

func (s *Service) tick(ctx context.Context) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[artguess] tick panic recovered: %v", r)
		}
	}()
	cfg := s.Config()
	now := time.Now().In(s.loc)
	today := now.Format("2006-01-02")

	// Midnight crossing: settle the day that just ended.
	if s.lastDate != "" && today != s.lastDate {
		s.runSummaries(ctx, s.lastDate)
		s.lastPingDate = ""
	}
	s.lastDate = today

	if cfg.Enabled && cfg.ChatPostEnabled && cfg.MorningPing.Enabled && s.lastPingDate != today {
		if h, m, ok := parseClock(cfg.MorningPing.Time); ok {
			if now.Hour() > h || (now.Hour() == h && now.Minute() >= m) {
				s.runMorningPing(ctx, today)
				s.lastPingDate = today
			}
		}
	}
}

// runMorningPing posts (or refreshes) the scoreboard in every registered chat.
func (s *Service) runMorningPing(ctx context.Context, date string) {
	chats, err := s.repo.GetSpawnChats()
	if err != nil {
		log.Printf("[artguess] morning ping: %v", err)
		return
	}
	posted := 0
	for _, c := range chats {
		if s.broadcasters[c.Platform] == nil {
			continue
		}
		s.rdb.SAdd(ctx, keyBoardsSet+date, c.Platform+":"+strconv.FormatInt(c.ChatID, 10))
		s.rdb.Expire(ctx, keyBoardsSet+date, dayTTL)
		s.refreshBoard(ctx, c.Platform, c.ChatID, date)
		posted++
	}
	log.Printf("[artguess] morning ping to %d chats", posted)
}

// runSummaries posts the day's results (with the answer revealed) to every chat
// that had at least one participant, and advances each chat's group streak.
func (s *Service) runSummaries(ctx context.Context, date string) {
	cfg := s.Config()
	if !cfg.ChatPostEnabled {
		return
	}
	members, err := s.rdb.SMembers(ctx, keyBoardsSet+date).Result()
	if err != nil {
		return
	}
	rmap, err := s.rarityMap()
	if err != nil {
		return
	}
	var card *models.Card
	if id, err := s.rdb.Get(ctx, keyDailyPrefix+date).Int(); err == nil {
		card, _ = s.repo.GetCardByID(id)
	}
	number := s.puzzleNumberFor(date, cfg)

	for _, m := range members {
		platform, chatID, ok := parseBoardMember(m)
		if !ok {
			continue
		}
		b := s.broadcasters[platform]
		if b == nil {
			continue
		}
		parts := s.loadParticipants(ctx, platform, chatID, date)
		if len(parts) == 0 {
			continue
		}
		streak := s.bumpStreak(ctx, platform, chatID, date)
		name, rarity := "—", ""
		if card != nil {
			name, rarity = card.Name, rmap[card.RarityID].Name
		}
		text := renderSummary(number, cfg.MaxAttempts, parts, name, rarity, streak)
		if card == nil {
			continue // can't reveal art; skip rather than post a broken summary
		}
		if err := b.PostSummary(models.Chat{Platform: platform, ChatID: chatID}, text, card); err != nil {
			log.Printf("[artguess] summary failed (%s:%d): %v", platform, chatID, err)
		}
	}
}
