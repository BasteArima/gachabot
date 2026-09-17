package models

import (
	"database/sql"
	"encoding/json"
	"time"
)

type Card struct {
	ID         int
	Name       string
	RarityID   int
	ImageURL   string
	PowerLevel int
	SetID      *int `db:"set_id"` // Pointer, since the card may not belong to the set
}

type User struct {
	ID             int64
	TelegramID     sql.NullInt64
	DiscordID      sql.NullInt64
	Username       string
	FirstName      string
	LastName       string
	Balance        int
	StreakDays     int
	LastRollTime   sql.NullTime
	LastStreakDate sql.NullTime
	PremiumRolls   int
	LanguageCode   string
	ActiveSetID    *int `db:"active_set_id"`
	IsAdult        sql.NullBool
	AvatarURL      string
}

type Rarity struct {
	ID            int
	Name          string
	DropChance    float64
	BaseReward    int
	PityThreshold int
	CraftCost     int
	// RequiresFragments: a normal roll of this rarity yields a fragment instead of the card itself.
	RequiresFragments bool
	// FragmentsRequired: how many fragments are needed to assemble a card of this rarity.
	FragmentsRequired int
}

type UserProfile struct {
	Balance          int
	StreakDays       int
	UniqueCardsCount int
	TotalCardsCount  int
	DuplicatesCount  int
	PremiumRolls     int
	CompletedSets    int
	TotalSets        int
}

type UserCardView struct {
	CardName   string
	RarityName string
	ImageURL   string
	Quantity   int
	PowerLevel int
	SetName    string
}

type LeaderboardEntry struct {
	DisplayName string
	Value       int // Balance, streak, or number of cards depending on the filter
	ActiveAura  string
}

type CompletedSetInfo struct {
	Name   string
	Reward int
}

type PromoReward struct {
	Points        int                `json:"points,omitempty"`
	PremiumRolls  int                `json:"premium_rolls,omitempty"`
	Cards         []int              `json:"cards,omitempty"`
	RandomCards   map[string]int     `json:"random_cards,omitempty"`
	CompletedSets []CompletedSetInfo `json:"-"`
}

// Spawn is one card drop in one chat (part of a wave). Hot claim state lives in
// Redis; this mirrors the DB audit row.
type Spawn struct {
	ID        int64
	WaveID    int64
	Platform  string
	ChatID    int64
	CardID    int
	MessageID sql.NullInt64
	ExpiresAt time.Time
	ClaimedBy sql.NullInt64
}

// SpawnReward is the outcome of granting a spawned (or otherwise awarded) card:
// reuses the roll pipeline, so it carries fragment/set info too.
type SpawnReward struct {
	Coins              int
	IsFragment         bool
	FragmentsCount     int
	CardAssembled      bool
	IsDuplicate        bool // duplicates disabled and already owned: coins only
	CompletedSetName   string
	CompletedSetReward int
	StreakDays         int
	StreakUpdated      bool
}

// InventoryCard is one owned card with full display info, for the web inventory.
type InventoryCard struct {
	CardID     int
	Name       string
	RarityName string
	ImageURL   string
	PowerLevel int
	Quantity   int
	SetName    string
}

// Chat is a registered place the bot posts to (group / main channel), used by
// spawns and broadcasts. GuildID is set only for Discord.
type Chat struct {
	Platform     string
	ChatID       int64
	GuildID      sql.NullInt64
	Title        string
	SpawnEnabled bool
}

type CardSet struct {
	ID           int    `db:"id"`
	Name         string `db:"name"`
	BuffType     string `db:"buff_type"`
	BuffValue    int    `db:"buff_value"`
	RewardPoints int    `db:"reward_points"`
}

type UserSetProgress struct {
	SetID          int
	SetName        string
	CollectedCards int
	TotalCards     int
	IsCompleted    bool
	IsActive       bool
	BuffType       string
	BuffValue      int
	RewardPoints   int
}

type RollResult struct {
	OnCooldown       bool
	CooldownTimeLeft string

	Card       *Card
	RarityName string
	Reward     int

	IsFragment     bool
	FragmentsCount int
	CardAssembled  bool

	CraftCost int

	StreakDays    int
	StreakUpdated bool

	CompletedSetName   string
	CompletedSetReward int

	// AllCollected is set (when duplicates are disabled) if the player already owns
	// every card: no card is awarded, only points and streak.
	AllCollected bool
}

type DuelAuraInfo struct {
	Name      string
	BuffType  string
	BuffValue int
}

type DuelResult struct {
	WinnerID         int64
	WinnerName       string
	CardChallenger   *Card
	CardTarget       *Card
	ChanceChallenger float64
	ChanceTarget     float64
	Roll             float64
	AmountWon        int
	ChallengerAura   *DuelAuraInfo
	TargetAura       *DuelAuraInfo
	IsFair           bool
}

type DuelRequest struct {
	ID             string `json:"id"`
	ChallengerID   int64  `json:"challenger_id"`
	ChallengerName string `json:"challenger_name"`
	TargetID       int64  `json:"target_id"`
	TargetName     string `json:"target_name"`
	Amount         int    `json:"amount"`
	IsFair         bool   `json:"is_fair"`
}

// Season is one run of the resettable scoreboard. EndsAt is a target shown as a
// countdown, not a deadline: seasons are finished by hand from the admin panel.
type Season struct {
	ID         int
	Number     int
	Title      string
	StartedAt  time.Time
	EndsAt     *time.Time
	FinishedAt *time.Time
	Config     json.RawMessage
}

// Running reports whether this season is the one currently counting.
func (s *Season) Running() bool { return s != nil && s.FinishedAt == nil }

// SeasonStat is one player's progress in a season, joined with the names the
// boards display.
type SeasonStat struct {
	UserID         int64
	Name           string
	CoinsEarned    int64
	CardPoints     int64
	CardsCount     int
	ActiveDays     int
	BestStreak     int
	BestDropCardID sql.NullInt64
	BestDropName   string
	BestDropAt     sql.NullTime
}

// SeasonResult is one player's finished season: the numbers as they stood when
// medals were handed out. Kept apart from the live counters because a trophy
// read years later must not change when anything is recounted.
type SeasonResult struct {
	SeasonID     int
	SeasonNumber int
	SeasonTitle  string
	StartedAt    time.Time
	FinishedAt   time.Time
	UserID       int64
	Place        int
	Points       int
	Tier         string
	Detail       json.RawMessage
}

// Badge is what sits next to a nickname: the best medal a player has ever
// earned, and how many times they earned it.
type Badge struct {
	Tier  string `json:"tier"`
	Count int    `json:"count"`
}
