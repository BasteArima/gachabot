package telegram

import (
	"context"
	"fmt"
	"gachabot/internal/i18n"
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

	scopeName := b.loc.Translate(lang, "top_global_title")
	if scope == "local" {
		scopeName = b.loc.Translate(lang, "top_local_title")
	}

	critName, emoji := "", ""
	switch criteria {
	case "balance":
		critName, emoji = b.loc.Translate(lang, "top_crit_balance"), `<tg-emoji emoji-id="4918300654197277832">🪙</tg-emoji>`
	case "cards":
		critName, emoji = b.loc.Translate(lang, "top_crit_cards"), "🃏"
	case "streak":
		critName, emoji = b.loc.Translate(lang, "top_crit_streak"), "🔥"
	}

	text := b.loc.Translate(lang, "top_header", i18n.Args{"scope": scopeName, "crit": critName})
	if len(board) == 0 {
		text += b.loc.Translate(lang, "top_empty")
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
				displayName = fmt.Sprintf("%s  [✨ %s]", entry.DisplayName, entry.ActiveAura)
			}

			text += b.loc.Translate(lang, "top_entry", i18n.Args{"medal": medal, "name": displayName, "value": entry.Value, "emoji": emoji})
		}
	}
	text += "</blockquote>"

	menu := &tele.ReplyMarkup{}
	btnBal := menu.Data(b.loc.Translate(lang, "btn_top_balance"), "top_btn", "balance|"+scope)
	btnCards := menu.Data(b.loc.Translate(lang, "btn_top_cards"), "top_btn", "cards|"+scope)
	btnStreak := menu.Data(b.loc.Translate(lang, "btn_top_streak"), "top_btn", "streak|"+scope)

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
		btnAdd := menu.URL(b.loc.Translate(lang, "btn_add_group"), b.groupLink)
		menu.Inline(menu.Row(btnAdd))
		return ctx.Send(b.loc.Translate(lang, "err_top_private"), menu)
	}

	text, menu, err := b.buildTopMessage("balance", "local", ctx.Chat().ID, lang)
	if err != nil {
		return ctx.Send(b.loc.Translate(lang, "err_top_load"))
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
		return ctx.Send(b.loc.Translate(lang, "err_top_load"))
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
		return ctx.Send(b.loc.Translate(lang, "err_top_update"))
	}

	err = ctx.Edit(text, tele.ModeHTML, menu)
	if err != nil && !strings.Contains(err.Error(), "message is not modified") {
		return err
	}
	return nil
}

func (b *Bot) HandleLinkStart(ctx tele.Context) error {
	tgUser := ctx.Sender()
	dbUser, err := b.repo.GetOrCreateUserByTelegramID(tgUser.ID, tgUser.Username, tgUser.FirstName, tgUser.LastName)
	if err != nil {
		return ctx.Send("Database error: " + err.Error())
	}

	lang := getLang(dbUser, tgUser)

	// If the user already has a Discord account, we warn them
	if dbUser.DiscordID.Valid {
		return ctx.Send(b.loc.Translate(lang, "link_already_done"), tele.ModeHTML)
	}

	code := generateLinkCode()

	rCtx := context.Background()
	err = b.rdb.Set(rCtx, "link_code:"+code, dbUser.ID, 5*time.Minute).Err()
	if err != nil {
		log.Println("Redis error:", err)
		return ctx.Send("Ошибка генерации кода.")
	}

	msg := b.loc.Translate(lang, "link_code_msg", i18n.Args{"code": code})
	return ctx.Send(msg, tele.ModeHTML)
}

func generateLinkCode() string {
	const charset = "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, 6)
	for i := range b {
		b[i] = charset[rand.IntN(len(charset))]
	}
	return string(b)
}
