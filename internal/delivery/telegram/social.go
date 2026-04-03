package telegram

import (
	"context"
	"fmt"
	"log"
	"math/rand/v2"
	"strings"
	"time"

	tele "gopkg.in/telebot.v3"
)

func (b *Bot) buildTopMessage(criteria string, scope string, chatID int64, lang string) (string, *tele.ReplyMarkup, error) {
	targetChatID := chatID
	if scope == "global" {
		targetChatID = 0
	}

	board, err := b.service.GetLeaderboard(criteria, targetChatID)
	if err != nil {
		return "", nil, err
	}

	scopeName := b.loc.T(lang, "top_global_title")
	if scope == "local" {
		scopeName = b.loc.T(lang, "top_local_title")
	}

	critName, emoji := "", ""
	switch criteria {
	case "balance":
		critName, emoji = b.loc.T(lang, "top_crit_balance"), `<tg-emoji emoji-id="4918300654197277832">🪙</tg-emoji>`
	case "cards":
		critName, emoji = b.loc.T(lang, "top_crit_cards"), "🃏"
	case "streak":
		critName, emoji = b.loc.T(lang, "top_crit_streak"), "🔥"
	}

	text := b.loc.T(lang, "top_header", scopeName, critName)
	if len(board) == 0 {
		text += b.loc.T(lang, "top_empty")
	} else {
		for i, entry := range board {
			medal := "🏅"
			if i == 0 {
				medal = "🥇"
			} else if i == 1 {
				medal = "🥈"
			} else if i == 2 {
				medal = "🥉"
			}
			displayName := entry.DisplayName
			if entry.ActiveAura != "" {
				// Если есть аура, приклеиваем её к имени
				displayName = fmt.Sprintf("%s [✨ %s]", entry.DisplayName, entry.ActiveAura)
			}
			// -------------

			text += b.loc.T(lang, "top_entry", medal, displayName, entry.Value, emoji)
		}
	}
	text += "</blockquote>"

	menu := &tele.ReplyMarkup{}
	btnBal := menu.Data(b.loc.T(lang, "btn_top_balance"), "top_btn", "balance|"+scope)
	btnCards := menu.Data(b.loc.T(lang, "btn_top_cards"), "top_btn", "cards|"+scope)
	btnStreak := menu.Data(b.loc.T(lang, "btn_top_streak"), "top_btn", "streak|"+scope)

	menu.Inline(menu.Row(btnBal, btnCards, btnStreak))

	return text, menu, nil
}

func (b *Bot) HandleLocalTop(ctx tele.Context) error {
	tgUser := ctx.Sender()
	dbUser, _ := b.repo.GetOrCreateUserByTelegramID(tgUser.ID, tgUser.Username, tgUser.FirstName, tgUser.LastName)
	lang := getLang(dbUser, tgUser)
	b.service.TrackChat(dbUser.ID, ctx.Chat().ID)

	if ctx.Chat().Type == tele.ChatPrivate {
		menu := &tele.ReplyMarkup{}
		btnAdd := menu.URL(b.loc.T(lang, "btn_add_group"), "https://t.me/HentaiCard_bot?startgroup=true")
		menu.Inline(menu.Row(btnAdd))
		return ctx.Send(b.loc.T(lang, "err_top_private"), menu)
	}

	text, menu, err := b.buildTopMessage("balance", "local", ctx.Chat().ID, lang)
	if err != nil {
		return ctx.Send(b.loc.T(lang, "err_top_load"))
	}
	return ctx.Send(text, tele.ModeHTML, menu)
}

func (b *Bot) HandleGlobalTop(ctx tele.Context) error {
	tgUser := ctx.Sender()
	dbUser, _ := b.repo.GetOrCreateUserByTelegramID(tgUser.ID, tgUser.Username, tgUser.FirstName, tgUser.LastName)
	lang := getLang(dbUser, tgUser)
	b.service.TrackChat(dbUser.ID, ctx.Chat().ID)

	text, menu, err := b.buildTopMessage("balance", "global", ctx.Chat().ID, lang)
	if err != nil {
		return ctx.Send(b.loc.T(lang, "err_top_load"))
	}
	return ctx.Send(text, tele.ModeHTML, menu)
}

func (b *Bot) HandleTopCallback(ctx tele.Context) error {
	_ = ctx.Respond()
	tgUser := ctx.Sender()
	dbUser, _ := b.repo.GetOrCreateUserByTelegramID(tgUser.ID, tgUser.Username, tgUser.FirstName, tgUser.LastName)
	lang := getLang(dbUser, tgUser)

	data := strings.Split(ctx.Callback().Data, "|")
	if len(data) != 2 {
		return nil
	}
	criteria, scope := data[0], data[1]

	text, menu, err := b.buildTopMessage(criteria, scope, ctx.Chat().ID, lang)
	if err != nil {
		return ctx.Send(b.loc.T(lang, "err_top_update"))
	}

	err = ctx.Edit(text, tele.ModeHTML, menu)
	if err != nil && !strings.Contains(err.Error(), "message is not modified") {
		return err
	}
	return nil
}

