package discord

import (
	"gachabot/internal/service/duel"
	"gachabot/internal/service/gacha"
	"gachabot/internal/service/spawn"
	"gachabot/internal/service/suggest"
	"log"

	"gachabot/internal/i18n"
	"gachabot/internal/repository"

	"github.com/bwmarrin/discordgo"
	"github.com/redis/go-redis/v9"
)

type LinkProvider interface {
	GetIDByCode(code string) (int64, bool)
}

type Bot struct {
	session        *discordgo.Session
	repo           *repository.PostgresRepo
	service        *gacha.GachaService
	duelService    *duel.DuelService
	loc            *i18n.Localizer
	suggestService *suggest.SuggestService
	spawnService   *spawn.SpawnService
	lp             LinkProvider
	rdb            *redis.Client
	webAppURL      string
	NotifyAdmin    func(text string, imageURL string)
}

func NewBot(token string, repo *repository.PostgresRepo, rdb *redis.Client, gs *gacha.GachaService, ds *duel.DuelService, ss *suggest.SuggestService, sp *spawn.SpawnService, loc *i18n.Localizer, lp LinkProvider, webAppURL string, notifyAdmin func(string, string)) (*Bot, error) {
	dg, err := discordgo.New("Bot " + token)
	if err != nil {
		return nil, err
	}

	dg.Identify.Intents = discordgo.IntentsGuilds | discordgo.IntentsGuildMembers | discordgo.IntentsGuildMessages | discordgo.IntentsMessageContent

	b := &Bot{
		session:        dg,
		repo:           repo,
		rdb:            rdb,
		service:        gs,
		duelService:    ds,
		loc:            loc,
		suggestService: ss,
		spawnService:   sp,
		lp:             lp,
		webAppURL:      webAppURL,
		NotifyAdmin:    notifyAdmin,
	}

	dg.AddHandler(b.HandleInteraction)
	dg.AddHandler(b.HandleComponentInteraction)
	dg.AddHandler(b.HandleMessageCreate)
	dg.AddHandler(b.onGuildCreate)
	dg.AddHandler(b.onGuildDelete)

	return b, nil
}

func (b *Bot) Start() error {
	err := b.session.Open()
	if err != nil {
		return err
	}
	log.Println("[DISCORD] The bot has been successfully connected to the gateway!")

	b.setupCommands()
	return nil
}

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
		{
			Name:        "setmainchannel",
			Description: "Set this channel as the bot's main channel (spawns/announcements)",
			DescriptionLocalizations: &map[discordgo.Locale]string{
				discordgo.Russian: "Назначить этот канал основным для бота (спавны/объявления)",
			},
		},
		{
			Name:        "claim",
			Description: "Catch the active spawned card in this channel",
			DescriptionLocalizations: &map[discordgo.Locale]string{
				discordgo.Russian: "Поймать активную карту-спавн в этом канале",
			},
		},
	}

	// Register commands one by one (upsert) instead of BulkOverwrite: the latter
	// fails if the app has an auto-created Entry Point command (from enabling an
	// Activity), because bulk overwrite would remove it (HTTP 400, code 50240).
	registered := 0
	for _, cmd := range commands {
		if _, err := b.session.ApplicationCommandCreate(b.session.State.User.ID, "", cmd); err != nil {
			log.Printf("[DISCORD ERROR] register %q failed: %v", cmd.Name, err)
		} else {
			registered++
		}
	}
	log.Printf("[DISCORD] %d/%d slash commands registered", registered, len(commands))
}
