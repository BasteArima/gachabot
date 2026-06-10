// Package artguess implements the daily "guess the card by its art" game: one
// global card per day, revealed progressively as the player spends guesses, with
// directional rarity/power hints. Played privately in the web app; rewards reuse
// the gacha service (coins + daily streak). See docs/design/Art Guess.md.
package artguess

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"time"

	"gachabot/internal/models"
	"gachabot/internal/repository"
	"gachabot/internal/service/gacha"

	"github.com/redis/go-redis/v9"
)

const (
	keyDailyPrefix    = "artguess:daily:"    // +date -> card id of the day
	keyProgressPrefix = "artguess:progress:" // +uid:date -> progress JSON
	dayTTL            = 48 * time.Hour
)

// Service holds the game's dependencies. The card of the day is chosen lazily
// and deterministically per date; the scheduler (Start) only drives chat
// scoreboards and the midnight summary.
type Service struct {
	repo         *repository.PostgresRepo
	rdb          *redis.Client
	gacha        *gacha.GachaService
	loc          *time.Location
	secret       string                 // HMAC key for deep links (the bot token)
	broadcasters map[string]Broadcaster // platform -> delivery, for chat posts

	// scheduler bookkeeping (touched only by the Start goroutine)
	lastDate     string
	lastPingDate string
}

func New(repo *repository.PostgresRepo, rdb *redis.Client, gs *gacha.GachaService, secret string) *Service {
	return &Service{
		repo:         repo,
		rdb:          rdb,
		gacha:        gs,
		loc:          time.FixedZone("MSK", 3*60*60),
		secret:       secret,
		broadcasters: make(map[string]Broadcaster),
	}
}

// --- State DTOs (JSON-encoded directly by the HTTP layer) ---

type CardBrief struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Rarity string `json:"rarity"`
	Power  int    `json:"power"`
}

type GuessFeedback struct {
	CardBrief
	Correct     bool   `json:"correct"`
	RarityArrow string `json:"rarityArrow"`
	PowerArrow  string `json:"powerArrow"`
}

type AnswerCard struct {
	CardBrief
	ImageURL string `json:"imageUrl"`
}

type State struct {
	Number       int             `json:"number"`
	MaxAttempts  int             `json:"maxAttempts"`
	AttemptsUsed int             `json:"attemptsUsed"`
	Finished     bool            `json:"finished"`
	Solved       bool            `json:"solved"`
	RevealLevel  int             `json:"revealLevel"`
	MaxLevel     int             `json:"maxLevel"`
	Reward       int             `json:"reward"`
	Guesses      []GuessFeedback `json:"guesses"`
	Answer       *AnswerCard     `json:"answer"` // populated only once finished
}

// progress is the per-user, per-day state kept in Redis.
type progress struct {
	Guesses  []int `json:"guesses"`
	Solved   bool  `json:"solved"`
	Finished bool  `json:"finished"`
	Rewarded bool  `json:"rewarded"`
	Reward   int   `json:"reward"`
}

// --- Date helpers (all in MSK) ---

func (s *Service) midnight() time.Time {
	n := time.Now().In(s.loc)
	return time.Date(n.Year(), n.Month(), n.Day(), 0, 0, 0, 0, s.loc)
}

func dateKey(t time.Time) string { return t.Format("2006-01-02") }

// --- Config ---

// Config returns the stored config, falling back to defaults if unset/invalid.
func (s *Service) Config() Config {
	raw, err := s.repo.GetSetting(settingKey)
	if err != nil || len(raw) == 0 {
		return DefaultConfig()
	}
	c, err := ParseConfig(raw)
	if err != nil {
		return DefaultConfig()
	}
	return c
}

func (s *Service) rarityMap() (map[int]models.Rarity, error) {
	rs, err := s.repo.GetRarities()
	if err != nil {
		return nil, err
	}
	m := make(map[int]models.Rarity, len(rs))
	for _, r := range rs {
		m[r.ID] = r
	}
	return m, nil
}

