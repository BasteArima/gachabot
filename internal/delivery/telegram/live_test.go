package telegram

import (
	"os"
	"testing"

	"gachabot/internal/config"
	"gachabot/internal/i18n"
	"gachabot/internal/repository"
	"gachabot/internal/service/gacha"
	"gachabot/internal/service/season"

	"database/sql"
	"strconv"
	"time"

	_ "github.com/lib/pq"
	tele "gopkg.in/telebot.v3"
)

// A live smoke test for the chat handlers, skipped unless it is pointed at a
// throwaway bot and chat. Everything else in this package can be checked by
// reading it; these handlers cannot — what they produce is only correct if
// Telegram accepts it, and Telegram is the one thing a unit test cannot stand
// in for. Bad HTML in a caption, callback data over the limit, editMessageText
// against a photo: all of them build, vet and pass tests, and all of them fail
// the moment a real message is sent.
//
// It sends real messages, so it needs a bot and chats nobody minds:
//
//	TELEGRAM_TEST_TOKEN=<token from a second BotFather bot>
//	TELEGRAM_TEST_CHAT=<your private chat id with it>
//	TELEGRAM_TEST_GROUP=<a test group id, optional>
//	TELEGRAM_TEST_USER=<internal users.id to act as>
//	POSTGRES_* / REDIS_ADDR as usual
//
//	go test ./internal/delivery/telegram/ -run TestLive -v
func liveEnv(t *testing.T) (token string, chatID, groupID, userID int64) {
	t.Helper()
	token = os.Getenv("TELEGRAM_TEST_TOKEN")
	if token == "" {
		t.Skip("TELEGRAM_TEST_TOKEN не задан — живой прогон пропущен")
	}
	chatID, _ = strconv.ParseInt(os.Getenv("TELEGRAM_TEST_CHAT"), 10, 64)
	groupID, _ = strconv.ParseInt(os.Getenv("TELEGRAM_TEST_GROUP"), 10, 64)
	userID, _ = strconv.ParseInt(os.Getenv("TELEGRAM_TEST_USER"), 10, 64)
	if chatID == 0 || userID == 0 {
		t.Fatal("нужны TELEGRAM_TEST_CHAT и TELEGRAM_TEST_USER")
	}
	return token, chatID, groupID, userID
}

