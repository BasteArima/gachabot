package discord

import (
	"fmt"
	"log"
	"strings"

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
	duelService *service.DuelService // Добавляем это
	loc         *i18n.Localizer
	lp          LinkProvider
}

func NewHandler(repo *repository.PostgresRepo, service *service.GachaService, duelService *service.DuelService, loc *i18n.Localizer, lp LinkProvider) *Handler {
	return &Handler{
		repo:        repo,
		service:     service,
		duelService: duelService, // И это
		loc:         loc,
		lp:          lp,
	}
}

// Главный входной узел для всех взаимодействий в Discord
func (h *Handler) HandleInteraction(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if i.Type != discordgo.InteractionApplicationCommand {
		return
	}

	data := i.ApplicationCommandData()

	// Извлекаем пользователя (может быть в Member или в User в зависимости от типа чата)
	dsUser := i.Member.User
	if dsUser == nil {
		dsUser = i.User
	}

	// 1. АДАПТЕР: Получаем/создаем внутреннего юзера по Discord ID
	dbUser, err := h.repo.GetOrCreateUserByDiscordID(parseID(dsUser.ID), dsUser.Username)
	if err != nil {
		log.Printf("[DISCORD DB ERROR]: %v", err)
		return
	}

	lang := dbUser.LanguageCode
	if lang == "" {
		lang = "ru" // Дефолт для дискорда
	}

	if i.GuildID != "" {
		guildID := parseID(i.GuildID)
		_ = h.repo.TrackUserChat(dbUser.ID, guildID)
	}

	// Роутинг команд
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
	}
}

// Реализация компактного Help через выпадающий список
func (h *Handler) handleHelp(s *discordgo.Session, i *discordgo.InteractionCreate, lang string) {
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content:    h.loc.T(lang, "help_main"),
			Components: h.getHelpMenu(lang), // Используем наш метод
		},
	})
}

// getHelpMenu возвращает строку компонентов с выпадающим списком
func (h *Handler) getHelpMenu(lang string) []discordgo.MessageComponent {
	return []discordgo.MessageComponent{
		discordgo.ActionsRow{
			Components: []discordgo.MessageComponent{
				discordgo.SelectMenu{
					CustomID:    "help_select",
					Placeholder: h.loc.T(lang, "btn_help_select"),
					Options: []discordgo.SelectMenuOption{
						{
							Label:       "🏠 Главная справка",
							Value:       "main",
							Description: "Вернуться в начало",
						},
						{Label: "Карточки", Value: "cards", Emoji: &discordgo.ComponentEmoji{Name: "🃏"}},
						{Label: "Редкости", Value: "rarities", Emoji: &discordgo.ComponentEmoji{Name: "💎"}},
						{Label: "Стрики", Value: "streaks", Emoji: &discordgo.ComponentEmoji{Name: "🔥"}},
						{Label: "Гарант", Value: "pity", Emoji: &discordgo.ComponentEmoji{Name: "🛡"}},
						{Label: "Дуэли", Value: "duel", Emoji: &discordgo.ComponentEmoji{Name: "⚔️"}},
						{Label: "Крафт", Value: "craft", Emoji: &discordgo.ComponentEmoji{Name: "🛠"}},
					},
				},
			},
		},
	}
}

