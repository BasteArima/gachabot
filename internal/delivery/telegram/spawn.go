package telegram

import (
	"log"
	"strconv"

	"gachabot/internal/cardart"
	"gachabot/internal/i18n"
	"gachabot/internal/models"
	"gachabot/internal/service/spawn"

	tele "gopkg.in/telebot.v3"
)

// spawnLang is the language used for chat-wide spawn messages (no per-chat
// language is stored; default to Russian like the rest of group output).
const spawnLang = "ru"

// SendSpawn implements spawn.Spawner: posts a wild-card message with a claim button.
func (b *Bot) SendSpawn(chat models.Chat, view spawn.SpawnView) (int64, error) {
	caption := b.loc.Translate(spawnLang, "spawn_appeared", i18n.Args{
		"name":    view.Card.Name,
		"rarity":  b.loc.Rarity(spawnLang, view.RarityName),
		"seconds": view.WindowSeconds,
	})

	menu := &tele.ReplyMarkup{}
	btn := menu.Data(b.loc.Translate(spawnLang, "btn_spawn_claim"), "spawn_claim", strconv.FormatInt(view.SpawnID, 10))
	menu.Inline(menu.Row(btn))

	photo := &tele.Photo{File: tele.FromURL(cardart.Framed(view.Card.ImageURL)), Caption: caption}
	msg, err := b.bot.Send(&tele.Chat{ID: chat.ChatID}, photo, &tele.SendOptions{ParseMode: tele.ModeHTML, ReplyMarkup: menu})
	if err != nil {
		return 0, err
	}
	return int64(msg.ID), nil
}

// EditSpawnExpired implements spawn.Spawner: marks a spawn as escaped and drops
// the claim button.
func (b *Bot) EditSpawnExpired(meta spawn.SpawnMeta, card *models.Card) {
	caption := b.loc.Translate(spawnLang, "spawn_expired", i18n.Args{"name": card.Name})
	msg := &tele.Message{ID: int(meta.MessageID), Chat: &tele.Chat{ID: meta.ChatID}}
	if _, err := b.bot.EditCaption(msg, caption, &tele.SendOptions{ParseMode: tele.ModeHTML, ReplyMarkup: &tele.ReplyMarkup{}}); err != nil {
		log.Printf("[TG spawn] edit expired failed: %v", err)
	}
}

// HandleSpawnClaim handles the claim button.
func (b *Bot) HandleSpawnClaim(ctx tele.Context) error {
	spawnID, err := strconv.ParseInt(ctx.Data(), 10, 64)
	if err != nil {
		return ctx.Respond()
	}
	tgUser := ctx.Sender()
	dbUser, err := b.repo.GetOrCreateUserByTelegramID(tgUser.ID, tgUser.Username, tgUser.FirstName, tgUser.LastName)
	if err != nil {
		return ctx.Respond()
	}
	lang := getLang(dbUser, tgUser)

	res, err := b.spawnService.Claim(spawn.PlatformTelegram, dbUser.ID, spawnID)
	if err != nil {
		log.Printf("[TG spawn] claim error: %v", err)
		return ctx.Respond(&tele.CallbackResponse{Text: b.loc.Translate(lang, "error_tech"), ShowAlert: true})
	}

	switch res.Outcome {
	case spawn.OutcomeWon:
		b.announceSpawnClaim(res.Meta, spawnDisplayName(tgUser), res)
		return ctx.Respond(&tele.CallbackResponse{Text: b.loc.Translate(lang, "spawn_toast_won", i18n.Args{"coins": res.Reward.Coins})})
	case spawn.OutcomeTaken:
		return ctx.Respond(&tele.CallbackResponse{Text: b.loc.Translate(lang, "spawn_toast_taken"), ShowAlert: true})
	case spawn.OutcomeAlreadyWave:
		return ctx.Respond(&tele.CallbackResponse{Text: b.loc.Translate(lang, "spawn_toast_already"), ShowAlert: true})
	default: // expired
		return ctx.Respond(&tele.CallbackResponse{Text: b.loc.Translate(lang, "spawn_toast_expired"), ShowAlert: true})
	}
}

