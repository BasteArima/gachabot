package season

import (
	"encoding/json"
	"log"
	"time"

	"gachabot/internal/models"
)

// Finishing a season and everything that survives it: the medals, the trophy
// shelf, and the badge next to a nickname.

// tierOrder ranks the medals so "best one earned" has a meaning.
var tierOrder = map[string]int{
	TierParticipant: 1,
	TierElite:       2,
	TierBronze:      3,
	TierSilver:      4,
	TierChampion:    5,
}

// Detail is the frozen copy of a player's numbers, stored with their medal.
// A trophy opened a year later shows what was true when it was won, even if
// rarities, weights or the rest of the field have changed since.
type Detail struct {
	Cards      CategoryScore `json:"cards"`
	Coins      CategoryScore `json:"coins"`
	Days       CategoryScore `json:"days"`
	CardsCount int           `json:"cardsCount"`
	BestDrop   string        `json:"bestDrop"`
	Players    int           `json:"players"`
}

// Trophy is one medal on the shelf.
type Trophy struct {
	SeasonID     int       `json:"seasonId"`
	SeasonNumber int       `json:"seasonNumber"`
	SeasonTitle  string    `json:"seasonTitle"`
	StartedAt    time.Time `json:"startedAt"`
	FinishedAt   time.Time `json:"finishedAt"`
	Place        int       `json:"place"`
	Points       int       `json:"points"`
	Tier         string    `json:"tier"`
	Detail       Detail    `json:"detail"`
}

// Preview scores the season as it stands without changing anything — the same
// computation Finish would commit. minActiveOverride lets the panel try a
// different activity minimum before settling on one; zero means "use the
// season's own rule".
func (s *Service) Preview(minActiveOverride int) ([]Row, int, error) {
	cur := s.Current()
	if cur == nil {
		return nil, 0, ErrNoSeason
	}
	stats, err := s.repo.SeasonStandings(cur.ID)
	if err != nil {
		return nil, 0, err
	}
	_, minActive := s.Days()
	if minActiveOverride > 0 {
		minActive = minActiveOverride
	}
	return score(stats, minActive, s.CurrentConfig()), minActive, nil
}

// Finish awards the medals and closes the season. Only ranked players get a
// result row: a medal everyone receives says nothing, and someone who took two
// cards and left has a season record, not a trophy.
//
// It is deliberately not automatic. The owner presses the button after looking
// at the preview, because handing out medals cannot be undone.
func (s *Service) Finish(minActiveOverride int) ([]Row, error) {
	cur := s.Current()
	if cur == nil {
		return nil, ErrNoSeason
	}
	rows, _, err := s.Preview(minActiveOverride)
	if err != nil {
		return nil, err
	}

	results := make([]models.SeasonResult, 0, len(rows))
	players := 0
	for _, r := range rows {
		if r.Qualified {
			players++
		}
	}
	for _, r := range rows {
		if !r.Qualified {
			continue
		}
		detail, err := json.Marshal(Detail{
			Cards:      r.Cards,
			Coins:      r.Coins,
			Days:       r.Days,
			CardsCount: r.CardsCount,
			BestDrop:   r.BestDrop,
			Players:    players,
		})
		if err != nil {
			return nil, err
		}
		results = append(results, models.SeasonResult{
			UserID: r.UserID,
			Place:  r.Place,
			Points: r.Total,
			Tier:   r.Tier,
			Detail: detail,
		})
	}

	if err := s.repo.FinishSeason(cur.ID, results); err != nil {
		return nil, err
	}
	log.Printf("[SEASON] сезон %d завершён, выдано медалей: %d", cur.Number, len(results))
	if err := s.reload(); err != nil {
		return rows, err
	}
	return rows, nil
}