// Нужно добавить обработку выбора в SelectMenu
func (h *Handler) HandleComponentInteraction(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if i.Type != discordgo.InteractionMessageComponent {
		return
	}

	data := i.MessageComponentData()
	// Получаем данные юзера, который нажал на кнопку, чтобы знать его ID и язык
	dsUser := i.Member.User
	if dsUser == nil {
		dsUser = i.User
	}

	dbUser, _ := h.repo.GetOrCreateUserByDiscordID(parseID(dsUser.ID), dsUser.Username)
	lang := dbUser.LanguageCode
	if lang == "" {
		lang = "ru"
	}

	// --- ОБРАБОТКА ДУЭЛЕЙ ---
	if strings.HasPrefix(data.CustomID, "duel_") {
		parts := strings.Split(data.CustomID, ":")
		action := parts[0] // "duel_accept" или "duel_cancel"
		duelID := strings.Join(parts[1:], ":")

		duel, exists := h.duelService.GetDuel(duelID)
		if !exists {
			h.respond(s, i, h.loc.T(lang, "err_duel_expired"))
			return
		}

		// ОБРАБОТКА ОТМЕНЫ
		if action == "duel_cancel" {
			// Отменить может только автор или цель
			if dbUser.ID != duel.ChallengerID && dbUser.ID != duel.TargetID {
				h.respondEphemeral(s, i, "Это не ваша дуэль!") // Отвечаем скрыто
				return
			}
			h.duelService.PopDuel(duelID)
			h.updateWithComponents(s, i, "❌ Дуэль отменена.", []discordgo.MessageComponent{})
			return
		}

		// ОБРАБОТКА ПРИНЯТИЯ
		if action == "duel_accept" {
			if dbUser.ID != duel.TargetID {
				h.respondEphemeral(s, i, "Только оппонент может принять вызов!")
				return
			}

			h.duelService.PopDuel(duelID)
			res, err := h.duelService.ExecuteDuel(duel)
			if err != nil {
				h.updateWithComponents(s, i, "❌ Ошибка: "+err.Error(), []discordgo.MessageComponent{})
				return
			}

			// 1. Главный Embed с результатом
			mainEmbed := &discordgo.MessageEmbed{
				Title: "⚔️ БИТВА ЗАВЕРШЕНА",
				Description: fmt.Sprintf("🎲 Кубик: **%.1f**\n🏆 Победитель: **%s**\n💰 Выигрыш: **%d** 🪙",
					res.Roll, res.WinnerName, res.AmountWon*2),
				Color: 0xe67e22,
			}

			// 2. Карточка атакующего (Challenger)
			challengerEmbed := &discordgo.MessageEmbed{
				Author: &discordgo.MessageEmbedAuthor{Name: "Атакующий: " + duel.ChallengerName},
				Title:  res.CardChallenger.Name,
				Description: fmt.Sprintf("💪 Сила: **%d**\n📈 Шанс: **%.1f%%**",
					res.CardChallenger.PowerLevel, res.ChanceChallenger),
				Thumbnail: &discordgo.MessageEmbedThumbnail{URL: res.CardChallenger.ImageURL},
				Color:     0x3498db, // Синий
			}

			// 3. Карточка защищающегося (Target)
			targetEmbed := &discordgo.MessageEmbed{
				Author: &discordgo.MessageEmbedAuthor{Name: "Защитник: " + duel.TargetName},
				Title:  res.CardTarget.Name,
				Description: fmt.Sprintf("💪 Сила: **%d**\n📈 Шанс: **%.1f%%**",
					res.CardTarget.PowerLevel, res.ChanceTarget),
				Thumbnail: &discordgo.MessageEmbedThumbnail{URL: res.CardTarget.ImageURL},
				Color:     0xe74c3c, // Красный
			}

			// Отправляем все три эмбеда сразу
			_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseUpdateMessage,
				Data: &discordgo.InteractionResponseData{
					Embeds:     []*discordgo.MessageEmbed{mainEmbed, challengerEmbed, targetEmbed},
					Components: []discordgo.MessageComponent{}, // Убираем кнопки
				},
			})
			return
		}
	}

	if strings.HasPrefix(data.CustomID, "top:") {
		parts := strings.Split(data.CustomID, ":") // "top", "balance", "local"
		if len(parts) != 3 {
			return
		}

		criteria := parts[1]
		global := parts[2] == "global"

		// Достаем язык юзера
		dbUser, _ := h.repo.GetOrCreateUserByDiscordID(parseID(i.Member.User.ID), i.Member.User.Username)
		lang := dbUser.LanguageCode
		if lang == "" {
			lang = "ru"
		}

		h.handleTop(s, i, lang, criteria, global)
		return
	}

	if data.CustomID == "help_select" {
		// В продакшене лучше получать язык из БД по i.Member.User.ID
		dbUser, _ := h.repo.GetOrCreateUserByDiscordID(parseID(i.Member.User.ID), i.Member.User.Username)
		lang := dbUser.LanguageCode
		category := data.Values[0]

		// Определяем, какой ключ перевода использовать
		var responseKey string
		if category == "main" {
			responseKey = "help_main"
		} else {
			responseKey = "help_" + category
		}

		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseUpdateMessage,
			Data: &discordgo.InteractionResponseData{
				Content:    h.loc.T(lang, responseKey),
				Components: h.getHelpMenu(lang), // КРИТИЧНО: отправляем меню снова!
			},
		})
	}
}

