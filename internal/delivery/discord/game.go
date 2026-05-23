package discord

import (
	"gachabot/internal/models"
	"log"
	"strings"

	"github.com/bwmarrin/discordgo"
)

func (b *Bot) handleRoll(s *discordgo.Session, i *discordgo.InteractionCreate, user *models.User, lang string) {
	result, err := b.service.RollCard(user.ID)
	if err != nil {
		b.respond(s, i, b.loc.Translate(lang, "error_tech"))
		return
	}

	if result.OnCooldown {
		msg := b.loc.Translate(lang, "roll_cooldown", result.CooldownTimeLeft)
		if result.StreakUpdated {
			msg += "\n\n" + b.loc.Translate(lang, "streak_kept_alive", result.StreakDays)
		}
		b.respond(s, i, msg)
		return
	}

	title := b.loc.Translate(lang, "roll_success_title")
	desc := b.loc.Translate(lang, "roll_success_desc", result.Card.Name, result.RarityName, result.Card.PowerLevel, result.Reward)

	if result.IsFragment {
		if result.CardAssembled {
			desc = b.loc.Translate(lang, "roll_mythic_assembled", result.Card.Name, result.Card.PowerLevel, result.Reward)
		} else {
			desc = b.loc.Translate(lang, "roll_mythic_fragment", result.Card.Name, result.FragmentsCount, result.Reward)
		}
	}

	embed := &discordgo.MessageEmbed{
		Title:       title,
		Description: desc,
		Image:       &discordgo.MessageEmbedImage{URL: result.Card.ImageURL},
		Color:       0x00ff00,
	}

	components := []discordgo.MessageComponent{
		discordgo.ActionsRow{
			Components: []discordgo.MessageComponent{
				discordgo.Button{
					CustomID: "roll_again",
					Label:    b.loc.Translate(lang, "btn_roll_again"),
					Style:    discordgo.PrimaryButton,
				},
			},
		},
	}

	err = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Embeds:     []*discordgo.MessageEmbed{embed},
			Components: components,
		},
	})

	if err != nil {
		// Рекомендую всегда логировать ошибки Дискорда
		log.Printf("[DISCORD ERROR] Failed to send card: %v", err)
	}

	if result.StreakUpdated {
		streakMsg := b.loc.Translate(lang, "streak_continued", result.Reward, result.StreakDays)
		if result.StreakDays == 1 {
			streakMsg = b.loc.Translate(lang, "streak_started")
		}
		s.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{Content: "🔥 " + streakMsg})
	}
}

func (b *Bot) handleRollAgainCallback(s *discordgo.Session, i *discordgo.InteractionCreate, user *models.User, lang string) {
	b.handleRoll(s, i, user, lang)
}

func (b *Bot) handleCraft(s *discordgo.Session, i *discordgo.InteractionCreate, user *models.User, lang string) {
	result, err := b.service.CraftCard(user.ID)
	if err != nil {
		translatedReason := b.loc.Translate(lang, err.Error())
		b.respond(s, i, "❌ "+b.loc.Translate(lang, "err_craft_failed", translatedReason))
		return
	}

	var desc string
	if result.IsFragment {
		if !result.CardAssembled {
			desc = b.loc.Translate(lang, "craft_mythic_frag", result.CraftCost, result.Card.Name, result.FragmentsCount)
		} else {
			desc = b.loc.Translate(lang, "craft_mythic_assembled", result.CraftCost, result.Card.Name, result.Card.PowerLevel)
		}
	} else {
		desc = b.loc.Translate(lang, "craft_success", result.CraftCost, result.Card.Name, result.RarityName, result.Card.PowerLevel)
	}

	embed := &discordgo.MessageEmbed{
		Title:       "🛠 " + b.loc.Translate(lang, "craft_title"),
		Description: desc,
		Image:       &discordgo.MessageEmbedImage{URL: result.Card.ImageURL},
		Color:       0x9b59b6,
	}

	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{Embeds: []*discordgo.MessageEmbed{embed}},
	})
}

func (b *Bot) handlePromo(s *discordgo.Session, i *discordgo.InteractionCreate, dbUser *models.User, lang string) {
	options := i.ApplicationCommandData().Options
	if len(options) == 0 {
		b.respondEphemeral(s, i, b.loc.Translate(lang, "promo_usage"))
		return
	}

	code := options[0].StringValue()

	reward, cards, err := b.service.RedeemPromo(dbUser.ID, code)
	if err != nil {
		var errKey string
		switch err.Error() {
		case "not_found":
			errKey = "promo_err_not_found"
		case "limit_reached":
			errKey = "promo_err_limit"
		case "already_used":
			errKey = "promo_err_used"
		case "expired":
			errKey = "promo_err_expired"
		default:
			errKey = "error_db"
		}
		b.respondEphemeral(s, i, b.loc.Translate(lang, errKey))
		return
	}

	var sb strings.Builder
	sb.WriteString("**" + b.loc.Translate(lang, "promo_success_title") + "**\n\n")
	if reward.Points > 0 {
		sb.WriteString(b.loc.Translate(lang, "promo_reward_points", reward.Points) + "\n")
	}
	if reward.PremiumRolls > 0 {
		sb.WriteString(b.loc.Translate(lang, "promo_reward_rolls", reward.PremiumRolls) + "\n")
	}

	var embeds []*discordgo.MessageEmbed

	if len(cards) > 0 {
		sb.WriteString("\n" + b.loc.Translate(lang, "promo_reward_cards_count", len(cards)) + "\n")
		for _, c := range cards {
			sb.WriteString(b.loc.Translate(lang, "promo_reward_card", c.Name, c.PowerLevel) + "\n")
		}

		imgLimit := len(cards)
		if imgLimit > 10 {
			imgLimit = 10
		}

		for j := 0; j < imgLimit; j++ {
			embeds = append(embeds, &discordgo.MessageEmbed{
				Image: &discordgo.MessageEmbedImage{URL: cards[j].ImageURL},
			})
		}
	}

	_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: sb.String(),
			Embeds:  embeds,
		},
	})
}
