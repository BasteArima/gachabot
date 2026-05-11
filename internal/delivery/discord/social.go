package discord

import (
	"context"
	"fmt"
	"gachabot/internal/models"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
)

func (b *Bot) handleDuel(s *discordgo.Session, i *discordgo.InteractionCreate, challenger *models.User, lang string) {
	options := i.ApplicationCommandData().Options
	targetDsUser := options[0].UserValue(s)
	amount := int(options[1].IntValue())

	if targetDsUser.ID == i.Member.User.ID {
		b.respond(s, i, b.loc.Translate(lang, "err_duel_self"))
		return
	}

	if challenger.Balance < amount {
		b.respond(s, i, b.loc.Translate(lang, "err_duel_funds"))
		return
	}

	targetDB, err := b.repo.GetOrCreateUserByDiscordID(parseID(targetDsUser.ID), targetDsUser.Username)
	if err != nil {
		b.respond(s, i, b.loc.Translate(lang, "error_db"))
		return
	}

	duelID := fmt.Sprintf("duel:%d:%d:%d", challenger.ID, targetDB.ID, amount)
	b.duelService.CreateDuel(duelID, challenger.ID, challenger.Username, targetDB.ID, targetDB.Username, amount, false) // в конце false временно, нужно перенести IsFair сюда тоже

	buttons := []discordgo.MessageComponent{discordgo.ActionsRow{
		Components: []discordgo.MessageComponent{
			discordgo.Button{Label: b.loc.Translate(lang, "btn_duel_accept"), Style: discordgo.SuccessButton, CustomID: "duel_accept:" + duelID},
			discordgo.Button{Label: b.loc.Translate(lang, "btn_duel_cancel"), Style: discordgo.DangerButton, CustomID: "duel_cancel:" + duelID},
		},
	}}

	b.respondWithComponents(s, i, b.loc.Translate(lang, "duel_challenge", challenger.Username, targetDB.Username, amount), buttons)
}

func (b *Bot) handleLink(s *discordgo.Session, i *discordgo.InteractionCreate, dsUser *models.User, lang string) {
	options := i.ApplicationCommandData().Options
	if len(options) == 0 {
		b.respond(s, i, b.loc.Translate(lang, "link_err_invalid"))
		return
	}

	code := strings.ToUpper(options[0].StringValue())
	tgInternalID, exists := b.lp.GetIDByCode(code)
	if !exists {
		b.respond(s, i, b.loc.Translate(lang, "link_err_invalid"))
		return
	}

	tgProfile, err1 := b.service.GetUserProfile(tgInternalID)
	dsProfile, err2 := b.service.GetUserProfile(dsUser.ID)
	if err1 != nil || err2 != nil {
		b.respond(s, i, b.loc.Translate(lang, "error_db"))
		return
	}

	// Save the binding session in Redis for 10 minutes
	rCtx := context.Background()
	b.rdb.Set(rCtx, fmt.Sprintf("pending_link:%d", dsUser.ID), tgInternalID, 10*time.Minute)

	msg := b.loc.Translate(lang, "link_choice_msg", tgProfile.Balance, tgProfile.UniqueCardsCount, dsProfile.Balance, dsProfile.UniqueCardsCount)

	buttons := []discordgo.MessageComponent{discordgo.ActionsRow{
		Components: []discordgo.MessageComponent{
			discordgo.Button{Label: b.loc.Translate(lang, "link_keep_tg"), Style: discordgo.PrimaryButton, CustomID: "link:keep:tg"},
			discordgo.Button{Label: b.loc.Translate(lang, "link_keep_ds"), Style: discordgo.PrimaryButton, CustomID: "link:keep:ds"},
			discordgo.Button{Label: b.loc.Translate(lang, "btn_cancel"), Style: discordgo.DangerButton, CustomID: "link:cancel"},
		},
	}}

	b.respondWithComponents(s, i, msg, buttons)
}

func (b *Bot) handleTop(s *discordgo.Session, i *discordgo.InteractionCreate, lang, criteria string, global bool) {
	var targetChatID int64
	scope := "local"
	if global {
		scope = "global"
		targetChatID = 0
	} else {
		targetChatID = parseID(i.GuildID)
	}

	board, err := b.service.GetLeaderboard(criteria, targetChatID)
	if err != nil {
		b.respond(s, i, b.loc.Translate(lang, "error_top_load"))
		return
	}

	emoji := "🪙"
	if criteria == "cards" {
		emoji = "🃏"
	}
	if criteria == "streak" {
		emoji = "🔥"
	}

	var sb strings.Builder
	if len(board) == 0 {
		sb.WriteString(b.loc.Translate(lang, "top_empty"))
	} else {
		for idx, entry := range board {
			medal := "🏅"
			if idx == 0 {
				medal = "🥇"
			} else if idx == 1 {
				medal = "🥈"
			} else if idx == 2 {
				medal = "🥉"
			}
			sb.WriteString(fmt.Sprintf("%s **%s** — %d %s\n", medal, entry.DisplayName, entry.Value, emoji))
		}
	}

	embed := &discordgo.MessageEmbed{
		Title:       "🏆 " + b.loc.Translate(lang, "top_"+scope+"_title") + " (" + b.loc.Translate(lang, "top_crit_"+criteria) + ")",
		Description: sb.String(),
		Color:       0xf1c40f,
	}

	buttons := discordgo.ActionsRow{
		Components: []discordgo.MessageComponent{
			discordgo.Button{Label: b.loc.Translate(lang, "btn_top_balance"), Style: discordgo.SecondaryButton, CustomID: fmt.Sprintf("top:balance:%s", scope), Emoji: &discordgo.ComponentEmoji{Name: "🪙"}},
			discordgo.Button{Label: b.loc.Translate(lang, "btn_top_cards"), Style: discordgo.SecondaryButton, CustomID: fmt.Sprintf("top:cards:%s", scope), Emoji: &discordgo.ComponentEmoji{Name: "🃏"}},
			discordgo.Button{Label: b.loc.Translate(lang, "btn_top_streak"), Style: discordgo.SecondaryButton, CustomID: fmt.Sprintf("top:streak:%s", scope), Emoji: &discordgo.ComponentEmoji{Name: "🔥"}},
		},
	}

	responseType := discordgo.InteractionResponseChannelMessageWithSource
	if i.Type == discordgo.InteractionMessageComponent {
		responseType = discordgo.InteractionResponseUpdateMessage
	}

	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: responseType,
		Data: &discordgo.InteractionResponseData{
			Embeds:     []*discordgo.MessageEmbed{embed},
			Components: []discordgo.MessageComponent{buttons},
		},
	})
}