// Команда /roll
func (h *Handler) handleRoll(s *discordgo.Session, i *discordgo.InteractionCreate, user *models.User, lang string) {
	result, err := h.service.RollCard(user.ID)
	if err != nil {
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{Content: "Error: " + err.Error()},
		})
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

	// Формируем описание карты
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
		Data: &discordgo.InteractionResponseData{
			Embeds: []*discordgo.MessageEmbed{embed},
		},
	})

	// Если стрик обновился — отправляем доп. сообщение
	if result.StreakUpdated {
		streakMsg := h.loc.T(lang, "streak_continued", result.Reward, result.StreakDays)
		if result.StreakDays == 1 {
			streakMsg = h.loc.T(lang, "streak_started")
		}
		s.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{
			Content: "🔥 " + streakMsg,
		})
	}
}

// Команда /profile
func (h *Handler) handleProfile(s *discordgo.Session, i *discordgo.InteractionCreate, user *models.User, lang string) {
	profile, err := h.service.GetUserProfile(user.ID)
	if err != nil {
		h.respond(s, i, "Error loading profile")
		return
	}

	// Discord Markdown для профиля
	desc := fmt.Sprintf("**%s**\n\n", user.Username)
	desc += h.loc.T(lang, "profile_stats",
		profile.UniqueCardsCount, profile.TotalCardsCount,
		profile.DuplicatesCount, profile.Balance, profile.StreakDays)

	embed := &discordgo.MessageEmbed{
		Title:       h.loc.T(lang, "profile_title"),
		Description: desc,
		Color:       0x7289da,
		Thumbnail: &discordgo.MessageEmbedThumbnail{
			URL: i.Member.User.AvatarURL("128"),
		},
	}

	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Embeds: []*discordgo.MessageEmbed{embed},
		},
	})
}

// Команда /link [код]
func (h *Handler) handleLink(s *discordgo.Session, i *discordgo.InteractionCreate, dsUser *models.User, lang string) {
	options := i.ApplicationCommandData().Options
	if len(options) == 0 {
		h.respond(s, i, "Введите код из Telegram!")
		return
	}

	code := strings.ToUpper(options[0].StringValue())

	// Проверяем код через провайдера (ТГ хэндлер)
	tgInternalID, exists := h.lp.GetIDByCode(code)
	if !exists {
		h.respond(s, i, h.loc.T(lang, "link_err_invalid"))
		return
	}

	// Если код верный, запускаем слияние
	err := h.repo.MergeAccounts(tgInternalID, dsUser.ID)
	if err != nil {
		log.Printf("[MERGE ERROR]: %v", err)
		h.respond(s, i, "Ошибка при слиянии аккаунтов. Возможно, они уже связаны.")
		return
	}

	h.respond(s, i, h.loc.T(lang, "link_success"))
}

// Вспомогательные функции
func (h *Handler) respond(s *discordgo.Session, i *discordgo.InteractionCreate, content string) {
	_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{Content: content},
	})
}

