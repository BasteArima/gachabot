package discord

import (
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"

	"gachabot/internal/i18n"
	"gachabot/internal/models"
	"gachabot/internal/repository"
	"gachabot/internal/service"

	"github.com/bwmarrin/discordgo"
)

// LinkProvider описывает интерфейс для связи с Telegram-хэндлером
type LinkProvider interface {
	GetIDByCode(code string) (int64, bool)
}

type Handler struct {
	repo        *repository.PostgresRepo
	service     *service.GachaService
	duelService *service.DuelService
	loc         *i18n.Localizer
	lp          LinkProvider

	mu           sync.RWMutex
	suggestState map[int64]bool

	// НОВОЕ ПОЛЕ: Функция для отправки в админ-чат
	NotifyAdmin func(text string, imageURL string)

	linkMu       sync.Mutex
	pendingLinks map[int64]int64 // Сохраняем сессии: [Discord ID] -> [Telegram ID]
}

func NewHandler(repo *repository.PostgresRepo, service *service.GachaService, duelService *service.DuelService, loc *i18n.Localizer, lp LinkProvider, notifyAdmin func(string, string)) *Handler {
	return &Handler{
		repo:         repo,
		service:      service,
		duelService:  duelService,
		loc:          loc,
		lp:           lp,
		suggestState: make(map[int64]bool),
		NotifyAdmin:  notifyAdmin, // Сохраняем колбэк
		pendingLinks: make(map[int64]int64),
	}
}

// Главный входной узел для всех взаимодействий в Discord (Слэш-команды)
func (h *Handler) HandleInteraction(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if i.Type != discordgo.InteractionApplicationCommand {
		return
	}

	data := i.ApplicationCommandData()
	dsUser := i.Member.User
	if dsUser == nil {
		dsUser = i.User
	}

	dbUser, err := h.repo.GetOrCreateUserByDiscordID(parseID(dsUser.ID), dsUser.Username)
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
		_ = h.repo.TrackUserChat(dbUser.ID, guildID)
	}

	switch data.Name {
	case "roll":
		h.handleRoll(s, i, dbUser, lang)
	case "profile":
		h.handleProfile(s, i, dbUser, lang)
	case "link":
		h.handleLink(s, i, dbUser, lang)
	case "help":
		h.handleHelp(s, i, lang)
	case "top":
		h.handleTop(s, i, lang, "balance", false)
	case "globaltop":
		h.handleTop(s, i, lang, "balance", true)
	case "craft":
		h.handleCraft(s, i, dbUser, lang)
	case "duel":
		h.handleDuel(s, i, dbUser, lang)
	case "locale":
		h.handleLocale(s, i, dbUser, lang)
	case "promo":
		h.handlePromo(s, i, dbUser, lang)
	}
}

// Обработчик обычных сообщений (для ловли фото предложки)
// Обработчик обычных сообщений (для ловли фото предложки)
func (h *Handler) HandleMessageCreate(s *discordgo.Session, m *discordgo.MessageCreate) {
	if m.Author.Bot {
		return
	}

	dbUser, _ := h.repo.GetOrCreateUserByDiscordID(parseID(m.Author.ID), m.Author.Username)
	lang := dbUser.LanguageCode
	if lang == "" {
		lang = "ru"
	}

	h.mu.RLock()
	isSuggesting := h.suggestState[dbUser.ID]
	h.mu.RUnlock()

	if isSuggesting {
		// 1. Проверяем, прикрепил ли юзер картинку
		if len(m.Attachments) == 0 {
			s.ChannelMessageSend(m.ChannelID, h.loc.T(lang, "suggest_err_no_photo"))
			return
		}

		// 2. Проверяем, написал ли юзер текст (описание)
		if strings.TrimSpace(m.Content) == "" {
			s.ChannelMessageSend(m.ChannelID, h.loc.T(lang, "suggest_err_no_caption"))
			return
		}

		// 3. Проверяем баланс
		profile, _ := h.service.GetUserProfile(dbUser.ID)
		if profile.Balance < 1000 {
			h.mu.Lock()
			delete(h.suggestState, dbUser.ID)
			h.mu.Unlock()
			s.ChannelMessageSend(m.ChannelID, h.loc.T(lang, "suggest_err_funds"))
			return
		}

		// 4. Списываем очки
		dbUser.Balance -= 1000
		_ = h.repo.UpdateUserAfterRoll(dbUser)

		// 5. Отправляем в Telegram через колбэк (как мы сделали в main.go)
		adminMsg := fmt.Sprintf("📩 <b>Новая предложка (Discord)!</b>\nОт: %s (DB_ID: %d)\n\nОписание:\n<i>%s</i>",
			m.Author.Username, dbUser.ID, m.Content)

		if h.NotifyAdmin != nil {
			h.NotifyAdmin(adminMsg, m.Attachments[0].URL)
		}

		// 6. Сбрасываем состояние
		h.mu.Lock()
		delete(h.suggestState, dbUser.ID)
		h.mu.Unlock()

		s.ChannelMessageSend(m.ChannelID, h.loc.T(lang, "suggest_done"))
	}
}

