package spawn

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math/rand/v2"
	"strconv"
	"time"

	"gachabot/internal/models"
	"gachabot/internal/repository"
	"gachabot/internal/service/gacha"

	"github.com/redis/go-redis/v9"
)

// Platform identifiers (match repository.chats.platform values).
const (
	PlatformTelegram = "telegram"
	PlatformDiscord  = "discord"
)

const settingKey = "spawn_config"

// Outcome of a claim attempt.
type Outcome int

const (
	OutcomeWon Outcome = iota
	OutcomeTaken          // someone else already won this chat's spawn
	OutcomeAlreadyWave    // this account already claimed the wave elsewhere
	OutcomeExpired        // spawn gone (window passed / unknown)
)

// SpawnView is what the delivery layer needs to render a spawn message.
type SpawnView struct {
	SpawnID       int64
	Card          *models.Card
	RarityName    string
	WindowSeconds int
}

// SpawnMeta is the Redis-stored hot state of an active spawn.
type SpawnMeta struct {
	SpawnID     int64  `json:"spawn_id"`
	WaveID      int64  `json:"wave_id"`
	Platform    string `json:"platform"`
	ChatID      int64  `json:"chat_id"`
	MessageID   int64  `json:"message_id"`
	CardID      int    `json:"card_id"`
	ExpiresUnix int64  `json:"expires"`
}

// ClaimResult is returned to the delivery layer after a claim attempt.
type ClaimResult struct {
	Outcome    Outcome
	Card       *models.Card
	RarityName string
	Reward     *models.SpawnReward
	Meta       *SpawnMeta
}

// Spawner is implemented by each delivery layer (Telegram, Discord) so the
// engine can post and expire spawn messages without knowing platform details.
type Spawner interface {
	SendSpawn(chat models.Chat, view SpawnView) (messageID int64, err error)
	EditSpawnExpired(meta SpawnMeta, card *models.Card)
}

type SpawnService struct {
	repo     *repository.PostgresRepo
	rdb      *redis.Client
	gacha    *gacha.GachaService
	loc      *time.Location
	spawners map[string]Spawner
}

func NewSpawnService(repo *repository.PostgresRepo, rdb *redis.Client, gs *gacha.GachaService) *SpawnService {
	return &SpawnService{
		repo:     repo,
		rdb:      rdb,
		gacha:    gs,
		loc:      time.FixedZone("MSK", 3*60*60),
		spawners: make(map[string]Spawner),
	}
}

// RegisterSpawner wires a delivery layer in. Call before Start (and after the
// underlying bot is connected, so sends succeed).
func (s *SpawnService) RegisterSpawner(platform string, sp Spawner) {
	s.spawners[platform] = sp
}

// Start launches the scheduler + expiry sweeper loop.
func (s *SpawnService) Start() {
	go func() {
		ticker := time.NewTicker(20 * time.Second)
		defer ticker.Stop()
		s.tick()
		for range ticker.C {
			s.tick()
		}
	}()
	log.Println("[SPAWN] engine started")
}

func (s *SpawnService) tick() {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[SPAWN] tick panic recovered: %v", r)
		}
	}()
	s.runSchedule()
	s.sweepExpired()
}

// --- Scheduling ---

type dayPlan struct {
	Times []int64 `json:"times"`
	Fired []bool  `json:"fired"`
}

func (s *SpawnService) runSchedule() {
	cfg := s.loadConfig()
	if !cfg.Enabled {
		return
	}
	now := time.Now().In(s.loc)
	date := now.Format("2006-01-02")
	plan := s.loadOrCreatePlan(cfg, now, date)

	changed := false
	for i, ts := range plan.Times {
		if plan.Fired[i] || ts > now.Unix() {
			continue
		}
		plan.Fired[i] = true
		changed = true
		s.spawnWave(cfg)
	}
	if changed {
		s.savePlan(date, plan)
	}
}

