package discord

import (
	"bytes"
	"gachabot/internal/i18n"
	"gachabot/internal/models"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
)

func (b *Bot) handleRoll(s *discordgo.Session, i *discordgo.InteractionCreate, user *models.User, lang string) {
	result, err := b.service.RollCard(user.ID)
	if err != nil {
		b.respond(s, i, b.loc.Translate(lang, "error_tech"))
		return
	}

	if result.OnCooldown {
		msg := b.loc.Translate(lang, "roll_cooldown", i18n.Args{"time": result.CooldownTimeLeft})
		if result.StreakUpdated {
			msg += "\n\n" + b.loc.Translate(lang, "streak_kept_alive", i18n.Args{"days": result.StreakDays})
		}
		b.respond(s, i, msg)
		return
	}

	if result.AllCollected {
		b.respond(s, i, b.loc.Translate(lang, "roll_all_collected", i18n.Args{"reward": result.Reward, "days": result.StreakDays}))
		return
	}

	title := b.loc.Translate(lang, "roll_success_title")
	desc := b.loc.Translate(lang, "roll_success_desc", i18n.Args{"name": result.Card.Name, "rarity": b.loc.Rarity(lang, result.RarityName), "power": result.Card.PowerLevel, "reward": result.Reward})

	if result.IsFragment {
		if result.CardAssembled {
			desc = b.loc.Translate(lang, "roll_mythic_assembled", i18n.Args{"name": result.Card.Name, "power": result.Card.PowerLevel, "reward": result.Reward})
		} else {
			desc = b.loc.Translate(lang, "roll_mythic_fragment", i18n.Args{"name": result.Card.Name, "fragments": result.FragmentsCount, "reward": result.Reward})
		}
	}

	embed := &discordgo.MessageEmbed{
		Title:       title,
		Description: desc,
		Image:       &discordgo.MessageEmbedImage{URL: result.Card.ImageURL},
		Color:       0x00ff00,
	}

	// "Open app" launches the embedded Discord Activity (handled via a
	// LAUNCH_ACTIVITY interaction response on click — see HandleComponentInteraction).
	components := []discordgo.MessageComponent{
		discordgo.ActionsRow{Components: []discordgo.MessageComponent{
			discordgo.Button{
				CustomID: "roll_again",
				Label:    b.loc.Translate(lang, "btn_roll_again"),
				Style:    discordgo.PrimaryButton,
			},
			discordgo.Button{
				CustomID: "launch_app",
				Label:    b.loc.Translate(lang, "btn_open_app"),
				Style:    discordgo.SuccessButton,
			},
		}},
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
		streakMsg := b.loc.Translate(lang, "streak_continued", i18n.Args{"reward": result.Reward, "days": result.StreakDays})
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
		b.respond(s, i, "❌ "+b.loc.Translate(lang, "err_craft_failed", i18n.Args{"reason": translatedReason}))
		return
	}

	var desc string
	if result.IsFragment {
		if !result.CardAssembled {
			desc = b.loc.Translate(lang, "craft_mythic_frag", i18n.Args{"cost": result.CraftCost, "name": result.Card.Name, "fragments": result.FragmentsCount})
		} else {
			desc = b.loc.Translate(lang, "craft_mythic_assembled", i18n.Args{"cost": result.CraftCost, "name": result.Card.Name, "power": result.Card.PowerLevel})
		}
	} else {
		desc = b.loc.Translate(lang, "craft_success", i18n.Args{"cost": result.CraftCost, "name": result.Card.Name, "rarity": b.loc.Rarity(lang, result.RarityName), "power": result.Card.PowerLevel})
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
		sb.WriteString(b.loc.Translate(lang, "promo_reward_points", i18n.Args{"points": reward.Points}) + "\n")
	}
	if reward.PremiumRolls > 0 {
		sb.WriteString(b.loc.Translate(lang, "promo_reward_rolls", i18n.Args{"rolls": reward.PremiumRolls}) + "\n")
	}
	if len(cards) > 0 {
		sb.WriteString("\n" + b.loc.Translate(lang, "promo_reward_cards_count", i18n.Args{"count": len(cards)}) + "\n")
		for _, c := range cards {
			sb.WriteString(b.loc.Translate(lang, "promo_reward_card", i18n.Args{"name": c.Name, "power": c.PowerLevel}) + "\n")
		}
	}

	err = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: sb.String(),
		},
	})
	if err != nil {
		log.Printf("[DISCORD] Failed to respond to interaction: %v", err)
		return
	}

	if len(cards) > 0 {
		imgLimit := len(cards)
		if imgLimit > 10 {
			imgLimit = 10
		}

		var files []*discordgo.File
		httpClient := &http.Client{Timeout: 5 * time.Second}

		for j := 0; j < imgLimit; j++ {
			resp, err := httpClient.Get(cards[j].ImageURL)
			if err != nil {
				log.Printf("[DISCORD] Failed to download image %s: %v", cards[j].ImageURL, err)
				continue
			}

			imgData, err := io.ReadAll(resp.Body)
			resp.Body.Close()
			if err != nil {
				log.Printf("[DISCORD] Failed to read image data: %v", err)
				continue
			}

			fileName := "card.png"
			if strings.HasSuffix(cards[j].ImageURL, ".jpg") || strings.HasSuffix(cards[j].ImageURL, ".jpeg") {
				fileName = "card.jpg"
			} else if strings.HasSuffix(cards[j].ImageURL, ".webp") {
				fileName = "card.webp"
			}

			files = append(files, &discordgo.File{
				Name:        fileName,
				ContentType: resp.Header.Get("Content-Type"),
				Reader:      bytes.NewReader(imgData),
			})
		}

		if len(files) > 0 {
			_, err = s.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{
				Files: files,
			})
			if err != nil {
				log.Printf("[DISCORD] Failed to send images as attachments: %v", err)
			}
		}
	}
}