func (h *Handler) handleHelp(s *discordgo.Session, i *discordgo.InteractionCreate, lang string) {
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content:    h.loc.T(lang, "help_main"),
			Components: h.getHelpMenu(lang),
		},
	})
}

func (h *Handler) getHelpMenu(lang string) []discordgo.MessageComponent {
	return []discordgo.MessageComponent{
		discordgo.ActionsRow{
			Components: []discordgo.MessageComponent{
				discordgo.SelectMenu{
					CustomID:    "help_select",
					Placeholder: h.loc.T(lang, "btn_help_select"),
					Options: []discordgo.SelectMenuOption{
						{Label: h.loc.T(lang, "btn_help_main"), Value: "main", Emoji: &discordgo.ComponentEmoji{Name: "🏠"}},
						{Label: h.loc.T(lang, "btn_help_cards"), Value: "cards", Emoji: &discordgo.ComponentEmoji{Name: "🃏"}},
						{Label: h.loc.T(lang, "btn_help_rarities"), Value: "rarities", Emoji: &discordgo.ComponentEmoji{Name: "💎"}},
						{Label: h.loc.T(lang, "btn_help_streaks"), Value: "streaks", Emoji: &discordgo.ComponentEmoji{Name: "🔥"}},
						{Label: h.loc.T(lang, "btn_help_pity"), Value: "pity", Emoji: &discordgo.ComponentEmoji{Name: "🛡"}},
						{Label: h.loc.T(lang, "btn_help_duel"), Value: "duel", Emoji: &discordgo.ComponentEmoji{Name: "⚔️"}},
						{Label: h.loc.T(lang, "btn_help_craft"), Value: "craft", Emoji: &discordgo.ComponentEmoji{Name: "🛠"}},
					},
				},
			},
		},
	}
}

