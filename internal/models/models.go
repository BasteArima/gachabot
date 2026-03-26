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
	ID         int
	Name       string
	DropChance float64
	BaseReward int
}

type UserProfile struct {
	Balance          int
	StreakDays       int
	UniqueCardsCount int
	TotalCardsCount  int
}
