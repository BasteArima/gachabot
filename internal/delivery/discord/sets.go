package discord

import (
	"fmt"
	"gachabot/internal/models"
	"strconv"
	"strings"

	"github.com/bwmarrin/discordgo"
)

// generateProgressBar создает текстовый прогресс-бар из 5 блоков (как в TG)
func generateProgressBar(collected, total int) string {
	if total == 0 {
		return "[░░░░░]"
	}
	filledBlocks := (collected * 5) / total
	if filledBlocks > 5 {
		filledBlocks = 5
	}

	bar := "["
	for j := 0; j < 5; j++ {
		if j < filledBlocks {
			bar += "█"
		} else {
			bar += "░"
		}
	}
	bar += "]"
	return bar
}

// handleSetsList выводит список сетов через Embed + Select Menu
func (b *Bot) handleSetsList(s *discordgo.Session, i *discordgo.InteractionCreate, user *models.User, lang string, page int) {
	setsProgress, err := b.service.GetUserSetsProgress(user.ID)
	if err != nil || len(setsProgress) == 0 {
		b.respondEphemeral(s, i, b.loc.T(lang, "sets_empty"))
		return
	}

	const pageSize = 5
	totalSets := len(setsProgress)
	totalPages := (totalSets + pageSize - 1) / pageSize

	// Цикличная пагинация
	if totalPages > 0 {
		if page >= totalPages {
			page = 0
		} else if page < 0 {
			page = totalPages - 1
		}
	}

	start := page * pageSize
	end := start + pageSize
	if end > totalSets {
		end = totalSets
	}

	pageSets := setsProgress[start:end]

	// --- Создаем выпадающий список (Select Menu) ---
	var options []discordgo.SelectMenuOption
	for _, sp := range pageSets {
		status := fmt.Sprintf("%d/%d", sp.CollectedCards, sp.TotalCards)
		if sp.IsCompleted {
			status = b.loc.T(lang, "set_status_completed") // ✅
		}
		activeMark := ""
		if sp.IsActive {
			activeMark = " ✨"
		}
		pBar := generateProgressBar(sp.CollectedCards, sp.TotalCards)

		options = append(options, discordgo.SelectMenuOption{
			// Label: [███░░] Название [3/5] ✨
			Label:       fmt.Sprintf("%s %s [%s]%s", pBar, sp.SetName, status, activeMark),
			Description: b.loc.T(lang, "btn_set_view"),
			Value:       strconv.Itoa(sp.SetID),
		})
	}

	selectMenu := discordgo.SelectMenu{
		CustomID:    "set_view_select",
		Placeholder: "Выбери коллекцию для просмотра...",
		Options:     options,
	}

	// --- Создаем кнопки навигации ---
	var navButtons []discordgo.MessageComponent
	if totalPages > 1 {
		navButtons = append(navButtons, discordgo.Button{Label: "⬅️", Style: discordgo.SecondaryButton, CustomID: fmt.Sprintf("sets_nav:%d", page-1)})
		navButtons = append(navButtons, discordgo.Button{Label: "➡️", Style: discordgo.SecondaryButton, CustomID: fmt.Sprintf("sets_nav:%d", page+1)})
	}
	navButtons = append(navButtons, discordgo.Button{Label: "🔙", Style: discordgo.DangerButton, CustomID: "back_to_profile"})

	components := []discordgo.MessageComponent{
		discordgo.ActionsRow{Components: []discordgo.MessageComponent{selectMenu}},
		discordgo.ActionsRow{Components: navButtons},
	}

	embed := &discordgo.MessageEmbed{
		Title:       b.loc.T(lang, "sets_list_title"),
		Description: fmt.Sprintf("`─── Стр. %d / %d ───`", page+1, totalPages),
		Color:       0x5865F2, // Discord Blurple
	}

	b.updateWithEmbedAndComponents(s, i, "", embed, components)
}

// handleSetView обрабатывает выбор коллекции из Select Menu
func (b *Bot) handleSetView(s *discordgo.Session, i *discordgo.InteractionCreate, user *models.User, lang string) {
	setIDStr := i.MessageComponentData().Values[0]
	setID, _ := strconv.Atoi(setIDStr)

	b.renderSetView(s, i, user, lang, setID, false)
}

