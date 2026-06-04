package telegram

import (
	"gachabot/internal/i18n"
	"strings"

	tele "gopkg.in/telebot.v3"
)

func (b *Bot) HandleSuggestStart(ctx tele.Context) error {
	tgUser := ctx.Sender()
	dbUser, _ := b.repo.GetOrCreateUserByTelegramID(tgUser.ID, tgUser.Username, tgUser.FirstName, tgUser.LastName)
	lang := getLang(dbUser, tgUser)

	profile, err := b.service.GetUserProfile(dbUser.ID)
	if err != nil || profile.Balance < 1000 {
		return ctx.Respond(&tele.CallbackResponse{Text: b.loc.Translate(lang, "suggest_err_funds")})
	}

	_ = ctx.Respond()

	rules := b.loc.Translate(lang, "suggest_rules")
	q1 := b.loc.Translate(lang, "suggest_q1")
	text := rules + "\n\n" + q1

	menu := &tele.ReplyMarkup{}
	btnYes := menu.Data(b.loc.Translate(lang, "btn_q1_yes"), "s_q1_yes")
	btnNo := menu.Data(b.loc.Translate(lang, "btn_q1_no"), "s_q1_no")
	btnCancel := menu.Data(b.loc.Translate(lang, "btn_suggest_cancel"), "s_cancel")
	menu.Inline(menu.Row(btnYes), menu.Row(btnNo), menu.Row(btnCancel))

	return ctx.Send(text, tele.ModeHTML, menu)
}

func (b *Bot) HandleSuggestQuiz(ctx tele.Context) error {
	_ = ctx.Respond()
	tgUser := ctx.Sender()
	dbUser, _ := b.repo.GetOrCreateUserByTelegramID(tgUser.ID, tgUser.Username, tgUser.FirstName, tgUser.LastName)
	lang := getLang(dbUser, tgUser)

	data := strings.TrimPrefix(ctx.Callback().Unique, "\f")

	menu := &tele.ReplyMarkup{}
	rules := b.loc.Translate(lang, "suggest_rules")

	switch data {
	case "s_q1_yes", "s_q2_yes", "s_q3_43", "s_q3_11":
		text := rules + "\n\n" + b.loc.Translate(lang, "suggest_fail")
		btnClose := menu.Data(b.loc.Translate(lang, "suggest_close"), "s_close")
		menu.Inline(menu.Row(btnClose))
		return ctx.Edit(text, tele.ModeHTML, menu)

	case "s_q1_no":
		text := rules + "\n\n" + b.loc.Translate(lang, "suggest_q2")
		btnYes := menu.Data(b.loc.Translate(lang, "btn_q2_yes"), "s_q2_yes")
		btnNo := menu.Data(b.loc.Translate(lang, "btn_q2_no"), "s_q2_no")
		btnCancel := menu.Data(b.loc.Translate(lang, "btn_suggest_cancel"), "s_cancel")
		menu.Inline(menu.Row(btnYes), menu.Row(btnNo), menu.Row(btnCancel))
		return ctx.Edit(text, tele.ModeHTML, menu)

	case "s_q2_no":
		text := rules + "\n\n" + b.loc.Translate(lang, "suggest_q3")
		btn43 := menu.Data(b.loc.Translate(lang, "btn_q3_43"), "s_q3_43")
		btn34 := menu.Data(b.loc.Translate(lang, "btn_q3_34"), "s_q3_34")
		btn11 := menu.Data(b.loc.Translate(lang, "btn_q3_11"), "s_q3_11")
		btnCancel := menu.Data(b.loc.Translate(lang, "btn_suggest_cancel"), "s_cancel")
		menu.Inline(menu.Row(btn43), menu.Row(btn34), menu.Row(btn11), menu.Row(btnCancel))
		return ctx.Edit(text, tele.ModeHTML, menu)

	case "s_q3_34":
		b.suggestService.SetSuggestState(dbUser.ID, true)
		text := b.loc.Translate(lang, "suggest_success")
		btnCancel := menu.Data(b.loc.Translate(lang, "btn_suggest_cancel"), "s_cancel")
		menu.Inline(menu.Row(btnCancel))
		return ctx.Edit(text, tele.ModeHTML, menu)
	}

	return nil
}

func (b *Bot) HandleSuggestCancel(ctx tele.Context) error {
	_ = ctx.Respond()
	tgUser := ctx.Sender()
	dbUser, _ := b.repo.GetOrCreateUserByTelegramID(tgUser.ID, tgUser.Username, tgUser.FirstName, tgUser.LastName)

	b.suggestService.SetSuggestState(dbUser.ID, false)
	lang := getLang(dbUser, tgUser)
	return ctx.Edit(b.loc.Translate(lang, "suggest_cancelled"), tele.ModeHTML)
}

func (b *Bot) HandleMediaSuggest(ctx tele.Context) error {
	tgUser := ctx.Sender()
	dbUser, _ := b.repo.GetOrCreateUserByTelegramID(tgUser.ID, tgUser.Username, tgUser.FirstName, tgUser.LastName)
	lang := getLang(dbUser, tgUser)

	if !b.suggestService.IsSuggesting(dbUser.ID) {
		return nil
	}

	caption := ctx.Message().Caption
	if caption == "" {

		return ctx.Send(b.loc.Translate(lang, "suggest_err_no_caption"), tele.ModeHTML)
	}

	err := b.suggestService.SubmitSuggestion(dbUser.ID)
	if err != nil {
		b.suggestService.SetSuggestState(dbUser.ID, false)
		return ctx.Send(b.loc.Translate(lang, "suggest_err_funds"))
	}

	adminMsg := b.loc.Translate(lang, "suggest_admin_msg", i18n.Args{
		"username": tgUser.Username, "user_id": tgUser.ID, "db_id": dbUser.ID, "description": caption})

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

	b.suggestService.SetSuggestState(dbUser.ID, false)
	return ctx.Send(b.loc.Translate(lang, "suggest_done"), tele.ModeHTML)
}

func (b *Bot) HandleTextFallback(ctx tele.Context) error {
	tgUser := ctx.Sender()
	dbUser, _ := b.repo.GetOrCreateUserByTelegramID(tgUser.ID, tgUser.Username, tgUser.FirstName, tgUser.LastName)
	lang := getLang(dbUser, tgUser)

	if b.suggestService.IsSuggesting(dbUser.ID) {
		return ctx.Send(b.loc.Translate(lang, "suggest_err_no_file"), tele.ModeHTML)
	}
	return nil
}

func (b *Bot) HandleSuggestClose(ctx tele.Context) error {
	_ = ctx.Respond()
	return ctx.Delete()
}
