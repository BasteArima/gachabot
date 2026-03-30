package telegram

import (
	"fmt"
	"gachabot/internal/i18n"
	"log"
	"strconv"
	"strings"
	"time"

	"gachabot/internal/repository"
	"gachabot/internal/service"

	tele "gopkg.in/telebot.v3"
)

type Handler struct {
	repo        *repository.PostgresRepo
	service     *service.GachaService
	duelService *service.DuelService
	loc         *i18n.Localizer
}

func NewHandler(repo *repository.PostgresRepo, service *service.GachaService, duelService *service.DuelService, loc *i18n.Localizer) *Handler {
	return &Handler{
		repo:        repo,
		service:     service,
		duelService: duelService,
		loc:         loc,
	}
}

func (h *Handler) HandleStart(ctx tele.Context) error {
	lang := ctx.Sender().LanguageCode
	sticker := &tele.Sticker{File: tele.File{FileID: "CAACAgIAAxkBAAMGacUaTK2nsNg77On1KstHV1B6SbMAAj-HAAJOpnFK7SHSkw_YzeE6BA"}}

	menu := &tele.ReplyMarkup{}
	btnAddGroup := menu.URL(h.loc.T(lang, "btn_add_group"), "https://t.me/HentaiCard_bot?startgroup=true")
	menu.Inline(menu.Row(btnAddGroup))

	if err := ctx.Send(sticker); err != nil {
		log.Println("Cant send sticker:", err)
	}

	return ctx.Send(h.loc.T(lang, "start_msg"), tele.ModeHTML, menu)
}

func (h *Handler) HandleRoll(ctx tele.Context) error {
	user := ctx.Sender()
	lang := user.LanguageCode
	h.service.TrackChat(user.ID, ctx.Chat().ID)

	result, err := h.service.RollCard(user.ID, user.Username, user.FirstName, user.LastName)
	if err != nil {
		log.Printf("[ROLL ERROR] Ошибка сервиса для %d (%s): %v", user.ID, user.Username, err)
		return ctx.Send(h.loc.T(lang, "error_tech"))
	}

	if result.OnCooldown {
		msg := h.loc.T(lang, "roll_cooldown", result.CooldownTimeLeft)
		menu := &tele.ReplyMarkup{}
		btnBuy := menu.Data(h.loc.T(lang, "btn_buy_rolls"), "shop_rolls_menu")
		menu.Inline(menu.Row(btnBuy))

		return ctx.Send(msg, &tele.SendOptions{ReplyTo: ctx.Message(), ParseMode: tele.ModeHTML, ReplyMarkup: menu})
	}

	var caption string
	if result.IsFragment {
		if result.CardAssembled {
			caption = h.loc.T(lang, "roll_mythic_assembled", result.Card.Name, result.Card.PowerLevel, result.Reward)
		} else {
			caption = h.loc.T(lang, "roll_mythic_fragment", result.Card.Name, result.FragmentsCount, result.Reward)
		}
	} else {
		caption = h.loc.T(lang, "roll_success", result.Card.Name, result.RarityName, result.Card.PowerLevel, result.Reward)
	}

	dynamicURL := fmt.Sprintf("%s?v=%d", result.Card.ImageURL, time.Now().Unix())
	photo := &tele.Photo{
		File:    tele.FromURL(dynamicURL),
		Caption: caption,
	}

	menu := &tele.ReplyMarkup{}
	btnRoll := menu.Data(h.loc.T(lang, "btn_roll_again"), "roll_again")
	menu.Inline(menu.Row(btnRoll))

	err = ctx.Send(photo, &tele.SendOptions{
		ParseMode:   tele.ModeHTML,
		ReplyTo:     ctx.Message(),
		ReplyMarkup: menu,
	})

	if err != nil {
		log.Printf("[TELEGRAM ERROR] Не удалось отправить фото! URL: %s | Ошибка: %v", result.Card.ImageURL, err)
		return ctx.Send(caption+h.loc.T(lang, "error_image"), tele.ModeHTML)
	}

	return nil
}

