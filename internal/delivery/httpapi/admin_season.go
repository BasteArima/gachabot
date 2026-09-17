package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"time"

	"gachabot/internal/service/season"
)

// Season administration. The whole cycle is meant to live here rather than in
// env vars: a season is started, extended, re-started and (later) finished from
// the panel, because all of that happens while the bot is running.

// msk matches the fixed zone the game services use, so "31 декабря" means the
// end of that day as players experience it.
var msk = time.FixedZone("MSK", 3*60*60)

type seasonConfigDTO struct {
	Weights            map[string]int `json:"weights"` // rarity id (as text) -> points
	ElitePercent       int            `json:"elitePercent"`
	MinActivityPercent int            `json:"minActivityPercent"`
	PodiumMinPlayers   int            `json:"podiumMinPlayers"`
}

func configToDTO(c season.Config) seasonConfigDTO {
	w := make(map[string]int, len(c.Weights))
	for id, v := range c.Weights {
		w[strconv.Itoa(id)] = v
	}
	return seasonConfigDTO{
		Weights:            w,
		ElitePercent:       c.ElitePercent,
		MinActivityPercent: c.MinActivityPercent,
		PodiumMinPlayers:   c.PodiumMinPlayers,
	}
}

func (d seasonConfigDTO) toConfig() season.Config {
	c := season.Config{
		Weights:            make(map[int]int, len(d.Weights)),
		ElitePercent:       d.ElitePercent,
		MinActivityPercent: d.MinActivityPercent,
		PodiumMinPlayers:   d.PodiumMinPlayers,
	}
	for k, v := range d.Weights {
		if id, err := strconv.Atoi(k); err == nil {
			c.Weights[id] = v
		}
	}
	c.Normalize()
	return c
}

// GET /api/admin/season — the running season, its rules, and what has been
// counted so far.
func (s *Server) handleAdminSeason(w http.ResponseWriter, _ *http.Request) {
	cur := s.season.Current()
	out := map[string]any{"season": nil}

	if cur != nil {
		elapsed, minActive := s.season.Days()
		out["season"] = map[string]any{
			"id":        cur.ID,
			"number":    cur.Number,
			"title":     cur.Title,
			"startedAt": cur.StartedAt,
			"endsAt":    dateOrEmpty(cur.EndsAt),
			"days":      elapsed,
			"minActive": minActive,
		}
		out["config"] = configToDTO(s.season.CurrentConfig())

		stats, err := s.season.Standings()
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "не удалось прочитать счёт сезона")
			return
		}
		rows := make([]map[string]any, 0, len(stats))
		for _, st := range stats {
			rows = append(rows, map[string]any{
				"userId":      st.UserID,
				"name":        st.Name,
				"coinsEarned": st.CoinsEarned,
				"cardPoints":  st.CardPoints,
				"cardsCount":  st.CardsCount,
				"activeDays":  st.ActiveDays,
				"bestStreak":  st.BestStreak,
				"bestDrop":    st.BestDropName,
			})
		}
		out["standings"] = rows
	}

	// Rarity names travel with the weights: the config keys are ids, and a list
	// of numbers is not something anyone can tune.
	rarities, err := s.repo.GetRarities()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "не удалось прочитать редкости")
		return
	}
	rs := make([]map[string]any, 0, len(rarities))
	for _, r := range rarities {
		rs = append(rs, map[string]any{"id": r.ID, "name": r.Name, "dropChance": r.DropChance})
	}
	out["rarities"] = rs

	writeJSON(w, http.StatusOK, out)
}

// POST /api/admin/season — start a season.
func (s *Server) handleAdminSeasonStart(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Title  string `json:"title"`
		EndsAt string `json:"endsAt"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 8<<10)).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "плохой json")
		return
	}
	ends, err := parseEndsAt(in.EndsAt)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if s.season.Current() != nil {
		writeErr(w, http.StatusConflict, "сезон уже идёт")
		return
	}
	if _, err := s.season.Start(in.Title, ends); err != nil {
		writeErr(w, http.StatusInternalServerError, "не удалось начать сезон")
		return
	}
	s.handleAdminSeason(w, r)
}

// PUT /api/admin/season — rename, move the target date, retune the scoring.
func (s *Server) handleAdminSeasonUpdate(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Title  string          `json:"title"`
		EndsAt string          `json:"endsAt"`
		Config seasonConfigDTO `json:"config"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 32<<10)).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "плохой json")
		return
	}
	ends, err := parseEndsAt(in.EndsAt)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.season.Update(in.Title, ends, in.Config.toConfig()); err != nil {
		if errors.Is(err, season.ErrNoSeason) {
			writeErr(w, http.StatusNotFound, err.Error())
			return
		}
		writeErr(w, http.StatusInternalServerError, "не удалось сохранить сезон")
		return
	}
	s.handleAdminSeason(w, r)
}

// POST /api/admin/season/restart — wipe what has been counted and start the
// clock again, for when the run-up was only a rehearsal.
func (s *Server) handleAdminSeasonRestart(w http.ResponseWriter, r *http.Request) {
	if err := s.season.Restart(); err != nil {
		if errors.Is(err, season.ErrNoSeason) {
			writeErr(w, http.StatusNotFound, err.Error())
			return
		}
		writeErr(w, http.StatusInternalServerError, "не удалось обнулить счёт")
		return
	}
	s.handleAdminSeason(w, r)
}

// parseEndsAt accepts a date from the panel's date field ("2026-12-31", meaning
// the end of that day in MSK) or a full timestamp; empty means "no target yet".
func parseEndsAt(raw string) (*time.Time, error) {
	if raw == "" {
		return nil, nil
	}
	if t, err := time.Parse(time.RFC3339, raw); err == nil {
		return &t, nil
	}
	d, err := time.ParseInLocation("2006-01-02", raw, msk)
	if err != nil {
		return nil, errors.New("дата должна быть в формате ГГГГ-ММ-ДД")
	}
	end := d.Add(24*time.Hour - time.Second)
	return &end, nil
}

func dateOrEmpty(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.In(msk).Format("2006-01-02")
}
