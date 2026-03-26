package telegram

import (
	"fmt"
	"log"

	"gachabot/internal/repository"
	"gachabot/internal/service"

	tele "gopkg.in/telebot.v3"
)

type Handler struct {
	repo    *repository.PostgresRepo
	service *service.GachaService
}

func NewHandler(repo *repository.PostgresRepo, service *service.GachaService) *Handler {
	return &Handler{repo: repo, service: service}
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

	// 1. Вызываем наш умный сервис
	result, err := h.service.RollCard(user.ID, user.Username)
	if err != nil {
		log.Printf("Ошибка RollCard для юзера %d: %v", user.ID, err)
		return ctx.Send("🔧 Произошла техническая ошибка. База данных отдыхает, попробуйте позже.")
	}

	// 2. Обрабатываем кулдаун
	if result.OnCooldown {
		msg := fmt.Sprintf("<blockquote>⏳ Следующую карточку можно будет получить через: <b>%s</b></blockquote>", result.CooldownTimeLeft)
		return ctx.Send(msg, &tele.SendOptions{ReplyTo: ctx.Message(), ParseMode: tele.ModeHTML})
	}

	// 3. Формируем текст в зависимости от того, что выпало
	var caption string
	if result.IsFragment {
		if result.CardAssembled {
			caption = fmt.Sprintf("<blockquote>🔥 <b>ЭПИЧЕСКАЯ УДАЧА!</b> Вы собрали 10 осколков воедино!\n\nПолучена Мифическая карта: <b>%s</b>\n🪙 Токены: <b>+%d</b></blockquote>", result.Card.Name, result.Reward)
		} else {
			caption = fmt.Sprintf("<blockquote>🔮 Вы нашли осколок Мифической карты: <b>%s</b>!\n\nСобрано осколков: <b>%d / 10</b>\n🪙 Токены: <b>+%d</b></blockquote>", result.Card.Name, result.FragmentsCount, result.Reward)
		}
	} else {
		caption = fmt.Sprintf("<blockquote>"+
			"<tg-emoji emoji-id=\"4996755833950831347\">🎉</tg-emoji>Поздравляем! Вы получили карточку: <b>%s</b>\n\n"+
			"<tg-emoji emoji-id=\"4956525562483967357\">🃏</tg-emoji>Редкость: <b>%s</b>\n"+
			"<tg-emoji emoji-id=\"4918300654197277832\">🪙</tg-emoji>Токены: <b>+%d</b>"+
			"</blockquote>",
			result.Card.Name, result.RarityName, result.Reward)
	}

	// 4. Отправляем фото
	photo := &tele.Photo{
		File:    tele.FromURL(result.Card.ImageURL),
		Caption: caption,
	}

	return ctx.Send(photo, &tele.SendOptions{
		ParseMode: tele.ModeHTML,
		ReplyTo:   ctx.Message(),
	})
}

func (h *Handler) HandleProfile(ctx tele.Context) error {
	user := ctx.Sender()

	// 1. Получаем данные из сервиса
	profile, err := h.service.GetUserProfile(user.ID)
	if err != nil {
		log.Printf("Ошибка получения профиля юзера %d: %v", user.ID, err)
		return ctx.Send("❌ Не удалось загрузить профиль.")
	}

	// 2. Формируем красивый текст (как на твоем скрине)
	// Ты можешь поменять ID эмодзи на те, которые нравятся тебе
	caption := fmt.Sprintf(`<blockquote>👤 Ты - %s %s

<tg-emoji emoji-id="5368324170671202286">🃏</tg-emoji> У тебя %d из %d карточек
<tg-emoji emoji-id="4918300654197277832">🪙</tg-emoji> У тебя %d токенов

<tg-emoji emoji-id="5368324170671202286">🔥</tg-emoji> Твой стрик составляет %d дней</blockquote>`,
		user.FirstName,
		user.LastName,
		profile.UniqueCardsCount,
		profile.TotalCardsCount,
		profile.Balance,
		profile.StreakDays,
	)

	// 3. Пытаемся получить аватарку
	// Эта функция возвращает массив фотографий (от новых к старым)
	photos, err := ctx.Bot().ProfilePhotosOf(user)

	// Если нет ошибки и у юзера есть хотя бы одно фото профиля
	if err == nil && len(photos) > 0 {
		// Берем самую первую (текущую) аватарку
		// photos[0] - это массив разных размеров одного фото. Берем самый большой.
		photo := &tele.Photo{
			File:    photos[0].File,
			Caption: caption,
		}
		return ctx.Send(photo, tele.ModeHTML)
	}

	// 4. Если фото нет (или скрыто приватностью) — отправляем просто текст
	return ctx.Send(caption, tele.ModeHTML)
}