func (h *Handler) HandleRollAgainCallback(ctx tele.Context) error {
	_ = ctx.Respond()
	return h.HandleRoll(ctx)
}

func (h *Handler) HandleProfile(ctx tele.Context) error {
	user := ctx.Sender()
	lang := user.LanguageCode

	profile, err := h.service.GetUserProfile(user.ID)
	if err != nil {
		log.Printf("Ошибка получения профиля юзера %d: %v", user.ID, err)
		return ctx.Send(h.loc.T(lang, "profile_err_load"))
	}

	caption := h.loc.T(lang, "profile_caption",
		user.FirstName, user.LastName,
		profile.UniqueCardsCount, profile.TotalCardsCount,
		profile.DuplicatesCount, profile.Balance, profile.StreakDays)

	photos, err := ctx.Bot().ProfilePhotosOf(user)

	menu := &tele.ReplyMarkup{}
	var rows []tele.Row
	if profile.UniqueCardsCount > 0 {
		btnMyCards := menu.Data(h.loc.T(lang, "btn_my_cards", profile.UniqueCardsCount), "cards_nav", "0")
		rows = append(rows, menu.Row(btnMyCards))
	}

	btnAddGroup := menu.URL(h.loc.T(lang, "btn_add_group_bot"), "https://t.me/HentaiCard_bot?startgroup=true")
	rows = append(rows, menu.Row(btnAddGroup))

	menu.Inline(rows...)

	if err == nil && len(photos) > 0 {
		photo := &tele.Photo{
			File:    photos[0].File,
			Caption: caption,
		}
		return ctx.Send(photo, tele.ModeHTML, menu)
	}

	return ctx.Send(caption, tele.ModeHTML, menu)
}

func (h *Handler) HandleCardsNav(ctx tele.Context) error {
	_ = ctx.Respond()
	user := ctx.Sender()
	lang := user.LanguageCode

	offsetStr := ctx.Callback().Data
	offset, _ := strconv.Atoi(offsetStr)

	card, total, err := h.service.GetUserCardPagination(user.ID, offset)
	if err != nil {
		return ctx.Send(h.loc.T(lang, "cards_err_load"))
	}

	if card == nil {
		_ = ctx.Delete()
		return ctx.Send(h.loc.T(lang, "cards_empty"))
	}

	caption := h.loc.T(lang, "card_nav_caption",
		card.CardName, card.RarityName, card.PowerLevel, card.Quantity, offset+1, total)

	menu := &tele.ReplyMarkup{}
	var row []tele.Btn

	if offset > 0 {
		row = append(row, menu.Data(h.loc.T(lang, "btn_back"), "cards_nav", strconv.Itoa(offset-1)))
	}
	if offset < total-1 {
		row = append(row, menu.Data(h.loc.T(lang, "btn_forward"), "cards_nav", strconv.Itoa(offset+1)))
	}

	btnProfile := menu.Data(h.loc.T(lang, "btn_to_profile"), "back_profile")

	if len(row) > 0 {
		menu.Inline(menu.Row(row...), menu.Row(btnProfile))
	} else {
		menu.Inline(menu.Row(btnProfile))
	}

	photo := &tele.Photo{
		File:    tele.FromURL(card.ImageURL),
		Caption: caption,
	}

	err = ctx.Edit(photo, tele.ModeHTML, menu)
	if err != nil {
		if strings.Contains(err.Error(), "message is not modified") {
			return nil
		}
		return err
	}
	return nil
}

func (h *Handler) HandleBackToProfile(ctx tele.Context) error {
	_ = ctx.Respond()
	_ = ctx.Delete()
	return h.HandleProfile(ctx)
}

