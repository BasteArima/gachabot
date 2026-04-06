package telegram

import (
	"fmt"
	"strconv"
	"strings"

	"gachabot/internal/models"

	tele "gopkg.in/telebot.v3"
)

// HandleSetsList выводит список открытых сетов юзера (постранично)
// generateProgressBar создает текстовый прогресс-бар из 5 блоков
func generateProgressBar(collected, total int) string {
	if total == 0 {
		return "[░░░░░]"
	}
	filledBlocks := (collected * 5) / total
	if filledBlocks > 5 {
		filledBlocks = 5
	}

	bar := "["
	for i := 0; i < 5; i++ {
		if i < filledBlocks {
			bar += "█"
		} else {
			bar += "░"
		}
	}
	bar += "]"
	return bar
}

// HandleSetsList выводит список открытых сетов юзера (постранично)
func (b *Bot) HandleSetsList(ctx tele.Context) error {
	tgUser := ctx.Sender()
	dbUser, _ := b.repo.GetOrCreateUserByTelegramID(tgUser.ID, tgUser.Username, tgUser.FirstName, tgUser.LastName)
	lang := getLang(dbUser, tgUser)

	page := 0
	if ctx.Callback() != nil && ctx.Callback().Data != "" {
		page, _ = strconv.Atoi(ctx.Callback().Data)
	}

	// Задел на будущее: тут лучше вызывать b.service.GetUserSetsProgress
	setsProgress, err := b.service.GetUserSetsProgress(dbUser.ID)
	if err != nil || len(setsProgress) == 0 {
		if ctx.Callback() != nil {
			return ctx.Respond(&tele.CallbackResponse{Text: b.loc.T(lang, "sets_empty"), ShowAlert: true})
		}
		return ctx.Send(b.loc.T(lang, "sets_empty"))
	}

	if ctx.Callback() != nil {
		_ = ctx.Respond()
	}

	const pageSize = 5
	totalSets := len(setsProgress)
	totalPages := (totalSets + pageSize - 1) / pageSize

	// --- ЦИКЛИЧНАЯ ПАГИНАЦИЯ ---
	if totalPages > 0 {
		if page >= totalPages {
			page = 0 // Если листаем вперед с последней -> на первую
		} else if page < 0 {
			page = totalPages - 1 // Если листаем назад с первой -> на последнюю
		}
	}

	start := page * pageSize
	end := start + pageSize
	if end > totalSets {
		end = totalSets
	}

	pageSets := setsProgress[start:end]

	var sb strings.Builder
	sb.WriteString(b.loc.T(lang, "sets_list_title"))

	menu := &tele.ReplyMarkup{}
	var rows []tele.Row

	// Собираем кнопки с сетами
	for _, sp := range pageSets {
		status := fmt.Sprintf("%d/%d", sp.CollectedCards, sp.TotalCards)
		if sp.IsCompleted {
			status = "✅"
		}
		activeMark := ""
		if sp.IsActive {
			activeMark = " ✨"
		}
		pBar := generateProgressBar(sp.CollectedCards, sp.TotalCards)

		btnText := fmt.Sprintf("%s %s [%s]%s", pBar, sp.SetName, status, activeMark)
		btnView := menu.Data(b.loc.T(lang, "btn_set_view", btnText), "set_view", strconv.Itoa(sp.SetID))
		rows = append(rows, menu.Row(btnView))
	}

	// --- НОВОЕ РАСПОЛОЖЕНИЕ КНОПОК НАВИГАЦИИ ---
	// Показываем стрелки, только если страниц больше одной
	if totalPages > 1 {
		btnBack := menu.Data(b.loc.T(lang, "btn_back"), "sets_nav", strconv.Itoa(page-1))
		btnForward := menu.Data(b.loc.T(lang, "btn_forward"), "sets_nav", strconv.Itoa(page+1))
		// Ряд 1: Назад | Вперед
		rows = append(rows, menu.Row(btnBack, btnForward))
	}

	// Ряд 2 (или 1, если страниц нет): В профиль (на всю ширину)
	btnProfile := menu.Data(b.loc.T(lang, "btn_to_profile"), "back_profile")
	rows = append(rows, menu.Row(btnProfile))

	menu.Inline(rows...)

	if ctx.Callback() != nil {
		if ctx.Message() != nil && ctx.Message().Photo != nil {
			_ = ctx.Delete()
			return ctx.Send(sb.String(), tele.ModeHTML, menu)
		}
		return ctx.Edit(sb.String(), tele.ModeHTML, menu)
	}
	return ctx.Send(sb.String(), tele.ModeHTML, menu)
}

