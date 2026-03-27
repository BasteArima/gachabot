package telegram

import (
	"fmt"
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
}

func NewHandler(repo *repository.PostgresRepo, service *service.GachaService, duelService *service.DuelService) *Handler {
	return &Handler{
		repo:        repo,
		service:     service,
		duelService: duelService,
	}
}

func (h *Handler) HandleStart(ctx tele.Context) error {
	sticker := &tele.Sticker{File: tele.File{FileID: "CAACAgIAAxkBAAMGacUaTK2nsNg77On1KstHV1B6SbMAAj-HAAJOpnFK7SHSkw_YzeE6BA"}}

	menu := &tele.ReplyMarkup{}
	btnAddGroup := menu.URL("👥 Добавить в группу", "https://t.me/HentaiCard_bot?startgroup=true")
	menu.Inline(menu.Row(btnAddGroup))

	if err := ctx.Send(sticker); err != nil {
		log.Println("Cant send sticker:", err)
	}

	msgText := "👋 Привет! Тут ты можешь собирать уникальные карточки и соревноваться с другими игроками\n\n" +
		"Как получить карточки?\n" +
		"<blockquote>отправь команду \"/roll\"</blockquote>\n\n" +
		"Узнать все функции можно по команде /help"

	return ctx.Send(msgText, tele.ModeHTML, menu)
}

func (h *Handler) HandleRoll(ctx tele.Context) error {
	user := ctx.Sender()
	h.service.TrackChat(user.ID, ctx.Chat().ID)

	result, err := h.service.RollCard(user.ID, user.Username, user.FirstName, user.LastName)
	if err != nil {
		log.Printf("[ROLL ERROR] Ошибка сервиса для %d (%s): %v", user.ID, user.Username, err)
		return ctx.Send("🔧 Произошла техническая ошибка. База данных отдыхает, попробуйте позже.")
	}

	if result.OnCooldown {
		msg := fmt.Sprintf("<blockquote>⏳ Следующую карточку можно будет получить через: <b>%s</b></blockquote>", result.CooldownTimeLeft)
		return ctx.Send(msg, &tele.SendOptions{ReplyTo: ctx.Message(), ParseMode: tele.ModeHTML})
	}

	var caption string
	if result.IsFragment {
		if result.CardAssembled {
			caption = fmt.Sprintf("<blockquote>🔥 <b>ЭПИЧЕСКАЯ УДАЧА!</b> Вы собрали 10 осколков воедино!\n\nПолучена Мифическая карта: <b>%s</b>\n<tg-emoji emoji-id=\"4918300654197277832\">🪙</tg-emoji> Очки: <b>+%d</b></blockquote>", result.Card.Name, result.Reward)
		} else {
			caption = fmt.Sprintf("<blockquote>🔮 Вы нашли осколок Мифической карты: <b>%s</b>!\n\nСобрано осколков: <b>%d / 10</b>\n<tg-emoji emoji-id=\"4918300654197277832\">🪙</tg-emoji> Очки: <b>+%d</b></blockquote>", result.Card.Name, result.FragmentsCount, result.Reward)
		}
	} else {
		caption = fmt.Sprintf("<blockquote>"+
			"<tg-emoji emoji-id=\"4996755833950831347\">🎉</tg-emoji>Поздравляем! Вы получили карточку: <b>%s</b>\n\n"+
			"<tg-emoji emoji-id=\"4956525562483967357\">🃏</tg-emoji>Редкость: <b>%s</b>\n"+
			"<tg-emoji emoji-id=\"4918300654197277832\">🪙</tg-emoji>Очки: <b>+%d</b>"+
			"</blockquote>",
			result.Card.Name, result.RarityName, result.Reward)
	}

	// --- ЛОГИРОВАНИЕ ПЕРЕД ОТПРАВКОЙ ---
	//log.Printf("[ROLL ATTEMPT] User: %s (%d) | Card: %s | Rarity: %s | URL: %s", user.Username, user.ID, result.Card.Name, result.RarityName, result.Card.ImageURL)

	photo := &tele.Photo{
		File:    tele.FromURL(result.Card.ImageURL),
		Caption: caption,
	}

	err = ctx.Send(photo, &tele.SendOptions{
		ParseMode: tele.ModeHTML,
		ReplyTo:   ctx.Message(),
	})

	if err != nil {
		log.Printf("[TELEGRAM ERROR] Не удалось отправить фото! URL: %s | Ошибка: %v", result.Card.ImageURL, err)
		// Если фото не ушло, попробуем отправить хотя бы текст, чтобы юзер не ждал зря
		return ctx.Send(caption+"\n\n⚠️ (Ошибка загрузки изображения)", tele.ModeHTML)
	}

	return nil
}

