package discord

import (
	"strconv"

	"gachabot/internal/models"

	"github.com/bwmarrin/discordgo"
)

const (
	artGuessBoardColor   = 0x22d3ee
	artGuessSummaryColor = 0xb474e2
)

// artGuessComponents is the "Play" button row. It reuses the launch_app custom
// id, which HandleComponentInteraction answers with LAUNCH_ACTIVITY (type 12),
// opening the Activity in this channel (so the play attributes back here).
func (b *Bot) artGuessComponents() []discordgo.MessageComponent {
	return []discordgo.MessageComponent{discordgo.ActionsRow{Components: []discordgo.MessageComponent{
		discordgo.Button{CustomID: "launch_app", Label: "▶️ Играть", Style: discordgo.PrimaryButton},
	}}}
}

// PostBoard implements artguess.Broadcaster. startParam is unused on Discord
// (the Play button launches the Activity instead of carrying a deep link).
func (b *Bot) PostBoard(chat models.Chat, text, startParam string) (int64, error) {
	msg, err := b.session.ChannelMessageSendComplex(strconv.FormatInt(chat.ChatID, 10), &discordgo.MessageSend{
		Embeds:     []*discordgo.MessageEmbed{{Description: text, Color: artGuessBoardColor}},
		Components: b.artGuessComponents(),
	})
	if err != nil {
		return 0, err
	}
	return parseID(msg.ID), nil
}

// EditBoard implements artguess.Broadcaster: updates the live scoreboard in place.
func (b *Bot) EditBoard(chat models.Chat, messageID int64, text, startParam string) error {
	embeds := []*discordgo.MessageEmbed{{Description: text, Color: artGuessBoardColor}}
	comps := b.artGuessComponents()
	_, err := b.session.ChannelMessageEditComplex(&discordgo.MessageEdit{
		Channel:    strconv.FormatInt(chat.ChatID, 10),
		ID:         strconv.FormatInt(messageID, 10),
		Embeds:     &embeds,
		Components: &comps,
	})
	return err
}

// PostSummary implements artguess.Broadcaster: posts results with the answer art.
func (b *Bot) PostSummary(chat models.Chat, text string, art *models.Card) error {
	_, err := b.session.ChannelMessageSendComplex(strconv.FormatInt(chat.ChatID, 10), &discordgo.MessageSend{
		Embeds: []*discordgo.MessageEmbed{{
			Description: text,
			Image:       &discordgo.MessageEmbedImage{URL: art.ImageURL},
			Color:       artGuessSummaryColor,
		}},
	})
	return err
}