func (s *SpawnService) loadOrCreatePlan(cfg Config, now time.Time, date string) dayPlan {
	ctx := context.Background()
	if raw, err := s.rdb.Get(ctx, planKey(date)).Result(); err == nil {
		var p dayPlan
		if json.Unmarshal([]byte(raw), &p) == nil && len(p.Times) == len(p.Fired) {
			return p
		}
	}
	// Build a fresh plan; times already in the past are marked fired so a
	// mid-day (re)start doesn't backfire the whole day at once.
	times := cfg.planDay(now, s.loc)
	p := dayPlan{Times: make([]int64, len(times)), Fired: make([]bool, len(times))}
	for i, t := range times {
		p.Times[i] = t.Unix()
		p.Fired[i] = t.Unix() <= now.Unix()
	}
	s.savePlan(date, p)
	return p
}

func (s *SpawnService) savePlan(date string, p dayPlan) {
	if b, err := json.Marshal(p); err == nil {
		s.rdb.Set(context.Background(), planKey(date), b, 48*time.Hour)
	}
}

// --- Spawning ---

func (s *SpawnService) spawnWave(cfg Config) {
	chats, err := s.repo.GetSpawnChats()
	if err != nil {
		log.Printf("[SPAWN] get chats failed: %v", err)
		return
	}
	if len(chats) == 0 {
		return
	}
	card, err := s.pickCard(cfg)
	if err != nil || card == nil {
		log.Printf("[SPAWN] no card to spawn (err=%v)", err)
		return
	}

	waveID := time.Now().In(s.loc).UnixNano()
	sent := 0
	for _, chat := range chats {
		if s.spawnOne(cfg, card, chat, waveID) {
			sent++
		}
	}
	log.Printf("[SPAWN] wave %d: %q spawned in %d chat(s)", waveID, card.Name, sent)
}

// spawnOne posts one card into one chat as part of waveID. Returns true on success.
func (s *SpawnService) spawnOne(cfg Config, card *models.Card, chat models.Chat, waveID int64) bool {
	sp := s.spawners[chat.Platform]
	if sp == nil {
		return false
	}
	window := time.Duration(cfg.ClaimWindowSeconds) * time.Second
	expires := time.Now().Add(window)

	spawnID, err := s.repo.InsertSpawn(waveID, chat.Platform, chat.ChatID, card.ID, expires)
	if err != nil {
		log.Printf("[SPAWN] insert failed for chat %d: %v", chat.ChatID, err)
		return false
	}
	msgID, err := sp.SendSpawn(chat, SpawnView{
		SpawnID: spawnID, Card: card, RarityName: s.rarityName(card.RarityID), WindowSeconds: cfg.ClaimWindowSeconds,
	})
	if err != nil {
		log.Printf("[SPAWN] send failed for chat %d: %v", chat.ChatID, err)
		return false
	}
	_ = s.repo.SetSpawnMessageID(spawnID, msgID)
	s.registerActive(chat.Platform, chat.ChatID, spawnID, waveID, card.ID, msgID, expires, window+60*time.Second)
	return true
}

// SpawnNow forces an immediate spawn into a single chat (admin/test helper).
// Does not touch the daily plan. Returns the spawned card's name.
func (s *SpawnService) SpawnNow(platform string, chatID int64) (string, error) {
	cfg := s.loadConfig()
	if s.spawners[platform] == nil {
		return "", fmt.Errorf("platform %q is not connected", platform)
	}
	card, err := s.pickCard(cfg)
	if err != nil {
		return "", err
	}
	if card == nil {
		return "", fmt.Errorf("no card available to spawn")
	}
	chat := models.Chat{Platform: platform, ChatID: chatID}
	if !s.spawnOne(cfg, card, chat, time.Now().In(s.loc).UnixNano()) {
		return "", fmt.Errorf("failed to send spawn")
	}
	return card.Name, nil
}

func (s *SpawnService) pickCard(cfg Config) (*models.Card, error) {
	if cfg.Pool.Mode == PoolCardList {
		return s.repo.GetRandomCardFromList(cfg.Pool.CardIDs, cfg.Pool.ExcludeCardIDs)
	}
	rarities, err := s.repo.GetRarities()
	if err != nil {
		return nil, err
	}
	if len(rarities) == 0 {
		return nil, nil
	}
	rarityID := pickRarity(rarities, cfg.Pool.RarityWeights)
	card, err := s.repo.GetRandomCardByRarityExcluding(rarityID, cfg.Pool.ExcludeCardIDs)
	if err != nil {
		return nil, err
	}
	if card == nil {
		// Chosen rarity has no eligible card — fall back to the lowest rarity.
		card, err = s.repo.GetRandomCardByRarityExcluding(rarities[0].ID, cfg.Pool.ExcludeCardIDs)
	}
	return card, err
}