// eligibleCardIDs returns the sorted pool of cards that can be the card of the
// day, honouring the config's curation rules.
func (s *Service) eligibleCardIDs(cfg Config, rmap map[int]models.Rarity) ([]int, error) {
	cards, err := s.repo.ListCards()
	if err != nil {
		return nil, err
	}
	allow := make(map[int]bool, len(cfg.Pool.CardIDs))
	for _, id := range cfg.Pool.CardIDs {
		allow[int(id)] = true
	}
	exclude := make(map[int]bool, len(cfg.Pool.ExcludeCardIDs))
	for _, id := range cfg.Pool.ExcludeCardIDs {
		exclude[int(id)] = true
	}

	ids := make([]int, 0, len(cards))
	for _, c := range cards {
		if c.ImageURL == "" {
			continue
		}
		if len(allow) > 0 && !allow[c.ID] {
			continue
		}
		if exclude[c.ID] {
			continue
		}
		if cfg.Pool.ExcludeMythic && rmap[c.RarityID].RequiresFragments {
			continue
		}
		ids = append(ids, c.ID)
	}
	sort.Ints(ids)
	return ids, nil
}

// dailyCardID resolves the card of the day, caching the choice in Redis so it is
// stable across the day and bot restarts. The pick is deterministic by date to
// keep concurrent first-requests in agreement before the cache is written.
func (s *Service) dailyCardID(ctx context.Context, cfg Config, dateStr string, rmap map[int]models.Rarity) (int, error) {
	key := keyDailyPrefix + dateStr
	if id, err := s.rdb.Get(ctx, key).Int(); err == nil {
		return id, nil
	}
	ids, err := s.eligibleCardIDs(cfg, rmap)
	if err != nil {
		return 0, err
	}
	if len(ids) == 0 {
		return 0, fmt.Errorf("no eligible cards for art guess")
	}
	chosen := ids[pickIndex(dateStr, len(ids))]
	ok, err := s.rdb.SetNX(ctx, key, chosen, dayTTL).Result()
	if err == nil && !ok {
		if id, err2 := s.rdb.Get(ctx, key).Int(); err2 == nil {
			return id, nil
		}
	}
	return chosen, nil
}

// --- Progress ---

func (s *Service) progressKey(uid int64, dateStr string) string {
	return keyProgressPrefix + strconv.FormatInt(uid, 10) + ":" + dateStr
}

func (s *Service) loadProgress(ctx context.Context, uid int64, dateStr string) (progress, error) {
	var p progress
	raw, err := s.rdb.Get(ctx, s.progressKey(uid, dateStr)).Bytes()
	if err == redis.Nil {
		return p, nil
	}
	if err != nil {
		return p, err
	}
	_ = json.Unmarshal(raw, &p)
	return p, nil
}

func (s *Service) saveProgress(ctx context.Context, uid int64, dateStr string, p progress) error {
	raw, err := json.Marshal(p)
	if err != nil {
		return err
	}
	return s.rdb.Set(ctx, s.progressKey(uid, dateStr), raw, dayTTL).Err()
}

// --- Public API ---

// GetState returns the player's current view of today's puzzle.
func (s *Service) GetState(ctx context.Context, uid int64) (*State, error) {
	cfg := s.Config()
	dateStr := dateKey(s.midnight())
	rmap, err := s.rarityMap()
	if err != nil {
		return nil, err
	}
	targetID, err := s.dailyCardID(ctx, cfg, dateStr, rmap)
	if err != nil {
		return nil, err
	}
	target, err := s.repo.GetCardByID(targetID)
	if err != nil {
		return nil, err
	}
	p, err := s.loadProgress(ctx, uid, dateStr)
	if err != nil {
		return nil, err
	}
	return s.buildState(cfg, p, target, rmap)
}

