package httpapi

import (
	"net/http"
	"strconv"

	"gachabot/internal/service/season"
)

// GET /api/trophies — the caller's own shelf: the medals they hold, the badge
// that goes next to their nickname, and the seasons they sat out.
//
// The gaps are here on purpose and only here: on your own shelf a missing
// season is your history, while on a board other people read it would only be
// a jab. Every other surface shows medals earned and nothing else.
func (s *Server) handleTrophies(w http.ResponseWriter, r *http.Request) {
	uid := userIDFrom(r)

	trophies, missed, err := s.season.Trophies(uid)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "не удалось прочитать трофеи")
		return
	}
	badges, err := s.season.Badges([]int64{uid})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "не удалось прочитать значок")
		return
	}

	out := map[string]any{
		"trophies": trophies,
		"missed":   missed,
		"crown":    s.season.ReigningChampion() == uid,
	}
	if b, ok := badges[uid]; ok {
		out["badge"] = b
	}
	// A season in progress gets a place on the shelf too, so the newest tile is
	// not a hole until it is awarded.
	if cur := s.season.Current(); cur != nil {
		out["current"] = map[string]any{"number": cur.Number, "title": cur.Title}
	}
	writeJSON(w, http.StatusOK, out)
}

// decorate fills in the career bits of a board: the badge beside each nickname
// and the crown of the reigning champion.
func (s *Server) decorate(rows []season.Row) {
	if len(rows) == 0 {
		return
	}
	ids := make([]int64, 0, len(rows))
	for _, r := range rows {
		ids = append(ids, r.UserID)
	}
	badges, err := s.season.Badges(ids)
	if err != nil {
		// A missing badge costs the board nothing; a failed board costs the
		// player the standings.
		return
	}
	champ := s.season.ReigningChampion()
	for i := range rows {
		if b, ok := badges[rows[i].UserID]; ok {
			badge := b
			rows[i].Badge = &badge
		}
		rows[i].Crown = champ != 0 && rows[i].UserID == champ
	}
}

// intQuery reads a small positive number from the query string, or 0.
func intQuery(r *http.Request, key string) int {
	if v := r.URL.Query().Get(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n < 100000 {
			return n
		}
	}
	return 0
}