// pickRarity does a weighted pick. Empty weights -> use each rarity's drop_chance;
// otherwise rarities absent from the map get weight 0 (curated pool).
func pickRarity(rarities []models.Rarity, weights map[string]float64) int {
	useDropChance := len(weights) == 0
	w := make([]float64, len(rarities))
	total := 0.0
	for i, r := range rarities {
		wt := 0.0
		if useDropChance {
			wt = r.DropChance
		} else if v, ok := weights[r.Name]; ok {
			wt = v
		}
		if wt < 0 {
			wt = 0
		}
		w[i] = wt
		total += wt
	}
	if total <= 0 {
		return rarities[0].ID
	}
	target := rand.Float64() * total
	sum := 0.0
	for i := range rarities {
		sum += w[i]
		if target <= sum {
			return rarities[i].ID
		}
	}
	return rarities[len(rarities)-1].ID
}

func (s *SpawnService) registerActive(platform string, chatID, spawnID, waveID int64, cardID int, messageID int64, expires time.Time, ttl time.Duration) {
	ctx := context.Background()
	meta := SpawnMeta{
		SpawnID: spawnID, WaveID: waveID, Platform: platform, ChatID: chatID,
		MessageID: messageID, CardID: cardID, ExpiresUnix: expires.Unix(),
	}
	if b, err := json.Marshal(meta); err == nil {
		s.rdb.Set(ctx, metaKey(spawnID), b, ttl)
	}
	s.rdb.Set(ctx, activeKey(platform, chatID), spawnID, time.Until(expires))
	s.rdb.ZAdd(ctx, expiringKey, redis.Z{Score: float64(expires.Unix()), Member: strconv.FormatInt(spawnID, 10)})
}

// --- Claiming ---

// claimScript atomically enforces: one win per account per wave, one winner per
// chat. Returns "OK" | "TAKEN" | "ALREADY_WAVE".
var claimScript = redis.NewScript(`
local winner = KEYS[1]
local wave   = KEYS[2]
local uid    = ARGV[1]
local ttl    = tonumber(ARGV[2])
if redis.call('SISMEMBER', wave, uid) == 1 then return 'ALREADY_WAVE' end
if redis.call('EXISTS', winner) == 1 then return 'TAKEN' end
redis.call('SET', winner, uid, 'EX', ttl)
redis.call('SADD', wave, uid)
redis.call('EXPIRE', wave, ttl)
return 'OK'
`)

// Claim attempts to claim a spawn for a user. Safe to call concurrently.
func (s *SpawnService) Claim(platform string, userID, spawnID int64) (ClaimResult, error) {
	ctx := context.Background()

	raw, err := s.rdb.Get(ctx, metaKey(spawnID)).Result()
	if err == redis.Nil {
		return ClaimResult{Outcome: OutcomeExpired}, nil
	}
	if err != nil {
		return ClaimResult{}, err
	}
	var meta SpawnMeta
	if err := json.Unmarshal([]byte(raw), &meta); err != nil {
		return ClaimResult{}, err
	}
	if time.Now().Unix() > meta.ExpiresUnix {
		return ClaimResult{Outcome: OutcomeExpired, Meta: &meta}, nil
	}

	ttl := int(time.Until(time.Unix(meta.ExpiresUnix, 0)).Seconds()) + 60
	if ttl < 60 {
		ttl = 60
	}
	code, err := claimScript.Run(ctx, s.rdb,
		[]string{winnerKey(spawnID), waveKey(meta.WaveID)},
		strconv.FormatInt(userID, 10), ttl).Text()
	if err != nil {
		return ClaimResult{}, err
	}
	switch code {
	case "ALREADY_WAVE":
		return ClaimResult{Outcome: OutcomeAlreadyWave, Meta: &meta}, nil
	case "TAKEN":
		return ClaimResult{Outcome: OutcomeTaken, Meta: &meta}, nil
	case "OK":
		// won — fall through
	default:
		return ClaimResult{Outcome: OutcomeTaken, Meta: &meta}, nil
	}

	_, _ = s.repo.ClaimSpawnRow(spawnID, userID)

	card, err := s.repo.GetCardByID(meta.CardID)
	if err != nil {
		return ClaimResult{}, fmt.Errorf("load card: %w", err)
	}

	cfg := s.loadConfig()
	coins := randRange(cfg.RewardCoins.Min, cfg.RewardCoins.Max)
	reward, err := s.gacha.GrantCardReward(userID, card, coins)
	if err != nil {
		return ClaimResult{}, fmt.Errorf("grant reward: %w", err)
	}
	days, updated, _ := s.gacha.ApplyDailyStreak(userID)
	reward.StreakDays = days
	reward.StreakUpdated = updated

	// Stop /claim from re-finding it and the sweeper from touching it.
	s.rdb.Del(ctx, activeKey(platform, meta.ChatID))
	s.rdb.ZRem(ctx, expiringKey, strconv.FormatInt(spawnID, 10))

	return ClaimResult{
		Outcome:    OutcomeWon,
		Card:       card,
		RarityName: s.rarityName(card.RarityID),
		Reward:     reward,
		Meta:       &meta,
	}, nil
}

