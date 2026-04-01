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
}

type UserCardView struct {
	CardName   string
	RarityName string
	ImageURL   string
	Quantity   int
	PowerLevel int
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