func (h *Handler) HandleComponentInteraction(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if i.Type != discordgo.InteractionMessageComponent {
		return
	}

	data := i.MessageComponentData()
	dsUser := i.Member.User
	if dsUser == nil {
		dsUser = i.User
	}

	dbUser, _ := h.repo.GetOrCreateUserByDiscordID(parseID(dsUser.ID), dsUser.Username)
	lang := dbUser.LanguageCode
	if lang == "" {
		lang = "ru"
	}

	// 1. НАВИГАЦИЯ ПО КАРТОЧКАМ
	if strings.HasPrefix(data.CustomID, "cards_nav:") {
		offset, _ := strconv.Atoi(strings.TrimPrefix(data.CustomID, "cards_nav:"))
		h.handleCardsNav(s, i, dbUser, lang, offset)
		return
	}

	// 2. ВОЗВРАТ В ПРОФИЛЬ
	if data.CustomID == "back_to_profile" {
		embed, buttons := h.getProfileData(dbUser, lang)

		// --- ДОБАВЛЯЕМ АВАТАРКУ ВОЗВРАТНОМУ ЭМБЕДУ ---
		if i.Member != nil && i.Member.User != nil && i.Member.User.Avatar != "" {
			embed.Thumbnail = &discordgo.MessageEmbedThumbnail{URL: i.Member.User.AvatarURL("128")}
		} else if i.User != nil && i.User.Avatar != "" {
			embed.Thumbnail = &discordgo.MessageEmbedThumbnail{URL: i.User.AvatarURL("128")}
		}
		// ----------------------------------------------

		h.updateWithEmbedAndComponents(s, i, "", embed, buttons)
		return
	}

	// 3. ПРЕДЛОЖКА: СТАРТ
	if data.CustomID == "suggest_start" {
		profile, _ := h.service.GetUserProfile(dbUser.ID)
		if profile.Balance < 1000 {
			h.respondEphemeral(s, i, h.loc.T(lang, "suggest_err_funds"))
			return
		}
		msg := h.loc.T(lang, "suggest_rules") + "\n\n" + h.loc.T(lang, "suggest_q1")
		buttons := []discordgo.MessageComponent{discordgo.ActionsRow{Components: []discordgo.MessageComponent{
			discordgo.Button{Label: h.loc.T(lang, "btn_yes"), Style: discordgo.SuccessButton, CustomID: "s_q1_yes"},
			discordgo.Button{Label: h.loc.T(lang, "btn_no"), Style: discordgo.DangerButton, CustomID: "s_q1_no"},
		}}}
		h.updateWithComponents(s, i, msg, buttons)
		return
	}

	// 4. ПРЕДЛОЖКА: КВИЗ
	switch data.CustomID {
	case "s_q1_yes", "s_q2_yes", "s_q3_43", "s_q3_11":
		msg := h.loc.T(lang, "suggest_rules") + "\n\n" + h.loc.T(lang, "suggest_fail")
		buttons := []discordgo.MessageComponent{discordgo.ActionsRow{Components: []discordgo.MessageComponent{
			discordgo.Button{Label: h.loc.T(lang, "btn_try_again"), Style: discordgo.PrimaryButton, CustomID: "suggest_start"},
		}}}
		h.updateWithComponents(s, i, msg, buttons)
		return
	case "s_q1_no":
		msg := h.loc.T(lang, "suggest_rules") + "\n\n" + h.loc.T(lang, "suggest_q2")
		buttons := []discordgo.MessageComponent{discordgo.ActionsRow{Components: []discordgo.MessageComponent{
			discordgo.Button{Label: h.loc.T(lang, "btn_yes"), Style: discordgo.SuccessButton, CustomID: "s_q2_yes"},
			discordgo.Button{Label: h.loc.T(lang, "btn_no"), Style: discordgo.DangerButton, CustomID: "s_q2_no"},
		}}}
		h.updateWithComponents(s, i, msg, buttons)
		return
	case "s_q2_no":
		msg := h.loc.T(lang, "suggest_rules") + "\n\n" + h.loc.T(lang, "suggest_q3")
		buttons := []discordgo.MessageComponent{discordgo.ActionsRow{Components: []discordgo.MessageComponent{
			discordgo.Button{Label: "4:3", Style: discordgo.SecondaryButton, CustomID: "s_q3_43"},
			discordgo.Button{Label: "3:4", Style: discordgo.SecondaryButton, CustomID: "s_q3_34"},
			discordgo.Button{Label: "1:1", Style: discordgo.SecondaryButton, CustomID: "s_q3_11"},
		}}}
		h.updateWithComponents(s, i, msg, buttons)
		return
	case "s_q3_34":
		h.mu.Lock()
		h.suggestState[dbUser.ID] = true
		h.mu.Unlock()
		h.updateWithComponents(s, i, h.loc.T(lang, "suggest_success"), []discordgo.MessageComponent{
			discordgo.ActionsRow{Components: []discordgo.MessageComponent{
				discordgo.Button{Label: h.loc.T(lang, "btn_cancel"), Style: discordgo.DangerButton, CustomID: "s_cancel"},
			}},
		})
		return
	case "s_cancel":
		h.mu.Lock()
		delete(h.suggestState, dbUser.ID)
		h.mu.Unlock()
		h.updateWithComponents(s, i, h.loc.T(lang, "suggest_cancelled"), []discordgo.MessageComponent{})
		return
	}

	// 5. ДУЭЛИ
	if strings.HasPrefix(data.CustomID, "duel_") {
		parts := strings.Split(data.CustomID, ":")
		action := parts[0]
		duelID := strings.Join(parts[1:], ":")

		duel, exists := h.duelService.GetDuel(duelID)
		if !exists {
			h.respond(s, i, h.loc.T(lang, "err_duel_expired"))
			return
		}

		if action == "duel_cancel" {
			if dbUser.ID != duel.ChallengerID && dbUser.ID != duel.TargetID {
				h.respondEphemeral(s, i, h.loc.T(lang, "err_duel_not_yours"))
				return
			}
			h.duelService.PopDuel(duelID)
			h.updateWithComponents(s, i, h.loc.T(lang, "duel_cancelled", duel.ChallengerName, duel.TargetName), []discordgo.MessageComponent{})
			return
		}

		if action == "duel_accept" {
			if dbUser.ID != duel.TargetID {
				h.respondEphemeral(s, i, h.loc.T(lang, "err_duel_not_called"))
				return
			}

			h.duelService.PopDuel(duelID)
			res, err := h.duelService.ExecuteDuel(duel)
			if err != nil {
				h.updateWithComponents(s, i, h.loc.T(lang, "error_tech")+" "+err.Error(), []discordgo.MessageComponent{})
				return
			}

			mainEmbed := &discordgo.MessageEmbed{
				Title:       h.loc.T(lang, "duel_ds_title"),
				Description: h.loc.T(lang, "duel_ds_desc", res.Roll, res.WinnerName, res.AmountWon*2),
				Color:       0xe67e22,
			}

			challengerEmbed := &discordgo.MessageEmbed{
				Author:      &discordgo.MessageEmbedAuthor{Name: h.loc.T(lang, "duel_ds_attacker", duel.ChallengerName)},
				Title:       res.CardChallenger.Name,
				Description: h.loc.T(lang, "duel_ds_stats", res.CardChallenger.PowerLevel, res.ChanceChallenger),
				Thumbnail:   &discordgo.MessageEmbedThumbnail{URL: res.CardChallenger.ImageURL},
				Color:       0x3498db,
			}

			targetEmbed := &discordgo.MessageEmbed{
				Author:      &discordgo.MessageEmbedAuthor{Name: h.loc.T(lang, "duel_ds_defender", duel.TargetName)},
				Title:       res.CardTarget.Name,
				Description: h.loc.T(lang, "duel_ds_stats", res.CardTarget.PowerLevel, res.ChanceTarget),
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
		h.handleTop(s, i, lang, criteria, global)
		return
	}

	// 7. ПОМОЩЬ (МЕНЮ)
	if data.CustomID == "help_select" {
		category := data.Values[0]
		responseKey := "help_" + category
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseUpdateMessage,
			Data: &discordgo.InteractionResponseData{
				Content:    h.loc.T(lang, responseKey),
				Components: h.getHelpMenu(lang),
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

		h.linkMu.Lock()
		tgInternalID, exists := h.pendingLinks[dbUser.ID]
		h.linkMu.Unlock()

		if !exists && action != "cancel" {
			h.respondEphemeral(s, i, h.loc.T(lang, "link_err_expired"))
			return
		}

		switch action {
		case "cancel":
			h.linkMu.Lock()
			delete(h.pendingLinks, dbUser.ID)
			h.linkMu.Unlock()
			h.updateWithComponents(s, i, h.loc.T(lang, "link_cancelled"), nil)

		case "keep":
			// Выдаем финальное предупреждение
			var msg string
			if target == "tg" {
				msg = h.loc.T(lang, "link_warn_tg")
			} else {
				msg = h.loc.T(lang, "link_warn_ds")
			}

			buttons := []discordgo.MessageComponent{discordgo.ActionsRow{
				Components: []discordgo.MessageComponent{
					discordgo.Button{Label: h.loc.T(lang, "btn_yes_im_sure"), Style: discordgo.DangerButton, CustomID: "link:confirm:" + target},
					discordgo.Button{Label: h.loc.T(lang, "btn_cancel"), Style: discordgo.SecondaryButton, CustomID: "link:cancel"},
				},
			}}
			h.updateWithComponents(s, i, msg, buttons)

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

			err := h.repo.LinkAccountsOverwrite(keepID, deleteID)
			if err != nil {
				log.Printf("[DISCORD LINK ERROR]: %v", err) // <--- ВОТ ЭТО ДОБАВЬ, чтобы видеть ошибку в терминале
				h.updateWithComponents(s, i, h.loc.T(lang, "error_db"), nil)
				return
			}

			// Очищаем сессию
			h.linkMu.Lock()
			delete(h.pendingLinks, dbUser.ID)
			h.linkMu.Unlock()

			h.updateWithComponents(s, i, h.loc.T(lang, "link_success"), nil)
		}
		return
	}
}

func (h *Handler) handleRoll(s *discordgo.Session, i *discordgo.InteractionCreate, user *models.User, lang string) {
	result, err := h.service.RollCard(user.ID)
	if err != nil {
		h.respond(s, i, h.loc.T(lang, "error_tech"))
		return
	}

	if result.OnCooldown {
		msg := h.loc.T(lang, "roll_cooldown", result.CooldownTimeLeft)
		if result.StreakUpdated {
			msg += "\n\n" + h.loc.T(lang, "streak_kept_alive", result.StreakDays)
		}
		h.respond(s, i, msg)
		return
	}

	title := h.loc.T(lang, "roll_success_title")
	desc := h.loc.T(lang, "roll_success_desc", result.Card.Name, result.RarityName, result.Card.PowerLevel, result.Reward)

	if result.IsFragment {
		if result.CardAssembled {
			desc = h.loc.T(lang, "roll_mythic_assembled", result.Card.Name, result.Card.PowerLevel, result.Reward)
		} else {
			desc = h.loc.T(lang, "roll_mythic_fragment", result.Card.Name, result.FragmentsCount, result.Reward)
		}
	}

	embed := &discordgo.MessageEmbed{
		Title:       title,
		Description: desc,
		Image:       &discordgo.MessageEmbedImage{URL: result.Card.ImageURL},
		Color:       0x00ff00,
	}

	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{Embeds: []*discordgo.MessageEmbed{embed}},
	})

	if result.StreakUpdated {
		streakMsg := h.loc.T(lang, "streak_continued", result.Reward, result.StreakDays)
		if result.StreakDays == 1 {
			streakMsg = h.loc.T(lang, "streak_started")
		}
		s.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{Content: "🔥 " + streakMsg})
	}
}

