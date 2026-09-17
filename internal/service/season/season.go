// Package season keeps the resettable scoreboard that runs alongside the
// all-time tops: how much each player earned, pulled and played during the
// current season.
//
// Recording is deliberately best-effort. A season counter is bookkeeping, and a
// failure to write one must never cost a player the roll that produced it — so
// every Record* call swallows its error into the log and returns nothing.
package season

import (
	"encoding/json"
	"log"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"gachabot/internal/models"
	"gachabot/internal/repository"
)

// Tier names, shared with the medals the trophy shelf shows.
const (
	TierChampion    = "champion"
	TierSilver      = "silver"
	TierBronze      = "bronze"
	TierElite       = "elite"
	TierParticipant = "participant"
)

// Config holds the scoring rules, stored with the season so a past season keeps
// the rules it was played under.
type Config struct {
	// Weights maps a rarity id to the points one card of it is worth.
	Weights map[int]int `json:"weights"`
	// ElitePercent: Elite covers the top N % of qualified players, always at
	// least two places behind the podium so it cannot vanish in a small group.
	ElitePercent int `json:"elitePercent"`
	// MinActivityPercent: share of the season's days a player must be active on
	// to be eligible for any medal at all.
	MinActivityPercent int `json:"minActivityPercent"`
	// PodiumMinPlayers: below this many qualified players there is no podium —
	// with four players, "third place" is next to last.
	PodiumMinPlayers int `json:"podiumMinPlayers"`
}

// Defaults chosen in the design pass: a quarter of the season active to earn
// anything, a quarter of the field in Elite, and a podium only once there is a
// field to speak of.
const (
	defaultElitePercent       = 25
	defaultMinActivityPercent = 25
	defaultPodiumMinPlayers   = 5
)

// fibWeights keeps the gap between rarities gentle: a mythic is worth thirteen
// commons rather than fifty, so a season rewards playing more than one lucky
// pull. Steeper numbers are a config change away.
var fibWeights = []int{1, 2, 3, 5, 8, 13, 21, 34, 55, 89}

// Normalize fills in anything missing or out of range, so a hand-edited config
// can never leave the scoring in a nonsensical state.
func (c *Config) Normalize() {
	if c.Weights == nil {
		c.Weights = map[int]int{}
	}
	for id, w := range c.Weights {
		if w < 0 {
			c.Weights[id] = 0
		}
	}
	if c.ElitePercent < 1 || c.ElitePercent > 100 {
		c.ElitePercent = defaultElitePercent
	}
	if c.MinActivityPercent < 0 || c.MinActivityPercent > 100 {
		c.MinActivityPercent = defaultMinActivityPercent
	}
	if c.PodiumMinPlayers < 1 {
		c.PodiumMinPlayers = defaultPodiumMinPlayers
	}
}

// DefaultConfig builds the rules a fresh season starts with: rarities ordered
// from the most common to the rarest, weighted along the gentle curve above.
func DefaultConfig(rarities []models.Rarity) Config {
	ordered := append([]models.Rarity(nil), rarities...)
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].DropChance != ordered[j].DropChance {
			return ordered[i].DropChance > ordered[j].DropChance
		}
		return ordered[i].ID < ordered[j].ID
	})

	cfg := Config{
		Weights:            make(map[int]int, len(ordered)),
		ElitePercent:       defaultElitePercent,
		MinActivityPercent: defaultMinActivityPercent,
		PodiumMinPlayers:   defaultPodiumMinPlayers,
	}
	for i, r := range ordered {
		if i < len(fibWeights) {
			cfg.Weights[r.ID] = fibWeights[i]
		} else {
			cfg.Weights[r.ID] = fibWeights[len(fibWeights)-1]
		}
	}
	return cfg
}

type Service struct {
	repo *repository.PostgresRepo
	loc  *time.Location

	mu      sync.RWMutex
	current *models.Season
	cfg     Config

	// champion is read on every drop message and changes only when a season is
	// awarded, so it is cached rather than queried per roll.
	champion atomic.Int64
}

// New loads the running season, if there is one. A missing season is not an
// error: the bot works exactly as before, it just counts nothing.
func New(repo *repository.PostgresRepo) *Service {
	s := &Service{repo: repo, loc: time.FixedZone("MSK", 3*60*60)}
	if err := s.reload(); err != nil {
		log.Printf("[SEASON] не удалось прочитать текущий сезон: %v (счёт не ведётся)", err)
	}
	return s
}

func (s *Service) reload() error {
	cur, err := s.repo.ActiveSeason()
	if err != nil {
		return err
	}
	var cfg Config
	if cur != nil && len(cur.Config) > 0 {
		if err := json.Unmarshal(cur.Config, &cfg); err != nil {
			log.Printf("[SEASON] битый конфиг сезона %d: %v (беру значения по умолчанию)", cur.Number, err)
		}
	}
	cfg.Normalize()

	s.mu.Lock()
	s.current, s.cfg = cur, cfg
	s.mu.Unlock()

	champ, err := s.repo.ReigningChampion()
	if err != nil {
		log.Printf("[SEASON] действующий чемпион не определён: %v", err)
	} else {
		s.champion.Store(champ)
	}
	return nil
}

