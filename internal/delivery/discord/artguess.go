package discord

import (
	"bytes"
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

const artGuessBoardImage = "attachment://blur.jpg"

// PostBoard implements artguess.Broadcaster: posts the scoreboard embed with the
// blurred daily art attached. startParam is unused on Discord (the Play button
// launches the Activity instead of carrying a deep link).
func (b *Bot) PostBoard(chat models.Chat, text, startParam string, image []byte) (int64, error) {
	embed := &discordgo.MessageEmbed{Description: text, Color: artGuessBoardColor}
	send := &discordgo.MessageSend{
		Embeds:     []*discordgo.MessageEmbed{embed},
		Components: b.artGuessComponents(),
	}
	if len(image) > 0 {
		embed.Image = &discordgo.MessageEmbedImage{URL: artGuessBoardImage}
		send.Files = []*discordgo.File{{Name: "blur.jpg", ContentType: "image/jpeg", Reader: bytes.NewReader(image)}}
	}
	msg, err := b.session.ChannelMessageSendComplex(strconv.FormatInt(chat.ChatID, 10), send)
	if err != nil {
		return 0, err
	}
	return parseID(msg.ID), nil
}

// EditBoard implements artguess.Broadcaster: updates the scoreboard text in place.
// The embed keeps referencing the originally-attached image (retained on edit).
func (b *Bot) EditBoard(chat models.Chat, messageID int64, text, startParam string) error {
	embeds := []*discordgo.MessageEmbed{{
		Description: text,
		Image:       &discordgo.MessageEmbedImage{URL: artGuessBoardImage},
		Color:       artGuessBoardColor,
	}}
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