func parseID(idStr string) int64 {
	var id int64
	fmt.Sscanf(idStr, "%d", &id)
	return id
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
		h.respond(s, i, "Error loading leaderboard")
		return
	}

	// Заголовок в зависимости от критерия
	emoji := "🪙"
	if criteria == "cards" {
		emoji = "🃏"
	}
	if criteria == "streak" {
		emoji = "🔥"
	}

	var sb strings.Builder
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

	embed := &discordgo.MessageEmbed{
		Title:       "🏆 " + h.loc.T(lang, "top_"+scope+"_title") + " (" + h.loc.T(lang, "top_crit_"+criteria) + ")",
		Description: sb.String(),
		Color:       0xf1c40f,
	}

	// Создаем кнопки
	buttons := discordgo.ActionsRow{
		Components: []discordgo.MessageComponent{
			discordgo.Button{
				Label:    h.loc.T(lang, "btn_top_balance"),
				Style:    discordgo.SecondaryButton,
				CustomID: fmt.Sprintf("top:balance:%s", scope),
				Emoji:    &discordgo.ComponentEmoji{Name: "🪙"},
			},
			discordgo.Button{
				Label:    h.loc.T(lang, "btn_top_cards"),
				Style:    discordgo.SecondaryButton,
				CustomID: fmt.Sprintf("top:cards:%s", scope),
				Emoji:    &discordgo.ComponentEmoji{Name: "🃏"},
			},
			discordgo.Button{
				Label:    h.loc.T(lang, "btn_top_streak"),
				Style:    discordgo.SecondaryButton,
				CustomID: fmt.Sprintf("top:streak:%s", scope),
				Emoji:    &discordgo.ComponentEmoji{Name: "🔥"},
			},
		},
	}

	// Если это вызов команды — ChannelMessageWithSource
	// Если это нажатие кнопки — UpdateMessage
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
		// Выводим ошибку (например, "недостаточно дубликатов")
		h.respond(s, i, "❌ "+h.loc.T(lang, "err_craft_failed", err.Error()))
		return
	}

	desc := h.loc.T(lang, "craft_success", result.CraftCost, result.Card.Name, result.RarityName, result.Card.PowerLevel)

	embed := &discordgo.MessageEmbed{
		Title:       "🛠 " + h.loc.T(lang, "craft_title"),
		Description: desc,
		Image:       &discordgo.MessageEmbedImage{URL: result.Card.ImageURL},
		Color:       0x9b59b6, // Фиолетовый
	}

	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Embeds: []*discordgo.MessageEmbed{embed},
		},
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

	// Ищем противника в БД
	targetDB, err := h.repo.GetOrCreateUserByDiscordID(parseID(targetDsUser.ID), targetDsUser.Username)
	if err != nil {
		h.respond(s, i, "Error: database error")
		return
	}

	duelID := fmt.Sprintf("duel:%d:%d:%d", challenger.ID, targetDB.ID, amount)
	// Сохраняем в сервис (используем тот же механизм, что в ТГ)
	h.duelService.CreateDuel(duelID, challenger.ID, challenger.Username, targetDB.ID, targetDB.Username, amount)

	h.respondWithComponents(s, i, h.loc.T(lang, "duel_challenge", challenger.Username, targetDB.Username, amount), []discordgo.MessageComponent{
		discordgo.ActionsRow{
			Components: []discordgo.MessageComponent{
				discordgo.Button{Label: "Принять", Style: discordgo.SuccessButton, CustomID: "duel_accept:" + duelID},
				discordgo.Button{Label: "Отмена", Style: discordgo.DangerButton, CustomID: "duel_cancel:" + duelID},
			},
		},
	})
}

// respondWithComponents — сокращение для отправки сообщения с кнопками или меню
func (h *Handler) respondWithComponents(s *discordgo.Session, i *discordgo.InteractionCreate, content string, components []discordgo.MessageComponent) {
	_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content:    content,
			Components: components,
		},
	})
}

// updateWithComponents — для обновления уже существующего сообщения (например, при нажатии кнопки)
func (h *Handler) updateWithComponents(s *discordgo.Session, i *discordgo.InteractionCreate, content string, components []discordgo.MessageComponent) {
	_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseUpdateMessage,
		Data: &discordgo.InteractionResponseData{
			Content:    content,
			Components: components,
		},
	})
}

func (h *Handler) respondEphemeral(s *discordgo.Session, i *discordgo.InteractionCreate, content string) {
	_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: content,
			Flags:   discordgo.MessageFlagsEphemeral, // Сообщение увидит только тот, кто нажал
		},
	})
}