func (h *Handler) HandleProfile(ctx tele.Context) error {
	user := ctx.Sender()

	// 1. Получаем данные из сервиса
	profile, err := h.service.GetUserProfile(user.ID)
	if err != nil {
		log.Printf("Ошибка получения профиля юзера %d: %v", user.ID, err)
		return ctx.Send("❌ Не удалось загрузить профиль.")
	}

	// 2. Формируем красивый текст
	caption := fmt.Sprintf(`<blockquote>👤 Ты - %s %s

<tg-emoji emoji-id="5368324170671202286">🃏</tg-emoji> У тебя %d из %d карточек
♻️ Дубликаты для крафта: <b>%d</b>
<tg-emoji emoji-id="4918300654197277832">🪙</tg-emoji> У тебя %d очков

<tg-emoji emoji-id="5368324170671202286">🔥</tg-emoji> Твой стрик составляет %d дней</blockquote>`,
		user.FirstName,
		user.LastName,
		profile.UniqueCardsCount,
		profile.TotalCardsCount,
		profile.DuplicatesCount,
		profile.Balance,
		profile.StreakDays,
	)

	// 3. Пытаемся получить аватарку
	// Эта функция возвращает массив фотографий (от новых к старым)
	photos, err := ctx.Bot().ProfilePhotosOf(user)

	// Добавляем клавиатуру, ЕСЛИ у юзера есть хотя бы 1 карта
	menu := &tele.ReplyMarkup{}
	if profile.UniqueCardsCount > 0 {
		// Создаем кнопку. "cards_nav" — это внутренний ID кнопки, "0" — передаваемые данные (отступ)
		btnMyCards := menu.Data(fmt.Sprintf("🎴 Мои карточки [%d]", profile.UniqueCardsCount), "cards_nav", "0")
		menu.Inline(menu.Row(btnMyCards))
	}

	// Если нет ошибки и у юзера есть хотя бы одно фото профиля
	if err == nil && len(photos) > 0 {
		// Берем самую первую (текущую) аватарку
		// photos[0] - это массив разных размеров одного фото. Берем самый большой.
		photo := &tele.Photo{
			File:    photos[0].File,
			Caption: caption,
		}
		return ctx.Send(photo, tele.ModeHTML, menu)
	}

	// 4. Если фото нет (или скрыто приватностью) — отправляем просто текст
	return ctx.Send(caption, tele.ModeHTML, menu)
}

// Обработчик перелистывания карт
func (h *Handler) HandleCardsNav(ctx tele.Context) error {
	// Обязательно отвечаем на колбек, иначе кнопка зависнет с часиками "загрузка..."
	_ = ctx.Respond()

	user := ctx.Sender()
	// Получаем переданный отступ из кнопки (строку "0", "1" и т.д.)
	offsetStr := ctx.Callback().Data
	offset, _ := strconv.Atoi(offsetStr) // Превращаем в число

	card, total, err := h.service.GetUserCardPagination(user.ID, offset)
	if err != nil {
		return ctx.Send("Не удалось загрузить карточки.")
	}

	// Красивый текст карточки
	caption := fmt.Sprintf("<blockquote><tg-emoji emoji-id=\"4956525562483967357\">🃏</tg-emoji> <b>%s</b>\n\n✨ Редкость: <b>%s</b>\n📦 У вас: <b>%d шт.</b>\n\n<i>Карточка %d из %d</i></blockquote>",
		card.CardName, card.RarityName, card.Quantity, offset+1, total)

	// Строим кнопки навигации
	menu := &tele.ReplyMarkup{}
	var row []tele.Btn

	// Если это не первая карта — показываем кнопку "Назад"
	if offset > 0 {
		row = append(row, menu.Data("⬅️ Назад", "cards_nav", strconv.Itoa(offset-1)))
	}
	// Если это не последняя карта — показываем кнопку "Вперед"
	if offset < total-1 {
		row = append(row, menu.Data("Вперед ➡️", "cards_nav", strconv.Itoa(offset+1)))
	}

	// Кнопка возврата в профиль
	btnProfile := menu.Data("🔙 В профиль", "back_profile")

	if len(row) > 0 {
		menu.Inline(menu.Row(row...), menu.Row(btnProfile))
	} else {
		menu.Inline(menu.Row(btnProfile)) // Если карта всего одна
	}

	photo := &tele.Photo{
		File:    tele.FromURL(card.ImageURL),
		Caption: caption,
	}

	// МАГИЯ: ctx.Edit меняет текущее сообщение (картинку, текст и кнопки), а не шлет новое!
	err = ctx.Edit(photo, tele.ModeHTML, menu)
	if err != nil {
		// Если ошибка связана с тем, что текст не изменился — просто игнорируем её (возвращаем nil)
		if strings.Contains(err.Error(), "message is not modified") {
			return nil
		}
		// Если это реальная ошибка сети или БД — возвращаем её, чтобы она попала в логи
		return err
	}
	return nil
}

