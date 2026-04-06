package discord

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"

	"github.com/bwmarrin/discordgo"
)

// HandleInteraction - Главный входной узел для всех слэш-команд
func (b *Bot) HandleInteraction(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if i.Type != discordgo.InteractionApplicationCommand {
		return
	}

	data := i.ApplicationCommandData()
	dsUser := i.Member.User
	if dsUser == nil {
		dsUser = i.User
	}

	dbUser, err := b.repo.GetOrCreateUserByDiscordID(parseID(dsUser.ID), dsUser.Username)
	if err != nil {
		log.Printf("[DISCORD DB ERROR]: %v", err)
		return
	}

	lang := dbUser.LanguageCode
	if lang == "" {
		lang = "ru"
	}

	if i.GuildID != "" {
		guildID := parseID(i.GuildID)
		_ = b.repo.TrackUserChat(dbUser.ID, guildID)
	}

	switch data.Name {
	case "roll":
		b.handleRoll(s, i, dbUser, lang)
	case "profile":
		b.handleProfile(s, i, dbUser, lang)
	case "link":
		b.handleLink(s, i, dbUser, lang)
	case "help":
		b.handleHelp(s, i, lang)
	case "top":
		b.handleTop(s, i, lang, "balance", false)
	case "globaltop":
		b.handleTop(s, i, lang, "balance", true)
	case "craft":
		b.handleCraft(s, i, dbUser, lang)
	case "duel":
		b.handleDuel(s, i, dbUser, lang)
	case "locale":
		b.handleLocale(s, i, dbUser, lang)
	case "promo":
		b.handlePromo(s, i, dbUser, lang)
	}
}

