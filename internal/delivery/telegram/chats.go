package telegram

import (
	"log"

	tele "gopkg.in/telebot.v3"
)

const platformTelegram = "telegram"

// isGroup reports whether the chat is a group/supergroup (spawns never go to DMs).
func isGroup(chat *tele.Chat) bool {
	return chat != nil && (chat.Type == tele.ChatGroup || chat.Type == tele.ChatSuperGroup)
}

// HandleMyChatMember keeps the chats registry in sync when the bot is added to
// or removed from a group. Registry drives spawns and broadcasts.
func (b *Bot) HandleMyChatMember(ctx tele.Context) error {
	upd := ctx.ChatMember()
	if upd == nil || upd.Chat == nil || upd.NewChatMember == nil {
		return nil
	}
	if !isGroup(upd.Chat) {
		return nil // ignore private chats and channels
	}

	chat := upd.Chat
	switch upd.NewChatMember.Role {
	case tele.Member, tele.Administrator, tele.Creator:
		if err := b.repo.UpsertChatEnabled(platformTelegram, chat.ID, nil, chat.Title); err != nil {
			log.Printf("[TG chats] register %d failed: %v", chat.ID, err)
		} else {
			log.Printf("[TG chats] registered group %d (%s)", chat.ID, chat.Title)
		}
	case tele.Left, tele.Kicked:
		if err := b.repo.SetChatSpawnEnabled(platformTelegram, chat.ID, false); err != nil {
			log.Printf("[TG chats] disable %d failed: %v", chat.ID, err)
		} else {
			log.Printf("[TG chats] disabled group %d (%s)", chat.ID, chat.Title)
		}
	}
	return nil
}

// lazyRegisterChat records a group on normal activity, a fallback for groups the
// bot already sat in before my_chat_member tracking existed. Never re-enables a
// chat an admin disabled (uses EnsureChat).
func (b *Bot) lazyRegisterChat(ctx tele.Context) {
	chat := ctx.Chat()
	if !isGroup(chat) {
		return
	}
	if err := b.repo.EnsureChat(platformTelegram, chat.ID, nil, chat.Title); err != nil {
		log.Printf("[TG chats] lazy register %d failed: %v", chat.ID, err)
	}
}
