package httpapi

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"strconv"

	"gachabot/internal/cardart"

	"github.com/go-chi/chi/v5"
)

// GET /api/admin/players?search=
func (s *Server) handleAdminSearchPlayers(w http.ResponseWriter, r *http.Request) {
	players, err := s.repo.SearchPlayers(r.URL.Query().Get("search"), 50)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	writeJSON(w, http.StatusOK, players)
}

// GET /api/admin/players/{id} — profile plus the player's collection.
func (s *Server) handleAdminGetPlayer(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad id")
		return
	}
	player, err := s.repo.GetAdminPlayer(id)
	if err == sql.ErrNoRows {
		writeErr(w, http.StatusNotFound, "игрок не найден")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}

	inv, err := s.repo.GetUserInventoryList(id, "All")
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	cards := make([]cardDTO, 0, len(inv))
	for _, c := range inv {
		cards = append(cards, cardDTO{
			ID:       strconv.Itoa(c.CardID),
			Name:     c.Name,
			Power:    c.PowerLevel,
			Rarity:   c.RarityName,
			ImageURL: cardart.Framed(c.ImageURL),
			Quantity: c.Quantity,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"player": player, "cards": cards})
}

// playerActionInput covers every admin action on a player; only the fields
// relevant to the chosen action are read.
type playerActionInput struct {
	Action string `json:"action"` // coins | rolls | cooldown | streak | grantCard | removeCard
	Amount int    `json:"amount"`
	CardID int    `json:"cardId"`
}

// POST /api/admin/players/{id}/action — grant/take resources and fix state.
// Every action is logged: these directly change a player's balance/collection.
func (s *Server) handleAdminPlayerAction(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad id")
		return
	}
	var in playerActionInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "bad request")
		return
	}

	var result string
	switch in.Action {
	case "coins":
		balance, err := s.repo.AdjustBalance(id, in.Amount)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "не удалось изменить баланс")
			return
		}
		result = "баланс: " + strconv.Itoa(balance)

	case "rolls":
		rolls, err := s.repo.AdjustPremiumRolls(id, in.Amount)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "не удалось изменить премиум-роллы")
			return
		}
		result = "премиум-роллов: " + strconv.Itoa(rolls)

	case "cooldown":
		if err := s.repo.ResetRollCooldown(id); err != nil {
			writeErr(w, http.StatusBadRequest, "не удалось сбросить кулдаун")
			return
		}
		result = "кулдаун сброшен"

	case "streak":
		if err := s.repo.SetStreak(id, in.Amount); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		result = "стрик: " + strconv.Itoa(in.Amount)

	case "grantCard":
		if in.CardID <= 0 {
			writeErr(w, http.StatusBadRequest, "укажи карту")
			return
		}
		if err := s.repo.AddCardToInventory(id, in.CardID); err != nil {
			writeErr(w, http.StatusBadRequest, "не удалось выдать карту")
			return
		}
		result = "карта выдана"

	case "removeCard":
		if in.CardID <= 0 {
			writeErr(w, http.StatusBadRequest, "укажи карту")
			return
		}
		if err := s.repo.RemoveCardCopy(id, in.CardID); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		result = "копия карты убрана"

	default:
		writeErr(w, http.StatusBadRequest, "неизвестное действие")
		return
	}

	log.Printf("[ADMIN] player %d: action=%s amount=%d cardID=%d -> %s", id, in.Action, in.Amount, in.CardID, result)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "result": result})
}
