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
	Username       string
	FirstName      string // Новое
	LastName       string // Новое
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
	DuplicatesCount  int
}

type UserCardView struct {
	CardName   string
	RarityName string
	ImageURL   string
	Quantity   int
}

type LeaderboardEntry struct {
	DisplayName string
	Value       int // Сюда положим баланс, стрик или кол-во карт в зависимости от фильтра
}