func (h *Handler) getProfileData(user *models.User, lang string) (*discordgo.MessageEmbed, []discordgo.MessageComponent) {
	profile, _ := h.service.GetUserProfile(user.ID)

	desc := fmt.Sprintf("**%s**\n\n", user.Username)
	desc += h.loc.T(lang, "profile_stats",
		profile.UniqueCardsCount, profile.TotalCardsCount,
		profile.DuplicatesCount, profile.Balance, profile.StreakDays)

	embed := &discordgo.MessageEmbed{
		Title:       h.loc.T(lang, "profile_title"),
		Description: desc,
		Color:       0x7289da,
	}

	buttons := []discordgo.MessageComponent{
		discordgo.ActionsRow{
			Components: []discordgo.MessageComponent{
				discordgo.Button{
					Label:    h.loc.T(lang, "btn_my_cards_ds"),
					Style:    discordgo.PrimaryButton,
					CustomID: "cards_nav:0",
					Emoji:    &discordgo.ComponentEmoji{Name: "🎴"},
				},
				discordgo.Button{
					Label:    h.loc.T(lang, "btn_profile_suggest"),
					Style:    discordgo.SecondaryButton,
					CustomID: "suggest_start",
					Emoji:    &discordgo.ComponentEmoji{Name: "💡"},
				},
			},
		},
	}
	return embed, buttons
}

