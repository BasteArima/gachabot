package telegram

import (
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	tele "gopkg.in/telebot.v3"
)

func (b *Bot) HandleCraft(ctx tele.Context) error {
	tgUser := ctx.Sender()
	dbUser, _ := b.repo.GetOrCreateUserByTelegramID(tgUser.ID, tgUser.Username, tgUser.FirstName, tgUser.LastName)
	lang := getLang(dbUser, tgUser)

	result, err := b.service.CraftCard(dbUser.ID)
	if err != nil {
		return ctx.Send(b.loc.T(lang, "err_craft_failed", err.Error()), tele.ModeHTML)
	}

	var caption string
	if result.IsFragment {
		if !result.CardAssembled {
			caption = b.loc.T(lang, "craft_mythic_frag", result.CraftCost, result.Card.Name, result.FragmentsCount)
		} else {
			caption = b.loc.T(lang, "craft_mythic_assembled", result.CraftCost, result.Card.Name, result.Card.PowerLevel)
		}
	} else {
		caption = b.loc.T(lang, "craft_success", result.CraftCost, result.Card.Name, result.RarityName, result.Card.PowerLevel)
	}

	// Если сет был собран, добавляем поздравление к тексту!
	if result.CompletedSetName != "" {
		caption += b.loc.T(lang, "set_completed_msg", result.CompletedSetName, result.CompletedSetReward)
	}

	photo := &tele.Photo{
		File:    tele.FromURL(result.Card.ImageURL),
		Caption: caption,
	}

	return ctx.Send(photo, tele.ModeHTML)
}

func (b *Bot) HandleDuel(ctx tele.Context) error {
	tgUser := ctx.Sender()
	challengerDB, _ := b.repo.GetOrCreateUserByTelegramID(tgUser.ID, tgUser.Username, tgUser.FirstName, tgUser.LastName)
	lang := getLang(challengerDB, tgUser)

	if ctx.Chat().Type == tele.ChatPrivate {
		return ctx.Send(b.loc.T(lang, "err_duel_private"))
	}

	args := ctx.Args()
	if len(args) < 2 {
		return ctx.Send(b.loc.T(lang, "err_duel_usage"))
	}

	targetUsername := strings.TrimPrefix(args[0], "@")
	amount, err := strconv.Atoi(args[1])
	if err != nil || amount <= 0 {
		return ctx.Send(b.loc.T(lang, "err_duel_amount"))
	}

	if challengerDB.Balance < amount {
		return ctx.Send(b.loc.T(lang, "err_duel_funds"))
	}

	targetUserDB, err := b.repo.GetUserByUsername(targetUsername)
	if err != nil {
		return ctx.Send(b.loc.T(lang, "err_duel_not_found", targetUsername))
	}

	if targetUserDB.ID == challengerDB.ID {
		return ctx.Send(b.loc.T(lang, "err_duel_self"))
	}

	duelID := fmt.Sprintf("%d_%d_%d", challengerDB.ID, targetUserDB.ID, time.Now().Unix())

	b.duelService.CreateDuel(duelID, challengerDB.ID, challengerDB.FirstName, targetUserDB.ID, targetUsername, amount)

	menu := &tele.ReplyMarkup{}
	btnAccept := menu.Data(b.loc.T(lang, "btn_duel_accept"), "duel_accept", duelID)
	btnCancel := menu.Data(b.loc.T(lang, "btn_duel_cancel"), "duel_cancel", duelID)
	menu.Inline(menu.Row(btnAccept, btnCancel))

	caption := b.loc.T(lang, "duel_challenge", challengerDB.FirstName, targetUsername, amount)
	return ctx.Send(caption, tele.ModeHTML, menu)
}

