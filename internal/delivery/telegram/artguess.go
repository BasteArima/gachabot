package telegram

import (
	"bytes"
	"context"
	"fmt"
	"strings"

	"gachabot/internal/models"
	"gachabot/internal/service/artguess"

	tele "gopkg.in/telebot.v3"
)

// artGuessPlayURL deep-links into the Mini App carrying the (signed) chat context.
func (b *Bot) artGuessPlayURL(startParam string) string {
	return fmt.Sprintf("https://t.me/%s?startapp=%s", b.bot.Me.Username, startParam)
}

// openAppURL is the "Открыть приложение" target. In a group it carries the
// signed Art Guess chat context so opening the app from there still attributes
// the play to this chat's scoreboard (same as the "Играть" button); in private
// chats there's no chat to attribute to.
func (b *Bot) openAppURL(chat *tele.Chat) string {
	base := "https://t.me/" + b.bot.Me.Username + "?startapp"
	if chat != nil && isGroup(chat) {
		// Context-only param: attributes Art Guess play to this chat, but opens the
		// hub (not the game) — unlike the board's "Играть" button.
		return base + "=" + b.artguess.ContextParam(artguess.PlatformTelegram, chat.ID)
	}
	return base
}

func (b *Bot) artGuessBoardMarkup(startParam string) *tele.ReplyMarkup {
	menu := &tele.ReplyMarkup{}
	btn := menu.URL("▶️ Играть", b.artGuessPlayURL(startParam))
	menu.Inline(menu.Row(btn))
	return menu
}

// PostBoard implements artguess.Broadcaster: posts the scoreboard as a photo
// (blurred daily art + caption text) with a Play button. Caption is plain (no
// parse mode) so it renders verbatim.
func (b *Bot) PostBoard(chat models.Chat, text, startParam string, image []byte) (int64, error) {
	photo := &tele.Photo{File: tele.FromReader(bytes.NewReader(image)), Caption: text}
	msg, err := b.bot.Send(&tele.Chat{ID: chat.ChatID}, photo, &tele.SendOptions{
		ReplyMarkup: b.artGuessBoardMarkup(startParam),
	})
	if err != nil {
		return 0, err
	}
	return int64(msg.ID), nil
}

// EditBoard implements artguess.Broadcaster: updates the scoreboard caption in
// place (the blurred photo stays).
func (b *Bot) EditBoard(chat models.Chat, messageID int64, text, startParam string) error {
	msg := &tele.Message{ID: int(messageID), Chat: &tele.Chat{ID: chat.ChatID}}
	_, err := b.bot.EditCaption(msg, text, &tele.SendOptions{
		ReplyMarkup: b.artGuessBoardMarkup(startParam),
	})
	// "message is not modified" just means nothing changed since the last edit
	// (e.g. the morning ping re-rendering an unchanged board) — treat as success.
	if err != nil && strings.Contains(err.Error(), "not modified") {
		return nil
	}
	return err
}

// PostSummary implements artguess.Broadcaster: posts the daily results with the
// answer art revealed.
func (b *Bot) PostSummary(chat models.Chat, text string, art *models.Card) error {
	photo := &tele.Photo{File: tele.FromURL(art.ImageURL), Caption: text}
	_, err := b.bot.Send(&tele.Chat{ID: chat.ChatID}, photo, &tele.SendOptions{})
	return err
}

// HandleArtGuessCmd posts the "card of the day" scoreboard in the current group.
func (b *Bot) HandleArtGuessCmd(ctx tele.Context) error {
	chat := ctx.Chat()
	if !isGroup(chat) {
		return ctx.Send("Команда «Карта дня» работает в группах.")
	}
	// Lazily register the chat so it also gets morning pings and the daily summary.
	_ = b.repo.EnsureChat(artguess.PlatformTelegram, chat.ID, nil, chat.Title)
	b.artguess.ShowBoard(context.Background(), artguess.PlatformTelegram, chat.ID)
	return nil
}
