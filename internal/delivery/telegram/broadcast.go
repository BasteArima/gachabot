package telegram

import (
	tele "gopkg.in/telebot.v3"
)

// SendChatMessage implements broadcast.Sender: posts an admin announcement.
func (b *Bot) SendChatMessage(chatID int64, text string) error {
	_, err := b.bot.Send(&tele.Chat{ID: chatID}, text, &tele.SendOptions{
		ParseMode:             tele.ModeHTML,
		DisableWebPagePreview: true,
	})
	return err
}

// LeaveChat implements broadcast.Sender: the bot leaves the group.
func (b *Bot) LeaveChat(chatID int64) error {
	return b.bot.Leave(&tele.Chat{ID: chatID})
}