func (h *Handler) handleProfile(s *discordgo.Session, i *discordgo.InteractionCreate, user *models.User, lang string) {
	embed, buttons := h.getProfileData(user, lang)

	// В Discord не всегда есть аватарка в i.Member.User, лучше брать из i.Interaction.Member.User
	if i.Member != nil && i.Member.User != nil && i.Member.User.Avatar != "" {
		embed.Thumbnail = &discordgo.MessageEmbedThumbnail{URL: i.Member.User.AvatarURL("128")}
	} else if i.User != nil && i.User.Avatar != "" {
		embed.Thumbnail = &discordgo.MessageEmbedThumbnail{URL: i.User.AvatarURL("128")}
	}

	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Embeds:     []*discordgo.MessageEmbed{embed},
			Components: buttons,
		},
	})
}

func (h *Handler) handleCardsNav(s *discordgo.Session, i *discordgo.InteractionCreate, user *models.User, lang string, offset int) {
	card, total, err := h.service.GetUserCardPagination(user.ID, offset)
	if err != nil || card == nil {
		h.respondEphemeral(s, i, h.loc.T(lang, "cards_empty"))
		return
	}

	desc := h.loc.T(lang, "card_nav_caption",
		card.RarityName, card.PowerLevel, card.Quantity, offset+1, total)

	embed := &discordgo.MessageEmbed{
		Title:       "🃏 " + card.CardName,
		Description: desc,
		Image:       &discordgo.MessageEmbedImage{URL: card.ImageURL},
		Color:       0x3498db,
	}

	var navButtons []discordgo.MessageComponent
	if offset > 0 {
		navButtons = append(navButtons, discordgo.Button{Label: "⬅️", Style: discordgo.SecondaryButton, CustomID: fmt.Sprintf("cards_nav:%d", offset-1)})
	}
	if offset < total-1 {
		navButtons = append(navButtons, discordgo.Button{Label: "➡️", Style: discordgo.SecondaryButton, CustomID: fmt.Sprintf("cards_nav:%d", offset+1)})
	}
	navButtons = append(navButtons, discordgo.Button{Label: "🔙", Style: discordgo.DangerButton, CustomID: "back_to_profile"})

	row := discordgo.ActionsRow{Components: navButtons}
	h.updateWithEmbedAndComponents(s, i, "", embed, []discordgo.MessageComponent{row})
}

