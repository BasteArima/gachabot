package season

import (
	"fmt"
	"strings"
	"time"
)

// Text for chats. Telegram and Discord get the same words — only the markup
// differs, and these strings carry none, so each delivery layer wraps them the
// way its platform wants.
//
// Medals are emoji here because a chat has nothing else to draw with. The app
// draws them properly; this is the same information at the resolution a message
// allows.

// chatBoardLines is how much of the standings fits in a message before it turns
// into a wall. The reader's own line is added below when they are further down.
const chatBoardLines = 7

var tierEmoji = map[string]string{
	TierChampion:    "🥇",
	TierSilver:      "🥈",
	TierBronze:      "🥉",
	TierElite:       "🏅",
	TierParticipant: "🎖",
}

var tierName = map[string]string{
	TierChampion:    "Чемпион",
	TierSilver:      "Серебро",
	TierBronze:      "Бронза",
	TierElite:       "Элита",
	TierParticipant: "Участник",
}

const superscripts = "⁰¹²³⁴⁵⁶⁷⁸⁹"

// BadgeText is what goes in front of a nickname in a message: the best medal a
// player holds and, past the first, how many. One glyph and a small number,
// rather than a row of medals.
func BadgeText(tier string, count int) string {
	e := tierEmoji[tier]
	if e == "" {
		return ""
	}
	if count <= 1 {
		return e
	}
	var sup strings.Builder
	for _, d := range fmt.Sprint(count) {
		sup.WriteString(string([]rune(superscripts)[d-'0']))
	}
	return e + sup.String()
}

// TierName is the medal's name in Russian, for messages that spell it out.
func TierName(tier string) string { return tierName[tier] }

// BoardText renders the season standings for a chat: where the season is, the
// top of the table, and — if they are below it — the reader's own line.
func BoardText(number int, title string, days, daysLeft int, rows []Row, meID int64) string {
	var b strings.Builder

	fmt.Fprintf(&b, "🏆 Сезон %d", number)
	if title != "" {
		fmt.Fprintf(&b, " · %s", title)
	}
	if daysLeft >= 0 {
		fmt.Fprintf(&b, " · осталось %d %s\n\n", daysLeft, pluralDays(daysLeft))
	} else {
		fmt.Fprintf(&b, " · идёт %d %s\n\n", days, pluralDays(days))
	}

	if len(rows) == 0 {
		b.WriteString("Счёт пока пустой — первая же крутка попадёт сюда.")
		return b.String()
	}

	shown := 0
	var me *Row
	for i := range rows {
		if rows[i].UserID == meID {
			me = &rows[i]
		}
		if !rows[i].Qualified || shown >= chatBoardLines {
			continue
		}
		shown++
		fmt.Fprintf(&b, "%d. %s%s — %d\n", rows[i].Place, badgePrefix(rows[i]), rows[i].Name, rows[i].Total)
	}
	if shown == 0 {
		b.WriteString("В зачёте пока никого — не хватает дней активности.\n")
	}

	if me != nil && (me.Place == 0 || me.Place > shown) {
		b.WriteString("\n")
		if me.Qualified {
			fmt.Fprintf(&b, "Ты: %d место, %d %s", me.Place, me.Total, pluralPoints(me.Total))
		} else {
			fmt.Fprintf(&b, "Ты пока вне зачёта: %d %s активности", me.Days.Value, pluralDays(int(me.Days.Value)))
		}
	}
	return b.String()
}

// badgePrefix is the career mark in front of a name: the best medal ever earned
// and, for the reigning champion, the crown.
func badgePrefix(r Row) string {
	var parts []string
	if r.Badge != nil {
		if t := BadgeText(r.Badge.Tier, r.Badge.Count); t != "" {
			parts = append(parts, t)
		}
	}
	if r.Crown {
		parts = append(parts, "👑")
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, "") + " "
}

// ShelfText renders a player's trophies for a chat. Unlike the app, a chat shelf
// shows only what was earned: the gaps are a private matter.
func ShelfText(name string, trophies []Trophy, badge string, crown bool) string {
	var b strings.Builder

	fmt.Fprintf(&b, "🏆 Трофеи · %s", name)
	if crown {
		b.WriteString(" 👑")
	}
	b.WriteString("\n")
	if badge != "" {
		fmt.Fprintf(&b, "Лучшая медаль: %s\n", badge)
	}
	b.WriteString("\n")

	if len(trophies) == 0 {
		b.WriteString("Медалей пока нет — их выдают, когда заканчивается сезон.")
		return b.String()
	}
	for _, t := range trophies {
		fmt.Fprintf(&b, "%s Сезон %d — %d место из %d\n",
			tierEmoji[t.Tier], t.SeasonNumber, t.Place, t.Detail.Players)
	}
	return b.String()
}

