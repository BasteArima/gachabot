package telegram

import (
	"fmt"
	"gachabot/internal/i18n"
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
		translatedReason := b.loc.Translate(lang, err.Error())
		return ctx.Send(b.loc.Translate(lang, "err_craft_failed", i18n.Args{"reason": translatedReason}), tele.ModeHTML)
	}

	var caption string
	if result.IsFragment {
		if !result.CardAssembled {
			caption = b.loc.Translate(lang, "craft_mythic_frag", i18n.Args{"cost": result.CraftCost, "name": result.Card.Name, "fragments": result.FragmentsCount})
		} else {
			caption = b.loc.Translate(lang, "craft_mythic_assembled", i18n.Args{"cost": result.CraftCost, "name": result.Card.Name, "power": result.Card.PowerLevel})
		}
	} else {
		caption = b.loc.Translate(lang, "craft_success", i18n.Args{"cost": result.CraftCost, "name": result.Card.Name, "rarity": b.loc.Rarity(lang, result.RarityName), "power": result.Card.PowerLevel})
	}

	photo := &tele.Photo{
		File:    tele.FromURL(result.Card.ImageURL),
		Caption: caption,
	}

	err = ctx.Send(photo, tele.ModeHTML)

	// Separate message when collecting a set
	if result.CompletedSetName != "" {
		msg := b.loc.Translate(lang, "set_completed_msg", i18n.Args{"name": result.CompletedSetName, "reward": result.CompletedSetReward})
		_ = ctx.Reply(msg, tele.ModeHTML)
	}

	return err
}