func (h *Handler) handleLink(s *discordgo.Session, i *discordgo.InteractionCreate, dsUser *models.User, lang string) {
	options := i.ApplicationCommandData().Options
	if len(options) == 0 {
		h.respond(s, i, h.loc.T(lang, "link_err_invalid"))
		return
	}

	code := strings.ToUpper(options[0].StringValue())
	tgInternalID, exists := h.lp.GetIDByCode(code)
	if !exists {
		h.respond(s, i, h.loc.T(lang, "link_err_invalid"))
		return
	}

	// Получаем профили обоих аккаунтов, чтобы показать статистику
	tgProfile, err1 := h.service.GetUserProfile(tgInternalID)
	dsProfile, err2 := h.service.GetUserProfile(dsUser.ID)
	if err1 != nil || err2 != nil {
		h.respond(s, i, h.loc.T(lang, "error_db"))
		return
	}

	// Сохраняем сессию привязки
	h.linkMu.Lock()
	h.pendingLinks[dsUser.ID] = tgInternalID
	h.linkMu.Unlock()

	msg := h.loc.T(lang, "link_choice_msg", tgProfile.Balance, tgProfile.UniqueCardsCount, dsProfile.Balance, dsProfile.UniqueCardsCount)

	buttons := []discordgo.MessageComponent{discordgo.ActionsRow{
		Components: []discordgo.MessageComponent{
			discordgo.Button{Label: h.loc.T(lang, "link_keep_tg"), Style: discordgo.PrimaryButton, CustomID: "link:keep:tg"},
			discordgo.Button{Label: h.loc.T(lang, "link_keep_ds"), Style: discordgo.PrimaryButton, CustomID: "link:keep:ds"},
			discordgo.Button{Label: h.loc.T(lang, "btn_cancel"), Style: discordgo.DangerButton, CustomID: "link:cancel"},
		},
	}}

	h.respondWithComponents(s, i, msg, buttons)
}

