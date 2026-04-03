package models

import (
	"database/sql"
)

type Card struct {
	ID         int
	Name       string
	RarityID   int
	ImageURL   string
	PowerLevel int
	SetID      *int `db:"set_id"` // Pointer, так как карта может не принадлежать сету
}

type User struct {
	ID             int64         // Единый внутренний ID бота
	TelegramID     sql.NullInt64 // Уникальный ID из Telegram
	DiscordID      sql.NullInt64 // Уникальный ID из Discord
	Username       string
	FirstName      string
	LastName       string
	Balance        int
	StreakDays     int
	LastRollTime   sql.NullTime
	LastStreakDate sql.NullTime
	PremiumRolls   int
	LanguageCode   string
	ActiveSetID    *int `db:"active_set_id"` // Pointer, потому что может быть NULL
}

type Rarity struct {
	ID            int
	Name          string
	DropChance    float64
	BaseReward    int
	PityThreshold int
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
	Value       int // Сюда положим баланс, стрик или кол-во карт в зависимости от фильтра
}

type PromoReward struct {
	Points       int            `json:"points,omitempty"`
	PremiumRolls int            `json:"premium_rolls,omitempty"`
	Cards        []int          `json:"cards,omitempty"`        // ID конкретных карт (например [12, 45])
	RandomCards  map[string]int `json:"random_cards,omitempty"` // Случайные карты: ключ - ID редкости (строка), значение - количество
}

// CardSet представляет саму коллекцию
type CardSet struct {
	ID           int    `db:"id"`
	Name         string `db:"name"`
	BuffType     string `db:"buff_type"`
	BuffValue    int    `db:"buff_value"`
	RewardPoints int    `db:"reward_points"`
}

// UserSetProgress используется для красивого вывода прогресса в UI бота
type UserSetProgress struct {
	SetID          int
	SetName        string
	CollectedCards int  // Сколько уникальных карт из сета есть у юзера
	TotalCards     int  // Сколько всего карт в этом сете
	IsCompleted    bool // Забрал ли юзер награду
	IsActive       bool // Экипирована ли эта аура прямо сейчас
	BuffType       string
	BuffValue      int
	RewardPoints   int
}

// RollResult — структура для передачи ответа из сервиса в хэндлер
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

	// НОВЫЕ ПОЛЯ ДЛЯ КОЛЛЕКЦИЙ
	CompletedSetName   string
	CompletedSetReward int
}
