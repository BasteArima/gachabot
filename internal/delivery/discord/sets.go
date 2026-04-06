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

// handleSetsList выводит список сетов через вертикальные кнопки
func (b *Bot) handleSetsList(s *discordgo.Session, i *discordgo.InteractionCreate, user *models.User, lang string, page int) {
	setsProgress, err := b.service.GetUserSetsProgress(user.ID)
	if err != nil || len(setsProgress) == 0 {
		b.respondEphemeral(s, i, b.loc.T(lang, "sets_empty"))
		return
	}

	const pageSize = 4
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
	var components []discordgo.MessageComponent

	// --- Создаем по одной кнопке в КАЖДОМ ряду (вертикальный список) ---
	for _, sp := range pageSets {
		status := fmt.Sprintf("%d/%d", sp.CollectedCards, sp.TotalCards)

		btnStyle := discordgo.SecondaryButton // Серый по умолчанию
		if sp.IsCompleted {
			status = b.loc.T(lang, "set_status_completed") // ✅
			btnStyle = discordgo.SuccessButton             // Зеленый, если собран
		}

		activeMark := ""
		if sp.IsActive {
			activeMark = " ✨"
		}

		pBar := generateProgressBar(sp.CollectedCards, sp.TotalCards)

		btnText := fmt.Sprintf("%s %s [%s]%s", pBar, sp.SetName, status, activeMark)

		btn := discordgo.Button{
			Label:    btnText,
			Style:    btnStyle,
			CustomID: fmt.Sprintf("set_view:%d", sp.SetID), // Обрати внимание на формат ID
		}

		// Каждую кнопку кладем в свой собственный ряд
		components = append(components, discordgo.ActionsRow{
			Components: []discordgo.MessageComponent{btn},
		})
	}

	// --- Создаем кнопки навигации (Последний ряд) ---
	var navButtons []discordgo.MessageComponent
	if totalPages > 1 {
		navButtons = append(navButtons, discordgo.Button{Label: "⬅️", Style: discordgo.PrimaryButton, CustomID: fmt.Sprintf("sets_nav:%d", page-1)})
		navButtons = append(navButtons, discordgo.Button{Label: "➡️", Style: discordgo.PrimaryButton, CustomID: fmt.Sprintf("sets_nav:%d", page+1)})
	}
	navButtons = append(navButtons, discordgo.Button{Label: "🔙", Style: discordgo.DangerButton, CustomID: "back_to_profile"})

	// Добавляем ряд с навигацией в самый низ
	components = append(components, discordgo.ActionsRow{Components: navButtons})

	embed := &discordgo.MessageEmbed{
		Title:       b.loc.T(lang, "sets_list_title"),
		Description: fmt.Sprintf("`─── Стр. %d / %d ───`", page+1, totalPages),
		Color:       0x5865F2,
	}

	b.updateWithEmbedAndComponents(s, i, "", embed, components)
}

// handleSetView обрабатывает выбор коллекции из кнопки
func (b *Bot) handleSetView(s *discordgo.Session, i *discordgo.InteractionCreate, user *models.User, lang string) {
	// Достаем ID из CustomID кнопки (формат: set_view:ID)
	customID := i.MessageComponentData().CustomID
	parts := strings.Split(customID, ":")

	if len(parts) < 2 {
		return
	}

	setID, _ := strconv.Atoi(parts[1])

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
