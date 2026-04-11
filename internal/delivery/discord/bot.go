package discord

import (
	"log"

	"gachabot/internal/i18n"
	"gachabot/internal/repository"
	"gachabot/internal/service"

	"github.com/bwmarrin/discordgo"
	"github.com/redis/go-redis/v9"
)

// LinkProvider описывает интерфейс для связи с Telegram-хэндлером
type LinkProvider interface {
	GetIDByCode(code string) (int64, bool)
}

// Bot - обертка над Discord сессией
type Bot struct {
	session     *discordgo.Session
	repo        *repository.PostgresRepo
	service     *service.GachaService
	duelService *service.DuelService
	loc         *i18n.Localizer
	lp          LinkProvider
	rdb         *redis.Client
	NotifyAdmin func(text string, imageURL string)
}

// NewBot создает новый инстанс Discord бота
func NewBot(token string, repo *repository.PostgresRepo, rdb *redis.Client, gs *service.GachaService, ds *service.DuelService, loc *i18n.Localizer, lp LinkProvider, notifyAdmin func(string, string)) (*Bot, error) {
	dg, err := discordgo.New("Bot " + token)
	if err != nil {
		return nil, err
	}

	dg.Identify.Intents = discordgo.IntentsGuilds | discordgo.IntentsGuildMembers | discordgo.IntentsGuildMessages | discordgo.IntentsMessageContent

	b := &Bot{
		session:     dg,
		repo:        repo,
		rdb:         rdb,
		service:     gs,
		duelService: ds,
		loc:         loc,
		lp:          lp,
		NotifyAdmin: notifyAdmin,
	}

	// Регистрируем глобальные роутеры (они будут лежать в router.go)
	dg.AddHandler(b.HandleInteraction)
	dg.AddHandler(b.HandleComponentInteraction)
	dg.AddHandler(b.HandleMessageCreate)

	return b, nil
}

// Start открывает соединение и регистрирует слэш-команды
func (b *Bot) Start() error {
	err := b.session.Open()
	if err != nil {
		return err
	}
	log.Println("[DISCORD] Бот успешно подключен к шлюзу!")

	b.setupCommands()
	return nil
}

// setupCommands регистрирует слэш-команды в Discord
func (b *Bot) setupCommands() {
	commands := []*discordgo.ApplicationCommand{
		{
			Name:        "roll",
			Description: "Get a random card",
			DescriptionLocalizations: &map[discordgo.Locale]string{
				discordgo.Russian: "Получить случайную карточку",
			},
		},
		{
			Name:        "profile",
			Description: "View your profile",
			DescriptionLocalizations: &map[discordgo.Locale]string{
				discordgo.Russian: "Твой профиль",
			},
		},
		{
			Name:        "link",
			Description: "Link your account with Telegram",
			DescriptionLocalizations: &map[discordgo.Locale]string{
				discordgo.Russian: "Связать аккаунт с Telegram",
			},
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "code",
					Description: "Code from Telegram",
					DescriptionLocalizations: map[discordgo.Locale]string{
						discordgo.Russian: "Код из Telegram",
					},
					Required: true,
				},
			},
		},
		{
			Name:        "help",
			Description: "Game help and info",
			DescriptionLocalizations: &map[discordgo.Locale]string{
				discordgo.Russian: "Помощь по игре",
			},
		},
		/*		{
				Name:        "top",
				Description: "Server leaderboard",
				DescriptionLocalizations: &map[discordgo.Locale]string{
					discordgo.Russian: "Топ сервера по балансу",
				},
			},*/
		{
			Name:        "top",
			Description: "Global leaderboard",
			DescriptionLocalizations: &map[discordgo.Locale]string{
				discordgo.Russian: "Мировой топ",
			},
		},
		{
			Name:        "craft",
			Description: "Craft a card from duplicates",
			DescriptionLocalizations: &map[discordgo.Locale]string{
				discordgo.Russian: "Создать карту из дубликатов",
			},
		},
		{
			Name:        "duel",
			Description: "Challenge a player to a duel",
			DescriptionLocalizations: &map[discordgo.Locale]string{
				discordgo.Russian: "Вызвать игрока на дуэль",
			},
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionUser,
					Name:        "user",
					Description: "Who to challenge?",
					DescriptionLocalizations: map[discordgo.Locale]string{
						discordgo.Russian: "Кого вызываем?",
					},
					Required: true,
				},
				{
					Type:        discordgo.ApplicationCommandOptionInteger,
					Name:        "amount",
					Description: "Bet amount",
					DescriptionLocalizations: map[discordgo.Locale]string{
						discordgo.Russian: "Ставка",
					},
					Required: true,
				},
			},
		},
		{
			Name:        "locale",
			Description: "Change language (ru/en)",
			DescriptionLocalizations: &map[discordgo.Locale]string{
				discordgo.Russian: "Сменить язык (ru/en)",
			},
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "lang",
					Description: "ru or en",
					Required:    true,
				},
			},
		},
		{
			Name:        "promo",
			Description: "Activate a promo code",
			DescriptionLocalizations: &map[discordgo.Locale]string{
				discordgo.Russian: "Активировать промокод",
			},
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "code",
					Description: "Code",
					DescriptionLocalizations: map[discordgo.Locale]string{
						discordgo.Russian: "Код",
					},
					Required: true,
				},
			},
		},
	}

	_, err := b.session.ApplicationCommandBulkOverwrite(b.session.State.User.ID, "", commands)
	if err != nil {
		log.Printf("[DISCORD ERROR] Ошибка регистрации команд: %v", err)
	} else {
		log.Println("[DISCORD] Слэш-команды зарегистрированы!")
	}
}