// HandleSetView показывает список карт внутри сета
func (b *Bot) HandleSetView(ctx tele.Context) error {
	_ = ctx.Respond() // Чтобы кнопка не висла

	tgUser := ctx.Sender()
	dbUser, _ := b.repo.GetOrCreateUserByTelegramID(tgUser.ID, tgUser.Username, tgUser.FirstName, tgUser.LastName)
	lang := getLang(dbUser, tgUser)

	setID, _ := strconv.Atoi(ctx.Callback().Data)

	setsProgress, _ := b.service.GetUserSetsProgress(dbUser.ID)
	var currentSet *models.UserSetProgress
	for _, sp := range setsProgress {
		if sp.SetID == setID {
			currentSet = &sp
			break
		}
	}

	if currentSet == nil {
		return ctx.Send("Ошибка загрузки сета.")
	}

	cards, _ := b.service.GetSetCards(dbUser.ID, setID)

	var sb strings.Builder
	// В HandleSetView исправляем формирование buffDesc:

	// Локализуем тип баффа (например, power_percent -> "Буст силы")
	buffName := b.loc.T(lang, "buff_type_"+currentSet.BuffType)
	// Формируем красивую строку: "+5% (Буст силы)"
	buffDesc := fmt.Sprintf("+%d%% (%s)", currentSet.BuffValue, buffName)

	sb.WriteString(b.loc.T(lang, "set_view_title", currentSet.SetName, currentSet.RewardPoints, buffDesc))

	for i, c := range cards {
		if c.Name != "" {
			sb.WriteString(b.loc.T(lang, "set_card_owned", i+1, c.Name) + "\n")
		} else {
			sb.WriteString(b.loc.T(lang, "set_card_unknown", i+1) + "\n")
		}
	}

	menu := &tele.ReplyMarkup{}
	var rows []tele.Row

	if currentSet.IsCompleted {
		if currentSet.IsActive {
			btnUnequip := menu.Data(b.loc.T(lang, "btn_unequip_aura"), "set_equip", fmt.Sprintf("%d:off", setID))
			rows = append(rows, menu.Row(btnUnequip))
		} else {
			btnEquip := menu.Data(b.loc.T(lang, "btn_equip_aura"), "set_equip", fmt.Sprintf("%d:on", setID))
			rows = append(rows, menu.Row(btnEquip))
		}
	}

	btnBack := menu.Data(b.loc.T(lang, "btn_back"), "sets_nav", "0")
	rows = append(rows, menu.Row(btnBack))
	menu.Inline(rows...)

	return ctx.Edit(sb.String(), tele.ModeHTML, menu)
}

// HandleEquipAura управляет экипировкой
func (b *Bot) HandleEquipAura(ctx tele.Context) error {
	tgUser := ctx.Sender()
	dbUser, _ := b.repo.GetOrCreateUserByTelegramID(tgUser.ID, tgUser.Username, tgUser.FirstName, tgUser.LastName)
	lang := getLang(dbUser, tgUser)

	data := strings.Split(ctx.Callback().Data, ":")
	setID, _ := strconv.Atoi(data[0])
	action := data[1]

	var newActiveSetID *int
	var toastMsg string

	if action == "on" {
		newActiveSetID = &setID
		toastMsg = b.loc.T(lang, "aura_equipped_toast")
	} else {
		newActiveSetID = nil
		toastMsg = b.loc.T(lang, "aura_unequipped_toast")
	}

	_ = b.service.EquipSetAura(dbUser.ID, newActiveSetID)
	_ = ctx.Respond(&tele.CallbackResponse{Text: toastMsg})

	ctx.Callback().Data = strconv.Itoa(setID)
	return b.HandleSetView(ctx)
}
