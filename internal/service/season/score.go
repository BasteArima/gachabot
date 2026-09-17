package season

import (
	"sort"

	"gachabot/internal/models"
)

// Scoring. Three categories cannot simply be added up: coins run into tens of
// thousands and active days into dozens, so the raw numbers would make the coin
// table the only one that mattered. Each category is therefore scored by where a
// player stands in it — 100 × the share of the field they beat — which puts all
// three on the same 0..100 scale whatever the units. The three add up to 0..300.
//
// Only players who met the activity minimum are ranked against each other. The
// rest still see their own numbers; they just aren't competing yet.

const maxCategoryPoints = 100

// CategoryScore is one player's standing in one category.
type CategoryScore struct {
	Value  int64 `json:"value"`
	Place  int   `json:"place"`
	Points int   `json:"points"`
}

// Row is a player's line in the season standings.
type Row struct {
	UserID     int64         `json:"userId"`
	Name       string        `json:"name"`
	Cards      CategoryScore `json:"cards"`
	Coins      CategoryScore `json:"coins"`
	Days       CategoryScore `json:"days"`
	Total      int           `json:"total"`
	Place      int           `json:"place"`
	Tier       string        `json:"tier"`
	Qualified  bool          `json:"qualified"`
	CardsCount int           `json:"cardsCount"`
	BestDrop   string        `json:"bestDrop"`
	// Career, not this season: the best medal this player ever earned, and the
	// crown of the reigning champion. Filled in by the delivery layer, which
	// knows whether the caller is allowed to see the board at all.
	Badge *models.Badge `json:"badge,omitempty"`
	Crown bool          `json:"crown,omitempty"`
}

// Goal is what a player is next working towards: either the activity minimum
// that unlocks a medal at all, or the points that would lift them a tier.
type Goal struct {
	Tier      string `json:"tier"`
	DaysLeft  int    `json:"daysLeft,omitempty"`
	PointsGap int    `json:"pointsGap,omitempty"`
	Reached   bool   `json:"reached"`
}

// Scoreboard turns the raw counters into ranked rows. It also returns how many
// active days a medal currently needs, which is what unranked players are short
// of.
func (s *Service) Scoreboard() ([]Row, int, error) {
	cur := s.Current()
	if cur == nil {
		return nil, 0, nil
	}
	stats, err := s.repo.SeasonStandings(cur.ID)
	if err != nil {
		return nil, 0, err
	}
	_, minActive := s.Days()
	return score(stats, minActive, s.CurrentConfig()), minActive, nil
}

func score(stats []models.SeasonStat, minActive int, cfg Config) []Row {
	rows := make([]Row, 0, len(stats))
	qualified := make([]models.SeasonStat, 0, len(stats))
	for _, st := range stats {
		if st.ActiveDays >= minActive {
			qualified = append(qualified, st)
		}
	}

	for _, st := range stats {
		row := Row{
			UserID:     st.UserID,
			Name:       st.Name,
			CardsCount: st.CardsCount,
			BestDrop:   st.BestDropName,
			Qualified:  st.ActiveDays >= minActive,
			Cards:      CategoryScore{Value: st.CardPoints},
			Coins:      CategoryScore{Value: st.CoinsEarned},
			Days:       CategoryScore{Value: int64(st.ActiveDays)},
		}
		if row.Qualified {
			row.Cards = rank(qualified, st.CardPoints, func(o models.SeasonStat) int64 { return o.CardPoints })
			row.Coins = rank(qualified, st.CoinsEarned, func(o models.SeasonStat) int64 { return o.CoinsEarned })
			row.Days = rank(qualified, int64(st.ActiveDays), func(o models.SeasonStat) int64 { return int64(o.ActiveDays) })
			row.Total = row.Cards.Points + row.Coins.Points + row.Days.Points
		}
		rows = append(rows, row)
	}

	// Ranked players first, best total on top. Everyone else keeps their numbers
	// but sits below, ordered by what they have collected.
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].Qualified != rows[j].Qualified {
			return rows[i].Qualified
		}
		if rows[i].Total != rows[j].Total {
			return rows[i].Total > rows[j].Total
		}
		return rows[i].Cards.Value > rows[j].Cards.Value
	})

	assignTiers(rows, len(qualified), cfg)
	return rows
}

// rank scores one value against the qualified field: the share of players it
// beats, as 0..100. Equal values score equally.
func rank(field []models.SeasonStat, value int64, get func(models.SeasonStat) int64) CategoryScore {
	if len(field) < 2 {
		return CategoryScore{Value: value, Place: 1, Points: maxCategoryPoints}
	}
	lower, higher := 0, 0
	for _, o := range field {
		switch v := get(o); {
		case v < value:
			lower++
		case v > value:
			higher++
		}
	}
	return CategoryScore{
		Value:  value,
		Place:  higher + 1,
		Points: lower * maxCategoryPoints / (len(field) - 1),
	}
}

// assignTiers hands out places and medal tiers. A podium needs a field to be a
// podium — below the configured minimum only the champion is singled out — and
// Elite always covers at least two places behind the podium so it cannot vanish
// in a small group.
func assignTiers(rows []Row, qualified int, cfg Config) {
	eliteEnd := qualified * cfg.ElitePercent / 100
	if eliteEnd < 5 {
		eliteEnd = 5
	}
	podium := qualified >= cfg.PodiumMinPlayers

	place := 0
	for i := range rows {
		if !rows[i].Qualified {
			continue
		}
		place++
		rows[i].Place = place
		switch {
		case place == 1:
			rows[i].Tier = TierChampion
		case !podium:
			rows[i].Tier = TierParticipant
		case place == 2:
			rows[i].Tier = TierSilver
		case place == 3:
			rows[i].Tier = TierBronze
		case place <= eliteEnd:
			rows[i].Tier = TierElite
		default:
			rows[i].Tier = TierParticipant
		}
	}
}

// NextGoal describes what the given player is working towards. Unranked players
// are told how many days they still owe; ranked ones, how many points separate
// them from the next tier up.
func NextGoal(rows []Row, userID int64, minActive int, activeDays int) *Goal {
	var me *Row
	for i := range rows {
		if rows[i].UserID == userID {
			me = &rows[i]
			break
		}
	}
	if me == nil || !me.Qualified {
		left := minActive - activeDays
		if left < 0 {
			left = 0
		}
		return &Goal{Tier: TierParticipant, DaysLeft: left}
	}
	if me.Tier == TierChampion {
		return &Goal{Tier: TierChampion, Reached: true}
	}

	// The nearest player above whose tier is better than mine: passing them is
	// what actually changes the medal, and their total is the bar to clear.
	for i := len(rows) - 1; i >= 0; i-- {
		r := rows[i]
		if !r.Qualified || r.Place >= me.Place || r.Tier == me.Tier {
			continue
		}
		gap := r.Total - me.Total + 1
		if gap < 1 {
			gap = 1
		}
		return &Goal{Tier: r.Tier, PointsGap: gap}
	}
	return &Goal{Tier: me.Tier, Reached: true}
}