func (b *Bot) HandleDuelCallback(ctx tele.Context) error {
	duelID := ctx.Callback().Data
	callbackUnique := strings.TrimPrefix(ctx.Callback().Unique, "\f")

	tgUser := ctx.Sender()
	dbUser, _ := b.repo.GetOrCreateUserByTelegramID(tgUser.ID, tgUser.Username, tgUser.FirstName, tgUser.LastName)
	lang := getLang(dbUser, tgUser)

	duel, exists := b.duelService.GetDuel(duelID)
	if !exists {
		_ = ctx.Respond(&tele.CallbackResponse{Text: b.loc.T(lang, "err_duel_expired")})
		return ctx.Delete()
	}

	internalUserID := dbUser.ID

	if callbackUnique == "duel_cancel" {
		if internalUserID != duel.ChallengerID && internalUserID != duel.TargetID {
			return ctx.Respond(&tele.CallbackResponse{Text: b.loc.T(lang, "err_duel_not_yours")})
		}
		b.duelService.PopDuel(duelID)
		_ = ctx.Respond(&tele.CallbackResponse{Text: b.loc.T(lang, "duel_cancelled_toast")})
		return ctx.Edit(b.loc.T(lang, "duel_cancelled", duel.ChallengerName, duel.TargetName), tele.ModeHTML)
	}

	if callbackUnique == "duel_accept" {
		if internalUserID != duel.TargetID {
			return ctx.Respond(&tele.CallbackResponse{Text: b.loc.T(lang, "err_duel_not_called")})
		}

		_ = ctx.Respond() // Убираем часики с кнопки

		// 2. Редактируем сообщение с кнопками, превращая его в статус-бар
		// Убираем кнопки, чтобы никто не нажал лишнего
		statusText := b.loc.T(lang, "duel_start")
		_ = ctx.Edit(statusText, tele.ModeHTML)

		// 3. Анимируем многоточие (3 шага с паузой)
		for k := 0; k < 3; k++ {
			time.Sleep(600 * time.Millisecond)
			dots := strings.Repeat(".", k+1)
			_ = ctx.Edit(statusText+"<b>"+dots+"</b>", tele.ModeHTML)
		}

		// 4. Пока идет "анимация", готовим данные боя в фоне
		b.duelService.PopDuel(duelID)
		result, err := b.duelService.ExecuteDuel(duel)
		if err != nil {
			return ctx.Edit(b.loc.T(lang, "err_duel_exec", err.Error()))
		}

		// 5. Формируем финальный альбом
		resText := b.loc.T(lang, "duel_result",
			duel.ChallengerName, result.CardChallenger.Name, result.CardChallenger.PowerLevel, result.ChanceChallenger,
			duel.TargetName, result.CardTarget.Name, result.CardTarget.PowerLevel, result.ChanceTarget,
			result.Roll, result.WinnerName, result.AmountWon*2)

		album := tele.Album{
			&tele.Photo{File: tele.FromURL(result.CardChallenger.ImageURL), Caption: resText},
			&tele.Photo{File: tele.FromURL(result.CardTarget.ImageURL)},
		}

		// 6. ФИНАЛ: Удаляем всё лишнее и показываем результат
		_ = ctx.Delete() // Удаляем текстовый статус-бар

		return ctx.SendAlbum(album, tele.ModeHTML)
	}

	return nil
}

func (b *Bot) HandleStart(ctx tele.Context) error {
	tgUser := ctx.Sender()
	dbUser, _ := b.repo.GetOrCreateUserByTelegramID(tgUser.ID, tgUser.Username, tgUser.FirstName, tgUser.LastName)
	lang := getLang(dbUser, tgUser)

	sticker := &tele.Sticker{File: tele.File{FileID: "CAACAgIAAxkBAAMGacUaTK2nsNg77On1KstHV1B6SbMAAj-HAAJOpnFK7SHSkw_YzeE6BA"}}

	menu := &tele.ReplyMarkup{}
	btnAddGroup := menu.URL(b.loc.T(lang, "btn_add_group"), "https://t.me/HentaiCard_bot?startgroup=true")
	menu.Inline(menu.Row(btnAddGroup))

	if err := ctx.Send(sticker); err != nil {
		log.Println("Cant send sticker:", err)
	}

	return ctx.Send(b.loc.T(lang, "start_msg"), tele.ModeHTML, menu)
}

