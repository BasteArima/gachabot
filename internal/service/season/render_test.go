package season

import (
	"strings"
	"testing"
	"time"

	"gachabot/internal/models"
)

func TestBadgeTextIsOneGlyphAndACount(t *testing.T) {
	if got := BadgeText(TierChampion, 1); got != "🥇" {
		t.Errorf("одна медаль — без числа, а вышло %q", got)
	}
	if got := BadgeText(TierChampion, 10); got != "🥇¹⁰" {
		t.Errorf("десять чемпионств должны стать 🥇¹⁰, а вышло %q", got)
	}
	if got := BadgeText("", 3); got != "" {
		t.Errorf("без медали значка быть не должно, а вышло %q", got)
	}
}

func TestBoardTextAddsYourLineWhenYouAreBelowTheCut(t *testing.T) {
	var stats []models.SeasonStat
	for i := 0; i < 12; i++ {
		stats = append(stats, stat(int64(i+1), "Игрок", int64(100-i), int64(100-i), 10))
	}
	rows := score(stats, 1, defaults())

	// Player 12 is last, well past the seven lines a message shows.
	text := BoardText(3, "Третий", 10, 20, rows, 12)

	if strings.Count(text, "\n1. ") > 1 {
		t.Error("первая строка должна быть одна")
	}
	if !strings.Contains(text, "Ты: 12 место") {
		t.Errorf("своя строка должна добавляться отдельно:\n%s", text)
	}
	if lines := strings.Count(text, ". "); lines > chatBoardLines+2 {
		t.Errorf("таблица в чате не должна разрастаться:\n%s", text)
	}
}

func TestBoardTextTellsUnrankedWhatTheyLack(t *testing.T) {
	rows := score([]models.SeasonStat{
		stat(1, "Аня", 100, 100, 10),
		stat(2, "Боря", 90, 90, 9),
		stat(3, "Дима", 10, 10, 2),
	}, 5, defaults())

	text := BoardText(1, "", 20, 5, rows, 3)

	if !strings.Contains(text, "вне зачёта") || !strings.Contains(text, "2 дня") {
		t.Errorf("непрошедшему минимум надо показать, чего не хватает:\n%s", text)
	}
}

func TestFinishTextNamesThePodiumAndCountsTheRest(t *testing.T) {
	var stats []models.SeasonStat
	names := []string{"Аня", "Боря", "Вика", "Гоша", "Ева", "Дима", "Женя"}
	for i, n := range names {
		stats = append(stats, stat(int64(i+1), n, int64(100-i*10), int64(100-i*10), 10-i))
	}
	rows := score(stats, 1, defaults())
	ends := time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)

	text := FinishText(1, "Первый", rows, 2, &ends)

	for _, want := range []string{"🥇 Аня", "🥈 Боря", "🥉 Вика", "🏅 Элита:", "Сезон 2 идёт до 31 декабря"} {
		if !strings.Contains(text, want) {
			t.Errorf("в объявлении не хватает %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "🎖 Участники: ещё 0") {
		t.Error("пустую строку про участников писать не нужно")
	}
}

func TestShelfTextShowsOnlyEarnedMedals(t *testing.T) {
	trophies := []Trophy{
		{SeasonNumber: 4, Place: 1, Tier: TierChampion, Detail: Detail{Players: 18}},
		{SeasonNumber: 1, Place: 7, Tier: TierParticipant, Detail: Detail{Players: 14}},
	}

	text := ShelfText("Baste", trophies, BadgeText(TierChampion, 2), true)

	if !strings.Contains(text, "👑") {
		t.Error("действующего чемпиона надо отметить короной")
	}
	if !strings.Contains(text, "Сезон 4 — 1 место из 18") {
		t.Errorf("медаль должна читаться целиком:\n%s", text)
	}
	if strings.Contains(text, "пропущен") {
		t.Error("в чате пропуски не показываются")
	}
}

func TestTrophyTextKeepsTheFrozenNumbers(t *testing.T) {
	tr := Trophy{
		SeasonNumber: 2,
		SeasonTitle:  "Второй",
		StartedAt:    time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC),
		FinishedAt:   time.Date(2026, 11, 30, 12, 0, 0, 0, time.UTC),
		Place:        2,
		Points:       248,
		Tier:         TierSilver,
		Detail: Detail{
			Cards:    CategoryScore{Value: 1940, Place: 3},
			Coins:    CategoryScore{Value: 24600, Place: 1},
			Days:     CategoryScore{Value: 51, Place: 2},
			BestDrop: "Жрица рассвета",
			Players:  16,
		},
	}

	text := TrophyText(tr)

	// "Баллы" is the season score; the collection has points of its own, and the
	// two must not read as the same number.
	if strings.Contains(text, "1940 баллов") {
		t.Errorf("очки коллекции нельзя называть баллами:\n%s", text)
	}
	for _, want := range []string{"🥈 Серебро · сезон 2", "1 октября — 30 ноября", "2 место из 16", "1940 очков", "24600", "Жрица рассвета"} {
		if !strings.Contains(text, want) {
			t.Errorf("в медали не хватает %q:\n%s", want, text)
		}
	}
}

func TestNameWithBadgeOrdersTheParts(t *testing.T) {
	if got := NameWithBadge("🥇²", true, "Baste"); got != "🥇² Baste 👑" {
		t.Errorf("медаль перед именем, корона после: %q", got)
	}
	if got := NameWithBadge("", false, "Новичок"); got != "Новичок" {
		t.Errorf("без медалей имя не украшается: %q", got)
	}
	if got := NameWithBadge("", true, "Чемпион"); got != "Чемпион 👑" {
		t.Errorf("корона может быть и без значка: %q", got)
	}
	// The name is escaped by the caller, and the composition must not touch it.
	if got := NameWithBadge("🎖", false, "&lt;b&gt;"); got != "🎖 &lt;b&gt;" {
		t.Errorf("экранированное имя должно дойти целым: %q", got)
	}
}