// handleEquipAura обрабатывает нажатие кнопки экипировки
func (b *Bot) handleEquipAura(s *discordgo.Session, i *discordgo.InteractionCreate, user *models.User, lang string) {
	customID := i.MessageComponentData().CustomID
	data := strings.Split(customID, ":") // set_equip:ID:on/off
	setID, _ := strconv.Atoi(data[1])
	action := data[2]

	var newActiveSetID *int
	var toastMsg string

	if action == "on" {
		newActiveSetID = &setID
		toastMsg = b.loc.T(lang, "aura_equipped_toast")
	} else {
		newActiveSetID = nil
		toastMsg = b.loc.T(lang, "aura_unequipped_toast")
	}

	_ = b.service.EquipSetAura(user.ID, newActiveSetID)

	// Отправляем всплывающее уведомление (Toast), которое видит только юзер
	b.respondEphemeral(s, i, toastMsg)

	// Обновляем текущее сообщение (перерисовываем кнопки), чтобы изменить статус экипировки
	b.renderSetView(s, i, user, lang, setID, true)
}

// renderSetView собирает Embed для конкретного сета и обновляет сообщение
func (b *Bot) renderSetView(s *discordgo.Session, i *discordgo.InteractionCreate, user *models.User, lang string, setID int, isFollowUp bool) {
	setsProgress, _ := b.service.GetUserSetsProgress(user.ID)
	var currentSet *models.UserSetProgress
	for _, sp := range setsProgress {
		if sp.SetID == setID {
			currentSet = &sp
			break
		}
	}

	if currentSet == nil {
		return
	}

	cards, _ := b.service.GetSetCards(user.ID, setID)

	buffName := b.loc.T(lang, "buff_type_"+currentSet.BuffType)
	buffDesc := fmt.Sprintf("+%d%% (%s)", currentSet.BuffValue, buffName)

	desc := b.loc.T(lang, "set_view_title", currentSet.SetName, currentSet.RewardPoints, buffDesc)

	for j, c := range cards {
		if c.Name != "" {
			desc += b.loc.T(lang, "set_card_owned", j+1, c.Name) + "\n"
		} else {
			desc += b.loc.T(lang, "set_card_unknown", j+1) + "\n"
		}
	}

	embed := &discordgo.MessageEmbed{
		Description: desc,
		Color:       0x2ecc71, // Красивый зеленый цвет для сета
	}

	var buttons []discordgo.MessageComponent
	btnBack := discordgo.Button{
		Label:    b.loc.T(lang, "btn_back_to_sets"),
		Style:    discordgo.SecondaryButton,
		CustomID: "sets_nav:0",
	}

	if currentSet.IsCompleted {
		if currentSet.IsActive {
			btnUnequip := discordgo.Button{
				Label:    b.loc.T(lang, "btn_unequip_aura"),
				Style:    discordgo.DangerButton,
				CustomID: fmt.Sprintf("set_equip:%d:off", setID),
			}
			buttons = append(buttons, btnUnequip)
		} else {
			btnEquip := discordgo.Button{
				Label:    b.loc.T(lang, "btn_equip_aura"),
				Style:    discordgo.PrimaryButton,
				CustomID: fmt.Sprintf("set_equip:%d:on", setID),
			}
			buttons = append(buttons, btnEquip)
		}
	}

	buttons = append(buttons, btnBack)
	row := discordgo.ActionsRow{Components: buttons}

	if isFollowUp {
		// Если мы уже ответили (например, Toast уведомлением при экипировке), мы должны обновить оригинал
		_, _ = s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Embeds:     &[]*discordgo.MessageEmbed{embed},
			Components: &[]discordgo.MessageComponent{row},
		})
	} else {
		// Стандартное обновление сообщения (вызывается при выборе из меню)
		b.updateWithEmbedAndComponents(s, i, "", embed, []discordgo.MessageComponent{row})
	}
}
