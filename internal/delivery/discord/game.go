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
		b.respond(s, i, b.loc.T(lang, "error_tech"))
		return
	}

	if result.OnCooldown {
		msg := b.loc.T(lang, "roll_cooldown", result.CooldownTimeLeft)
		if result.StreakUpdated {
			msg += "\n\n" + b.loc.T(lang, "streak_kept_alive", result.StreakDays)
		}
		b.respond(s, i, msg)
		return
	}

	title := b.loc.T(lang, "roll_success_title")
	desc := b.loc.T(lang, "roll_success_desc", result.Card.Name, result.RarityName, result.Card.PowerLevel, result.Reward)

	if result.IsFragment {
		if result.CardAssembled {
			desc = b.loc.T(lang, "roll_mythic_assembled", result.Card.Name, result.Card.PowerLevel, result.Reward)
		} else {
			desc = b.loc.T(lang, "roll_mythic_fragment", result.Card.Name, result.FragmentsCount, result.Reward)
		}
	}

	embed := &discordgo.MessageEmbed{
		Title:       title,
		Description: desc,
		Image:       &discordgo.MessageEmbedImage{URL: result.Card.ImageURL},
		Color:       0x00ff00,
	}

	// --- ДОБАВЛЯЕМ КНОПКУ "КРУТИТЬ ЕЩЕ" ---
	components := []discordgo.MessageComponent{
		discordgo.ActionsRow{
			Components: []discordgo.MessageComponent{
				discordgo.Button{
					CustomID: "roll_again", // Тот же ID, что и в Telegram
					Label:    b.loc.T(lang, "btn_roll_again"),
					Style:    discordgo.PrimaryButton, // Синяя кнопка (можно discordgo.SuccessButton для зеленой)
				},
			},
		},
	}

	err = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Embeds:     []*discordgo.MessageEmbed{embed},
			Components: components, // Прикрепляем кнопку к сообщению
		},
	})

	if err != nil {
		// Рекомендую всегда логировать ошибки Дискорда
		log.Printf("[DISCORD ERROR] Не удалось отправить карту: %v", err)
	}

	if result.StreakUpdated {
		streakMsg := b.loc.T(lang, "streak_continued", result.Reward, result.StreakDays)
		if result.StreakDays == 1 {
			streakMsg = b.loc.T(lang, "streak_started")
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
		b.respond(s, i, "❌ "+b.loc.T(lang, "err_craft_failed", err.Error()))
		return
	}

	var desc string
	if result.IsFragment {
		if !result.CardAssembled {
			desc = b.loc.T(lang, "craft_mythic_frag", result.CraftCost, result.Card.Name, result.FragmentsCount)
		} else {
			desc = b.loc.T(lang, "craft_mythic_assembled", result.CraftCost, result.Card.Name, result.Card.PowerLevel)
		}
	} else {
		desc = b.loc.T(lang, "craft_success", result.CraftCost, result.Card.Name, result.RarityName, result.Card.PowerLevel)
	}

	embed := &discordgo.MessageEmbed{
		Title:       "🛠 " + b.loc.T(lang, "craft_title"),
		Description: desc,
		Image:       &discordgo.MessageEmbedImage{URL: result.Card.ImageURL},
		Color:       0x9b59b6,
	}

	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{Embeds: []*discordgo.MessageEmbed{embed}},
	})
}

// Активация кода игроками (Discord)
func (b *Bot) handlePromo(s *discordgo.Session, i *discordgo.InteractionCreate, dbUser *models.User, lang string) {
	// Никакого getLang здесь нет, lang уже пришел в аргументах функции!
	options := i.ApplicationCommandData().Options
	if len(options) == 0 {
		b.respondEphemeral(s, i, "❌ Укажите код.")
		return
	}

	code := options[0].StringValue()

	// ПРАВИЛЬНЫЙ ВЫЗОВ: 2 аргумента на вход, 3 на выход
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
		b.respondEphemeral(s, i, b.loc.T(lang, errKey))
		return
	}

	// Собираем текст
	var sb strings.Builder
	if reward.Points > 0 {
		sb.WriteString(b.loc.T(lang, "promo_reward_points", reward.Points) + "\n")
	}
	if reward.PremiumRolls > 0 {
		sb.WriteString(b.loc.T(lang, "promo_reward_rolls", reward.PremiumRolls) + "\n")
	}
	if len(cards) > 0 {
		sb.WriteString("\n" + b.loc.T(lang, "promo_reward_cards_count", len(cards)) + "\n")
		for _, c := range cards {
			sb.WriteString(b.loc.T(lang, "promo_reward_card", c.Name, c.PowerLevel) + "\n")
		}
	}

	// Главный эмбед с текстом награды
	embeds := []*discordgo.MessageEmbed{
		{
			Title:       b.loc.T(lang, "promo_success_title"),
			Description: sb.String(),
			Color:       0x2ecc71, // Зеленый цвет
		},
	}

	// Лимит Дискорда: 10 эмбедов на сообщение. 1 уже занят текстом, остается 9 под картинки.
	imgLimit := len(cards)
	if imgLimit > 9 {
		imgLimit = 9
	}

	for j := 0; j < imgLimit; j++ {
		embeds = append(embeds, &discordgo.MessageEmbed{
			Title: "🃏 " + cards[j].Name,
			Image: &discordgo.MessageEmbedImage{URL: cards[j].ImageURL},
			Color: 0x3498db,
		})
	}

	_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Embeds: embeds,
		},
	})
}