func (h *Handler) buildTopMessage(criteria string, scope string, chatID int64, lang string) (string, *tele.ReplyMarkup, error) {
	targetChatID := chatID
	if scope == "global" {
		targetChatID = 0
	}

	board, err := h.service.GetLeaderboard(criteria, targetChatID)
	if err != nil {
		return "", nil, err
	}

	scopeName := h.loc.T(lang, "top_global_title")
	if scope == "local" {
		scopeName = h.loc.T(lang, "top_local_title")
	}

	critName, emoji := "", ""
	switch criteria {
	case "balance":
		critName, emoji = h.loc.T(lang, "top_crit_balance"), "<tg-emoji emoji-id=\"4918300654197277832\">🪙</tg-emoji>"
	case "cards":
		critName, emoji = h.loc.T(lang, "top_crit_cards"), "🃏"
	case "streak":
		critName, emoji = h.loc.T(lang, "top_crit_streak"), "🔥"
	}

	text := h.loc.T(lang, "top_header", scopeName, critName)
	if len(board) == 0 {
		text += h.loc.T(lang, "top_empty")
	} else {
		for i, entry := range board {
			medal := "🏅"
			if i == 0 {
				medal = "🥇"
			} else if i == 1 {
				medal = "🥈"
			} else if i == 2 {
				medal = "🥉"
			}
			text += h.loc.T(lang, "top_entry", medal, entry.DisplayName, entry.Value, emoji)
		}
	}
	text += "</blockquote>"

	menu := &tele.ReplyMarkup{}
	btnBal := menu.Data(h.loc.T(lang, "btn_top_balance"), "top_btn", "balance|"+scope)
	btnCards := menu.Data(h.loc.T(lang, "btn_top_cards"), "top_btn", "cards|"+scope)
	btnStreak := menu.Data(h.loc.T(lang, "btn_top_streak"), "top_btn", "streak|"+scope)

	menu.Inline(menu.Row(btnBal, btnCards, btnStreak))

	return text, menu, nil
}

func (h *Handler) HandleLocalTop(ctx tele.Context) error {
	lang := ctx.Sender().LanguageCode
	h.service.TrackChat(ctx.Sender().ID, ctx.Chat().ID)

	if ctx.Chat().Type == tele.ChatPrivate {
		menu := &tele.ReplyMarkup{}
		btnAdd := menu.URL(h.loc.T(lang, "btn_add_group"), "https://t.me/HentaiCard_bot?startgroup=true")
		menu.Inline(menu.Row(btnAdd))
		return ctx.Send(h.loc.T(lang, "err_top_private"), menu)
	}

	text, menu, err := h.buildTopMessage("balance", "local", ctx.Chat().ID, lang)
	if err != nil {
		return ctx.Send(h.loc.T(lang, "err_top_load"))
	}
	return ctx.Send(text, tele.ModeHTML, menu)
}

func (h *Handler) HandleGlobalTop(ctx tele.Context) error {
	lang := ctx.Sender().LanguageCode
	h.service.TrackChat(ctx.Sender().ID, ctx.Chat().ID)

	text, menu, err := h.buildTopMessage("balance", "global", ctx.Chat().ID, lang)
	if err != nil {
		return ctx.Send(h.loc.T(lang, "err_top_load"))
	}
	return ctx.Send(text, tele.ModeHTML, menu)
}

func (h *Handler) HandleTopCallback(ctx tele.Context) error {
	_ = ctx.Respond()
	lang := ctx.Sender().LanguageCode

	data := strings.Split(ctx.Callback().Data, "|")
	if len(data) != 2 {
		return nil
	}
	criteria, scope := data[0], data[1]

	text, menu, err := h.buildTopMessage(criteria, scope, ctx.Chat().ID, lang)
	if err != nil {
		return ctx.Send(h.loc.T(lang, "err_top_update"))
	}

	err = ctx.Edit(text, tele.ModeHTML, menu)
	if err != nil {
		if strings.Contains(err.Error(), "message is not modified") {
			return nil
		}
		return err
	}
	return nil
}

