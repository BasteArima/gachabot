package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"gachabot/internal/cardart"
	"gachabot/internal/repository"

	"github.com/go-chi/chi/v5"
)

// --- Cards ---

type cardAdminDTO struct {
	repository.AdminCard
	// FramedURL is what reveal surfaces (roll/craft/spawn/duel) actually show, so
	// the editor can preview both variants without duplicating the mapping rule.
	FramedURL string `json:"framedUrl"`
}

// GET /api/admin/cards?search=&rarityId=&setId=&noArt=1
func (s *Server) handleAdminListCards(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	rarityID, _ := strconv.Atoi(q.Get("rarityId"))
	setID, _ := strconv.Atoi(q.Get("setId"))
	cards, err := s.repo.ListAdminCards(repository.CardFilter{
		Search:   strings.TrimSpace(q.Get("search")),
		RarityID: rarityID,
		SetID:    setID,
		NoArt:    q.Get("noArt") == "1" || q.Get("noArt") == "true",
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	out := make([]cardAdminDTO, 0, len(cards))
	for _, c := range cards {
		out = append(out, cardAdminDTO{AdminCard: c, FramedURL: cardart.Framed(c.ImageURL)})
	}
	writeJSON(w, http.StatusOK, out)
}

// cardInput is the create/update payload for a card.
type cardInput struct {
	Name       string `json:"name"`
	RarityID   int    `json:"rarityId"`
	PowerLevel int    `json:"powerLevel"`
	ImageURL   string `json:"imageUrl"`
	SetID      *int   `json:"setId"` // null = no set
}

func (in *cardInput) validate() error {
	in.Name = strings.TrimSpace(in.Name)
	in.ImageURL = strings.TrimSpace(in.ImageURL)
	if in.Name == "" {
		return fmt.Errorf("название карты обязательно")
	}
	if in.RarityID <= 0 {
		return fmt.Errorf("выбери редкость")
	}
	if in.PowerLevel < 0 {
		return fmt.Errorf("сила не может быть отрицательной")
	}
	if in.SetID != nil && *in.SetID <= 0 {
		in.SetID = nil
	}
	return nil
}

// POST /api/admin/cards
func (s *Server) handleAdminCreateCard(w http.ResponseWriter, r *http.Request) {
	var in cardInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "bad request")
		return
	}
	if err := in.validate(); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	id, err := s.repo.CreateCard(in.Name, in.RarityID, in.ImageURL, in.PowerLevel, in.SetID)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": id})
}

// PUT /api/admin/cards/{id}
func (s *Server) handleAdminUpdateCard(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad id")
		return
	}
	var in cardInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "bad request")
		return
	}
	if err := in.validate(); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.repo.UpdateCard(id, in.Name, in.RarityID, in.ImageURL, in.PowerLevel, in.SetID); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// --- Rarities ---

// GET /api/admin/rarities — full rarity rows plus sanity warnings, so the editor
// can flag a broken economy (drop chances that don't add up, crafting turned off
// by a zero cost) without blocking the save.
func (s *Server) handleAdminListRarities(w http.ResponseWriter, _ *http.Request) {
	rarities, err := s.repo.GetRarities()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	var dropSum float64
	warnings := make([]string, 0)
	for _, r := range rarities {
		dropSum += r.DropChance
		if r.CraftCost == 0 && !r.RequiresFragments {
			warnings = append(warnings, fmt.Sprintf("«%s»: craft_cost = 0 — крафт из этой редкости отключён", r.Name))
		}
		if r.RequiresFragments && r.FragmentsRequired <= 0 {
			warnings = append(warnings, fmt.Sprintf("«%s»: собирается из осколков, но fragments_required = 0", r.Name))
		}
	}
	if len(rarities) > 0 && (dropSum < 99.0 || dropSum > 101.0) {
		warnings = append(warnings, fmt.Sprintf("Сумма шансов выпадения = %.2f%%, ожидается ~100%%", dropSum))
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"rarities": rarities,
		"warnings": warnings,
		"dropSum":  dropSum,
	})
}

// PUT /api/admin/rarities/{id}
func (s *Server) handleAdminUpdateRarity(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad id")
		return
	}
	var in struct {
		DropChance        float64 `json:"dropChance"`
		BaseReward        int     `json:"baseReward"`
		PityThreshold     int     `json:"pityThreshold"`
		CraftCost         int     `json:"craftCost"`
		RequiresFragments bool    `json:"requiresFragments"`
		FragmentsRequired int     `json:"fragmentsRequired"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "bad request")
		return
	}
	if in.DropChance < 0 || in.BaseReward < 0 || in.PityThreshold < 0 || in.CraftCost < 0 || in.FragmentsRequired < 0 {
		writeErr(w, http.StatusBadRequest, "значения не могут быть отрицательными")
		return
	}
	if err := s.repo.UpdateRarity(id, in.DropChance, in.BaseReward, in.PityThreshold, in.CraftCost, in.RequiresFragments, in.FragmentsRequired); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// --- Sets ---

// GET /api/admin/sets
func (s *Server) handleAdminListSets(w http.ResponseWriter, _ *http.Request) {
	sets, err := s.repo.ListAdminSets()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	writeJSON(w, http.StatusOK, sets)
}

type setInput struct {
	Name         string `json:"name"`
	BuffType     string `json:"buffType"`
	BuffValue    int    `json:"buffValue"`
	RewardPoints int    `json:"rewardPoints"`
}

func (in *setInput) validate() error {
	in.Name = strings.TrimSpace(in.Name)
	in.BuffType = strings.TrimSpace(in.BuffType)
	if in.Name == "" {
		return fmt.Errorf("название сета обязательно")
	}
	if in.BuffType == "" {
		in.BuffType = "power_percent"
	}
	if in.BuffValue < 0 || in.RewardPoints < 0 {
		return fmt.Errorf("значения не могут быть отрицательными")
	}
	return nil
}

// POST /api/admin/sets
func (s *Server) handleAdminCreateSet(w http.ResponseWriter, r *http.Request) {
	var in setInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "bad request")
		return
	}
	if err := in.validate(); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	id, err := s.repo.CreateSet(in.Name, in.BuffType, in.BuffValue, in.RewardPoints)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": id})
}

// PUT /api/admin/sets/{id}
func (s *Server) handleAdminUpdateSet(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad id")
		return
	}
	var in setInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "bad request")
		return
	}
	if err := in.validate(); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.repo.UpdateSet(id, in.Name, in.BuffType, in.BuffValue, in.RewardPoints); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
