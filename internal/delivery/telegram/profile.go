package telegram

import (
	"gachabot/internal/i18n"
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
		return ctx.Send(b.loc.Translate(lang, "profile_err_load"))
	}

	caption := b.loc.Translate(lang, "profile_caption", i18n.Args{
		"first_name": tgUser.FirstName,
		"last_name":  tgUser.LastName,
		"unique":     profile.UniqueCardsCount,
		"total":      profile.TotalCardsCount,
		"balance":    profile.Balance,
		"duplicates": profile.DuplicatesCount,
		"streak":     profile.StreakDays,
	})

	photos, err := ctx.Bot().ProfilePhotosOf(tgUser)

	menu := &tele.ReplyMarkup{}
	var rows []tele.Row
	if profile.UniqueCardsCount > 0 {
		btnMyCards := menu.Data(b.loc.Translate(lang, "btn_my_cards", i18n.Args{"count": profile.UniqueCardsCount}), "cards_nav", "0")
		btnMySets := menu.Data(b.loc.Translate(lang, "btn_my_sets", i18n.Args{"completed": profile.CompletedSets, "total": profile.TotalSets}), "sets_nav", "0")

		rows = append(rows, menu.Row(btnMyCards))
		rows = append(rows, menu.Row(btnMySets))
	}

	btnSuggest := menu.Data(b.loc.Translate(lang, "btn_profile_suggest"), "suggest_start")
	btnBack := menu.Data(b.loc.Translate(lang, "btn_back_to_start"), "start_menu")

	rows = append(rows, menu.Row(btnSuggest))
	rows = append(rows, menu.Row(btnBack))

	menu.Inline(rows...)

	if err == nil && len(photos) > 0 {
		photo := &tele.Photo{
			File:    photos[0].File,
			Caption: caption,
		}

		if ctx.Callback() != nil && ctx.Message().Photo != nil {
			return ctx.Edit(photo, tele.ModeHTML, menu)
		}

		return ctx.Send(photo, tele.ModeHTML, menu)
	}

	// If there is no avatar, just send text
	// If the user clicked the button, we delete the old Hub image and send the text
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

	card, total, err := b.service.GetUserCardPagination(dbUser.ID, offset)
	if err != nil {
		return ctx.Send(b.loc.Translate(lang, "cards_err_load"))
	}

	if card == nil {
		_ = ctx.Delete()
		return ctx.Send(b.loc.Translate(lang, "cards_empty"))
	}

	var caption string
	if card.SetName != "" {
		caption = b.loc.Translate(lang, "card_nav_caption_with_set", i18n.Args{
			"name": card.CardName, "set": card.SetName, "rarity": b.loc.Rarity(lang, card.RarityName),
			"power": card.PowerLevel, "quantity": card.Quantity, "num": offset + 1, "total": total})
	} else {
		caption = b.loc.Translate(lang, "card_nav_caption", i18n.Args{
			"name": card.CardName, "rarity": b.loc.Rarity(lang, card.RarityName),
			"power": card.PowerLevel, "quantity": card.Quantity, "num": offset + 1, "total": total})
	}

	menu := &tele.ReplyMarkup{}
	var navRow []tele.Btn

	if total > 1 {
		prev := (offset - 1 + total) % total
		next := (offset + 1) % total

		btnBack := menu.Data(b.loc.Translate(lang, "btn_back"), "cards_nav", strconv.Itoa(prev))
		btnForward := menu.Data(b.loc.Translate(lang, "btn_forward"), "cards_nav", strconv.Itoa(next))

		navRow = append(navRow, btnBack, btnForward)
	}

	btnProfile := menu.Data(b.loc.Translate(lang, "btn_to_profile"), "back_profile")

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
