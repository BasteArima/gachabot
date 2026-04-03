package telegram

import (
	"log"

	tele "gopkg.in/telebot.v3"
	"gopkg.in/telebot.v3/middleware"
)

func (b *Bot) Register(bot *tele.Bot) {
	bot.Use(middleware.Logger())

	bot.Handle("/start", b.HandleStart)
	bot.Handle("/roll", b.HandleRoll)
	bot.Handle("/profile", b.HandleProfile)
	bot.Handle("\fcards_nav", b.HandleCardsNav)
	bot.Handle("\fback_profile", b.HandleBackToProfile)

	// Топы
	bot.Handle("/top", b.HandleLocalTop)
	bot.Handle("/globaltop", b.HandleGlobalTop)
	bot.Handle("\ftop_btn", b.HandleTopCallback)

	// Помощь и язык
	bot.Handle("/help", b.HandleHelp)
	bot.Handle("\fhelp_nav", b.HandleHelpCallback)
	bot.Handle("/locale", b.HandleLocale)
	bot.Handle("\flang_set", b.HandleLanguageSetCallback)

	// Дуэли
	bot.Handle("/duel", b.HandleDuel)
	bot.Handle("\fduel_accept", b.HandleDuelCallback)
	bot.Handle("\fduel_cancel", b.HandleDuelCallback)

	// Крафт и донат
	bot.Handle("/craft", b.HandleCraft)
	bot.Handle("/shop", b.HandleDonate)
	bot.Handle("\fshop_rolls_menu", b.HandleShopMenu)
	bot.Handle("\fbuy_invoice", b.HandleSendInvoice)
	bot.Handle("\froll_again", b.HandleRollAgainCallback)
	bot.Handle(tele.OnCheckout, b.HandlePreCheckout)
	bot.Handle(tele.OnPayment, b.HandlePayment)

	// Предложка карточек
	bot.Handle("\fsuggest_start", b.HandleSuggestStart)
	bot.Handle("\fs_q1_yes", b.HandleSuggestQuiz)
	bot.Handle("\fs_q1_no", b.HandleSuggestQuiz)
	bot.Handle("\fs_q2_yes", b.HandleSuggestQuiz)
	bot.Handle("\fs_q2_no", b.HandleSuggestQuiz)
	bot.Handle("\fs_q3_43", b.HandleSuggestQuiz)
	bot.Handle("\fs_q3_34", b.HandleSuggestQuiz)
	bot.Handle("\fs_q3_11", b.HandleSuggestQuiz)
	bot.Handle("\fs_cancel", b.HandleSuggestCancel)
	bot.Handle("\fs_close", b.HandleSuggestClose)

	bot.Handle("/promo", b.HandlePromo)
	bot.Handle("/addpromo", b.HandleAddPromo) // Админская команда
	bot.Handle("/createpromo", b.HandleCreatePromo)

	bot.Handle("\fsets_nav", b.HandleSetsList)
	bot.Handle("\fset_view", b.HandleSetView)
	bot.Handle("\fset_equip", b.HandleEquipAura)

	// Перехватчики контента
	bot.Handle(tele.OnPhoto, b.HandleMediaSuggest)
	bot.Handle(tele.OnDocument, b.HandleMediaSuggest)
	bot.Handle(tele.OnText, b.HandleTextFallback) // Защита от дурака

	bot.Handle("/link", b.HandleLinkStart)

	// Перехватчик всех ненайденных коллбэков (для дебага)
	bot.Handle(tele.OnCallback, func(ctx tele.Context) error {
		log.Printf("⚠️ ПРИШЕЛ НЕИЗВЕСТНЫЙ КОЛЛБЭК! Unique: '%s', Data: '%s'", ctx.Callback().Unique, ctx.Callback().Data)
		return ctx.Respond(&tele.CallbackResponse{Text: "Неизвестная кнопка!"})
	})
}