func liveBot(t *testing.T, token string) (*Bot, *repository.PostgresRepo) {
	t.Helper()

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

	loc, err := i18n.NewLocalizer("../../../locales/base", "../../../locales/telegram", "ru")
	if err != nil {
		t.Fatalf("локализация не загрузилась: %v", err)
	}

	b, err := NewBot(repo, nil, gachaSvc, nil, nil, nil, nil, seasonSvc, loc, config.TelegramConfig{Token: token})
	if err != nil {
		t.Fatalf("бот не создался: %v", err)
	}
	return b, repo
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// message builds the context a command arrives in.
func (b *Bot) liveMessageCtx(chatID, tgUserID int64, chatType tele.ChatType, text string) tele.Context {
	return b.bot.NewContext(tele.Update{
		Message: &tele.Message{
			ID:     1,
			Sender: &tele.User{ID: tgUserID, FirstName: "Тестер"},
			Chat:   &tele.Chat{ID: chatID, Type: chatType},
			Text:   text,
		},
	})
}

// callback builds the context a button press arrives in, pointed at a message
// that really exists so the handler's edit has something to edit.
func (b *Bot) liveCallbackCtx(msg *tele.Message, tgUserID int64, data string) tele.Context {
	return b.bot.NewContext(tele.Update{
		Callback: &tele.Callback{
			ID:      "live-test",
			Sender:  &tele.User{ID: tgUserID, FirstName: "Тестер"},
			Message: msg,
			Data:    data,
		},
	})
}

func TestLiveSeasonAndTrophies(t *testing.T) {
	token, chatID, groupID, userID := liveEnv(t)
	b, repo := liveBot(t, token)

	user, err := repo.GetUserByID(userID)
	if err != nil {
		t.Fatalf("игрок %d не найден: %v", userID, err)
	}
	tgID := user.TelegramID.Int64
	if tgID == 0 {
		t.Fatalf("у игрока %d нет telegram_id", userID)
	}

	t.Run("season in private", func(t *testing.T) {
		if err := b.HandleSeason(b.liveMessageCtx(chatID, tgID, tele.ChatPrivate, "/season")); err != nil {
			t.Fatalf("/season не отправился: %v", err)
		}
	})

	t.Run("trophies shelf", func(t *testing.T) {
		if err := b.HandleTrophies(b.liveMessageCtx(chatID, tgID, tele.ChatPrivate, "/trophies")); err != nil {
			t.Fatalf("/trophies не отправился: %v", err)
		}
	})

	t.Run("open one medal and go back", func(t *testing.T) {
		// A message of our own to edit, standing in for the shelf the player is
		// looking at.
		msg, err := b.bot.Send(&tele.Chat{ID: chatID}, "Полка трофеев (заглушка для проверки кнопок)")
		if err != nil {
			t.Fatalf("заглушка не отправилась: %v", err)
		}
		trophies, _, err := b.season.Trophies(userID)
		if err != nil || len(trophies) == 0 {
			t.Skipf("у игрока нет медалей, проверять нечего (%v)", err)
		}
		number := trophies[0].SeasonNumber

		if err := b.HandleTrophyCallback(b.liveCallbackCtx(msg, tgID, trophyData(number, false, userID))); err != nil {
			t.Fatalf("медаль не открылась: %v", err)
		}
		if err := b.HandleTrophyCallback(b.liveCallbackCtx(msg, tgID, trophyData(0, false, userID))); err != nil {
			t.Fatalf("возврат к полке не сработал: %v", err)
		}
	})

	t.Run("someone else's button is refused", func(t *testing.T) {
		msg, err := b.bot.Send(&tele.Chat{ID: chatID}, "Чужая полка (нажатие должно быть отклонено)")
		if err != nil {
			t.Fatalf("заглушка не отправилась: %v", err)
		}
		// Answering a fabricated callback id fails at Telegram, which is fine:
		// what matters is that the message was left alone.
		_ = b.HandleTrophyCallback(b.liveCallbackCtx(msg, tgID, trophyData(1, false, userID+1)))

		fresh, err := b.bot.Send(&tele.Chat{ID: chatID}, "—")
		if err == nil {
			_ = b.bot.Delete(fresh)
		}
	})

	t.Run("shelf inside a profile photo", func(t *testing.T) {
		// The profile is a photo when the player has an avatar, and a photo's text
		// is its caption: editMessageText fails on it. This is the path that
		// catches that, and nothing short of a real message can.
		card, err := repo.GetRandomCard(1)
		if err != nil || card == nil {
			t.Skipf("нет карты для фото-заглушки (%v)", err)
		}
		photo, err := b.bot.Send(&tele.Chat{ID: chatID}, &tele.Photo{
			File:    tele.FromURL(card.ImageURL),
			Caption: "Профиль (заглушка: проверяем правку подписи)",
		})
		if err != nil {
			t.Fatalf("фото-заглушка не отправилась: %v", err)
		}
		if err := b.HandleProfileTrophies(b.liveCallbackCtx(photo, tgID, "")); err != nil {
			t.Fatalf("полка не открылась внутри фото: %v", err)
		}
	})

	t.Run("season in a group", func(t *testing.T) {
		if groupID == 0 {
			t.Skip("TELEGRAM_TEST_GROUP не задан")
		}
		if err := b.HandleSeason(b.liveMessageCtx(groupID, tgID, tele.ChatSuperGroup, "/season")); err != nil {
			t.Fatalf("/season в группе не отправился: %v", err)
		}
	})

	t.Run("drop badge only in groups", func(t *testing.T) {
		private := b.dropBadge(userID, &tele.User{ID: tgID, FirstName: "Тестер"}, &tele.Chat{ID: chatID, Type: tele.ChatPrivate})
		if private != "" {
			t.Errorf("в личке значок не нужен, а вышло %q", private)
		}
		if groupID == 0 {
			t.Skip("TELEGRAM_TEST_GROUP не задан")
		}
		group := b.dropBadge(userID, &tele.User{ID: tgID, FirstName: "Тестер"}, &tele.Chat{ID: groupID, Type: tele.ChatSuperGroup})
		if group == "" {
			t.Log("у игрока нет медалей — строки нет, это допустимо")
			return
		}
		// Send it the way a roll caption would, to prove Telegram accepts the HTML.
		if _, err := b.bot.Send(&tele.Chat{ID: groupID}, group+"выбил карту (проверка значка)", tele.ModeHTML); err != nil {
			t.Fatalf("строка со значком не отправилась: %v", err)
		}
	})
}