// HandleComponentInteraction - узел для кнопок
func (b *Bot) HandleComponentInteraction(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if i.Type != discordgo.InteractionMessageComponent {
		return
	}

	data := i.MessageComponentData()
	dsUser := i.Member.User
	if dsUser == nil {
		dsUser = i.User
	}

	dbUser, _ := b.repo.GetOrCreateUserByDiscordID(parseID(dsUser.ID), dsUser.Username)
	lang := dbUser.LanguageCode
	if lang == "" {
		lang = "ru"
	}

	// 1. НАВИГАЦИЯ ПО КАРТОЧКАМ
	if strings.HasPrefix(data.CustomID, "cards_nav:") {
		offset, _ := strconv.Atoi(strings.TrimPrefix(data.CustomID, "cards_nav:"))
		b.handleCardsNav(s, i, dbUser, lang, offset)
		return
	}

	// 2. ВОЗВРАТ В ПРОФИЛЬ
	if data.CustomID == "back_to_profile" {
		embed, buttons := b.getProfileData(dbUser, lang)

		// --- ДОБАВЛЯЕМ АВАТАРКУ ВОЗВРАТНОМУ ЭМБЕДУ ---
		if i.Member != nil && i.Member.User != nil && i.Member.User.Avatar != "" {
			embed.Thumbnail = &discordgo.MessageEmbedThumbnail{URL: i.Member.User.AvatarURL("128")}
		} else if i.User != nil && i.User.Avatar != "" {
			embed.Thumbnail = &discordgo.MessageEmbedThumbnail{URL: i.User.AvatarURL("128")}
		}
		// ----------------------------------------------

		b.updateWithEmbedAndComponents(s, i, "", embed, buttons)
		return
	}

	// 3. ПРЕДЛОЖКА: СТАРТ
	if data.CustomID == "suggest_start" {
		profile, _ := b.service.GetUserProfile(dbUser.ID)
		if profile.Balance < 1000 {
			b.respondEphemeral(s, i, b.loc.T(lang, "suggest_err_funds"))
			return
		}
		msg := b.loc.T(lang, "suggest_rules") + "\n\n" + b.loc.T(lang, "suggest_q1")
		buttons := []discordgo.MessageComponent{discordgo.ActionsRow{Components: []discordgo.MessageComponent{
			discordgo.Button{Label: b.loc.T(lang, "btn_yes"), Style: discordgo.SuccessButton, CustomID: "s_q1_yes"},
			discordgo.Button{Label: b.loc.T(lang, "btn_no"), Style: discordgo.DangerButton, CustomID: "s_q1_no"},
		}}}
		b.updateWithComponents(s, i, msg, buttons)
		return
	}

	// 4. ПРЕДЛОЖКА: КВИЗ
	switch data.CustomID {
	case "s_q1_yes", "s_q2_yes", "s_q3_43", "s_q3_11":
		msg := b.loc.T(lang, "suggest_rules") + "\n\n" + b.loc.T(lang, "suggest_fail")
		buttons := []discordgo.MessageComponent{discordgo.ActionsRow{Components: []discordgo.MessageComponent{
			discordgo.Button{Label: b.loc.T(lang, "btn_try_again"), Style: discordgo.PrimaryButton, CustomID: "suggest_start"},
		}}}
		b.updateWithComponents(s, i, msg, buttons)
		return
	case "s_q1_no":
		msg := b.loc.T(lang, "suggest_rules") + "\n\n" + b.loc.T(lang, "suggest_q2")
		buttons := []discordgo.MessageComponent{discordgo.ActionsRow{Components: []discordgo.MessageComponent{
			discordgo.Button{Label: b.loc.T(lang, "btn_yes"), Style: discordgo.SuccessButton, CustomID: "s_q2_yes"},
			discordgo.Button{Label: b.loc.T(lang, "btn_no"), Style: discordgo.DangerButton, CustomID: "s_q2_no"},
		}}}
		b.updateWithComponents(s, i, msg, buttons)
		return
	case "s_q2_no":
		msg := b.loc.T(lang, "suggest_rules") + "\n\n" + b.loc.T(lang, "suggest_q3")
		buttons := []discordgo.MessageComponent{discordgo.ActionsRow{Components: []discordgo.MessageComponent{
			discordgo.Button{Label: "4:3", Style: discordgo.SecondaryButton, CustomID: "s_q3_43"},
			discordgo.Button{Label: "3:4", Style: discordgo.SecondaryButton, CustomID: "s_q3_34"},
			discordgo.Button{Label: "1:1", Style: discordgo.SecondaryButton, CustomID: "s_q3_11"},
		}}}
		b.updateWithComponents(s, i, msg, buttons)
		return
	case "s_q3_34":
		b.setSuggestState(dbUser.ID, true) // Включаем предложку через Redis
		b.updateWithComponents(s, i, b.loc.T(lang, "suggest_success"), []discordgo.MessageComponent{
			discordgo.ActionsRow{Components: []discordgo.MessageComponent{
				discordgo.Button{Label: b.loc.T(lang, "btn_cancel"), Style: discordgo.DangerButton, CustomID: "s_cancel"},
			}},
		})
		return
	case "s_cancel":
		b.setSuggestState(dbUser.ID, false) // Выключаем предложку через Redis
		b.updateWithComponents(s, i, b.loc.T(lang, "suggest_cancelled"), []discordgo.MessageComponent{})
		return
	}

	// 5. ДУЭЛИ
	if strings.HasPrefix(data.CustomID, "duel_") {
		parts := strings.Split(data.CustomID, ":")
		action := parts[0]
		duelID := strings.Join(parts[1:], ":")

		duel, exists := b.duelService.GetDuel(duelID)
		if !exists {
			b.respond(s, i, b.loc.T(lang, "err_duel_expired"))
			return
		}

		if action == "duel_cancel" {
			if dbUser.ID != duel.ChallengerID && dbUser.ID != duel.TargetID {
				b.respondEphemeral(s, i, b.loc.T(lang, "err_duel_not_yours"))
				return
			}
			b.duelService.PopDuel(duelID)
			b.updateWithComponents(s, i, b.loc.T(lang, "duel_cancelled", duel.ChallengerName, duel.TargetName), []discordgo.MessageComponent{})
			return
		}

		if action == "duel_accept" {
			if dbUser.ID != duel.TargetID {
				b.respondEphemeral(s, i, b.loc.T(lang, "err_duel_not_called"))
				return
			}

			b.duelService.PopDuel(duelID)
			res, err := b.duelService.ExecuteDuel(duel)
			if err != nil {
				b.updateWithComponents(s, i, b.loc.T(lang, "error_tech")+" "+err.Error(), []discordgo.MessageComponent{})
				return
			}

			mainEmbed := &discordgo.MessageEmbed{
				Title:       b.loc.T(lang, "duel_ds_title"),
				Description: b.loc.T(lang, "duel_ds_desc", res.Roll, res.WinnerName, res.AmountWon*2),
				Color:       0xe67e22,
			}

			challengerEmbed := &discordgo.MessageEmbed{
				Author:      &discordgo.MessageEmbedAuthor{Name: b.loc.T(lang, "duel_ds_attacker", duel.ChallengerName)},
				Title:       res.CardChallenger.Name,
				Description: b.loc.T(lang, "duel_ds_stats", res.CardChallenger.PowerLevel, res.ChanceChallenger),
				Thumbnail:   &discordgo.MessageEmbedThumbnail{URL: res.CardChallenger.ImageURL},
				Color:       0x3498db,
			}

			targetEmbed := &discordgo.MessageEmbed{
				Author:      &discordgo.MessageEmbedAuthor{Name: b.loc.T(lang, "duel_ds_defender", duel.TargetName)},
				Title:       res.CardTarget.Name,
				Description: b.loc.T(lang, "duel_ds_stats", res.CardTarget.PowerLevel, res.ChanceTarget),
				Thumbnail:   &discordgo.MessageEmbedThumbnail{URL: res.CardTarget.ImageURL},
				Color:       0xe74c3c,
			}

			_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseUpdateMessage,
				Data: &discordgo.InteractionResponseData{
					Embeds:     []*discordgo.MessageEmbed{mainEmbed, challengerEmbed, targetEmbed},
					Components: []discordgo.MessageComponent{},
				},
			})
			return
		}
	}

	// 6. ТОПЫ
	if strings.HasPrefix(data.CustomID, "top:") {
		parts := strings.Split(data.CustomID, ":")
		if len(parts) != 3 {
			return
		}
		criteria, global := parts[1], parts[2] == "global"
		b.handleTop(s, i, lang, criteria, global)
		return
	}

	// 7. ПОМОЩЬ (МЕНЮ)
	if data.CustomID == "help_select" {
		category := data.Values[0]
		responseKey := "help_" + category
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseUpdateMessage,
			Data: &discordgo.InteractionResponseData{
				Content:    b.loc.T(lang, responseKey),
				Components: b.getHelpMenu(lang),
			},
		})
	}

	// 8. ПРИВЯЗКА АККАУНТОВ (ВЫБОР И ПОДТВЕРЖДЕНИЕ)
	if strings.HasPrefix(data.CustomID, "link:") {
		parts := strings.Split(data.CustomID, ":")
		action := parts[1] // "keep", "confirm" или "cancel"
		target := ""
		if len(parts) > 2 {
			target = parts[2] // "tg" или "ds"
		}

		// ИЩЕМ СЕССИЮ В REDIS
		rCtx := context.Background()
		key := fmt.Sprintf("pending_link:%d", dbUser.ID)

		tgInternalIDStr, err := b.rdb.Get(rCtx, key).Result()
		exists := err == nil
		tgInternalID, _ := strconv.ParseInt(tgInternalIDStr, 10, 64)

		if !exists && action != "cancel" {
			b.respondEphemeral(s, i, b.loc.T(lang, "link_err_expired"))
			return
		}

		switch action {
		case "cancel":
			b.rdb.Del(rCtx, key) // Удаляем сессию
			b.updateWithComponents(s, i, b.loc.T(lang, "link_cancelled"), nil)

		case "keep":
			// Выдаем финальное предупреждение
			var msg string
			if target == "tg" {
				msg = b.loc.T(lang, "link_warn_tg")
			} else {
				msg = b.loc.T(lang, "link_warn_ds")
			}

			buttons := []discordgo.MessageComponent{discordgo.ActionsRow{
				Components: []discordgo.MessageComponent{
					discordgo.Button{Label: b.loc.T(lang, "btn_yes_im_sure"), Style: discordgo.DangerButton, CustomID: "link:confirm:" + target},
					discordgo.Button{Label: b.loc.T(lang, "btn_cancel"), Style: discordgo.SecondaryButton, CustomID: "link:cancel"},
				},
			}}
			b.updateWithComponents(s, i, msg, buttons)

		case "confirm":
			// Выполняем объединение
			var keepID, deleteID int64
			if target == "tg" {
				keepID = tgInternalID
				deleteID = dbUser.ID
			} else {
				keepID = dbUser.ID
				deleteID = tgInternalID
			}

			err := b.repo.LinkAccountsOverwrite(keepID, deleteID)
			if err != nil {
				log.Printf("[DISCORD LINK ERROR]: %v", err)
				b.updateWithComponents(s, i, b.loc.T(lang, "error_db"), nil)
				return
			}

			// Очищаем сессию в Redis
			b.rdb.Del(rCtx, key)

			b.updateWithComponents(s, i, b.loc.T(lang, "link_success"), nil)
		}
		return
	}

	// 9. КОЛЛЕКЦИИ (НАВИГАЦИЯ)
	if strings.HasPrefix(data.CustomID, "sets_nav:") {
		parts := strings.Split(data.CustomID, ":")
		page, _ := strconv.Atoi(parts[1])
		b.handleSetsList(s, i, dbUser, lang, page)
		return
	}

	// 10. КОЛЛЕКЦИИ (ПРОСМОТР СЕТА ЧЕРЕЗ ВЫПАДАЮЩИЙ СПИСОК)
	if data.CustomID == "set_view_select" {
		b.handleSetView(s, i, dbUser, lang)
		return
	}

	// 11. КОЛЛЕКЦИИ (ЭКИПИРОВКА АУРЫ)
	if strings.HasPrefix(data.CustomID, "set_equip:") {
		b.handleEquipAura(s, i, dbUser, lang)
		return
	}
}

