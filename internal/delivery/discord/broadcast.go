package discord

import (
	"errors"
	"strconv"
)

// errLeaveUnsupported explains why a Discord "leave" is not offered.
var errLeaveUnsupported = errors.New("в Discord бот не может покинуть отдельный канал — убери бота с сервера вручную или отключи канал")

// SendChatMessage implements broadcast.Sender: posts an admin announcement into
// the guild's registered channel.
func (b *Bot) SendChatMessage(chatID int64, text string) error {
	_, err := b.session.ChannelMessageSend(strconv.FormatInt(chatID, 10), text)
	return err
}

// LeaveChat implements broadcast.Sender. On Discord the registry stores a
// channel, and a bot cannot leave a single channel — it would have to leave the
// whole guild, which is too blunt to trigger from an admin list. The caller is
// told to remove the bot from the server manually instead.
func (b *Bot) LeaveChat(chatID int64) error {
	return errLeaveUnsupported
}
