package telegram

import (
	"log"
	"strconv"
	"strings"

	tele "gopkg.in/telebot.v3"
)

func (b *Bot) HandleProfile(ctx tele.Context) error {
	tgUser := ctx.Sender()
	dbUser, _ := b.repo.GetOrCreateUserByTelegramID(tgUser.ID, tgUser.Username, tgUser.FirstName, tgUser.LastName)
	lang := getLang(dbUser, tgUser)

	profile, err := b.service.GetUserProfile(dbUser.ID)
	if err != nil {
		log.Printf("Ошибка получения профиля юзера %d: %v", dbUser.ID, err)
		return ctx.Send(b.loc.T(lang, "profile_err_load"))
	}

	caption := b.loc.T(lang, "profile_caption",
		tgUser.FirstName, tgUser.LastName,
		profile.UniqueCardsCount, profile.TotalCardsCount,
		profile.Balance, profile.DuplicatesCount, profile.StreakDays)

	photos, err := ctx.Bot().ProfilePhotosOf(tgUser)

	menu := &tele.ReplyMarkup{}
	var rows []tele.Row
	if profile.UniqueCardsCount > 0 {
		btnMyCards := menu.Data(b.loc.T(lang, "btn_my_cards", profile.UniqueCardsCount), "cards_nav", "0")
		btnMySets := menu.Data(b.loc.T(lang, "btn_my_sets", profile.CompletedSets, profile.TotalSets), "sets_nav", "0")

		rows = append(rows, menu.Row(btnMyCards))
		rows = append(rows, menu.Row(btnMySets))
	}

	btnSuggest := menu.Data(b.loc.T(lang, "btn_profile_suggest"), "suggest_start")
	btnBack := menu.Data(b.loc.T(lang, "btn_back_to_start"), "start_menu")

	rows = append(rows, menu.Row(btnSuggest))
	rows = append(rows, menu.Row(btnBack))

	menu.Inline(rows...)

	if err == nil && len(photos) > 0 {
		photo := &tele.Photo{
			File:    photos[0].File,
			Caption: caption,
		}

		// Если это вызов по кнопке из Хаба (который тоже картинка), делаем красивый Edit!
		if ctx.Callback() != nil && ctx.Message().Photo != nil {
			return ctx.Edit(photo, tele.ModeHTML, menu)
		}

		// Иначе просто отправляем
		return ctx.Send(photo, tele.ModeHTML, menu)
	}

	// Если аватарки нет, шлем просто текст.
	// Если пришли по кнопке, удаляем старую картинку Хаба и шлем текст
	if ctx.Callback() != nil {
		_ = ctx.Delete()
	}
	return ctx.Send(caption, tele.ModeHTML, menu)
}

func (b *Bot) HandleCardsNav(ctx tele.Context) error {
	_ = ctx.Respond()
	tgUser := ctx.Sender()
	dbUser, _ := b.repo.GetOrCreateUserByTelegramID(tgUser.ID, tgUser.Username, tgUser.FirstName, tgUser.LastName)
	lang := getLang(dbUser, tgUser)

	offsetStr := ctx.Callback().Data
	offset, _ := strconv.Atoi(offsetStr)

	// Получаем текущую карточку и общее кол-во уникальных карт
	card, total, err := b.service.GetUserCardPagination(dbUser.ID, offset)
	if err != nil {
		return ctx.Send(b.loc.T(lang, "cards_err_load"))
	}

	if card == nil {
		_ = ctx.Delete()
		return ctx.Send(b.loc.T(lang, "cards_empty"))
	}

	var caption string
	if card.SetName != "" {
		caption = b.loc.T(lang, "card_nav_caption_with_set",
			card.CardName, card.SetName, card.RarityName, card.PowerLevel, card.Quantity, offset+1, total)
	} else {
		caption = b.loc.T(lang, "card_nav_caption",
			card.CardName, card.RarityName, card.PowerLevel, card.Quantity, offset+1, total)
	}

	menu := &tele.ReplyMarkup{}
	var navRow []tele.Btn

	// --- ЛОГИКА ЦИКЛИЧНОГО ЛИСТАНИЯ ---
	if total > 1 {
		// Если мы на 0, то (0-1 + total) % total даст индекс последней карты
		prev := (offset - 1 + total) % total
		// Если мы на последней, то (last+1) % total даст 0
		next := (offset + 1) % total

		btnBack := menu.Data(b.loc.T(lang, "btn_back"), "cards_nav", strconv.Itoa(prev))
		btnForward := menu.Data(b.loc.T(lang, "btn_forward"), "cards_nav", strconv.Itoa(next))

		navRow = append(navRow, btnBack, btnForward)
	}

	btnProfile := menu.Data(b.loc.T(lang, "btn_to_profile"), "back_profile")

	// Собираем меню: кнопки навигации в один ряд, кнопка профиля под ними
	if len(navRow) > 0 {
		menu.Inline(menu.Row(navRow...), menu.Row(btnProfile))
	} else {
		menu.Inline(menu.Row(btnProfile))
	}

	photo := &tele.Photo{
		File:    tele.FromURL(card.ImageURL),
		Caption: caption,
	}

	err = ctx.Edit(photo, tele.ModeHTML, menu)
	if err != nil && !strings.Contains(err.Error(), "message is not modified") {
		return err
	}
	return nil
}

func (b *Bot) HandleBackToProfile(ctx tele.Context) error {
	_ = ctx.Respond()
	return b.HandleProfile(ctx)
}
