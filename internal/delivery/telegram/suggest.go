package telegram

import (
	"context"
	"fmt"
	"strings"
	"time"

	tele "gopkg.in/telebot.v3"
)

// --- ПРЕДЛОЖКА КАРТОЧЕК ---
func (b *Bot) HandleSuggestStart(ctx tele.Context) error {
	tgUser := ctx.Sender()
	dbUser, _ := b.repo.GetOrCreateUserByTelegramID(tgUser.ID, tgUser.Username, tgUser.FirstName, tgUser.LastName)
	lang := getLang(dbUser, tgUser)

	profile, err := b.service.GetUserProfile(dbUser.ID)
	if err != nil || profile.Balance < 1000 {
		return ctx.Respond(&tele.CallbackResponse{Text: b.loc.T(lang, "suggest_err_funds")})
	}

	_ = ctx.Respond()

	rules := b.loc.T(lang, "suggest_rules")
	q1 := b.loc.T(lang, "suggest_q1")
	text := rules + "\n\n" + q1

	menu := &tele.ReplyMarkup{}
	btnYes := menu.Data(b.loc.T(lang, "btn_q1_yes"), "s_q1_yes")
	btnNo := menu.Data(b.loc.T(lang, "btn_q1_no"), "s_q1_no")
	menu.Inline(menu.Row(btnYes), menu.Row(btnNo))

	return ctx.Send(text, tele.ModeHTML, menu)
}

func (b *Bot) HandleSuggestQuiz(ctx tele.Context) error {
	_ = ctx.Respond()
	tgUser := ctx.Sender()
	dbUser, _ := b.repo.GetOrCreateUserByTelegramID(tgUser.ID, tgUser.Username, tgUser.FirstName, tgUser.LastName)
	lang := getLang(dbUser, tgUser)

	data := strings.TrimPrefix(ctx.Callback().Unique, "\f")

	menu := &tele.ReplyMarkup{}
	rules := b.loc.T(lang, "suggest_rules")

	switch data {
	case "s_q1_yes", "s_q2_yes", "s_q3_43", "s_q3_11":
		text := rules + "\n\n" + b.loc.T(lang, "suggest_fail")
		btnClose := menu.Data("❌ Закрыть", "s_close")
		menu.Inline(menu.Row(btnClose))
		return ctx.Edit(text, tele.ModeHTML, menu)

	case "s_q1_no":
		text := rules + "\n\n" + b.loc.T(lang, "suggest_q2")
		btnYes := menu.Data(b.loc.T(lang, "btn_q2_yes"), "s_q2_yes")
		btnNo := menu.Data(b.loc.T(lang, "btn_q2_no"), "s_q2_no")
		menu.Inline(menu.Row(btnYes), menu.Row(btnNo))
		return ctx.Edit(text, tele.ModeHTML, menu)

	case "s_q2_no":
		text := rules + "\n\n" + b.loc.T(lang, "suggest_q3")
		btn43 := menu.Data(b.loc.T(lang, "btn_q3_43"), "s_q3_43")
		btn34 := menu.Data(b.loc.T(lang, "btn_q3_34"), "s_q3_34")
		btn11 := menu.Data(b.loc.T(lang, "btn_q3_11"), "s_q3_11")
		menu.Inline(menu.Row(btn43), menu.Row(btn34), menu.Row(btn11))
		return ctx.Edit(text, tele.ModeHTML, menu)

	case "s_q3_34":
		b.setSuggestState(dbUser.ID, true)
		text := b.loc.T(lang, "suggest_success")
		btnCancel := menu.Data(b.loc.T(lang, "btn_suggest_cancel"), "s_cancel")
		menu.Inline(menu.Row(btnCancel))
		return ctx.Edit(text, tele.ModeHTML, menu)
	}

	return nil
}

func (b *Bot) HandleSuggestCancel(ctx tele.Context) error {
	_ = ctx.Respond()
	tgUser := ctx.Sender()
	dbUser, _ := b.repo.GetOrCreateUserByTelegramID(tgUser.ID, tgUser.Username, tgUser.FirstName, tgUser.LastName)

	b.setSuggestState(dbUser.ID, false)
	lang := getLang(dbUser, tgUser)
	return ctx.Edit(b.loc.T(lang, "suggest_cancelled"), tele.ModeHTML)
}

func (b *Bot) HandleMediaSuggest(ctx tele.Context) error {
	tgUser := ctx.Sender()
	dbUser, _ := b.repo.GetOrCreateUserByTelegramID(tgUser.ID, tgUser.Username, tgUser.FirstName, tgUser.LastName)
	lang := getLang(dbUser, tgUser)

	if !b.isSuggesting(dbUser.ID) {
		return nil
	}

	caption := ctx.Message().Caption
	if caption == "" {
		return ctx.Send(b.loc.T(lang, "suggest_err_no_caption"), tele.ModeHTML)
	}

	if dbUser.Balance < 1000 {
		b.setSuggestState(dbUser.ID, false)
		return ctx.Send(b.loc.T(lang, "suggest_err_funds"))
	}

	dbUser.Balance -= 1000
	_ = b.repo.UpdateUserAfterRoll(dbUser)

	adminMsg := fmt.Sprintf("📩 <b>Новая предложка!</b>\nОт: @%s (TG_ID: %d, DB_ID: %d)\n\nОписание юзера:\n<i>%s</i>",
		tgUser.Username, tgUser.ID, dbUser.ID, caption)

	adminChat := &tele.Chat{ID: b.adminChatID}

	if ctx.Message().Photo != nil {
		photo := ctx.Message().Photo
		photo.Caption = adminMsg
		_, _ = ctx.Bot().Send(adminChat, photo, tele.ModeHTML)
	} else if ctx.Message().Document != nil {
		doc := ctx.Message().Document
		doc.Caption = adminMsg
		_, _ = ctx.Bot().Send(adminChat, doc, tele.ModeHTML)
	}

	b.setSuggestState(dbUser.ID, false)
	return ctx.Send(b.loc.T(lang, "suggest_done"), tele.ModeHTML)
}

func (b *Bot) HandleTextFallback(ctx tele.Context) error {
	tgUser := ctx.Sender()
	dbUser, _ := b.repo.GetOrCreateUserByTelegramID(tgUser.ID, tgUser.Username, tgUser.FirstName, tgUser.LastName)

	if b.isSuggesting(dbUser.ID) {
		return ctx.Send("⚠️ Я жду от тебя **Фотографию** или **Файл**, а не просто текст!\n\nЕсли передумал, нажми кнопку отмены в предыдущем сообщении.", tele.ModeMarkdown)
	}
	return nil
}

func (b *Bot) HandleSuggestClose(ctx tele.Context) error {
	_ = ctx.Respond()
	return ctx.Delete()
}

// Вспомогательные методы для безопасной работы с состоянием (Теперь через Redis)
func (b *Bot) setSuggestState(internalUserID int64, state bool) {
	rCtx := context.Background()
	key := fmt.Sprintf("suggest_state:%d", internalUserID)
	if state {
		// Даем юзеру 10 минут, чтобы отправить фото. Потом состояние сбросится само!
		b.rdb.Set(rCtx, key, "1", 10*time.Minute)
	} else {
		// Принудительно удаляем состояние
		b.rdb.Del(rCtx, key)
	}
}

func (b *Bot) isSuggesting(internalUserID int64) bool {
	rCtx := context.Background()
	key := fmt.Sprintf("suggest_state:%d", internalUserID)

	val, err := b.rdb.Get(rCtx, key).Result()
	if err != nil {
		return false // Ключа нет (или он протух)
	}
	return val == "1"
}