// Обработчик возврата в профиль
func (h *Handler) HandleBackToProfile(ctx tele.Context) error {
	_ = ctx.Respond()
	// Чтобы вернуть профиль с аватаркой, проще удалить сообщение с картой
	// и вызвать твою же функцию HandleProfile заново
	_ = ctx.Delete()
	return h.HandleProfile(ctx)
}

// Вспомогательная функция для генерации текста и кнопок топа
func (h *Handler) buildTopMessage(criteria string, scope string, chatID int64) (string, *tele.ReplyMarkup, error) {
	// Определяем ID чата (0 для глобального)
	targetChatID := chatID
	if scope == "global" {
		targetChatID = 0
	}

	board, err := h.service.GetLeaderboard(criteria, targetChatID)
	if err != nil {
		return "", nil, err
	}

	// Заголовки
	scopeName := "🌍 Глобальный Топ"
	if scope == "local" {
		scopeName = "👥 Топ Чата"
	}

	critName, emoji := "", ""
	switch criteria {
	case "balance":
		critName, emoji = "По очкам", "<tg-emoji emoji-id=\"4918300654197277832\">🪙</tg-emoji>"
	case "cards":
		critName, emoji = "По Уникальным Картам", "🃏"
	case "streak":
		critName, emoji = "По Стрику", "🔥"
	}

	// Собираем текст
	text := fmt.Sprintf("<blockquote><b>%s: %s</b>\n\n", scopeName, critName)
	if len(board) == 0 {
		text += "Тут пока пусто 😔\n"
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
			text += fmt.Sprintf("%s <b>%s</b> — %d %s\n", medal, entry.DisplayName, entry.Value, emoji)
		}
	}
	text += "</blockquote>"

	// Собираем кнопки
	menu := &tele.ReplyMarkup{}

	// Данные кнопки: "top|критерий|область" (например "top|balance|local")
	btnBal := menu.Data("🪙 Баланс", "top_btn", "balance|"+scope)
	btnCards := menu.Data("🃏 Карты", "top_btn", "cards|"+scope)
	btnStreak := menu.Data("🔥 Стрик", "top_btn", "streak|"+scope)

	menu.Inline(menu.Row(btnBal, btnCards, btnStreak))

	return text, menu, nil
}

// Обработчик команды /top (Локальный)
func (h *Handler) HandleLocalTop(ctx tele.Context) error {
	h.service.TrackChat(ctx.Sender().ID, ctx.Chat().ID)

	if ctx.Chat().Type == tele.ChatPrivate {
		return ctx.Send("❌ Локальный топ работает только в группах. Используйте /globaltop")
	}

	text, menu, err := h.buildTopMessage("balance", "local", ctx.Chat().ID)
	if err != nil {
		return ctx.Send("Ошибка загрузки топа.")
	}
	return ctx.Send(text, tele.ModeHTML, menu)
}

// Обработчик команды /globaltop (Глобальный)
func (h *Handler) HandleGlobalTop(ctx tele.Context) error {
	h.service.TrackChat(ctx.Sender().ID, ctx.Chat().ID)

	text, menu, err := h.buildTopMessage("balance", "global", ctx.Chat().ID)
	if err != nil {
		return ctx.Send("Ошибка загрузки топа.")
	}
	return ctx.Send(text, tele.ModeHTML, menu)
}

