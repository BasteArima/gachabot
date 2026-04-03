package telegram

import (
	"log"
	"strings"

	tele "gopkg.in/telebot.v3"
)

func (b *Bot) buildShopMenu(lang string) (string, *tele.ReplyMarkup) {
	menu := &tele.ReplyMarkup{}

	btn1 := menu.Data(b.loc.T(lang, "btn_shop_1"), "buy_invoice", "1")
	btn5 := menu.Data(b.loc.T(lang, "btn_shop_5"), "buy_invoice", "5")
	btn25 := menu.Data(b.loc.T(lang, "btn_shop_25"), "buy_invoice", "25")
	btn100 := menu.Data(b.loc.T(lang, "btn_shop_100"), "buy_invoice", "100")

	menu.Inline(
		menu.Row(btn1),
		menu.Row(btn5),
		menu.Row(btn25),
		menu.Row(btn100),
	)

	return b.loc.T(lang, "shop_menu_text"), menu
}

func (b *Bot) HandleDonate(ctx tele.Context) error {
	tgUser := ctx.Sender()
	dbUser, _ := b.repo.GetOrCreateUserByTelegramID(tgUser.ID, tgUser.Username, tgUser.FirstName, tgUser.LastName)
	lang := getLang(dbUser, tgUser)

	text, menu := b.buildShopMenu(lang)
	return ctx.Send(text, tele.ModeHTML, menu)
}

func (b *Bot) HandleShopMenu(ctx tele.Context) error {
	_ = ctx.Respond()
	tgUser := ctx.Sender()
	dbUser, _ := b.repo.GetOrCreateUserByTelegramID(tgUser.ID, tgUser.Username, tgUser.FirstName, tgUser.LastName)
	lang := getLang(dbUser, tgUser)

	text, menu := b.buildShopMenu(lang)
	return ctx.Edit(text, tele.ModeHTML, menu)
}

func (b *Bot) HandleSendInvoice(ctx tele.Context) error {
	_ = ctx.Respond()
	tgUser := ctx.Sender()
	dbUser, _ := b.repo.GetOrCreateUserByTelegramID(tgUser.ID, tgUser.Username, tgUser.FirstName, tgUser.LastName)
	lang := getLang(dbUser, tgUser)

	packageType := ctx.Callback().Data

	var title, description, payload string
	var price int

	switch packageType {
	case "1":
		title = b.loc.T(lang, "shop_title_1")
		description = b.loc.T(lang, "shop_desc_1")
		payload = "buy_rolls_1"
		price = 15
	case "5":
		title = b.loc.T(lang, "shop_title_5")
		description = b.loc.T(lang, "shop_desc_5")
		payload = "buy_rolls_5"
		price = 65
	case "25":
		title = b.loc.T(lang, "shop_title_25")
		description = b.loc.T(lang, "shop_desc_25")
		payload = "buy_rolls_25"
		price = 290
	case "100":
		title = b.loc.T(lang, "shop_title_100")
		description = b.loc.T(lang, "shop_desc_100")
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

func (b *Bot) HandlePreCheckout(ctx tele.Context) error {
	checkout := ctx.PreCheckoutQuery()
	if strings.HasPrefix(checkout.Payload, "buy_rolls_") {
		return ctx.Accept()
	}
	return ctx.Accept()
}

func (b *Bot) HandlePayment(ctx tele.Context) error {
	payment := ctx.Message().Payment
	tgUser := ctx.Sender()
	dbUser, err := b.repo.GetOrCreateUserByTelegramID(tgUser.ID, tgUser.Username, tgUser.FirstName, tgUser.LastName)
	if err != nil {
		return err
	}
	lang := getLang(dbUser, tgUser)

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

	err = b.repo.AddPremiumRolls(dbUser.ID, rollsToAdd)
	if err != nil {
		log.Printf("Ошибка начисления роллов %d: %v", dbUser.ID, err)
		return ctx.Send(b.loc.T(lang, "err_payment_db"))
	}

	bonusText := ""

	if isEpicFragment {
		epicCard, err := b.repo.GetRandomCard(4)
		if err == nil {
			fragCount, _ := b.repo.AddFragment(dbUser.ID, epicCard.ID)
			bonusText = b.loc.T(lang, "payment_bonus_frag", epicCard.Name, fragCount)

			if fragCount >= 10 {
				_ = b.repo.ClearFragments(dbUser.ID, epicCard.ID)
				_ = b.repo.AddCardToInventory(dbUser.ID, epicCard.ID)
				bonusText += b.loc.T(lang, "payment_bonus_assembled")
			}
		} else {
			log.Printf("Не удалось выдать эпик осколок: %v", err)
		}
	}

	// Возврат средств админу для тестов
	if tgUser.ID == 348389728 {
		params := map[string]interface{}{
			"user_id":                    tgUser.ID,
			"telegram_payment_charge_id": payment.TelegramChargeID,
		}
		_, refundErr := ctx.Bot().Raw("refundStarPayment", params)
		if refundErr == nil {
			bonusText += b.loc.T(lang, "payment_test_refund")
		}
	}

	successMsg := b.loc.T(lang, "payment_success", rollsToAdd, bonusText)
	return ctx.Send(successMsg, tele.ModeHTML)
}
