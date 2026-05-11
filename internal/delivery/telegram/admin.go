package telegram

import (
	"fmt"
	"strconv"
	"strings"

	tele "gopkg.in/telebot.v3"
)

// Smart Promo Code Generator (accepts JSON from a web builder)
func (b *Bot) HandleCreatePromo(ctx tele.Context) error {
	tgUser := ctx.Sender()

	if tgUser.ID != b.adminID {
		return nil
	}

	args := ctx.Args()
	if len(args) < 4 {
		return ctx.Send("Формат: `/createpromo КОД ЛИМИТ ЧАСЫ {\"points\": 100}`\n(0 = без ограничений)", tele.ModeMarkdown)
	}

	code := strings.ToUpper(args[0])
	maxUses, _ := strconv.Atoi(args[1])
	hoursValid, _ := strconv.Atoi(args[2])
	jsonPayload := strings.Join(args[3:], " ")

	err := b.service.CreatePromoFromJson(code, maxUses, hoursValid, jsonPayload)
	if err != nil {
		return ctx.Send("❌ Ошибка: " + err.Error())
	}

	msg := fmt.Sprintf("✅ Промокод **%s** успешно загружен!\nЛимит: %d юзов\nВремя жизни: %d часов", code, maxUses, hoursValid)
	return ctx.Send(msg, tele.ModeMarkdown)
}

// A simple promo code generator for admins
func (b *Bot) HandleAddPromo(ctx tele.Context) error {
	tgUser := ctx.Sender()

	if tgUser.ID != b.adminID {
		return nil
	}

	args := ctx.Args()
	if len(args) < 4 {
		return ctx.Send("Формат: `/addpromo КОД ОЧКИ КРУТКИ МАКС_ЮЗОВ`\nПример: `/addpromo START 5000 5 100`", tele.ModeMarkdown)
	}

	code := strings.ToUpper(args[0])
	points, _ := strconv.Atoi(args[1])
	rolls, _ := strconv.Atoi(args[2])
	maxUses, _ := strconv.Atoi(args[3])

	err := b.service.CreateSimplePromo(code, points, rolls, maxUses)
	if err != nil {
		return ctx.Send("❌ Ошибка БД: " + err.Error())
	}

	return ctx.Send(fmt.Sprintf("✅ Промокод **%s** успешно создан!\nОчки: %d, Крутки: %d, Лимит: %d", code, points, rolls, maxUses), tele.ModeMarkdown)
}