func (b *Bot) HandleRoll(ctx tele.Context) error {
	tgUser := ctx.Sender()

	// АДАПТЕР: превращаем TG User во внутреннего пользователя
	dbUser, err := b.repo.GetOrCreateUserByTelegramID(tgUser.ID, tgUser.Username, tgUser.FirstName, tgUser.LastName)
	if err != nil {
		log.Printf("[DB ERROR] Ошибка получения юзера: %v", err)
		return ctx.Send("Техническая ошибка БД.")
	}

	lang := getLang(dbUser, tgUser)
	b.service.TrackChat(dbUser.ID, ctx.Chat().ID)

	// БИЗНЕС-ЛОГИКА: передаем ВНУТРЕННИЙ ID
	result, err := b.service.RollCard(dbUser.ID)
	if err != nil {
		log.Printf("[ROLL ERROR] Ошибка сервиса для ID %d (%s): %v", dbUser.ID, dbUser.Username, err)
		return ctx.Send(b.loc.T(lang, "error_tech"))
	}

	if result.OnCooldown {
		msg := b.loc.T(lang, "roll_cooldown", result.CooldownTimeLeft)

		if result.StreakUpdated {
			streakMsg := b.loc.T(lang, "streak_kept_alive", result.StreakDays)
			msg = msg + "\n\n" + streakMsg
		}

		menu := &tele.ReplyMarkup{}
		btnBuy := menu.Data(b.loc.T(lang, "btn_buy_rolls"), "shop_rolls_menu")
		menu.Inline(menu.Row(btnBuy))

		return ctx.Send(msg, &tele.SendOptions{ReplyTo: ctx.Message(), ParseMode: tele.ModeHTML, ReplyMarkup: menu})
	}

	var caption string
	if result.IsFragment {
		if result.CardAssembled {
			caption = b.loc.T(lang, "roll_mythic_assembled", result.Card.Name, result.Card.PowerLevel, result.Reward)
		} else {
			caption = b.loc.T(lang, "roll_mythic_fragment", result.Card.Name, result.FragmentsCount, result.Reward)
		}
	} else {
		caption = b.loc.T(lang, "roll_success", result.Card.Name, result.RarityName, result.Card.PowerLevel, result.Reward)
	}

	// Если сет был собран, добавляем поздравление к тексту!
	if result.CompletedSetName != "" {
		caption += b.loc.T(lang, "set_completed_msg", result.CompletedSetName, result.CompletedSetReward)
	}

	dynamicURL := fmt.Sprintf("%s?v=%d", result.Card.ImageURL, time.Now().Unix())
	photo := &tele.Photo{
		File:    tele.FromURL(dynamicURL),
		Caption: caption,
	}

	menu := &tele.ReplyMarkup{}
	btnRoll := menu.Data(b.loc.T(lang, "btn_roll_again"), "roll_again")
	menu.Inline(menu.Row(btnRoll))

	err = ctx.Send(photo, &tele.SendOptions{
		ParseMode:   tele.ModeHTML,
		ReplyTo:     ctx.Message(),
		ReplyMarkup: menu,
	})

	if err != nil {
		log.Printf("[TELEGRAM ERROR] Не удалось отправить фото! URL: %s | Ошибка: %v", result.Card.ImageURL, err)
		return ctx.Send(caption+b.loc.T(lang, "error_image"), tele.ModeHTML)
	}

	if result.StreakUpdated {
		if result.StreakDays == 1 {
			_ = ctx.Send(`<tg-emoji emoji-id="4956499161319998529">🔥</tg-emoji>`, tele.ModeHTML)
			_ = ctx.Send(b.loc.T(lang, "streak_started"), tele.ModeHTML)
		} else {
			msg := b.loc.T(lang, "streak_continued", result.Reward, result.StreakDays)
			_ = ctx.Send(msg, tele.ModeHTML)
		}
	}

	return nil
}

func (b *Bot) HandleRollAgainCallback(ctx tele.Context) error {
	_ = ctx.Respond()
	return b.HandleRoll(ctx)
}
