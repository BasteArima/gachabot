package telegram

import (
	"gachabot/internal/models"
	"strings"

	tele "gopkg.in/telebot.v3"
)

// getLang берет язык из БД, а если его нет — из настроек Телеграма юзера
func getLang(dbUser *models.User, tgUser *tele.User) string {
	if dbUser != nil && dbUser.LanguageCode != "" {
		return dbUser.LanguageCode
	}
	if tgUser != nil && (tgUser.LanguageCode == "ru" || tgUser.LanguageCode == "en") {
		return tgUser.LanguageCode
	}
	return "en"
}

// --- ЛОКАЛИЗАЦИЯ И НАСТРОЙКИ ---
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
		return ctx.Send("❌ Неизвестный язык / Unknown language. Используйте 'ru' или 'en'.")
	}

	b.repo.SetUserLanguage(dbUser.ID, targetLang)
	return ctx.Send(b.loc.T(targetLang, "lang_changed"), tele.ModeHTML)
}

func (b *Bot) HandleLanguageSetCallback(ctx tele.Context) error {
	tgUser := ctx.Sender()
	dbUser, _ := b.repo.GetOrCreateUserByTelegramID(tgUser.ID, tgUser.Username, tgUser.FirstName, tgUser.LastName)
	targetLang := ctx.Callback().Data

	b.repo.SetUserLanguage(dbUser.ID, targetLang)
	_ = ctx.Respond(&tele.CallbackResponse{Text: b.loc.T(targetLang, "lang_changed_toast")})

	text, menu := b.buildHelpMessage("language", targetLang)
	return ctx.Edit(text, tele.ModeHTML, menu)
}

func (b *Bot) buildHelpMessage(section string, lang string) (string, *tele.ReplyMarkup) {
	menu := &tele.ReplyMarkup{}

	btnMain := menu.Data(b.loc.T(lang, "btn_help_main"), "help_nav", "main")
	btnCards := menu.Data(b.loc.T(lang, "btn_help_cards"), "help_nav", "cards")
	btnRarities := menu.Data(b.loc.T(lang, "btn_help_rarities"), "help_nav", "rarities")
	btnStreaks := menu.Data(b.loc.T(lang, "btn_help_streaks"), "help_nav", "streaks")
	btnPity := menu.Data(b.loc.T(lang, "btn_help_pity"), "help_nav", "pity")
	btnDuel := menu.Data(b.loc.T(lang, "btn_help_duel"), "help_nav", "duel")
	btnCraft := menu.Data(b.loc.T(lang, "btn_help_craft"), "help_nav", "craft")
	btnLang := menu.Data(b.loc.T(lang, "btn_help_lang"), "help_nav", "language")

	if section == "main" {
		menu.Inline(
			menu.Row(btnCards, btnRarities),
			menu.Row(btnStreaks, btnPity),
			menu.Row(btnDuel, btnCraft),
			menu.Row(btnLang),
		)
	} else if section == "language" {
		btnRu := menu.Data("🇷🇺 Русский", "lang_set", "ru")
		btnEn := menu.Data("🇬🇧 English", "lang_set", "en")

		menu.Inline(
			menu.Row(btnRu, btnEn),
			menu.Row(btnMain),
		)
	} else {
		menu.Inline(
			menu.Row(btnCards, btnRarities),
			menu.Row(btnStreaks, btnPity),
			menu.Row(btnDuel, btnCraft),
			menu.Row(btnMain),
		)
	}

	key := "help_" + section
	text := b.loc.T(lang, key)

	if text == key {
		text = b.loc.T(lang, "err_help_not_found")
	}

	return text, menu
}

func (b *Bot) HandleHelp(ctx tele.Context) error {
	tgUser := ctx.Sender()
	dbUser, _ := b.repo.GetOrCreateUserByTelegramID(tgUser.ID, tgUser.Username, tgUser.FirstName, tgUser.LastName)
	lang := getLang(dbUser, tgUser)

	text, menu := b.buildHelpMessage("main", lang)
	return ctx.Send(text, tele.ModeHTML, menu)
}

func (b *Bot) HandleHelpCallback(ctx tele.Context) error {
	_ = ctx.Respond()
	tgUser := ctx.Sender()
	dbUser, _ := b.repo.GetOrCreateUserByTelegramID(tgUser.ID, tgUser.Username, tgUser.FirstName, tgUser.LastName)
	lang := getLang(dbUser, tgUser)

	section := ctx.Callback().Data
	text, menu := b.buildHelpMessage(section, lang)

	err := ctx.Edit(text, tele.ModeHTML, menu)
	if err != nil && !strings.Contains(err.Error(), "message is not modified") {
		return err
	}
	return nil
}