// ActiveSpawnInChat returns the id of the active spawn in a chat, for /claim.
func (s *SpawnService) ActiveSpawnInChat(platform string, chatID int64) (int64, bool) {
	id, err := s.rdb.Get(context.Background(), activeKey(platform, chatID)).Int64()
	if err != nil {
		return 0, false
	}
	return id, true
}

// --- Expiry ---

func (s *SpawnService) sweepExpired() {
	ctx := context.Background()
	now := strconv.FormatInt(time.Now().Unix(), 10)
	ids, err := s.rdb.ZRangeByScore(ctx, expiringKey, &redis.ZRangeBy{Min: "-inf", Max: now}).Result()
	if err != nil || len(ids) == 0 {
		return
	}
	for _, idStr := range ids {
		s.rdb.ZRem(ctx, expiringKey, idStr)
		spawnID, _ := strconv.ParseInt(idStr, 10, 64)

		// Claimed spawns are edited by the claim handler; skip.
		if s.rdb.Exists(ctx, winnerKey(spawnID)).Val() == 1 {
			continue
		}
		raw, err := s.rdb.Get(ctx, metaKey(spawnID)).Result()
		if err != nil {
			continue
		}
		var m SpawnMeta
		if json.Unmarshal([]byte(raw), &m) != nil {
			continue
		}
		if sp := s.spawners[m.Platform]; sp != nil {
			if card, err := s.repo.GetCardByID(m.CardID); err == nil {
				sp.EditSpawnExpired(m, card)
			}
		}
		s.rdb.Del(ctx, metaKey(spawnID))
	}
}

// --- helpers ---

func (s *SpawnService) loadConfig() Config {
	raw, err := s.repo.GetSetting(settingKey)
	if err != nil || raw == nil {
		return DefaultConfig()
	}
	cfg, err := ParseConfig(raw)
	if err != nil {
		log.Printf("[SPAWN] bad spawn_config, using default: %v", err)
		return DefaultConfig()
	}
	return cfg
}

func (s *SpawnService) rarityName(rarityID int) string {
	rarities, err := s.repo.GetRarities()
	if err != nil {
		return ""
	}
	for _, r := range rarities {
		if r.ID == rarityID {
			return r.Name
		}
	}
	return ""
}

func randRange(min, max int) int {
	if max <= min {
		return min
	}
	return min + rand.IntN(max-min+1)
}

func planKey(date string) string { return "spawn:plan:" + date }
func metaKey(spawnID int64) string {
	return "spawn:meta:" + strconv.FormatInt(spawnID, 10)
}
func winnerKey(spawnID int64) string {
	return "spawn:winner:" + strconv.FormatInt(spawnID, 10)
}
func waveKey(waveID int64) string {
	return "spawn:wave:" + strconv.FormatInt(waveID, 10) + ":claimers"
}
func activeKey(platform string, chatID int64) string {
	return "spawn:active:" + platform + ":" + strconv.FormatInt(chatID, 10)
}

const expiringKey = "spawn:expiring"
