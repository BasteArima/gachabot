package httpapi

import (
	"net/http"
	"time"

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
			"daysLeft":      daysLeft(cur.EndsAt),
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

// daysLeft counts whole days to the target date, or -1 when no date is set —
// the season then runs until it is finished by hand, and the app shows no
// countdown rather than a made-up one.
func daysLeft(endsAt *time.Time) int {
	if endsAt == nil {
		return -1
	}
	now := time.Now().In(msk)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, msk)
	end := endsAt.In(msk)
	endDay := time.Date(end.Year(), end.Month(), end.Day(), 0, 0, 0, 0, msk)
	d := int(endDay.Sub(today).Hours() / 24)
	if d < 0 {
		return 0
	}
	return d
}
