package telegram

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

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

func (b *Bot) HandleGlobalMsg(ctx tele.Context) error {
	if ctx.Sender().ID != b.adminID {
		return nil
	}

	b.rdb.Set(context.Background(), fmt.Sprintf("admin_state:%d", b.adminID), "waiting_global_msg", 1*time.Hour)

	btnCancel := tele.InlineButton{Unique: "global_cancel", Text: "❌ Отмена"}
	markup := &tele.ReplyMarkup{
		InlineKeyboard: [][]tele.InlineButton{{btnCancel}},
	}

	return ctx.Send("Ожидаю вашего глобального одноразового сообщения (можно с картинкой).\nПришлите его следующим сообщением.", markup)
}

func (b *Bot) AdminStateMiddleware(next tele.HandlerFunc) tele.HandlerFunc {
	return func(ctx tele.Context) error {
		if ctx.Sender() != nil && ctx.Sender().ID == b.adminID && ctx.Callback() == nil {
			state, _ := b.rdb.Get(context.Background(), fmt.Sprintf("admin_state:%d", b.adminID)).Result()

			if state == "waiting_global_msg" && ctx.Text() != "/globalmsg" {
				msgID := ctx.Message().ID
				b.rdb.Set(context.Background(), fmt.Sprintf("global_msg_id:%d", b.adminID), msgID, 1*time.Hour)

				b.rdb.Set(context.Background(), fmt.Sprintf("admin_state:%d", b.adminID), "confirming_global_msg", 1*time.Hour)

				btnYes := tele.InlineButton{Unique: "global_send", Text: "✅ Да, отправить"}
				btnNo := tele.InlineButton{Unique: "global_cancel", Text: "❌ Нет, отмена"}
				markup := &tele.ReplyMarkup{InlineKeyboard: [][]tele.InlineButton{{btnYes, btnNo}}}

				return ctx.Send("Сообщение получено. Готов ли я его отправить во все чаты?", markup)
			}

			if state == "confirming_global_msg" && ctx.Text() != "/globalmsg" {
				return ctx.Send("Пожалуйста, нажмите 'Да' или 'Нет' на предыдущем сообщении для подтверждения или отмены рассылки.")
			}
		}
		return next(ctx)
	}
}

func (b *Bot) HandleGlobalCancel(ctx tele.Context) error {
	if ctx.Sender().ID != b.adminID {
		return nil
	}

	b.rdb.Del(context.Background(), fmt.Sprintf("admin_state:%d", b.adminID))
	b.rdb.Del(context.Background(), fmt.Sprintf("global_msg_id:%d", b.adminID))

	ctx.Edit("❌ Создание глобального сообщения отменено.")
	return ctx.Respond()
}

func (b *Bot) HandleGlobalSend(ctx tele.Context) error {
	if ctx.Sender().ID != b.adminID {
		return nil
	}

	state, _ := b.rdb.Get(context.Background(), fmt.Sprintf("admin_state:%d", b.adminID)).Result()
	if state != "confirming_global_msg" {
		return ctx.Respond(&tele.CallbackResponse{Text: "Состояние устарело или рассылка уже отменена.", ShowAlert: true})
	}

	msgIDStr, err := b.rdb.Get(context.Background(), fmt.Sprintf("global_msg_id:%d", b.adminID)).Result()
	if err != nil {
		return ctx.Edit("❌ Ошибка: сообщение не найдено в памяти.")
	}
	msgID, _ := strconv.Atoi(msgIDStr)

	chatIDs, err := b.repo.GetAllActiveChatIDs()
	if err != nil {
		return ctx.Edit("❌ Ошибка получения списка чатов из БД.")
	}

	ctx.Edit("⏳ Начинаю рассылку... Это может занять время.")

	successCount := 0
	for _, chatID := range chatIDs {
		recipient := &tele.Chat{ID: chatID}
		_, err := b.bot.Copy(recipient, &tele.Message{ID: msgID, Chat: &tele.Chat{ID: ctx.Sender().ID}})
		if err == nil {
			successCount++
		}

		time.Sleep(50 * time.Millisecond)
	}

	b.rdb.Del(context.Background(), fmt.Sprintf("admin_state:%d", b.adminID))
	b.rdb.Del(context.Background(), fmt.Sprintf("global_msg_id:%d", b.adminID))

	ctx.Send(fmt.Sprintf("✅ Рассылка успешно завершена!\nДоставлено: %d из %d", successCount, len(chatIDs)))
	return ctx.Respond()
}
