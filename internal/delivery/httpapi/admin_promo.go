package httpapi

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"

	"gachabot/internal/models"

	"github.com/go-chi/chi/v5"
)

// GET /api/admin/promo — every promo code with its usage counters.
func (s *Server) handleAdminListPromo(w http.ResponseWriter, _ *http.Request) {
	promos, err := s.repo.ListPromoCodes()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	writeJSON(w, http.StatusOK, promos)
}

// POST /api/admin/promo — create a code. Existing codes are not silently
// overwritten from the web UI (the chat command's upsert is easy to do by
// accident); delete and recreate instead.
func (s *Server) handleAdminCreatePromo(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Code         string         `json:"code"`
		Points       int            `json:"points"`
		PremiumRolls int            `json:"premiumRolls"`
		Cards        []int          `json:"cards"`
		RandomCards  map[string]int `json:"randomCards"`
		MaxUses      int            `json:"maxUses"`   // 0 = unlimited
		ExpiresHours int            `json:"expiresIn"` // hours from now; 0 = never
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "bad request")
		return
	}

	code := strings.ToUpper(strings.TrimSpace(in.Code))
	if code == "" {
		writeErr(w, http.StatusBadRequest, "укажи код")
		return
	}
	if in.Points < 0 || in.PremiumRolls < 0 || in.MaxUses < 0 || in.ExpiresHours < 0 {
		writeErr(w, http.StatusBadRequest, "значения не могут быть отрицательными")
		return
	}
	if in.Points == 0 && in.PremiumRolls == 0 && len(in.Cards) == 0 && len(in.RandomCards) == 0 {
		writeErr(w, http.StatusBadRequest, "промокод должен что-то выдавать")
		return
	}
	exists, err := s.repo.PromoExists(code)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	if exists {
		writeErr(w, http.StatusBadRequest, "промокод "+code+" уже существует — удали его, если нужно пересоздать")
		return
	}

	reward := models.PromoReward{
		Points:       in.Points,
		PremiumRolls: in.PremiumRolls,
		Cards:        in.Cards,
		RandomCards:  in.RandomCards,
	}
	var maxUses *int
	if in.MaxUses > 0 {
		maxUses = &in.MaxUses
	}
	var expiresAt *time.Time
	if in.ExpiresHours > 0 {
		t := time.Now().Add(time.Duration(in.ExpiresHours) * time.Hour)
		expiresAt = &t
	}

	if err := s.repo.CreatePromoCode(code, reward, maxUses, expiresAt); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	log.Printf("[ADMIN] promo created: %s (points=%d rolls=%d maxUses=%d)", code, in.Points, in.PremiumRolls, in.MaxUses)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "code": code})
}

// DELETE /api/admin/promo/{code}
func (s *Server) handleAdminDeletePromo(w http.ResponseWriter, r *http.Request) {
	code := chi.URLParam(r, "code")
	if err := s.repo.DeletePromoCode(code); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	log.Printf("[ADMIN] promo deleted: %s", strings.ToUpper(code))
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