// Current returns the running season, or nil when none is running.
func (s *Service) Current() *models.Season {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.current
}

// CurrentConfig returns the scoring rules of the running season.
func (s *Service) CurrentConfig() Config {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cfg
}

// Start opens a new season. It fails if one is already running — the database
// index guarantees it, and this turns that into a readable error.
func (s *Service) Start(title string, endsAt *time.Time) (*models.Season, error) {
	rarities, err := s.repo.GetRarities()
	if err != nil {
		return nil, err
	}
	cfg := DefaultConfig(rarities)
	raw, err := json.Marshal(cfg)
	if err != nil {
		return nil, err
	}
	created, err := s.repo.CreateSeason(title, endsAt, raw)
	if err != nil {
		return nil, err
	}
	if err := s.reload(); err != nil {
		return nil, err
	}
	log.Printf("[SEASON] сезон %d начат", created.Number)
	return created, nil
}

// Update changes the running season's name, target date and scoring rules.
func (s *Service) Update(title string, endsAt *time.Time, cfg Config) error {
	cur := s.Current()
	if cur == nil {
		return ErrNoSeason
	}
	cfg.Normalize()
	raw, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	if err := s.repo.UpdateSeason(cur.ID, title, endsAt, raw); err != nil {
		return err
	}
	return s.reload()
}

// Restart wipes what the running season has counted and starts its clock again.
func (s *Service) Restart() error {
	cur := s.Current()
	if cur == nil {
		return ErrNoSeason
	}
	if err := s.repo.RestartSeason(cur.ID); err != nil {
		return err
	}
	log.Printf("[SEASON] счётчики сезона %d обнулены, отсчёт начат заново", cur.Number)
	return s.reload()
}

// Standings returns the running season's raw counters.
func (s *Service) Standings() ([]models.SeasonStat, error) {
	cur := s.Current()
	if cur == nil {
		return nil, nil
	}
	return s.repo.SeasonStandings(cur.ID)
}

// Days reports how long the running season has lasted so far, and the minimum
// number of active days a medal currently requires. Both are computed from the
// real elapsed time rather than the target date, so moving the date does not
// move the goalposts under players who already qualified.
func (s *Service) Days() (elapsed, minActive int) {
	cur := s.Current()
	if cur == nil {
		return 0, 0
	}
	elapsed = int(s.today().Sub(s.startDay(cur)).Hours()/24) + 1
	if elapsed < 1 {
		elapsed = 1
	}
	minActive = elapsed * s.CurrentConfig().MinActivityPercent / 100
	if minActive < 1 {
		minActive = 1
	}
	return elapsed, minActive
}

func (s *Service) today() time.Time {
	now := time.Now().In(s.loc)
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, s.loc)
}

func (s *Service) startDay(cur *models.Season) time.Time {
	t := cur.StartedAt.In(s.loc)
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, s.loc)
}

// ---- recording ----------------------------------------------------------
//
// What counts is what the game paid out for playing: rolls, spawns, the daily
// streak, Art Guess, set completion. What does not count is anything moved
// between players or handed over — duels, promo codes and admin grants — so a
// medal cannot be gifted or farmed between two accounts.

// RecordCoins credits coins the game paid out. Negative amounts are ignored:
// spending is not a penalty in the standings.
func (s *Service) RecordCoins(userID int64, amount int) {
	cur := s.Current()
	if cur == nil || amount <= 0 {
		return
	}
	if err := s.repo.AddSeasonCoins(cur.ID, userID, amount); err != nil {
		log.Printf("[SEASON] монеты игрока %d не записаны: %v", userID, err)
	}
}

// RecordCard records one card the player obtained, worth its rarity's weight.
func (s *Service) RecordCard(userID int64, rarityID, cardID int) {
	cur := s.Current()
	if cur == nil {
		return
	}
	points := s.CurrentConfig().Weights[rarityID]
	if points <= 0 {
		// An unweighted rarity (added after the season began) still counts as a
		// card, just not for points — better than dropping it silently.
		points = 0
	}
	if err := s.repo.AddSeasonCard(cur.ID, userID, cardID, points); err != nil {
		log.Printf("[SEASON] карта игрока %d не записана: %v", userID, err)
	}
}

// RecordActivity marks today as played. Calling it repeatedly on the same day
// is harmless — the day is counted once.
func (s *Service) RecordActivity(userID int64, streak int) {
	cur := s.Current()
	if cur == nil {
		return
	}
	if err := s.repo.MarkSeasonActivity(cur.ID, userID, s.today(), streak); err != nil {
		log.Printf("[SEASON] активность игрока %d не записана: %v", userID, err)
	}
}