// Guess records a guess and returns the updated state. A guess after the game is
// finished, or a repeat of a card already guessed, is a no-op that returns the
// current state. launch is the (signed) deep-link value the player entered from,
// used to attribute play to a chat scoreboard; empty when there's no chat context.
func (s *Service) Guess(ctx context.Context, uid int64, cardID int, launch string) (*State, error) {
	cfg := s.Config()
	dateStr := dateKey(s.midnight())
	rmap, err := s.rarityMap()
	if err != nil {
		return nil, err
	}
	targetID, err := s.dailyCardID(ctx, cfg, dateStr, rmap)
	if err != nil {
		return nil, err
	}
	target, err := s.repo.GetCardByID(targetID)
	if err != nil {
		return nil, err
	}

	p, err := s.loadProgress(ctx, uid, dateStr)
	if err != nil {
		return nil, err
	}
	if p.Finished || contains(p.Guesses, cardID) {
		return s.buildState(cfg, p, target, rmap)
	}

	guessCard, err := s.repo.GetCardByID(cardID)
	if err != nil {
		return nil, fmt.Errorf("unknown card")
	}

	p.Guesses = append(p.Guesses, guessCard.ID)
	switch {
	case guessCard.ID == targetID:
		p.Solved = true
		p.Finished = true
	case len(p.Guesses) >= cfg.MaxAttempts:
		p.Finished = true
	}

	if p.Finished && !p.Rewarded {
		p.Rewarded = true
		if p.Solved {
			p.Reward = coinReward(cfg, len(p.Guesses))
			if err := s.repo.AddBalance(uid, p.Reward); err != nil {
				return nil, err
			}
			if cfg.GrantStreak {
				if _, _, err := s.gacha.ApplyDailyStreak(uid); err != nil {
					return nil, err
				}
			}
		}
	}

	if err := s.saveProgress(ctx, uid, dateStr, p); err != nil {
		return nil, err
	}

	// Attribute this play to the launch chat's scoreboard, if any.
	if launch != "" && cfg.Enabled && cfg.ChatPostEnabled {
		if platform, chatID, ok := s.resolveLaunch(ctx, launch); ok {
			s.onPlay(ctx, platform, chatID, uid, p)
		}
	}

	return s.buildState(cfg, p, target, rmap)
}

// HubStatus reports whether the game is enabled and whether the player solved
// today's puzzle (for the daily hub tile).
func (s *Service) HubStatus(ctx context.Context, uid int64) (enabled, solvedToday bool) {
	cfg := s.Config()
	if !cfg.Enabled {
		return false, false
	}
	p, err := s.loadProgress(ctx, uid, dateKey(s.midnight()))
	if err != nil {
		return true, false
	}
	return true, p.Solved
}

// buildState assembles the player-facing state from progress and the answer card.
func (s *Service) buildState(cfg Config, p progress, target *models.Card, rmap map[int]models.Rarity) (*State, error) {
	attemptsUsed := len(p.Guesses)
	st := &State{
		Number:       puzzleNumber(s.midnight(), s.epoch(cfg)),
		MaxAttempts:  cfg.MaxAttempts,
		AttemptsUsed: attemptsUsed,
		Finished:     p.Finished,
		Solved:       p.Solved,
		RevealLevel:  revealLevel(attemptsUsed, cfg.MaxAttempts, p.Finished),
		MaxLevel:     cfg.MaxAttempts,
		Reward:       p.Reward,
		Guesses:      make([]GuessFeedback, 0, attemptsUsed),
	}
	for _, gid := range p.Guesses {
		gc, err := s.repo.GetCardByID(gid)
		if err != nil {
			continue
		}
		st.Guesses = append(st.Guesses, GuessFeedback{
			CardBrief:   cardBrief(gc, rmap),
			Correct:     gc.ID == target.ID,
			RarityArrow: arrow(target.RarityID, gc.RarityID),
			PowerArrow:  arrow(target.PowerLevel, gc.PowerLevel),
		})
	}
	if p.Finished {
		st.Answer = &AnswerCard{CardBrief: cardBrief(target, rmap), ImageURL: target.ImageURL}
	}
	return st, nil
}

// epoch parses the configured epoch date (midnight MSK). An empty/invalid value
// makes today puzzle #1.
func (s *Service) epoch(cfg Config) time.Time {
	if cfg.EpochDate != "" {
		if t, err := time.ParseInLocation("2006-01-02", cfg.EpochDate, s.loc); err == nil {
			return t
		}
	}
	return s.midnight()
}

func cardBrief(c *models.Card, rmap map[int]models.Rarity) CardBrief {
	return CardBrief{
		ID:     strconv.Itoa(c.ID),
		Name:   c.Name,
		Rarity: rmap[c.RarityID].Name,
		Power:  c.PowerLevel,
	}
}

func contains(xs []int, x int) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}