// TrophyText is one medal opened: the season it was won in, and the numbers as
// they stood that day.
func TrophyText(t Trophy) string {
	var b strings.Builder

	fmt.Fprintf(&b, "%s %s · сезон %d\n", tierEmoji[t.Tier], tierName[t.Tier], t.SeasonNumber)
	if t.SeasonTitle != "" {
		fmt.Fprintf(&b, "%s\n", t.SeasonTitle)
	}
	fmt.Fprintf(&b, "%s — %s\n\n", fmtDay(t.StartedAt), fmtDay(t.FinishedAt))
	fmt.Fprintf(&b, "%d место из %d · %d %s\n\n", t.Place, t.Detail.Players, t.Points, pluralPoints(t.Points))
	// "Баллы" are the season score; the collection counts points of its own, and
	// calling both the same word makes one number read as two different things.
	fmt.Fprintf(&b, "🎴 Коллекция — %d место · %d %s\n", t.Detail.Cards.Place, t.Detail.Cards.Value, pluralCardPoints(int(t.Detail.Cards.Value)))
	fmt.Fprintf(&b, "🪙 Монеты — %d место · %d\n", t.Detail.Coins.Place, t.Detail.Coins.Value)
	fmt.Fprintf(&b, "🔥 Активность — %d место · %d %s\n", t.Detail.Days.Place, t.Detail.Days.Value, pluralDays(int(t.Detail.Days.Value)))
	if t.Detail.BestDrop != "" {
		fmt.Fprintf(&b, "\n🏆 Лучший дроп: %s", t.Detail.BestDrop)
	}
	return b.String()
}

// FinishText announces a finished season: the podium by name, the Elite by name
// too (a list that short is worth reading), and the rest as a count.
func FinishText(number int, title string, rows []Row, nextNumber int, nextEnds *time.Time) string {
	var b strings.Builder

	fmt.Fprintf(&b, "🏁 Сезон %d", number)
	if title != "" {
		fmt.Fprintf(&b, " · %s", title)
	}
	b.WriteString(" завершён!\n\n")

	var elite []string
	participants := 0
	for _, r := range rows {
		switch r.Tier {
		case TierChampion, TierSilver, TierBronze:
			fmt.Fprintf(&b, "%s %s — %d %s\n", tierEmoji[r.Tier], r.Name, r.Total, pluralPoints(r.Total))
		case TierElite:
			elite = append(elite, r.Name)
		case TierParticipant:
			participants++
		}
	}
	if len(elite) > 0 {
		fmt.Fprintf(&b, "\n🏅 Элита: %s\n", strings.Join(elite, ", "))
	}
	if participants > 0 {
		fmt.Fprintf(&b, "🎖 Участники: ещё %d %s\n", participants, pluralPlayers(participants))
	}

	if nextNumber > 0 {
		b.WriteString("\n")
		if nextEnds != nil {
			fmt.Fprintf(&b, "Сезон %d идёт до %s. ", nextNumber, fmtDay(*nextEnds))
		} else {
			fmt.Fprintf(&b, "Сезон %d уже идёт. ", nextNumber)
		}
		b.WriteString("Счёт у всех с нуля, коллекции остались.")
	}
	return b.String()
}

var months = [...]string{"января", "февраля", "марта", "апреля", "мая", "июня",
	"июля", "августа", "сентября", "октября", "ноября", "декабря"}

func fmtDay(t time.Time) string {
	msk := time.FixedZone("MSK", 3*60*60)
	t = t.In(msk)
	return fmt.Sprintf("%d %s", t.Day(), months[int(t.Month())-1])
}

func plural(n int, one, few, many string) string {
	m10, m100 := n%10, n%100
	switch {
	case m10 == 1 && m100 != 11:
		return one
	case m10 >= 2 && m10 <= 4 && (m100 < 10 || m100 >= 20):
		return few
	default:
		return many
	}
}

func pluralDays(n int) string    { return plural(n, "день", "дня", "дней") }
func pluralPoints(n int) string  { return plural(n, "балл", "балла", "баллов") }
func pluralPlayers(n int) string { return plural(n, "игрок", "игрока", "игроков") }

func pluralCardPoints(n int) string { return plural(n, "очко", "очка", "очков") }