func (h *Handler) buildHelpMessage(section string, lang string) (string, *tele.ReplyMarkup) {
	menu := &tele.ReplyMarkup{}

	btnMain := menu.Data(h.loc.T(lang, "btn_help_main"), "help_nav", "main")
	btnCards := menu.Data(h.loc.T(lang, "btn_help_cards"), "help_nav", "cards")
	btnRarities := menu.Data(h.loc.T(lang, "btn_help_rarities"), "help_nav", "rarities")
	btnStreaks := menu.Data(h.loc.T(lang, "btn_help_streaks"), "help_nav", "streaks")
	btnPity := menu.Data(h.loc.T(lang, "btn_help_pity"), "help_nav", "pity")
	btnDuel := menu.Data(h.loc.T(lang, "btn_help_duel"), "help_nav", "duel")
	btnCraft := menu.Data(h.loc.T(lang, "btn_help_craft"), "help_nav", "craft")

	if section == "main" {
		menu.Inline(
			menu.Row(btnCards, btnRarities),
			menu.Row(btnStreaks, btnPity),
			menu.Row(btnDuel, btnCraft),
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

	// Пытаемся получить перевод
	text := h.loc.T(lang, key)

	// Наш локализатор возвращает сам ключ, если перевод не найден.
	// Так мы можем понять, что раздела не существует.
	if text == key {
		text = h.loc.T(lang, "err_help_not_found")
	}

	return text, menu
}

func (h *Handler) HandleHelp(ctx tele.Context) error {
	lang := ctx.Sender().LanguageCode
	text, menu := h.buildHelpMessage("main", lang)
	return ctx.Send(text, tele.ModeHTML, menu)
}

func (h *Handler) HandleHelpCallback(ctx tele.Context) error {
	_ = ctx.Respond()
	lang := ctx.Sender().LanguageCode

	section := ctx.Callback().Data
	text, menu := h.buildHelpMessage(section, lang)

	err := ctx.Edit(text, tele.ModeHTML, menu)
	if err != nil {
		if strings.Contains(err.Error(), "message is not modified") {
			return nil
		}
		return err
	}
	return nil
}

func (h *Handler) HandleDuel(ctx tele.Context) error {
	lang := ctx.Sender().LanguageCode

	if ctx.Chat().Type == tele.ChatPrivate {
		return ctx.Send(h.loc.T(lang, "err_duel_private"))
	}

	args := ctx.Args()
	if len(args) < 2 {
		return ctx.Send(h.loc.T(lang, "err_duel_usage"))
	}

	targetUsername := strings.TrimPrefix(args[0], "@")
	amount, err := strconv.Atoi(args[1])
	if err != nil || amount <= 0 {
		return ctx.Send(h.loc.T(lang, "err_duel_amount"))
	}

	challenger := ctx.Sender()
	challengerDB, err := h.repo.GetUser(challenger.ID)
	if err != nil || challengerDB.Balance < amount {
		return ctx.Send(h.loc.T(lang, "err_duel_funds"))
	}

	targetUser, err := h.repo.GetUserByUsername(targetUsername)
	if err != nil {
		return ctx.Send(h.loc.T(lang, "err_duel_not_found", targetUsername))
	}

	if targetUser.TgID == challenger.ID {
		return ctx.Send(h.loc.T(lang, "err_duel_self"))
	}

	duelID := fmt.Sprintf("%d_%d_%d", challenger.ID, targetUser.TgID, time.Now().Unix())

	h.duelService.CreateDuel(duelID, challenger.ID, challenger.FirstName, targetUser.TgID, targetUsername, amount)

	menu := &tele.ReplyMarkup{}
	btnAccept := menu.Data(h.loc.T(lang, "btn_duel_accept"), "duel_accept", duelID)
	btnCancel := menu.Data(h.loc.T(lang, "btn_duel_cancel"), "duel_cancel", duelID)
	menu.Inline(menu.Row(btnAccept, btnCancel))

	caption := h.loc.T(lang, "duel_challenge", challenger.FirstName, targetUsername, amount)
	return ctx.Send(caption, tele.ModeHTML, menu)
}

func (h *Handler) HandleDuelCallback(ctx tele.Context) error {
	duelID := ctx.Callback().Data
	callbackUnique := strings.TrimPrefix(ctx.Callback().Unique, "\f")
	lang := ctx.Sender().LanguageCode

	duel, exists := h.duelService.GetDuel(duelID)
	if !exists {
		_ = ctx.Respond(&tele.CallbackResponse{Text: h.loc.T(lang, "err_duel_expired")})
		return ctx.Delete()
	}

	userID := ctx.Sender().ID

	if callbackUnique == "duel_cancel" {
		if userID != duel.ChallengerID && userID != duel.TargetID {
			return ctx.Respond(&tele.CallbackResponse{Text: h.loc.T(lang, "err_duel_not_yours")})
		}
		h.duelService.PopDuel(duelID)
		_ = ctx.Respond(&tele.CallbackResponse{Text: h.loc.T(lang, "duel_cancelled_toast")})
		return ctx.Edit(h.loc.T(lang, "duel_cancelled", duel.ChallengerName, duel.TargetName), tele.ModeHTML)
	}

	if callbackUnique == "duel_accept" {
		if userID != duel.TargetID {
			return ctx.Respond(&tele.CallbackResponse{Text: h.loc.T(lang, "err_duel_not_called")})
		}

		h.duelService.PopDuel(duelID)

		result, err := h.duelService.ExecuteDuel(duel)
		if err != nil {
			return ctx.Edit(h.loc.T(lang, "err_duel_exec", err.Error()))
		}

		resText := h.loc.T(lang, "duel_result",
			duel.ChallengerName, result.CardChallenger.Name, result.CardChallenger.PowerLevel, result.ChanceChallenger,
			duel.TargetName, result.CardTarget.Name, result.CardTarget.PowerLevel, result.ChanceTarget,
			result.Roll, result.WinnerName, result.AmountWon*2)

		return ctx.Edit(resText, tele.ModeHTML)
	}

	return nil
}

func (h *Handler) HandleCraft(ctx tele.Context) error {
	user := ctx.Sender()
	lang := user.LanguageCode

	result, err := h.service.CraftCard(user.ID)
	if err != nil {
		return ctx.Send(h.loc.T(lang, "err_craft_failed", err.Error()), tele.ModeHTML)
	}

	var caption string
	if result.IsFragment {
		if !result.CardAssembled {
			caption = h.loc.T(lang, "craft_mythic_frag", result.CraftCost, result.Card.Name, result.FragmentsCount)
		} else {
			caption = h.loc.T(lang, "craft_mythic_assembled", result.CraftCost, result.Card.Name, result.Card.PowerLevel)
		}
	} else {
		caption = h.loc.T(lang, "craft_success", result.CraftCost, result.Card.Name, result.RarityName, result.Card.PowerLevel)
	}

	photo := &tele.Photo{
		File:    tele.FromURL(result.Card.ImageURL),
		Caption: caption,
	}

	return ctx.Send(photo, tele.ModeHTML)
}

func (h *Handler) buildShopMenu(lang string) (string, *tele.ReplyMarkup) {
	menu := &tele.ReplyMarkup{}

	btn1 := menu.Data(h.loc.T(lang, "btn_shop_1"), "buy_invoice", "1")
	btn5 := menu.Data(h.loc.T(lang, "btn_shop_5"), "buy_invoice", "5")
	btn25 := menu.Data(h.loc.T(lang, "btn_shop_25"), "buy_invoice", "25")
	btn100 := menu.Data(h.loc.T(lang, "btn_shop_100"), "buy_invoice", "100")

	menu.Inline(
		menu.Row(btn1),
		menu.Row(btn5),
		menu.Row(btn25),
		menu.Row(btn100),
	)

	return h.loc.T(lang, "shop_menu_text"), menu
}

func (h *Handler) HandleDonate(ctx tele.Context) error {
	lang := ctx.Sender().LanguageCode
	text, menu := h.buildShopMenu(lang)
	return ctx.Send(text, tele.ModeHTML, menu)
}

func (h *Handler) HandleShopMenu(ctx tele.Context) error {
	_ = ctx.Respond()
	lang := ctx.Sender().LanguageCode
	text, menu := h.buildShopMenu(lang)
	return ctx.Edit(text, tele.ModeHTML, menu)
}

func (h *Handler) HandleSendInvoice(ctx tele.Context) error {
	_ = ctx.Respond()
	lang := ctx.Sender().LanguageCode
	packageType := ctx.Callback().Data

	var title, description, payload string
	var price int

	switch packageType {
	case "1":
		title = h.loc.T(lang, "shop_title_1")
		description = h.loc.T(lang, "shop_desc_1")
		payload = "buy_rolls_1"
		price = 15
	case "5":
		title = h.loc.T(lang, "shop_title_5")
		description = h.loc.T(lang, "shop_desc_5")
		payload = "buy_rolls_5"
		price = 65
	case "25":
		title = h.loc.T(lang, "shop_title_25")
		description = h.loc.T(lang, "shop_desc_25")
		payload = "buy_rolls_25"
		price = 290
	case "100":
		title = h.loc.T(lang, "shop_title_100")
		description = h.loc.T(lang, "shop_desc_100")
		payload = "buy_rolls_100"
		price = 999
	default:
		return nil
	}

	invoice := &tele.Invoice{
		Title:       title,
		Description: description,
		Payload:     payload,
		Currency:    "XTR",
		Prices:      []tele.Price{{Label: title, Amount: price}},
	}

	_, err := ctx.Bot().Send(ctx.Sender(), invoice)
	return err
}

func (h *Handler) HandlePreCheckout(ctx tele.Context) error {
	checkout := ctx.PreCheckoutQuery()
	if strings.HasPrefix(checkout.Payload, "buy_rolls_") {
		return ctx.Accept()
	}
	return ctx.Accept()
}

func (h *Handler) HandlePayment(ctx tele.Context) error {
	payment := ctx.Message().Payment
	user := ctx.Sender()
	lang := user.LanguageCode

	var rollsToAdd int
	var isEpicFragment bool

	switch payment.Payload {
	case "buy_rolls_1":
		rollsToAdd = 1
	case "buy_rolls_5":
		rollsToAdd = 5
	case "buy_rolls_25":
		rollsToAdd = 25
	case "buy_rolls_100":
		rollsToAdd = 100
		isEpicFragment = true
	default:
		return nil
	}

	err := h.repo.AddPremiumRolls(user.ID, rollsToAdd)
	if err != nil {
		log.Printf("Ошибка начисления роллов %d: %v", user.ID, err)
		return ctx.Send(h.loc.T(lang, "err_payment_db"))
	}

	bonusText := ""

	if isEpicFragment {
		epicCard, err := h.repo.GetRandomCard(4)
		if err == nil {
			fragCount, _ := h.repo.AddFragment(user.ID, epicCard.ID)
			bonusText = h.loc.T(lang, "payment_bonus_frag", epicCard.Name, fragCount)

			if fragCount >= 10 {
				_ = h.repo.ClearFragments(user.ID, epicCard.ID)
				_ = h.repo.AddCardToInventory(user.ID, epicCard.ID)
				bonusText += h.loc.T(lang, "payment_bonus_assembled")
			}
		} else {
			log.Printf("Не удалось выдать эпик осколок: %v", err)
		}
	}

	if user.ID == 348389728 {
		params := map[string]interface{}{
			"user_id":                    user.ID,
			"telegram_payment_charge_id": payment.TelegramChargeID,
		}
		_, refundErr := ctx.Bot().Raw("refundStarPayment", params)
		if refundErr == nil {
			bonusText += h.loc.T(lang, "payment_test_refund")
		}
	}

	successMsg := h.loc.T(lang, "payment_success", rollsToAdd, bonusText)
	return ctx.Send(successMsg, tele.ModeHTML)
}