func (h *Handler) handleTop(s *discordgo.Session, i *discordgo.InteractionCreate, lang, criteria string, global bool) {
	var targetChatID int64
	scope := "local"
	if global {
		scope = "global"
		targetChatID = 0
	} else {
		targetChatID = parseID(i.GuildID)
	}

	board, err := h.service.GetLeaderboard(criteria, targetChatID)
	if err != nil {
		h.respond(s, i, h.loc.T(lang, "error_top_load"))
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
		sb.WriteString(h.loc.T(lang, "top_empty"))
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
		Title:       "🏆 " + h.loc.T(lang, "top_"+scope+"_title") + " (" + h.loc.T(lang, "top_crit_"+criteria) + ")",
		Description: sb.String(),
		Color:       0xf1c40f,
	}

	buttons := discordgo.ActionsRow{
		Components: []discordgo.MessageComponent{
			discordgo.Button{Label: h.loc.T(lang, "btn_top_balance"), Style: discordgo.SecondaryButton, CustomID: fmt.Sprintf("top:balance:%s", scope), Emoji: &discordgo.ComponentEmoji{Name: "🪙"}},
			discordgo.Button{Label: h.loc.T(lang, "btn_top_cards"), Style: discordgo.SecondaryButton, CustomID: fmt.Sprintf("top:cards:%s", scope), Emoji: &discordgo.ComponentEmoji{Name: "🃏"}},
			discordgo.Button{Label: h.loc.T(lang, "btn_top_streak"), Style: discordgo.SecondaryButton, CustomID: fmt.Sprintf("top:streak:%s", scope), Emoji: &discordgo.ComponentEmoji{Name: "🔥"}},
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

func (h *Handler) handleCraft(s *discordgo.Session, i *discordgo.InteractionCreate, user *models.User, lang string) {
	result, err := h.service.CraftCard(user.ID)
	if err != nil {
		h.respond(s, i, "❌ "+h.loc.T(lang, "err_craft_failed", err.Error()))
		return
	}

	var desc string
	if result.IsFragment {
		if !result.CardAssembled {
			desc = h.loc.T(lang, "craft_mythic_frag", result.CraftCost, result.Card.Name, result.FragmentsCount)
		} else {
			desc = h.loc.T(lang, "craft_mythic_assembled", result.CraftCost, result.Card.Name, result.Card.PowerLevel)
		}
	} else {
		desc = h.loc.T(lang, "craft_success", result.CraftCost, result.Card.Name, result.RarityName, result.Card.PowerLevel)
	}

	embed := &discordgo.MessageEmbed{
		Title:       "🛠 " + h.loc.T(lang, "craft_title"),
		Description: desc,
		Image:       &discordgo.MessageEmbedImage{URL: result.Card.ImageURL},
		Color:       0x9b59b6,
	}

	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{Embeds: []*discordgo.MessageEmbed{embed}},
	})
}

func (h *Handler) handleDuel(s *discordgo.Session, i *discordgo.InteractionCreate, challenger *models.User, lang string) {
	options := i.ApplicationCommandData().Options
	targetDsUser := options[0].UserValue(s)
	amount := int(options[1].IntValue())

	if targetDsUser.ID == i.Member.User.ID {
		h.respond(s, i, h.loc.T(lang, "err_duel_self"))
		return
	}

	if challenger.Balance < amount {
		h.respond(s, i, h.loc.T(lang, "err_duel_funds"))
		return
	}

	targetDB, err := h.repo.GetOrCreateUserByDiscordID(parseID(targetDsUser.ID), targetDsUser.Username)
	if err != nil {
		h.respond(s, i, h.loc.T(lang, "error_db"))
		return
	}

	duelID := fmt.Sprintf("duel:%d:%d:%d", challenger.ID, targetDB.ID, amount)
	h.duelService.CreateDuel(duelID, challenger.ID, challenger.Username, targetDB.ID, targetDB.Username, amount)

	buttons := []discordgo.MessageComponent{discordgo.ActionsRow{
		Components: []discordgo.MessageComponent{
			discordgo.Button{Label: h.loc.T(lang, "btn_duel_accept"), Style: discordgo.SuccessButton, CustomID: "duel_accept:" + duelID},
			discordgo.Button{Label: h.loc.T(lang, "btn_duel_cancel"), Style: discordgo.DangerButton, CustomID: "duel_cancel:" + duelID},
		},
	}}

	h.respondWithComponents(s, i, h.loc.T(lang, "duel_challenge", challenger.Username, targetDB.Username, amount), buttons)
}

func (h *Handler) handleLocale(s *discordgo.Session, i *discordgo.InteractionCreate, dbUser *models.User, lang string) {
	options := i.ApplicationCommandData().Options
	if len(options) == 0 {
		h.respond(s, i, "❌ Please specify language code (ru/en)")
		return
	}

	targetLang := strings.ToLower(options[0].StringValue())
	if targetLang != "ru" && targetLang != "en" {
		h.respond(s, i, "❌ Unknown language. Use 'ru' or 'en'.")
		return
	}

	h.repo.SetUserLanguage(dbUser.ID, targetLang)
	h.respond(s, i, h.loc.T(targetLang, "lang_changed"))
}

// Вспомогательные функции
func parseID(idStr string) int64 {
	var id int64
	fmt.Sscanf(idStr, "%d", &id)
	return id
}

func (h *Handler) respond(s *discordgo.Session, i *discordgo.InteractionCreate, content string) {
	_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{Content: content},
	})
}

