package discord

import (
	"fmt"
	"gachabot/internal/models"

	"github.com/bwmarrin/discordgo"
)

func (b *Bot) handleProfile(s *discordgo.Session, i *discordgo.InteractionCreate, user *models.User, lang string) {
	embed, buttons := b.getProfileData(user, lang)

	// В Discord не всегда есть аватарка в i.Member.User, лучше брать из i.Interaction.Member.User
	if i.Member != nil && i.Member.User != nil && i.Member.User.Avatar != "" {
		embed.Thumbnail = &discordgo.MessageEmbedThumbnail{URL: i.Member.User.AvatarURL("128")}
	} else if i.User != nil && i.User.Avatar != "" {
		embed.Thumbnail = &discordgo.MessageEmbedThumbnail{URL: i.User.AvatarURL("128")}
	}

	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Embeds:     []*discordgo.MessageEmbed{embed},
			Components: buttons,
		},
	})
}

func (b *Bot) handleCardsNav(s *discordgo.Session, i *discordgo.InteractionCreate, user *models.User, lang string, offset int) {
	card, total, err := b.service.GetUserCardPagination(user.ID, offset)
	if err != nil || card == nil {
		b.respondEphemeral(s, i, b.loc.T(lang, "cards_empty"))
		return
	}

	var desc string
	if card.SetName != "" {
		// 6 аргументов: SetName, RarityName, PowerLevel, Quantity, Current, Total
		desc = b.loc.T(lang, "card_nav_caption_with_set",
			card.SetName, card.RarityName, card.PowerLevel, card.Quantity, offset+1, total)
	} else {
		// 5 аргументов: RarityName, PowerLevel, Quantity, Current, Total
		desc = b.loc.T(lang, "card_nav_caption",
			card.RarityName, card.PowerLevel, card.Quantity, offset+1, total)
	}

	embed := &discordgo.MessageEmbed{
		Title:       "🃏 " + card.CardName,
		Description: desc,
		Image:       &discordgo.MessageEmbedImage{URL: card.ImageURL},
		Color:       0x3498db,
	}

	var navButtons []discordgo.MessageComponent

	// --- ЛОГИКА ЦИКЛИЧНОГО ЛИСТАНИЯ ---
	if total > 1 {
		// Вычисляем индексы с закольцовыванием
		prev := (offset - 1 + total) % total
		next := (offset + 1) % total

		navButtons = append(navButtons, discordgo.Button{
			Label:    "⬅️",
			Style:    discordgo.SecondaryButton,
			CustomID: fmt.Sprintf("cards_nav:%d", prev),
		})
		navButtons = append(navButtons, discordgo.Button{
			Label:    "➡️",
			Style:    discordgo.SecondaryButton,
			CustomID: fmt.Sprintf("cards_nav:%d", next),
		})
	}

	// Кнопка возврата в профиль всегда в конце
	navButtons = append(navButtons, discordgo.Button{
		Label:    "🔙",
		Style:    discordgo.DangerButton,
		CustomID: "back_to_profile",
	})

	row := discordgo.ActionsRow{Components: navButtons}
	b.updateWithEmbedAndComponents(s, i, "", embed, []discordgo.MessageComponent{row})
}

func (b *Bot) getProfileData(user *models.User, lang string) (*discordgo.MessageEmbed, []discordgo.MessageComponent) {
	profile, _ := b.service.GetUserProfile(user.ID)

	desc := fmt.Sprintf("**%s**\n\n", user.Username)
	desc += b.loc.T(lang, "profile_stats",
		profile.UniqueCardsCount, profile.TotalCardsCount,
		profile.DuplicatesCount, profile.Balance, profile.StreakDays)

	embed := &discordgo.MessageEmbed{
		Title:       b.loc.T(lang, "profile_title"),
		Description: desc,
		Color:       0x7289da,
	}

	buttons := []discordgo.MessageComponent{
		discordgo.ActionsRow{
			Components: []discordgo.MessageComponent{
				discordgo.Button{
					Label:    b.loc.T(lang, "btn_my_cards_ds"),
					Style:    discordgo.PrimaryButton,
					CustomID: "cards_nav:0",
					Emoji:    &discordgo.ComponentEmoji{Name: "🎴"},
				},
				discordgo.Button{
					Label:    b.loc.T(lang, "btn_my_sets", profile.CompletedSets, profile.TotalSets),
					Style:    discordgo.SuccessButton, // Зеленый цвет выделяется
					CustomID: "sets_nav:0",
					Emoji:    &discordgo.ComponentEmoji{Name: "📚"},
				},
				discordgo.Button{
					Label:    b.loc.T(lang, "btn_profile_suggest"),
					Style:    discordgo.SecondaryButton,
					CustomID: "suggest_start",
					Emoji:    &discordgo.ComponentEmoji{Name: "💡"},
				},
			},
		},
	}
	return embed, buttons
}
