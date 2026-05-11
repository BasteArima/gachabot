package discord

import (
	"fmt"
	"gachabot/internal/models"
	"strconv"
	"strings"

	"github.com/bwmarrin/discordgo"
)

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

func (b *Bot) handleSetsList(s *discordgo.Session, i *discordgo.InteractionCreate, user *models.User, lang string, page int) {
	setsProgress, err := b.service.GetUserSetsProgress(user.ID)
	if err != nil || len(setsProgress) == 0 {
		b.respondEphemeral(s, i, b.loc.Translate(lang, "sets_empty"))
		return
	}

	const pageSize = 4
	totalSets := len(setsProgress)
	totalPages := (totalSets + pageSize - 1) / pageSize

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

	for _, sp := range pageSets {
		status := fmt.Sprintf("%d/%d", sp.CollectedCards, sp.TotalCards)

		btnStyle := discordgo.SecondaryButton // Серый по умолчанию
		if sp.IsCompleted {
			status = b.loc.Translate(lang, "set_status_completed") // ✅
			btnStyle = discordgo.SuccessButton
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
			CustomID: fmt.Sprintf("set_view:%d", sp.SetID),
		}

		components = append(components, discordgo.ActionsRow{
			Components: []discordgo.MessageComponent{btn},
		})
	}

	var navButtons []discordgo.MessageComponent
	if totalPages > 1 {
		navButtons = append(navButtons, discordgo.Button{Label: "⬅️", Style: discordgo.PrimaryButton, CustomID: fmt.Sprintf("sets_nav:%d", page-1)})
		navButtons = append(navButtons, discordgo.Button{Label: "➡️", Style: discordgo.PrimaryButton, CustomID: fmt.Sprintf("sets_nav:%d", page+1)})
	}
	navButtons = append(navButtons, discordgo.Button{Label: "🔙", Style: discordgo.DangerButton, CustomID: "back_to_profile"})

	components = append(components, discordgo.ActionsRow{Components: navButtons})

	embed := &discordgo.MessageEmbed{
		Title:       b.loc.Translate(lang, "sets_list_title"),
		Description: fmt.Sprintf("`─── Стр. %d / %d ───`", page+1, totalPages),
		Color:       0x5865F2,
	}

	b.updateWithEmbedAndComponents(s, i, "", embed, components)
}

func (b *Bot) handleSetView(s *discordgo.Session, i *discordgo.InteractionCreate, user *models.User, lang string) {
	customID := i.MessageComponentData().CustomID
	parts := strings.Split(customID, ":")

	if len(parts) < 2 {
		return
	}

	setID, _ := strconv.Atoi(parts[1])

	b.renderSetView(s, i, user, lang, setID, false)
}

func (b *Bot) handleEquipAura(s *discordgo.Session, i *discordgo.InteractionCreate, user *models.User, lang string) {
	customID := i.MessageComponentData().CustomID
	data := strings.Split(customID, ":") // set_equip:ID:on/off
	setID, _ := strconv.Atoi(data[1])
	action := data[2]

	var newActiveSetID *int
	var toastMsg string

	if action == "on" {
		newActiveSetID = &setID
		toastMsg = b.loc.Translate(lang, "aura_equipped_toast")
	} else {
		newActiveSetID = nil
		toastMsg = b.loc.Translate(lang, "aura_unequipped_toast")
	}

	_ = b.service.EquipSetAura(user.ID, newActiveSetID)

	// We send a pop-up notification (Toast) that is visible only to the user
	b.respondEphemeral(s, i, toastMsg)

	// Update the current message (redraw the buttons) to change the equipment status
	b.renderSetView(s, i, user, lang, setID, true)
}

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

	buffName := b.loc.Translate(lang, "buff_type_"+currentSet.BuffType)
	buffDesc := fmt.Sprintf("+%d%% (%s)", currentSet.BuffValue, buffName)

	desc := b.loc.Translate(lang, "set_view_title", currentSet.SetName, currentSet.RewardPoints, buffDesc)

	for j, c := range cards {
		if c.Name != "" {
			desc += b.loc.Translate(lang, "set_card_owned", j+1, c.Name) + "\n"
		} else {
			desc += b.loc.Translate(lang, "set_card_unknown", j+1) + "\n"
		}
	}

	embed := &discordgo.MessageEmbed{
		Description: desc,
		Color:       0x2ecc71,
	}

	var buttons []discordgo.MessageComponent
	btnBack := discordgo.Button{
		Label:    b.loc.Translate(lang, "btn_back_to_sets"),
		Style:    discordgo.SecondaryButton,
		CustomID: "sets_nav:0",
	}

	if currentSet.IsCompleted {
		if currentSet.IsActive {
			btnUnequip := discordgo.Button{
				Label:    b.loc.Translate(lang, "btn_unequip_aura"),
				Style:    discordgo.DangerButton,
				CustomID: fmt.Sprintf("set_equip:%d:off", setID),
			}
			buttons = append(buttons, btnUnequip)
		} else {
			btnEquip := discordgo.Button{
				Label:    b.loc.Translate(lang, "btn_equip_aura"),
				Style:    discordgo.PrimaryButton,
				CustomID: fmt.Sprintf("set_equip:%d:on", setID),
			}
			buttons = append(buttons, btnEquip)
		}
	}

	buttons = append(buttons, btnBack)
	row := discordgo.ActionsRow{Components: buttons}

	if isFollowUp {
		// If we have already responded (eg Toast notification on equip), we must update the original
		_, _ = s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Embeds:     &[]*discordgo.MessageEmbed{embed},
			Components: &[]discordgo.MessageComponent{row},
		})
	} else {
		b.updateWithEmbedAndComponents(s, i, "", embed, []discordgo.MessageComponent{row})
	}
}
