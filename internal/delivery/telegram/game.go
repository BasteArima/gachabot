package telegram

import (
	"fmt"
	"gachabot/internal/models"
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

	photo := &tele.Photo{
		File:    tele.FromURL(result.Card.ImageURL),
		Caption: caption,
	}

	err = ctx.Send(photo, tele.ModeHTML)

	// --- ОТДЕЛЬНОЕ СООБЩЕНИЕ ПРИ СБОРЕ СЕТА ---
	if result.CompletedSetName != "" {
		msg := b.loc.T(lang, "set_completed_msg", result.CompletedSetName, result.CompletedSetReward)
		_ = ctx.Reply(msg, tele.ModeHTML)
	}

	return err
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

	isFair := false
	for _, arg := range args {
		if strings.ToLower(arg) == "fair" {
			isFair = true
		}
	}

	b.duelService.CreateDuel(duelID, challengerDB.ID, challengerDB.FirstName, targetUserDB.ID, targetUsername, amount, isFair)

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
		// Подготавливаем красивые строчки с аурами, если они есть
		formatAura := func(aura *models.DuelAuraInfo) string {
			if aura == nil {
				return ""
			}
			// Локализуем тип баффа (например, power_percent -> "Буст силы")
			buffName := b.loc.T(lang, "buff_type_"+aura.BuffType)
			return fmt.Sprintf("\n╰ ✨ <b>%s</b> (+%d%% %s)", aura.Name, aura.BuffValue, buffName)
		}

		cAura := formatAura(result.ChallengerAura)
		tAura := formatAura(result.TargetAura)

		fairNote := ""
		if result.IsFair {
			fairNote = b.loc.T(lang, "duel_fair_note") + "\n\n"
		}

		// Собираем текст
		resText := b.loc.T(lang, "duel_result",
			duel.ChallengerName, cAura, result.CardChallenger.Name, result.CardChallenger.PowerLevel, result.ChanceChallenger,
			duel.TargetName, tAura, result.CardTarget.Name, result.CardTarget.PowerLevel, result.ChanceTarget,
			fairNote, result.Roll, result.WinnerName, result.AmountWon*2)

		album := tele.Album{
			&tele.Photo{File: tele.FromURL(result.CardChallenger.ImageURL), Caption: resText},
			&tele.Photo{File: tele.FromURL(result.CardTarget.ImageURL)},
		}

		_ = ctx.Delete()
		return ctx.SendAlbum(album, tele.ModeHTML)
	}

	return nil
}

func (b *Bot) HandleStart(ctx tele.Context) error {
	tgUser := ctx.Sender()
	dbUser, _ := b.repo.GetOrCreateUserByTelegramID(tgUser.ID, tgUser.Username, tgUser.FirstName, tgUser.LastName)
	lang := getLang(dbUser, tgUser)

	menu := &tele.ReplyMarkup{}
	btnRoll := menu.Data(b.loc.T(lang, "btn_roll_shortcut"), "roll_shortcut")
	btnProfile := menu.Data(b.loc.T(lang, "btn_profile_menu"), "profile_menu")
	btnHelp := menu.Data(b.loc.T(lang, "btn_help_menu"), "help_menu")
	btnAddGroup := menu.URL(b.loc.T(lang, "btn_add_group"), "https://t.me/HentaiCard_bot?startgroup=true")

	menu.Inline(
		menu.Row(btnProfile, btnHelp),
		menu.Row(btnAddGroup),
		menu.Row(btnRoll),
	)

	text := b.loc.T(lang, "start_msg")

	// Наш новый крутой баннер!
	banner := &tele.Photo{
		// Укажи тут URL к сгенерированной картинке или путь к локальному файлу
		File:    tele.FromURL("https://api.baste.ru/cards/banner.webp"),
		Caption: text,
	}

	// 1. Если это вызов по команде /start (НОВЫЙ ХАБ)
	if ctx.Callback() == nil {
		return ctx.Send(banner, tele.ModeHTML, menu)
	}

	// 2. ВОЗВРАТ В ХАБ ПО КНОПКЕ

	// Так как Хаб теперь картинка, мы проверяем, откуда мы пришли.
	// Если мы пришли из Профиля (там тоже картинка - аватарка), то мы можем сделать красивый Edit!
	if ctx.Message().Photo != nil {
		return ctx.Edit(banner, tele.ModeHTML, menu)
	}

	// Если мы пришли из Справки (там был просто текст), мы НЕ МОЖЕМ сделать Edit.
	// Поэтому удаляем текст Справки и присылаем новую картинку Хаба.
	_ = ctx.Delete()
	return ctx.Send(banner, tele.ModeHTML, menu)
}

// Обрабатывает нажатие на кнопку "Профиль" из главного меню
func (b *Bot) HandleProfileMenu(ctx tele.Context) error {
	_ = ctx.Respond()
	// Мы переходим из Хаба (Картинки) в Профиль (Картинку).
	// Поэтому НЕ удаляем сообщение! HandleProfile сможет сделать красивый ctx.Edit.
	return b.HandleProfile(ctx)
}

// Обрабатывает кнопку "Назад в главное меню"
func (b *Bot) HandleStartMenu(ctx tele.Context) error {
	_ = ctx.Respond()
	return b.HandleStart(ctx)
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

	if result.CompletedSetName != "" {
		msg := b.loc.T(lang, "set_completed_msg", result.CompletedSetName, result.CompletedSetReward)
		_ = ctx.Reply(msg, tele.ModeHTML)
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

// Обрабатывает нажатие на кнопку "Крутить" из меню
func (b *Bot) HandleRollShortcut(ctx tele.Context) error {
	_ = ctx.Respond() // Убираем часики с кнопки
	return b.HandleRoll(ctx)
}
