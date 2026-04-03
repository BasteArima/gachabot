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

	setsProgress, err := b.repo.GetUserSetsProgress(dbUser.ID)
	if err != nil || len(setsProgress) == 0 {
		if ctx.Callback() != nil {
			return ctx.Respond(&tele.CallbackResponse{Text: b.loc.T(lang, "sets_empty"), ShowAlert: true})
		}
		return ctx.Send(b.loc.T(lang, "sets_empty"))
	}

	if ctx.Callback() != nil {
		_ = ctx.Respond()
	}

	// Пагинация по 10 сетов на страницу
	const pageSize = 10
	totalSets := len(setsProgress)
	totalPages := (totalSets + pageSize - 1) / pageSize

	if page >= totalPages {
		page = totalPages - 1
	}
	if page < 0 {
		page = 0
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

	// Выводим сеты текущей страницы с прогресс-барами
	for i, sp := range pageSets {
		status := b.loc.T(lang, "set_status_progress", sp.CollectedCards, sp.TotalCards)
		if sp.IsCompleted {
			status = b.loc.T(lang, "set_status_completed")
		}

		activeMark := ""
		if sp.IsActive {
			activeMark = " ✨ (Экипировано)"
		}

		pBar := generateProgressBar(sp.CollectedCards, sp.TotalCards)

		// Пример строки: "1. Герои меча [███░░] 3/5 ✨ (Экипировано)"
		sb.WriteString(fmt.Sprintf("%d. <b>%s</b> %s [%s]%s\n", start+i+1, sp.SetName, pBar, status, activeMark))

		btnView := menu.Data(b.loc.T(lang, "btn_set_view", sp.SetName), "set_view", strconv.Itoa(sp.SetID))
		rows = append(rows, menu.Row(btnView))
	}

	// Кнопки навигации (автоматически скрываются на краях)
	var navRow []tele.Btn
	if page > 0 {
		navRow = append(navRow, menu.Data(b.loc.T(lang, "btn_back"), "sets_nav", strconv.Itoa(page-1)))
	}
	if page < totalPages-1 {
		navRow = append(navRow, menu.Data(b.loc.T(lang, "btn_forward"), "sets_nav", strconv.Itoa(page+1)))
	}
	if len(navRow) > 0 {
		rows = append(rows, navRow)
	}

	rows = append(rows, menu.Row(menu.Data(b.loc.T(lang, "btn_to_profile"), "back_profile")))
	menu.Inline(rows...)

	// Умная обработка редактирования (чтобы не падать из-за картинки профиля)
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

	setsProgress, _ := b.repo.GetUserSetsProgress(dbUser.ID)
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

	cards, _ := b.repo.GetSetCards(dbUser.ID, setID)

	var sb strings.Builder
	buffDesc := fmt.Sprintf("+%d (%s)", currentSet.BuffValue, currentSet.BuffType)

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

	_ = b.repo.EquipSetAura(dbUser.ID, newActiveSetID)
	_ = ctx.Respond(&tele.CallbackResponse{Text: toastMsg})

	ctx.Callback().Data = strconv.Itoa(setID)
	return b.HandleSetView(ctx)
}