// HandleClaimCommand handles /claim — claims the active spawn in this chat.
func (b *Bot) HandleClaimCommand(ctx tele.Context) error {
	chat := ctx.Chat()
	tgUser := ctx.Sender()
	dbUser, err := b.repo.GetOrCreateUserByTelegramID(tgUser.ID, tgUser.Username, tgUser.FirstName, tgUser.LastName)
	if err != nil {
		return ctx.Send(b.loc.Translate(spawnLang, "error_tech"))
	}
	lang := getLang(dbUser, tgUser)

	if !isGroup(chat) {
		return ctx.Send(b.loc.Translate(lang, "spawn_group_only"))
	}
	spawnID, ok := b.spawnService.ActiveSpawnInChat(spawn.PlatformTelegram, chat.ID)
	if !ok {
		return ctx.Send(b.loc.Translate(lang, "spawn_no_active"))
	}

	res, err := b.spawnService.Claim(spawn.PlatformTelegram, dbUser.ID, spawnID)
	if err != nil {
		log.Printf("[TG spawn] /claim error: %v", err)
		return ctx.Send(b.loc.Translate(lang, "error_tech"))
	}

	switch res.Outcome {
	case spawn.OutcomeWon:
		b.announceSpawnClaim(res.Meta, spawnDisplayName(tgUser), res)
		return nil
	case spawn.OutcomeTaken:
		return ctx.Send(b.loc.Translate(lang, "spawn_toast_taken"))
	case spawn.OutcomeAlreadyWave:
		return ctx.Send(b.loc.Translate(lang, "spawn_toast_already"))
	default:
		return ctx.Send(b.loc.Translate(lang, "spawn_no_active"))
	}
}

// announceSpawnClaim finalises a won spawn: it drops the claim button and marks
// the original photo message as caught, then posts a separate reply naming the
// winner and the reward — without re-sending the card image.
func (b *Bot) announceSpawnClaim(meta *spawn.SpawnMeta, winner string, res spawn.ClaimResult) {
	if meta == nil {
		return
	}
	msg := &tele.Message{ID: int(meta.MessageID), Chat: &tele.Chat{ID: meta.ChatID}}

	caught := b.loc.Translate(spawnLang, "spawn_msg_caught", i18n.Args{
		"name":   res.Card.Name,
		"rarity": b.loc.Rarity(spawnLang, res.RarityName),
	})
	// Pass everything via a single SendOptions: telebot's option extractor lets a
	// *SendOptions overwrite a separately-passed ParseMode, so combining them as
	// loose varargs silently drops HTML parsing.
	if _, err := b.bot.EditCaption(msg, caught, &tele.SendOptions{ParseMode: tele.ModeHTML, ReplyMarkup: &tele.ReplyMarkup{}}); err != nil {
		log.Printf("[TG spawn] edit caught failed: %v", err)
	}

	text := b.formatSpawnClaimed(spawnLang, winner, res)
	if _, err := b.bot.Send(msg.Chat, text, &tele.SendOptions{ParseMode: tele.ModeHTML, ReplyTo: msg}); err != nil {
		log.Printf("[TG spawn] send claim announce failed: %v", err)
	}
}

// formatSpawnClaimed picks the right "claimed" message variant.
func (b *Bot) formatSpawnClaimed(lang, winner string, res spawn.ClaimResult) string {
	r := res.Reward
	args := i18n.Args{
		"winner":    winner,
		"name":      res.Card.Name,
		"rarity":    b.loc.Rarity(lang, res.RarityName),
		"coins":     r.Coins,
		"fragments": r.FragmentsCount,
	}
	switch {
	case r.CardAssembled:
		return b.loc.Translate(lang, "spawn_claimed_assembled", args)
	case r.IsFragment:
		return b.loc.Translate(lang, "spawn_claimed_fragment", args)
	case r.IsDuplicate:
		return b.loc.Translate(lang, "spawn_claimed_duplicate", args)
	default:
		return b.loc.Translate(lang, "spawn_claimed", args)
	}
}

func spawnDisplayName(u *tele.User) string {
	if u.FirstName != "" {
		return u.FirstName
	}
	if u.Username != "" {
		return "@" + u.Username
	}
	return "Player"
}
