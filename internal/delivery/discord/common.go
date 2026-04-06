package discord

import (
	"fmt"
	"gachabot/internal/models"
	"log"
	"strings"

	"github.com/bwmarrin/discordgo"
)

func (b *Bot) handleHelp(s *discordgo.Session, i *discordgo.InteractionCreate, lang string) {
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content:    b.loc.T(lang, "help_main"),
			Components: b.getHelpMenu(lang),
		},
	})
}

func (b *Bot) getHelpMenu(lang string) []discordgo.MessageComponent {
	return []discordgo.MessageComponent{
		discordgo.ActionsRow{
			Components: []discordgo.MessageComponent{
				discordgo.SelectMenu{
					CustomID:    "help_select",
					Placeholder: b.loc.T(lang, "btn_help_select"),
					Options: []discordgo.SelectMenuOption{
						{Label: b.loc.T(lang, "btn_help_main"), Value: "main", Emoji: &discordgo.ComponentEmoji{Name: "🏠"}},
						{Label: b.loc.T(lang, "btn_help_cards"), Value: "cards", Emoji: &discordgo.ComponentEmoji{Name: "🃏"}},
						{Label: b.loc.T(lang, "btn_help_rarities"), Value: "rarities", Emoji: &discordgo.ComponentEmoji{Name: "💎"}},
						{Label: b.loc.T(lang, "btn_help_streaks"), Value: "streaks", Emoji: &discordgo.ComponentEmoji{Name: "🔥"}},
						{Label: b.loc.T(lang, "btn_help_pity"), Value: "pity", Emoji: &discordgo.ComponentEmoji{Name: "🛡"}},
						{Label: b.loc.T(lang, "btn_help_duel"), Value: "duel", Emoji: &discordgo.ComponentEmoji{Name: "⚔️"}},
						{Label: b.loc.T(lang, "btn_help_craft"), Value: "craft", Emoji: &discordgo.ComponentEmoji{Name: "🛠"}},
					},
				},
			},
		},
	}
}

func (b *Bot) handleLocale(s *discordgo.Session, i *discordgo.InteractionCreate, dbUser *models.User, lang string) {
	options := i.ApplicationCommandData().Options
	if len(options) == 0 {
		b.respond(s, i, "❌ Please specify language code (ru/en)")
		return
	}

	targetLang := strings.ToLower(options[0].StringValue())
	if targetLang != "ru" && targetLang != "en" {
		b.respond(s, i, "❌ Unknown language. Use 'ru' or 'en'.")
		return
	}

	b.repo.SetUserLanguage(dbUser.ID, targetLang)
	b.respond(s, i, b.loc.T(targetLang, "lang_changed"))
}

// Вспомогательные функции
func parseID(idStr string) int64 {
	var id int64
	fmt.Sscanf(idStr, "%d", &id)
	return id
}

func (b *Bot) respond(s *discordgo.Session, i *discordgo.InteractionCreate, content string) {
	_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{Content: content},
	})
}

func (b *Bot) respondEphemeral(s *discordgo.Session, i *discordgo.InteractionCreate, content string) {
	_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{Content: content, Flags: discordgo.MessageFlagsEphemeral},
	})
}

func (b *Bot) respondWithComponents(s *discordgo.Session, i *discordgo.InteractionCreate, content string, components []discordgo.MessageComponent) {
	_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{Content: content, Components: components},
	})
}

func (b *Bot) updateWithComponents(s *discordgo.Session, i *discordgo.InteractionCreate, content string, components []discordgo.MessageComponent) {
	_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseUpdateMessage,
		Data: &discordgo.InteractionResponseData{Content: content, Components: components},
	})
}

func (b *Bot) updateWithEmbedAndComponents(s *discordgo.Session, i *discordgo.InteractionCreate, content string, embed *discordgo.MessageEmbed, components []discordgo.MessageComponent) {
	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseUpdateMessage,
		Data: &discordgo.InteractionResponseData{
			Content:    content,
			Embeds:     []*discordgo.MessageEmbed{embed},
			Components: components,
		},
	})

	// ВОТ ЭТО СПАСЕТ ТЕБЕ МНОГО ЧАСОВ ДЕБАГА
	if err != nil {
		log.Printf("[DISCORD ERROR] Не удалось обновить сообщение: %v", err)
	}
}
