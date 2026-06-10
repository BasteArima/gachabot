package httpapi

import (
	"encoding/json"
	"net/http"
	"strconv"
)

// GET /api/artguess — the player's view of today's puzzle.
func (s *Server) handleArtGuess(w http.ResponseWriter, r *http.Request) {
	st, err := s.artguess.GetState(r.Context(), userIDFrom(r))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "artguess error")
		return
	}
	writeJSON(w, http.StatusOK, st)
}

// POST /api/artguess/guess {cardId} — submit a guess, returns the updated state.
func (s *Server) handleArtGuessGuess(w http.ResponseWriter, r *http.Request) {
	var req struct {
		CardID string `json:"cardId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad request")
		return
	}
	cardID, err := strconv.Atoi(req.CardID)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad cardId")
		return
	}
	st, err := s.artguess.Guess(r.Context(), userIDFrom(r), cardID)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, st)
}

// GET /api/artguess/image?level=N — today's art at the requested reveal level,
// clamped server-side to the player's progress.
func (s *Server) handleArtGuessImage(w http.ResponseWriter, r *http.Request) {
	level, _ := strconv.Atoi(r.URL.Query().Get("level"))
	data, err := s.artguess.ImageData(r.Context(), userIDFrom(r), level)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "image error")
		return
	}
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "no-store") // level is per-player; don't let proxies cache
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

// GET /api/cards — id/name/rarity/power for every card, for guess autocomplete.
// Deliberately omits image URLs so the answer can't be looked up.
func (s *Server) handleCards(w http.ResponseWriter, _ *http.Request) {
	cards, err := s.repo.ListCards()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	rarities, err := s.repo.GetRarities()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	rname := make(map[int]string, len(rarities))
	for _, rr := range rarities {
		rname[rr.ID] = rr.Name
	}
	out := make([]cardBriefDTO, 0, len(cards))
	for _, c := range cards {
		out = append(out, cardBriefDTO{
			ID:     strconv.Itoa(c.ID),
			Name:   c.Name,
			Rarity: rname[c.RarityID],
			Power:  c.PowerLevel,
		})
	}
	writeJSON(w, http.StatusOK, out)
}

type cardBriefDTO struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Rarity string `json:"rarity"`
	Power  int    `json:"power"`
}
