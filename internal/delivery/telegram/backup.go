package telegram

import (
	tele "gopkg.in/telebot.v3"
)

func (b *Bot) SendBackup(filePath string) error {
	if b.backupChatID == 0 {
		return nil // Backups are disabled
	}

	caption := b.loc.Translate("ru", "backup_caption")

	doc := &tele.Document{
		File:    tele.FromDisk(filePath),
		Caption: caption,
	}

	chat := &tele.Chat{ID: b.backupChatID}
	_, err := b.bot.Send(chat, doc, tele.ModeHTML)
	return err
}