// Активация кода игроками (Telegram)
func (b *Bot) HandlePromo(ctx tele.Context) error {
	tgUser := ctx.Sender()
	dbUser, _ := b.repo.GetOrCreateUserByTelegramID(tgUser.ID, tgUser.Username, tgUser.FirstName, tgUser.LastName)
	lang := getLang(dbUser, tgUser)

	args := ctx.Args()
	if len(args) == 0 {
		return ctx.Send(b.loc.T(lang, "promo_usage"), tele.ModeHTML)
	}

	code := args[0]
	// ПРАВИЛЬНЫЙ ВЫЗОВ: 2 аргумента на вход, 3 на выход
	reward, cards, err := b.service.RedeemPromo(dbUser.ID, code)
	if err != nil {
		var errKey string
		switch err.Error() {
		case "not_found":
			errKey = "promo_err_not_found"
		case "limit_reached":
			errKey = "promo_err_limit"
		case "already_used":
			errKey = "promo_err_used"
		case "expired":
			errKey = "promo_err_expired"
		default:
			errKey = "error_db"
		}
		return ctx.Send(b.loc.T(lang, errKey), tele.ModeHTML)
	}

	// Собираем красивый текст
	var sb strings.Builder
	sb.WriteString("<b>" + b.loc.T(lang, "promo_success_title") + "</b>\n\n")

	if reward.Points > 0 {
		sb.WriteString(b.loc.T(lang, "promo_reward_points", reward.Points) + "\n")
	}
	if reward.PremiumRolls > 0 {
		sb.WriteString(b.loc.T(lang, "promo_reward_rolls", reward.PremiumRolls) + "\n")
	}
	if len(cards) > 0 {
		sb.WriteString("\n" + b.loc.T(lang, "promo_reward_cards_count", len(cards)) + "\n")
		for _, c := range cards {
			sb.WriteString(b.loc.T(lang, "promo_reward_card", c.Name, c.PowerLevel) + "\n")
		}
	}

	text := sb.String()

	var sendErr error
	if len(cards) == 0 {
		sendErr = ctx.Send(text, tele.ModeHTML)
	} else if len(cards) == 1 {
		photo := &tele.Photo{File: tele.FromURL(cards[0].ImageURL), Caption: text}
		sendErr = ctx.Send(photo, tele.ModeHTML)
	} else {
		// Альбом
		albumLimit := len(cards)
		if albumLimit > 10 {
			albumLimit = 10
		}
		var album tele.Album
		for i := 0; i < albumLimit; i++ {
			p := &tele.Photo{File: tele.FromURL(cards[i].ImageURL)}
			if i == 0 {
				p.Caption = text
			}
			album = append(album, p)
		}
		sendErr = ctx.SendAlbum(album, tele.ModeHTML)
	}

	// --- ОТДЕЛЬНОЕ СООБЩЕНИЕ ПРИ СБОРЕ СЕТА ИЗ ПРОМОКОДА ---
	if len(reward.CompletedSets) > 0 {
		for _, set := range reward.CompletedSets {
			// ТЕПЕРЬ ПЕРЕДАЕМ set.Reward ВМЕСТО 0
			msg := b.loc.T(lang, "set_completed_msg", set.Name, set.Reward)
			_ = ctx.Reply(msg, tele.ModeHTML)
		}
	}

	return sendErr
}

// --- ПРИВЯЗКА АККАУНТОВ ---
func (b *Bot) HandleLinkStart(ctx tele.Context) error {
	tgUser := ctx.Sender()
	dbUser, err := b.repo.GetOrCreateUserByTelegramID(tgUser.ID, tgUser.Username, tgUser.FirstName, tgUser.LastName)
	if err != nil {
		return ctx.Send("Database error: " + err.Error())
	}

	lang := getLang(dbUser, tgUser)

	// Если у юзера уже привязан Discord, предупреждаем его
	if dbUser.DiscordID.Valid {
		return ctx.Send(b.loc.T(lang, "link_already_done"), tele.ModeHTML)
	}

	code := generateLinkCode()

	// Сохраняем код в Redis. Он САМ удалится ровно через 5 минут.
	rCtx := context.Background()                                             // Назвали rCtx (Redis Context)
	err = b.rdb.Set(rCtx, "link_code:"+code, dbUser.ID, 5*time.Minute).Err() // Используем = вместо :=
	if err != nil {
		log.Println("Redis error:", err)
		return ctx.Send("Ошибка генерации кода.")
	}

	// Формируем красивое сообщение
	msg := b.loc.T(lang, "link_code_msg", code, code)

	return ctx.Send(msg, tele.ModeHTML)
}

// Генерация случайного 6-значного кода
func generateLinkCode() string {
	const charset = "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, 6)
	for i := range b {
		b[i] = charset[rand.IntN(len(charset))]
	}
	return string(b)
}
