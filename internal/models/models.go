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

type User struct { // Переименовал UserDB в просто User
	TgID           int64
	Balance        int
	StreakDays     int
	LastRollTime   sql.NullTime
	LastStreakDate sql.NullTime
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
}

type UserCardView struct {
	CardName   string
	RarityName string
	ImageURL   string
	Quantity   int
}

type LeaderboardEntry struct {
	Username string
	Value    int // Сюда положим баланс, стрик или кол-во карт в зависимости от фильтра
}
