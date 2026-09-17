package discord

import (
	"database/sql"
	"os"
	"strconv"
	"testing"
	"time"

	"gachabot/internal/i18n"
	"gachabot/internal/repository"
	"gachabot/internal/service/gacha"
	"gachabot/internal/service/season"

	"github.com/bwmarrin/discordgo"
	_ "github.com/lib/pq"
)

// A live smoke test for the season embeds, skipped unless pointed at a bot and
// a channel nobody minds. Discord validates an embed and its components on the
// way in — field counts, label and custom_id lengths, whether a button's emoji
// is really an emoji — and none of that is visible from a unit test.
//
// Slash-command interactions cannot be faked (they need an interaction token
// Discord issues), so this posts the same embed and the same buttons as a plain
// message. The buttons are live: pressing one produces a component interaction
// that a running bot handles, which is the other half of the check.
//
//	DISCORD_TEST_TOKEN=<token of a throwaway bot in that server>
//	DISCORD_TEST_CHANNEL=<channel id>
//	DISCORD_TEST_USER=<internal users.id to render for>
//	POSTGRES_* as usual
//
//	go test ./internal/delivery/discord/ -run TestLive -v
func liveBot(t *testing.T) (*Bot, string, int64) {
	t.Helper()

	token := os.Getenv("DISCORD_TEST_TOKEN")
	if token == "" {
		t.Skip("DISCORD_TEST_TOKEN не задан — живой прогон пропущен")
	}
	channel := os.Getenv("DISCORD_TEST_CHANNEL")
	userID, _ := strconv.ParseInt(os.Getenv("DISCORD_TEST_USER"), 10, 64)
	if channel == "" || userID == 0 {
		t.Fatal("нужны DISCORD_TEST_CHANNEL и DISCORD_TEST_USER")
	}

	dsn := "postgres://" + os.Getenv("POSTGRES_USER") + ":" + os.Getenv("POSTGRES_PASSWORD") +
		"@" + envOr("POSTGRES_HOST", "localhost") + ":" + envOr("POSTGRES_PORT", "5432") +
		"/" + os.Getenv("POSTGRES_DB") + "?sslmode=disable"
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("база недоступна: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.Ping(); err != nil {
		t.Fatalf("база не отвечает: %v", err)
	}

	repo := repository.NewPostgresRepo(db)
	seasonSvc := season.New(repo)
	gachaSvc := gacha.NewGachaService(repo, nil, 0, time.Hour, true, true)
	gachaSvc.UseSeason(seasonSvc)

	loc, err := i18n.NewLocalizer("../../../locales/base", "../../../locales/discord", "ru")
	if err != nil {
		t.Fatalf("локализация не загрузилась: %v", err)
	}

	// No Start(): this talks to Discord over REST only, so it does not open a
	// second gateway session next to a bot that may already be running.
	b, err := NewBot(token, repo, nil, gachaSvc, nil, nil, nil, nil, seasonSvc, loc, nil, "", 0, nil)
	if err != nil {
		t.Fatalf("бот не создался: %v", err)
	}
	return b, channel, userID
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func TestLiveSeasonEmbeds(t *testing.T) {
	b, channel, userID := liveBot(t)

	t.Run("season board", func(t *testing.T) {
		cur := b.season.Current()
		if cur == nil {
			t.Skip("сезон не идёт")
		}
		rows, _, err := b.season.Scoreboard()
		if err != nil {
			t.Fatalf("счёт не прочитался: %v", err)
		}
		b.season.Decorate(rows)
		elapsed, _ := b.season.Days()

		embed := &discordgo.MessageEmbed{
			Description: season.BoardText(cur.Number, cur.Title, elapsed, b.season.DaysLeft(), rows, userID),
			Color:       seasonColor,
		}
		msg, err := b.session.ChannelMessageSendComplex(channel, &discordgo.MessageSend{
			Embeds: []*discordgo.MessageEmbed{embed},
			Components: []discordgo.MessageComponent{discordgo.ActionsRow{Components: []discordgo.MessageComponent{
				discordgo.Button{Label: "Мои трофеи", Style: discordgo.SecondaryButton, CustomID: trophyData(0, userID, false), Emoji: &discordgo.ComponentEmoji{Name: "🏆"}},
				b.launchAppButton(),
			}}},
		})
		if err != nil {
			t.Fatalf("Discord не принял доску сезона: %v", err)
		}
		t.Logf("доска сезона отправлена: %s", msg.ID)
	})

	t.Run("trophy shelf with its buttons", func(t *testing.T) {
		embed, components, err := b.shelfView(userID, "Тестер", true)
		if err != nil {
			t.Fatalf("полка не собралась: %v", err)
		}
		msg, err := b.session.ChannelMessageSendComplex(channel, &discordgo.MessageSend{
			Embeds:     []*discordgo.MessageEmbed{embed},
			Components: components,
		})
		if err != nil {
			t.Fatalf("Discord не принял полку трофеев: %v", err)
		}
		t.Logf("полка отправлена: %s — кнопки живые, их обработает запущенный бот", msg.ID)
	})

	t.Run("one opened medal", func(t *testing.T) {
		trophies, _, err := b.season.Trophies(userID)
		if err != nil || len(trophies) == 0 {
			t.Skipf("у игрока нет медалей (%v)", err)
		}
		embed := &discordgo.MessageEmbed{Description: season.TrophyText(trophies[0]), Color: trophyColor}
		if _, err := b.session.ChannelMessageSendComplex(channel, &discordgo.MessageSend{
			Embeds: []*discordgo.MessageEmbed{embed},
		}); err != nil {
			t.Fatalf("Discord не принял медаль: %v", err)
		}
	})

	t.Run("drop byline", func(t *testing.T) {
		author := b.dropAuthor(userID, "Тестер", "410786450809028608")
		if author == nil {
			t.Skip("у игрока нет медалей — подписи к дропу нет, это допустимо")
		}
		if _, err := b.session.ChannelMessageSendComplex(channel, &discordgo.MessageSend{
			Embeds: []*discordgo.MessageEmbed{{
				Title:       "Проверка подписи к дропу",
				Description: "Выше должна стоять строка со значком.",
				Author:      author,
				Color:       0x00ff00,
			}},
		}); err != nil {
			t.Fatalf("Discord не принял подпись к дропу: %v", err)
		}
	})
}