func (h *Handler) HandleTopCallback(ctx tele.Context) error {
	_ = ctx.Respond()

	// Парсим данные из кнопки (например "balance|local")
	data := strings.Split(ctx.Callback().Data, "|")
	if len(data) != 2 {
		return nil
	}
	criteria, scope := data[0], data[1]

	text, menu, err := h.buildTopMessage(criteria, scope, ctx.Chat().ID)
	if err != nil {
		return ctx.Send("Ошибка обновления топа.")
	}

	// Обновляем текущее сообщение
	err = ctx.Edit(text, tele.ModeHTML, menu)
	if err != nil {
		// Если ошибка связана с тем, что текст не изменился — просто игнорируем её (возвращаем nil)
		if strings.Contains(err.Error(), "message is not modified") {
			return nil
		}
		// Если это реальная ошибка сети или БД — возвращаем её, чтобы она попала в логи
		return err
	}
	return nil
}

// Вспомогательная функция для сборки текста и кнопок справки
func (h *Handler) buildHelpMessage(section string) (string, *tele.ReplyMarkup) {
	menu := &tele.ReplyMarkup{}

	btnMain := menu.Data("🏠 Главная", "help_nav", "main")
	btnCards := menu.Data("🎴 Карточки", "help_nav", "cards")
	btnRarities := menu.Data("✨ Редкости", "help_nav", "rarities")
	btnStreaks := menu.Data("🔥 Стрики", "help_nav", "streaks")
	btnPity := menu.Data("🛡 Гарант", "help_nav", "pity")
	btnDuel := menu.Data("⚔️ Дуэли", "help_nav", "duel")
	btnCraft := menu.Data("⚒ Крафт", "help_nav", "craft")

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

	text, exists := HelpMessages[section]
	if !exists {
		text = "Раздел не найден."
	}

	return text, menu
}

// Обработчик самой команды /help
func (h *Handler) HandleHelp(ctx tele.Context) error {
	// При вызове команды всегда показываем главную страницу ("main")
	text, menu := h.buildHelpMessage("main")
	return ctx.Send(text, tele.ModeHTML, menu)
}

// Обработчик нажатий на кнопки внутри меню /help
func (h *Handler) HandleHelpCallback(ctx tele.Context) error {
	_ = ctx.Respond() // Убираем часики на кнопке

	// Получаем раздел, на который кликнул юзер (например, "cards" или "pity")
	section := ctx.Callback().Data

	text, menu := h.buildHelpMessage(section)

	// Используем Edit, чтобы сообщение плавно менялось, а не отправлялось заново
	err := ctx.Edit(text, tele.ModeHTML, menu)
	if err != nil {
		// Если ошибка связана с тем, что текст не изменился — просто игнорируем её (возвращаем nil)
		if strings.Contains(err.Error(), "message is not modified") {
			return nil
		}
		// Если это реальная ошибка сети или БД — возвращаем её, чтобы она попала в логи
		return err
	}
	return nil
}

func (h *Handler) HandleDuel(ctx tele.Context) error {
	if ctx.Chat().Type == tele.ChatPrivate {
		return ctx.Send("❌ Дуэли возможны только в группах!")
	}

	args := ctx.Args()
	if len(args) < 2 {
		return ctx.Send("📝 Используй: /duel @юзернейм ставка")
	}

	targetUsername := strings.TrimPrefix(args[0], "@")
	amount, err := strconv.Atoi(args[1])
	if err != nil || amount <= 0 {
		return ctx.Send("💰 Укажите корректную сумму очков.")
	}

	challenger := ctx.Sender()
	// Проверяем баланс инициатора перед созданием
	challengerDB, err := h.repo.GetUser(challenger.ID)
	if err != nil || challengerDB.Balance < amount {
		return ctx.Send("📉 У тебя недостаточно очков!")
	}

	// Ищем цель
	targetUser, err := h.repo.GetUserByUsername(targetUsername)
	if err != nil {
		return ctx.Send("🔍 Игрок @" + targetUsername + " не найден в базе. Ему нужно хотя бы раз нажать /roll.")
	}

	if targetUser.TgID == challenger.ID {
		return ctx.Send("🤡 Вызывать самого себя — сомнительная затея.")
	}

	duelID := fmt.Sprintf("%d_%d_%d", challenger.ID, targetUser.TgID, time.Now().Unix())

	// Сохраняем с именами
	h.duelService.CreateDuel(duelID, challenger.ID, challenger.FirstName, targetUser.TgID, targetUsername, amount)

	menu := &tele.ReplyMarkup{}
	btnAccept := menu.Data("⚔️ Принять", "duel_accept", duelID)
	btnCancel := menu.Data("🚫 Отмена", "duel_cancel", duelID)
	menu.Inline(menu.Row(btnAccept, btnCancel))

	caption := fmt.Sprintf("<blockquote><b>⚔️ ВЫЗОВ НА ДУЭЛЬ</b>\n\n👤 <b>%s</b> вызывает на бой <b>@%s</b>!\n💰 Ставка: <b>%d очков</b></blockquote>",
		challenger.FirstName, targetUsername, amount)

	return ctx.Send(caption, tele.ModeHTML, menu)
}

