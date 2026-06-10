package telegram

import (
	"context"
	"fmt"

	"gachabot/internal/models"
	"gachabot/internal/service/artguess"

	tele "gopkg.in/telebot.v3"
)

// artGuessPlayURL deep-links into the Mini App carrying the (signed) chat context.
func (b *Bot) artGuessPlayURL(startParam string) string {
	return fmt.Sprintf("https://t.me/%s?startapp=%s", b.bot.Me.Username, startParam)
}

func (b *Bot) artGuessBoardMarkup(startParam string) *tele.ReplyMarkup {
	menu := &tele.ReplyMarkup{}
	btn := menu.URL("▶️ Играть", b.artGuessPlayURL(startParam))
	menu.Inline(menu.Row(btn))
	return menu
}

// PostBoard implements artguess.Broadcaster: posts the daily scoreboard with a
// Play button. Text is plain (no parse mode) so it renders verbatim.
func (b *Bot) PostBoard(chat models.Chat, text, startParam string) (int64, error) {
	msg, err := b.bot.Send(&tele.Chat{ID: chat.ChatID}, text, &tele.SendOptions{
		ReplyMarkup:           b.artGuessBoardMarkup(startParam),
		DisableWebPagePreview: true,
	})
	if err != nil {
		return 0, err
	}
	return int64(msg.ID), nil
}

// EditBoard implements artguess.Broadcaster: updates the live scoreboard in place.
func (b *Bot) EditBoard(chat models.Chat, messageID int64, text, startParam string) error {
	msg := &tele.Message{ID: int(messageID), Chat: &tele.Chat{ID: chat.ChatID}}
	_, err := b.bot.Edit(msg, text, &tele.SendOptions{
		ReplyMarkup:           b.artGuessBoardMarkup(startParam),
		DisableWebPagePreview: true,
	})
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