// Trophies returns a player's shelf: their medals, plus the finished seasons
// they have none for. A gap is part of the history — and it is only ever shown
// to the player themselves.
func (s *Service) Trophies(userID int64) ([]Trophy, []int, error) {
	results, err := s.repo.TrophiesOf(userID)
	if err != nil {
		return nil, nil, err
	}
	out := make([]Trophy, 0, len(results))
	won := map[int]bool{}
	for _, r := range results {
		var d Detail
		if len(r.Detail) > 0 {
			if err := json.Unmarshal(r.Detail, &d); err != nil {
				log.Printf("[SEASON] битые детали медали сезона %d: %v", r.SeasonNumber, err)
			}
		}
		won[r.SeasonNumber] = true
		out = append(out, Trophy{
			SeasonID:     r.SeasonID,
			SeasonNumber: r.SeasonNumber,
			SeasonTitle:  r.SeasonTitle,
			StartedAt:    r.StartedAt,
			FinishedAt:   r.FinishedAt,
			Place:        r.Place,
			Points:       r.Points,
			Tier:         r.Tier,
			Detail:       d,
		})
	}

	finished, err := s.repo.FinishedSeasons()
	if err != nil {
		return out, nil, err
	}
	var missed []int
	for _, f := range finished {
		if !won[f.Number] {
			missed = append(missed, f.Number)
		}
	}
	return out, missed, nil
}

// Badges picks what goes next to each nickname: the best medal a player ever
// earned and how many of that one they hold. Ten championships read as one
// icon and a small number, rather than a row of ten.
func (s *Service) Badges(userIDs []int64) (map[int64]models.Badge, error) {
	counts, err := s.repo.SeasonBadgeCounts(userIDs)
	if err != nil {
		return nil, err
	}
	out := make(map[int64]models.Badge, len(counts))
	for uid, byTier := range counts {
		if best := bestBadge(byTier); best.Tier != "" {
			out[uid] = best
		}
	}
	return out, nil
}

// bestBadge picks the highest tier a player holds, and how many of it. Counting
// the lower medals too would turn the badge into a scoreboard of its own; the
// whole shelf is one tap away for that.
func bestBadge(byTier map[string]int) models.Badge {
	best := models.Badge{}
	for tier, n := range byTier {
		if tierOrder[tier] > tierOrder[best.Tier] {
			best = models.Badge{Tier: tier, Count: n}
		}
	}
	return best
}

// ReigningChampion is who wears the crown right now: the winner of the last
// season that finished. It is 0 until a season has been awarded.
func (s *Service) ReigningChampion() int64 {
	uid, err := s.repo.ReigningChampion()
	if err != nil {
		log.Printf("[SEASON] не удалось определить действующего чемпиона: %v", err)
		return 0
	}
	return uid
}

// DaysLeft counts whole days to the target date, or -1 when none is set: the
// season then runs until it is finished by hand, and the interfaces show no
// countdown rather than inventing one.
func (s *Service) DaysLeft() int {
	cur := s.Current()
	if cur == nil || cur.EndsAt == nil {
		return -1
	}
	end := cur.EndsAt.In(s.loc)
	endDay := time.Date(end.Year(), end.Month(), end.Day(), 0, 0, 0, 0, s.loc)
	d := int(endDay.Sub(s.today()).Hours() / 24)
	if d < 0 {
		return 0
	}
	return d
}

// Decorate fills in the career bits of a board — the badge beside each nickname
// and the reigning champion's crown. Failure is silent on purpose: a missing
// badge costs the board nothing, while a failed board costs the player the
// standings.
func (s *Service) Decorate(rows []Row) {
	if len(rows) == 0 {
		return
	}
	ids := make([]int64, 0, len(rows))
	for _, r := range rows {
		ids = append(ids, r.UserID)
	}
	badges, err := s.Badges(ids)
	if err != nil {
		log.Printf("[SEASON] значки не прочитаны: %v", err)
		return
	}
	champ := s.ReigningChampion()
	for i := range rows {
		if bdg, ok := badges[rows[i].UserID]; ok {
			badge := bdg
			rows[i].Badge = &badge
		}
		rows[i].Crown = champ != 0 && rows[i].UserID == champ
	}
}
