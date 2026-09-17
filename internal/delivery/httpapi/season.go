package httpapi

import (
	"net/http"

	"gachabot/internal/service/season"
)

// How many lines the standings send. The whole board is a couple of kilobytes
// for a group of friends, but this server has a hard ceiling on what it can
// deliver, so a board that grows is cut — with the reader's own line always
// added back, since a player scrolling past a hundred names still wants to know
// where they stand.
const seasonBoardLimit = 100

// GET /api/season — the running season, the standings, and where the caller
// stands in them. One response serves every tab: each row already carries its
// points and place in all three categories, so switching tabs is a re-sort on
// the client rather than another round trip.
func (s *Server) handleSeason(w http.ResponseWriter, r *http.Request) {
	cur := s.season.Current()
	if cur == nil {
		writeJSON(w, http.StatusOK, map[string]any{"season": nil})
		return
	}

	rows, minActive, err := s.season.Scoreboard()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "не удалось прочитать счёт сезона")
		return
	}
	elapsed, _ := s.season.Days()
	s.season.Decorate(rows)

	uid := userIDFrom(r)
	var me *season.Row
	for i := range rows {
		if rows[i].UserID == uid {
			me = &rows[i]
			break
		}
	}
	activeDays := 0
	if me != nil {
		activeDays = int(me.Days.Value)
	}

	board := rows
	if len(board) > seasonBoardLimit {
		board = append([]season.Row(nil), rows[:seasonBoardLimit]...)
		if me != nil && me.Place > seasonBoardLimit {
			board = append(board, *me)
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"season": map[string]any{
			"number":        cur.Number,
			"title":         cur.Title,
			"endsAt":        dateOrEmpty(cur.EndsAt),
			"days":          elapsed,
			"daysLeft":      s.season.DaysLeft(),
			"minActiveDays": minActive,
			"players":       countQualified(rows),
		},
		"me":   me,
		"goal": season.NextGoal(rows, uid, minActive, activeDays),
		"rows": board,
	})
}

func countQualified(rows []season.Row) int {
	n := 0
	for _, r := range rows {
		if r.Qualified {
			n++
		}
	}
	return n
}
