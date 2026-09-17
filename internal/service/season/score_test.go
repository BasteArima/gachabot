package season

import (
	"testing"

	"gachabot/internal/models"
)

func stat(id int64, name string, cards, coins int64, days int) models.SeasonStat {
	return models.SeasonStat{UserID: id, Name: name, CardPoints: cards, CoinsEarned: coins, ActiveDays: days}
}

func defaults() Config {
	c := Config{}
	c.Normalize()
	return c
}

func find(rows []Row, id int64) Row {
	for _, r := range rows {
		if r.UserID == id {
			return r
		}
	}
	return Row{}
}

func TestPointsAreTheShareOfTheFieldBeaten(t *testing.T) {
	// Four players, each first in nothing or everything, so the arithmetic is
	// checkable by hand: with four in the field, beating three is 100 points,
	// beating none is 0.
	rows := score([]models.SeasonStat{
		stat(1, "Аня", 400, 400, 40),
		stat(2, "Боря", 300, 300, 30),
		stat(3, "Вика", 200, 200, 20),
		stat(4, "Гоша", 100, 100, 10),
	}, 1, defaults())

	if got := find(rows, 1).Total; got != 300 {
		t.Errorf("первый во всех трёх топах должен набрать 300, а набрал %d", got)
	}
	if got := find(rows, 4).Total; got != 0 {
		t.Errorf("последний во всех трёх должен набрать 0, а набрал %d", got)
	}
	if got := find(rows, 2).Cards.Points; got != 66 {
		t.Errorf("обошедший двоих из трёх должен получить 66, а получил %d", got)
	}
	if got := find(rows, 2).Cards.Place; got != 2 {
		t.Errorf("место не то: %d", got)
	}
}

func TestEqualValuesScoreEqually(t *testing.T) {
	rows := score([]models.SeasonStat{
		stat(1, "Аня", 100, 10, 5),
		stat(2, "Боря", 100, 20, 5),
		stat(3, "Вика", 50, 30, 5),
	}, 1, defaults())

	a, b := find(rows, 1), find(rows, 2)
	if a.Cards.Points != b.Cards.Points || a.Cards.Place != b.Cards.Place {
		t.Errorf("одинаковые очки должны давать одинаковое место и баллы: %+v против %+v", a.Cards, b.Cards)
	}
	if a.Days.Points != 0 {
		t.Errorf("если у всех поровну, никто никого не обошёл — ожидалось 0, вышло %d", a.Days.Points)
	}
}

func TestBelowTheMinimumIsNotRanked(t *testing.T) {
	// Гоша collected the most of anyone but played two days out of the five
	// required: he keeps his numbers and stays out of the ranking.
	rows := score([]models.SeasonStat{
		stat(1, "Аня", 100, 100, 10),
		stat(2, "Боря", 90, 90, 9),
		stat(3, "Гоша", 900, 900, 2),
	}, 5, defaults())

	gosha := find(rows, 3)
	if gosha.Qualified {
		t.Error("игрок ниже минимума не должен попадать в зачёт")
	}
	if gosha.Total != 0 || gosha.Place != 0 || gosha.Tier != "" {
		t.Errorf("вне зачёта не должно быть ни баллов, ни места, ни медали: %+v", gosha)
	}
	if gosha.Cards.Value != 900 {
		t.Errorf("свои цифры игрок видеть должен, а вышло %d", gosha.Cards.Value)
	}
	if find(rows, 1).Cards.Points != 100 {
		t.Error("лидер зачёта должен считаться только среди прошедших минимум")
	}
	if rows[len(rows)-1].UserID != 3 {
		t.Error("незачётные должны стоять в конце списка")
	}
}

func TestSmallFieldHasNoPodium(t *testing.T) {
	rows := score([]models.SeasonStat{
		stat(1, "Аня", 100, 100, 10),
		stat(2, "Боря", 90, 90, 9),
		stat(3, "Вика", 80, 80, 8),
		stat(4, "Гоша", 70, 70, 7),
	}, 1, defaults()) // четверо при пороге пьедестала в пять

	if got := find(rows, 1).Tier; got != TierChampion {
		t.Errorf("чемпион должен быть всегда, а вышло %q", got)
	}
	for _, id := range []int64{2, 3, 4} {
		if got := find(rows, id).Tier; got != TierParticipant {
			t.Errorf("в маленькой компании второе место не медаль, а вышло %q", got)
		}
	}
}

func TestEliteKeepsAtLeastTwoPlaces(t *testing.T) {
	// Ten players at 25 %: a quarter is two and a half places, which would leave
	// Elite empty behind the podium. It must still cover places 4 and 5.
	var stats []models.SeasonStat
	for i := 0; i < 10; i++ {
		stats = append(stats, stat(int64(i+1), "p", int64(100-i), int64(100-i), 10-i/3))
	}
	rows := score(stats, 1, defaults())

	byPlace := map[int]string{}
	for _, r := range rows {
		byPlace[r.Place] = r.Tier
	}
	want := map[int]string{1: TierChampion, 2: TierSilver, 3: TierBronze, 4: TierElite, 5: TierElite, 6: TierParticipant}
	for place, tier := range want {
		if byPlace[place] != tier {
			t.Errorf("место %d: ожидалось %q, вышло %q", place, tier, byPlace[place])
		}
	}
}

func TestNextGoalCountsDaysThenPoints(t *testing.T) {
	rows := score([]models.SeasonStat{
		stat(1, "Аня", 100, 100, 10),
		stat(2, "Боря", 90, 90, 9),
		stat(3, "Вика", 80, 80, 8),
		stat(4, "Гоша", 70, 70, 7),
		stat(5, "Дима", 60, 60, 6),
		stat(6, "Ева", 50, 50, 1),
	}, 3, defaults())

	// Ева played one day of the three needed.
	goal := NextGoal(rows, 6, 3, 1)
	if goal.Tier != TierParticipant || goal.DaysLeft != 2 {
		t.Errorf("непрошедшему минимум надо показывать дни: %+v", goal)
	}

	// Дима is last of the ranked; the tier above him starts with Вика.
	goal = NextGoal(rows, 5, 3, 6)
	if goal.PointsGap <= 0 || goal.Reached {
		t.Errorf("до следующей медали должен быть положительный разрыв: %+v", goal)
	}

	if g := NextGoal(rows, 1, 3, 10); !g.Reached || g.Tier != TierChampion {
		t.Errorf("лидеру уже некуда расти: %+v", g)
	}
}