func (h *Handler) respondEphemeral(s *discordgo.Session, i *discordgo.InteractionCreate, content string) {
	_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{Content: content, Flags: discordgo.MessageFlagsEphemeral},
	})
}

func (h *Handler) respondWithComponents(s *discordgo.Session, i *discordgo.InteractionCreate, content string, components []discordgo.MessageComponent) {
	_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{Content: content, Components: components},
	})
}

func (h *Handler) updateWithComponents(s *discordgo.Session, i *discordgo.InteractionCreate, content string, components []discordgo.MessageComponent) {
	_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseUpdateMessage,
		Data: &discordgo.InteractionResponseData{Content: content, Components: components},
	})
}

func (h *Handler) updateWithEmbedAndComponents(s *discordgo.Session, i *discordgo.InteractionCreate, content string, embed *discordgo.MessageEmbed, components []discordgo.MessageComponent) {
	_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseUpdateMessage,
		Data: &discordgo.InteractionResponseData{Content: content, Embeds: []*discordgo.MessageEmbed{embed}, Components: components},
	})
}

// Активация кода игроками (Discord)
func (h *Handler) handlePromo(s *discordgo.Session, i *discordgo.InteractionCreate, dbUser *models.User, lang string) {
	// Никакого getLang здесь нет, lang уже пришел в аргументах функции!
	options := i.ApplicationCommandData().Options
	if len(options) == 0 {
		h.respondEphemeral(s, i, "❌ Укажите код.")
		return
	}

	code := options[0].StringValue()

	// ПРАВИЛЬНЫЙ ВЫЗОВ: 2 аргумента на вход, 3 на выход
	reward, cards, err := h.service.RedeemPromo(dbUser.ID, code)
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
		h.respondEphemeral(s, i, h.loc.T(lang, errKey))
		return
	}

	// Собираем текст
	var sb strings.Builder
	if reward.Points > 0 {
		sb.WriteString(h.loc.T(lang, "promo_reward_points", reward.Points) + "\n")
	}
	if reward.PremiumRolls > 0 {
		sb.WriteString(h.loc.T(lang, "promo_reward_rolls", reward.PremiumRolls) + "\n")
	}
	if len(cards) > 0 {
		sb.WriteString("\n" + h.loc.T(lang, "promo_reward_cards_count", len(cards)) + "\n")
		for _, c := range cards {
			sb.WriteString(h.loc.T(lang, "promo_reward_card", c.Name, c.PowerLevel) + "\n")
		}
	}

	// Главный эмбед с текстом награды
	embeds := []*discordgo.MessageEmbed{
		{
			Title:       h.loc.T(lang, "promo_success_title"),
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