func (b *Bot) HandleDuel(ctx tele.Context) error {
	tgUser := ctx.Sender()
	challengerDB, _ := b.repo.GetOrCreateUserByTelegramID(tgUser.ID, tgUser.Username, tgUser.FirstName, tgUser.LastName)
	lang := getLang(challengerDB, tgUser)

	if ctx.Chat().Type == tele.ChatPrivate {
		return ctx.Send(b.loc.Translate(lang, "err_duel_private"))
	}

	args := ctx.Args()
	if len(args) < 2 {
		return ctx.Send(b.loc.Translate(lang, "err_duel_usage"))
	}

	targetUsername := strings.TrimPrefix(args[0], "@")
	amount, err := strconv.Atoi(args[1])
	if err != nil || amount <= 0 {
		return ctx.Send(b.loc.Translate(lang, "err_duel_amount"))
	}

	if challengerDB.Balance < amount {
		return ctx.Send(b.loc.Translate(lang, "err_duel_funds"))
	}

	targetUserDB, err := b.repo.GetUserByUsername(targetUsername)
	if err != nil {
		return ctx.Send(b.loc.Translate(lang, "err_duel_not_found", i18n.Args{"target": targetUsername}))
	}

	if targetUserDB.ID == challengerDB.ID {
		return ctx.Send(b.loc.Translate(lang, "err_duel_self"))
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
	btnAccept := menu.Data(b.loc.Translate(lang, "btn_duel_accept"), "duel_accept", duelID)
	btnCancel := menu.Data(b.loc.Translate(lang, "btn_duel_cancel"), "duel_cancel", duelID)
	menu.Inline(menu.Row(btnAccept, btnCancel))

	caption := b.loc.Translate(lang, "duel_challenge", i18n.Args{"challenger": challengerDB.FirstName, "target": targetUsername, "amount": amount})
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
		_ = ctx.Respond(&tele.CallbackResponse{Text: b.loc.Translate(lang, "err_duel_expired")})
		return ctx.Delete()
	}

	internalUserID := dbUser.ID

	if callbackUnique == "duel_cancel" {
		if internalUserID != duel.ChallengerID && internalUserID != duel.TargetID {
			return ctx.Respond(&tele.CallbackResponse{Text: b.loc.Translate(lang, "err_duel_not_yours")})
		}
		b.duelService.PopDuel(duelID)
		_ = ctx.Respond(&tele.CallbackResponse{Text: b.loc.Translate(lang, "duel_cancelled_toast")})
		return ctx.Edit(b.loc.Translate(lang, "duel_cancelled", i18n.Args{"challenger": duel.ChallengerName, "target": duel.TargetName}), tele.ModeHTML)
	}

	if callbackUnique == "duel_accept" {
		if internalUserID != duel.TargetID {
			return ctx.Respond(&tele.CallbackResponse{Text: b.loc.Translate(lang, "err_duel_not_called")})
		}

		_ = ctx.Respond() // Remove the clock from the button

		// Editing a message with buttons, turning it into a status bar
		// Remove buttons so that no one presses unnecessary ones
		statusText := b.loc.Translate(lang, "duel_start")
		_ = ctx.Edit(statusText, tele.ModeHTML)

		for k := 0; k < 3; k++ {
			time.Sleep(600 * time.Millisecond)
			dots := strings.Repeat(".", k+1)
			_ = ctx.Edit(statusText+"<b>"+dots+"</b>", tele.ModeHTML)
		}

		b.duelService.PopDuel(duelID)
		result, err := b.duelService.ExecuteDuel(duel)
		if err != nil {
			return ctx.Edit(b.loc.Translate(lang, "err_duel_exec", i18n.Args{"error": err.Error()}))
		}

		formatAura := func(aura *models.DuelAuraInfo) string {
			if aura == nil {
				return ""
			}
			// Локализуем тип баффа (например, power_percent -> "Буст силы")
			buffName := b.loc.Translate(lang, "buff_type_"+aura.BuffType)
			return fmt.Sprintf("\n╰ ✨ <b>%s</b> (+%d%% %s)", aura.Name, aura.BuffValue, buffName)
		}

		cAura := formatAura(result.ChallengerAura)
		tAura := formatAura(result.TargetAura)

		fairNote := ""
		if result.IsFair {
			fairNote = b.loc.Translate(lang, "duel_fair_note") + "\n\n"
		}

		// Собираем текст
		resText := b.loc.Translate(lang, "duel_result", i18n.Args{
			"challenger":        duel.ChallengerName,
			"challenger_aura":   cAura,
			"challenger_card":   result.CardChallenger.Name,
			"challenger_power":  result.CardChallenger.PowerLevel,
			"challenger_chance": fmt.Sprintf("%.1f", result.ChanceChallenger),
			"target":            duel.TargetName,
			"target_aura":       tAura,
			"target_card":       result.CardTarget.Name,
			"target_power":      result.CardTarget.PowerLevel,
			"target_chance":     fmt.Sprintf("%.1f", result.ChanceTarget),
			"fair_note":         fairNote,
			"roll":              fmt.Sprintf("%.1f", result.Roll),
			"winner":            result.WinnerName,
			"amount":            result.AmountWon * 2,
		})

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

	if b.require18Plus {
		if !dbUser.IsAdult.Valid || !dbUser.IsAdult.Bool {
			menu := &tele.ReplyMarkup{}
			btnYes := menu.Data(b.loc.Translate(lang, "btn_adult_yes"), "adult_yes")
			btnNo := menu.Data(b.loc.Translate(lang, "btn_adult_no"), "adult_no")
			menu.Inline(menu.Row(btnYes), menu.Row(btnNo))

			msg := b.loc.Translate(lang, "adult_verification_msg")

			if ctx.Callback() != nil && ctx.Message() != nil && ctx.Message().Photo != nil {
				_ = ctx.Delete()
			}
			return ctx.Send(msg, tele.ModeHTML, menu)
		}

		if dbUser.IsAdult.Valid && !dbUser.IsAdult.Bool {
			msg := b.loc.Translate(lang, "adult_rejected_msg")
			if ctx.Callback() != nil && ctx.Message() != nil && ctx.Message().Photo != nil {
				_ = ctx.Delete()
			}
			return ctx.Send(msg, tele.ModeHTML)
		}
	}

	menu := &tele.ReplyMarkup{}
	btnRoll := menu.Data(b.loc.Translate(lang, "btn_roll_shortcut"), "roll_shortcut")
	btnProfile := menu.Data(b.loc.Translate(lang, "btn_profile_menu"), "profile_menu")
	btnHelp := menu.Data(b.loc.Translate(lang, "btn_help_menu"), "help_menu")
	btnAddGroup := menu.URL(b.loc.Translate(lang, "btn_add_group"), b.groupLink)

	menu.Inline(
		menu.Row(btnProfile, btnHelp),
		menu.Row(btnAddGroup),
		menu.Row(btnRoll),
	)

	text := b.loc.Translate(lang, "start_msg")
	banner := &tele.Photo{
		File:    tele.FromURL(b.startBannerURL),
		Caption: text,
	}

	if ctx.Callback() == nil {
		return ctx.Send(banner, tele.ModeHTML, menu)
	}

	if ctx.Message() != nil && ctx.Message().Photo != nil {
		return ctx.Edit(banner, tele.ModeHTML, menu)
	}

	_ = ctx.Delete()
	return ctx.Send(banner, tele.ModeHTML, menu)
}

func (b *Bot) HandleAdultConfirm(ctx tele.Context) error {
	_ = ctx.Respond()
	tgUser := ctx.Sender()
	dbUser, _ := b.repo.GetOrCreateUserByTelegramID(tgUser.ID, tgUser.Username, tgUser.FirstName, tgUser.LastName)

	_ = b.repo.SetUserAdultStatus(dbUser.ID, true)

	_ = ctx.Delete()
	return b.HandleStart(ctx)
}

func (b *Bot) HandleAdultReject(ctx tele.Context) error {
	_ = ctx.Respond()
	tgUser := ctx.Sender()
	dbUser, _ := b.repo.GetOrCreateUserByTelegramID(tgUser.ID, tgUser.Username, tgUser.FirstName, tgUser.LastName)
	lang := getLang(dbUser, tgUser)

	_ = b.repo.SetUserAdultStatus(dbUser.ID, false)

	_ = ctx.Delete()
	return ctx.Send(b.loc.Translate(lang, "adult_rejected_msg"), tele.ModeHTML)
}

func (b *Bot) HandleProfileMenu(ctx tele.Context) error {
	_ = ctx.Respond()
	return b.HandleProfile(ctx)
}

func (b *Bot) HandleStartMenu(ctx tele.Context) error {
	_ = ctx.Respond()
	return b.HandleStart(ctx)
}

func (b *Bot) HandleRoll(ctx tele.Context) error {
	tgUser := ctx.Sender()

	dbUser, err := b.repo.GetOrCreateUserByTelegramID(tgUser.ID, tgUser.Username, tgUser.FirstName, tgUser.LastName)
	if err != nil {
		log.Printf("[DB ERROR] Ошибка получения юзера: %v", err)
		return ctx.Send("Техническая ошибка БД.")
	}

	lang := getLang(dbUser, tgUser)

	if b.require18Plus {
		if !dbUser.IsAdult.Valid || !dbUser.IsAdult.Bool {
			menu := &tele.ReplyMarkup{}
			btnYes := menu.Data(b.loc.Translate(lang, "btn_adult_yes"), "adult_yes")
			btnNo := menu.Data(b.loc.Translate(lang, "btn_adult_no"), "adult_no")
			menu.Inline(menu.Row(btnYes), menu.Row(btnNo))

			msg := b.loc.Translate(lang, "adult_verification_msg")

			if ctx.Callback() != nil {
				_ = ctx.Delete()
			}
			return ctx.Send(msg, tele.ModeHTML, menu)
		}

		if dbUser.IsAdult.Valid && !dbUser.IsAdult.Bool {
			msg := b.loc.Translate(lang, "adult_rejected_msg")
			if ctx.Callback() != nil {
				_ = ctx.Delete()
			}
			return ctx.Send(msg, tele.ModeHTML)
		}
	}

	b.service.TrackChat(dbUser.ID, ctx.Chat().ID)

	result, err := b.service.RollCard(dbUser.ID)
	if err != nil {
		log.Printf("[ROLL ERROR] Ошибка сервиса для ID %d (%s): %v", dbUser.ID, dbUser.Username, err)
		return ctx.Send(b.loc.Translate(lang, "error_tech"))
	}

	if result.OnCooldown {
		msg := b.loc.Translate(lang, "roll_cooldown", i18n.Args{"time": result.CooldownTimeLeft})

		if result.StreakUpdated {
			streakMsg := b.loc.Translate(lang, "streak_kept_alive", i18n.Args{"days": result.StreakDays})
			msg = msg + "\n\n" + streakMsg
		}

		menu := &tele.ReplyMarkup{}
		btnBuy := menu.Data(b.loc.Translate(lang, "btn_buy_rolls"), "shop_rolls_menu")
		menu.Inline(menu.Row(btnBuy))

		return ctx.Send(msg, &tele.SendOptions{ReplyTo: ctx.Message(), ParseMode: tele.ModeHTML, ReplyMarkup: menu})
	}

	var caption string
	if result.IsFragment {
		if result.CardAssembled {
			caption = b.loc.Translate(lang, "roll_mythic_assembled", i18n.Args{"name": result.Card.Name, "power": result.Card.PowerLevel, "reward": result.Reward})
		} else {
			caption = b.loc.Translate(lang, "roll_mythic_fragment", i18n.Args{"name": result.Card.Name, "fragments": result.FragmentsCount, "reward": result.Reward})
		}
	} else {
		caption = b.loc.Translate(lang, "roll_success", i18n.Args{"name": result.Card.Name, "rarity": b.loc.Rarity(lang, result.RarityName), "power": result.Card.PowerLevel, "reward": result.Reward})
	}

	dynamicURL := fmt.Sprintf("%s?v=%d", result.Card.ImageURL, time.Now().Unix())
	photo := &tele.Photo{
		File:    tele.FromURL(dynamicURL),
		Caption: caption,
	}

	menu := &tele.ReplyMarkup{}
	btnRoll := menu.Data(b.loc.Translate(lang, "btn_roll_again"), "roll_again")
	menu.Inline(menu.Row(btnRoll))

	err = ctx.Send(photo, &tele.SendOptions{
		ParseMode:   tele.ModeHTML,
		ReplyTo:     ctx.Message(),
		ReplyMarkup: menu,
	})

	if err != nil {
		log.Printf("[TELEGRAM ERROR] Не удалось отправить фото! URL: %s | Ошибка: %v", result.Card.ImageURL, err)
		return ctx.Send(caption+b.loc.Translate(lang, "error_image"), tele.ModeHTML)
	}

	if result.CompletedSetName != "" {
		msg := b.loc.Translate(lang, "set_completed_msg", i18n.Args{"name": result.CompletedSetName, "reward": result.CompletedSetReward})
		_ = ctx.Reply(msg, tele.ModeHTML)
	}

	if result.StreakUpdated {
		if result.StreakDays == 1 {
			_ = ctx.Send(`<tg-emoji emoji-id="4956499161319998529">🔥</tg-emoji>`, tele.ModeHTML)
			_ = ctx.Send(b.loc.Translate(lang, "streak_started"), tele.ModeHTML)
		} else {
			msg := b.loc.Translate(lang, "streak_continued", i18n.Args{"reward": result.Reward, "days": result.StreakDays})
			_ = ctx.Send(msg, tele.ModeHTML)
		}
	}

	return nil
}

func (b *Bot) HandleRollAgainCallback(ctx tele.Context) error {
	_ = ctx.Respond()
	return b.HandleRoll(ctx)
}

// Handles clicking on the "Roll" button from the menu
func (b *Bot) HandleRollShortcut(ctx tele.Context) error {
	_ = ctx.Respond()
	return b.HandleRoll(ctx)
}
