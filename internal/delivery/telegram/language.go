package telegram

import (
	"gachabot/internal/models"
	"strings"

	tele "gopkg.in/telebot.v3"
)

func getLang(dbUser *models.User, tgUser *tele.User) string {
	if dbUser != nil && dbUser.LanguageCode != "" {
		return dbUser.LanguageCode
	}
	if tgUser != nil && (tgUser.LanguageCode == "ru" || tgUser.LanguageCode == "en") {
		return tgUser.LanguageCode
	}
	return "en"
}

func (b *Bot) HandleLocale(ctx tele.Context) error {
	tgUser := ctx.Sender()
	dbUser, _ := b.repo.GetOrCreateUserByTelegramID(tgUser.ID, tgUser.Username, tgUser.FirstName, tgUser.LastName)
	args := ctx.Args()

	if len(args) == 0 {
		lang := getLang(dbUser, tgUser)
		text, menu := b.buildHelpMessage("language", lang)
		return ctx.Send(text, tele.ModeHTML, menu)
	}

	input := strings.ToLower(args[0])
	var targetLang string

	switch input {
	case "ru", "rus", "russian", "рус", "русский":
		targetLang = "ru"
	case "en", "eng", "english", "англ", "английский":
		targetLang = "en"
	default:
		return ctx.Send("❌ Неизвестный язык / Unknown language. Используйте/Use 'ru' или/or 'en'.")
	}

	b.repo.SetUserLanguage(dbUser.ID, targetLang)
	return ctx.Send(b.loc.Translate(targetLang, "lang_changed"), tele.ModeHTML)
}

func (b *Bot) HandleLanguageSetCallback(ctx tele.Context) error {
	tgUser := ctx.Sender()
	dbUser, _ := b.repo.GetOrCreateUserByTelegramID(tgUser.ID, tgUser.Username, tgUser.FirstName, tgUser.LastName)
	targetLang := ctx.Callback().Data

	b.repo.SetUserLanguage(dbUser.ID, targetLang)
	_ = ctx.Respond(&tele.CallbackResponse{Text: b.loc.Translate(targetLang, "lang_changed_toast")})

	text, menu := b.buildHelpMessage("language", targetLang)
	return ctx.Edit(text, tele.ModeHTML, menu)
}
