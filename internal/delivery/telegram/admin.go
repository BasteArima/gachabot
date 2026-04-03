package telegram

import (
	"encoding/json"
	"fmt"
	"gachabot/internal/models"
	"strconv"
	"strings"
	"time"

	tele "gopkg.in/telebot.v3"
)

// Генератор УМНЫХ промокодов (принимает JSON из веб-конструктора)
func (b *Bot) HandleCreatePromo(ctx tele.Context) error {
	tgUser := ctx.Sender()
	if tgUser.ID != 348389728 { // Твой ID
		return nil
	}

	args := ctx.Args()
	// Формат: /createpromo КОД ЛИМИТ ЧАСЫ JSON_СТРОКА
	if len(args) < 4 {
		return ctx.Send("Формат: `/createpromo КОД ЛИМИТ ЧАСЫ {\"points\": 100}`\n(0 = без ограничений)", tele.ModeMarkdown)
	}

	code := strings.ToUpper(args[0])
	maxUses, _ := strconv.Atoi(args[1])
	hoursValid, _ := strconv.Atoi(args[2]) // <-- Читаем время (в часах)

	jsonPayload := strings.Join(args[3:], " ")

	var reward models.PromoReward
	if err := json.Unmarshal([]byte(jsonPayload), &reward); err != nil {
		return ctx.Send("❌ Ошибка парсинга JSON: " + err.Error())
	}

	var usesPtr *int
	if maxUses > 0 {
		usesPtr = &maxUses
	}

	// Считаем точное время истечения
	var expiresAt *time.Time
	if hoursValid > 0 {
		t := time.Now().Add(time.Duration(hoursValid) * time.Hour)
		expiresAt = &t
	}

	err := b.repo.CreatePromoCode(code, reward, usesPtr, expiresAt)
	if err != nil {
		return ctx.Send("❌ Ошибка БД: " + err.Error())
	}

	msg := fmt.Sprintf("✅ Промокод **%s** успешно загружен!\nЛимит: %d юзов\nВремя жизни: %d часов", code, maxUses, hoursValid)
	return ctx.Send(msg, tele.ModeMarkdown)
}

// Генератор для админа
func (b *Bot) HandleAddPromo(ctx tele.Context) error {
	tgUser := ctx.Sender()
	// Проверка на админа (Подставь свой Telegram ID)
	if tgUser.ID != 348389728 {
		return nil // Игнорим
	}

	args := ctx.Args()
	// Формат: /addpromo КОД ОЧКИ КРУТКИ МАКС_ИСПОЛЬЗОВАНИЙ
	if len(args) < 4 {
		return ctx.Send("Формат: `/addpromo КОД ОЧКИ КРУТКИ МАКС_ЮЗОВ`\nПример: `/addpromo START 5000 5 100`", tele.ModeMarkdown)
	}

	code := args[0]
	points, _ := strconv.Atoi(args[1])
	rolls, _ := strconv.Atoi(args[2])
	maxUses, _ := strconv.Atoi(args[3])

	reward := models.PromoReward{
		Points:       points,
		PremiumRolls: rolls,
	}

	var usesPtr *int
	if maxUses > 0 {
		usesPtr = &maxUses
	}

	// ИСПРАВЛЕНИЕ ЗДЕСЬ: добавляем nil в конце (бессрочный промокод)
	err := b.repo.CreatePromoCode(code, reward, usesPtr, nil)
	if err != nil {
		return ctx.Send("❌ Ошибка БД: " + err.Error())
	}

	return ctx.Send(fmt.Sprintf("✅ Промокод **%s** успешно создан!\nОчки: %d, Крутки: %d, Лимит: %d", code, points, rolls, maxUses), tele.ModeMarkdown)
}