// HandleMessageCreate - узел для обычных сообщений (предложка)
func (b *Bot) HandleMessageCreate(s *discordgo.Session, m *discordgo.MessageCreate) {
	if m.Author.Bot {
		return
	}

	dbUser, _ := b.repo.GetOrCreateUserByDiscordID(parseID(m.Author.ID), m.Author.Username)
	lang := dbUser.LanguageCode
	if lang == "" {
		lang = "ru"
	}

	// ИСПОЛЬЗУЕМ REDIS:
	isSuggesting := b.isSuggesting(dbUser.ID)

	if isSuggesting {
		if len(m.Attachments) == 0 {
			s.ChannelMessageSend(m.ChannelID, b.loc.T(lang, "suggest_err_no_photo"))
			return
		}

		if strings.TrimSpace(m.Content) == "" {
			s.ChannelMessageSend(m.ChannelID, b.loc.T(lang, "suggest_err_no_caption"))
			return
		}

		profile, _ := b.service.GetUserProfile(dbUser.ID)
		if profile.Balance < 1000 {
			b.setSuggestState(dbUser.ID, false) // Сброс состояния в Redis
			s.ChannelMessageSend(m.ChannelID, b.loc.T(lang, "suggest_err_funds"))
			return
		}

		dbUser.Balance -= 1000
		_ = b.repo.UpdateUserAfterRoll(dbUser)

		adminMsg := fmt.Sprintf("📩 <b>Новая предложка (Discord)!</b>\nОт: %s (DB_ID: %d)\n\nОписание:\n<i>%s</i>",
			m.Author.Username, dbUser.ID, m.Content)

		if b.NotifyAdmin != nil {
			b.NotifyAdmin(adminMsg, m.Attachments[0].URL)
		}

		b.setSuggestState(dbUser.ID, false) // Успешный сброс состояния в Redis

		s.ChannelMessageSend(m.ChannelID, b.loc.T(lang, "suggest_done"))
	}
}