func (h *Handler) HandleDuelCallback(ctx tele.Context) error {
	duelID := ctx.Callback().Data
	callbackUnique := strings.TrimPrefix(ctx.Callback().Unique, "\f")

	duel, exists := h.duelService.GetDuel(duelID)
	if !exists {
		_ = ctx.Respond(&tele.CallbackResponse{Text: "❌ Дуэль истекла."})
		return ctx.Delete()
	}

	userID := ctx.Sender().ID

	if callbackUnique == "duel_cancel" {
		if userID != duel.ChallengerID && userID != duel.TargetID {
			return ctx.Respond(&tele.CallbackResponse{Text: "✋ Это не твоя дуэль."})
		}
		h.duelService.PopDuel(duelID)
		_ = ctx.Respond(&tele.CallbackResponse{Text: "Отменено"})
		return ctx.Edit(fmt.Sprintf("🚫 Дуэль между <b>%s</b> и <b>%s</b> отменена.", duel.ChallengerName, duel.TargetName), tele.ModeHTML)
	}

	if callbackUnique == "duel_accept" {
		if userID != duel.TargetID {
			return ctx.Respond(&tele.CallbackResponse{Text: "💢 Тебя не вызывали!"})
		}

		h.duelService.PopDuel(duelID) // Удаляем, чтобы нельзя было принять дважды

		result, err := h.duelService.ExecuteDuel(duel)
		if err != nil {
			return ctx.Edit("❌ Ошибка боя: " + err.Error())
		}

		resText := fmt.Sprintf(`<blockquote><b>⚔️ РЕЗУЛЬТАТ ДУЭЛИ</b>

🅰️ <b>%s</b> выставил:
└ %s (Сила: %d) — %.1f%%

🅱️ <b>%s</b> выставил:
└ %s (Сила: %d) — %.1f%%

🎲 Кубик: <b>%.1f</b>
🏆 Победитель: <b>%s</b>
💰 Выигрыш: <b>%d очков</b></blockquote>`,
			duel.ChallengerName, result.CardChallenger.Name, result.CardChallenger.PowerLevel, result.ChanceChallenger,
			duel.TargetName, result.CardTarget.Name, result.CardTarget.PowerLevel, result.ChanceTarget,
			result.Roll, result.WinnerName, result.AmountWon*2)

		return ctx.Edit(resText, tele.ModeHTML)
	}

	return nil
}

func (h *Handler) HandleCraft(ctx tele.Context) error {
	user := ctx.Sender()

	result, err := h.service.CraftCard(user.ID)
	if err != nil {
		return ctx.Send("⚒ <b>Алхимия не удалась:</b> "+err.Error(), tele.ModeHTML)
	}

	// Подставляем result.CraftCost
	caption := fmt.Sprintf("<blockquote>⚒ <b>УДАЧНЫЙ КРАФТ!</b>\n\nТы переплавил %d дубликатов и получил:\n🃏 Карта: <b>%s</b>\n✨ Редкость: <b>%s</b></blockquote>",
		result.CraftCost, result.Card.Name, result.RarityName)

	if result.IsFragment {
		if !result.CardAssembled {
			caption = fmt.Sprintf("<blockquote>⚒ <b>УДАЧНЫЙ КРАФТ!</b>\n\nТы переплавил %d легендарных дубликатов и получил осколок Эпохальной карты:\n🔮 <b>%s</b>\n📦 Собрано: <b>%d / 10</b></blockquote>",
				result.CraftCost, result.Card.Name, result.FragmentsCount)
		} else {
			caption = fmt.Sprintf("<blockquote>⚒ <b>ЭПОХАЛЬНЫЙ КРАФТ!</b>\n\nТы переплавил %d дубликатов и собрал последний осколок!\n🔥 Получена Эпохальная карта: <b>%s</b></blockquote>",
				result.CraftCost, result.Card.Name)
		}
	}

	photo := &tele.Photo{
		File:    tele.FromURL(result.Card.ImageURL),
		Caption: caption,
	}

	return ctx.Send(photo, tele.ModeHTML)
}
