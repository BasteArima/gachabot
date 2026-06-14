package httpapi

import (
	"net/http"
	"strconv"
	"time"

	"gachabot/internal/cardart"
	"gachabot/internal/models"
)

type rollResultDTO struct {
	Card            *cardDTO `json:"card"`
	NewCoinsBalance int      `json:"newCoinsBalance"`
}

type leaderboardEntryDTO struct {
	Rank     int    `json:"rank"`
	Username string `json:"username"`
	Power    int    `json:"power"`
}

// GET /api/daily-hub — countdown to the next daily reset (midnight MSK). World
// Boss is a placeholder until it ships; Art Guess reports its real status.
func (s *Server) handleDailyHub(w http.ResponseWriter, r *http.Request) {
	loc := time.FixedZone("MSK", 3*60*60)
	now := time.Now().In(loc)
	next := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc).Add(24 * time.Hour)

	agAvailable, agSolved := s.artguess.HubStatus(r.Context(), userIDFrom(r))

	writeJSON(w, http.StatusOK, map[string]any{
		"resetsInSeconds": int(next.Sub(now).Seconds()),
		"worldBoss":       map[string]any{"available": false, "name": "Скоро"},
		"artguess":        map[string]any{"available": agAvailable, "solvedToday": agSolved},
	})
}

// GET /api/rarities — rarity names in display order, for the inventory filter tabs.
func (s *Server) handleRarities(w http.ResponseWriter, _ *http.Request) {
	rarities, err := s.repo.GetRarities()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	names := make([]string, 0, len(rarities))
	for _, r := range rarities {
		names = append(names, r.Name)
	}
	writeJSON(w, http.StatusOK, names)
}

// GET /api/leaderboard — global top by balance.
func (s *Server) handleLeaderboard(w http.ResponseWriter, _ *http.Request) {
	board, err := s.gacha.GetLeaderboard("balance", 0)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	entries := make([]leaderboardEntryDTO, 0, len(board))
	for i, e := range board {
		entries = append(entries, leaderboardEntryDTO{Rank: i + 1, Username: e.DisplayName, Power: e.Value})
	}
	writeJSON(w, http.StatusOK, map[string]any{"entries": entries})
}

// POST /api/actions/roll — roll a card (shares the 3h cooldown with the chat /roll).
func (s *Server) handleRoll(w http.ResponseWriter, r *http.Request) {
	uid := userIDFrom(r)
	res, err := s.gacha.RollCard(uid)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "roll failed")
		return
	}
	if res.OnCooldown {
		writeErr(w, http.StatusTooManyRequests, "cooldown: "+res.CooldownTimeLeft)
		return
	}

	user, err := s.repo.GetUserByID(uid)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	writeJSON(w, http.StatusOK, rollResultDTO{
		Card:            cardFromRoll(res),
		NewCoinsBalance: user.Balance,
	})
}

// POST /api/actions/craft — craft a card by burning duplicates.
func (s *Server) handleCraft(w http.ResponseWriter, r *http.Request) {
	res, err := s.gacha.CraftCard(userIDFrom(r))
	if err != nil {
		// Business errors (not enough duplicates / disabled) → 400 with the reason key.
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"card": cardFromRoll(res)})
}

// cardFromRoll maps a RollResult's card to the API DTO (nil if no card was awarded).
func cardFromRoll(res *models.RollResult) *cardDTO {
	if res == nil || res.Card == nil {
		return nil
	}
	return &cardDTO{
		ID:       strconv.Itoa(res.Card.ID),
		Name:     res.Card.Name,
		Power:    res.Card.PowerLevel,
		Rarity:   res.RarityName,
		ImageURL: cardart.Framed(res.Card.ImageURL),
		Quantity: 1,
	}
}
