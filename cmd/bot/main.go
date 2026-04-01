package main

import (
	"context"
	"database/sql"
	"fmt"
	"gachabot/internal/delivery/discord"
	"log"
	"os"
	"time"

	"gachabot/internal/delivery/telegram"
	"gachabot/internal/i18n"
	"gachabot/internal/repository"
	"gachabot/internal/service"

	"github.com/bwmarrin/discordgo"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
	"github.com/redis/go-redis/v9"
	_ "github.com/redis/go-redis/v9"
	tele "gopkg.in/telebot.v3"
)

func main() {
	// 1. Пытаемся загрузить .env файл
	_ = godotenv.Load()

	// 2. Собираем строку подключения к БД
	dbUser := os.Getenv("POSTGRES_USER")
	dbPass := os.Getenv("POSTGRES_PASSWORD")
	dbHost := os.Getenv("POSTGRES_HOST")
	dbPort := os.Getenv("POSTGRES_PORT")
	dbName := os.Getenv("POSTGRES_DB")

	if dbHost == "" {
		dbHost = "localhost"
	}
	if dbPort == "" {
		dbPort = "5432"
	}

	connStr := fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable",
		dbUser, dbPass, dbHost, dbPort, dbName)

	db, err := sql.Open("postgres", connStr)
	if err != nil {
		log.Fatal("Ошибка инициализации БД:", err)
	}
	defer db.Close()

	if err = db.Ping(); err != nil {
		log.Fatal("Нет связи с БД:", err)
	}

	// --- ИНИЦИАЛИЗАЦИЯ REDIS ---
	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		redisAddr = "localhost:6379" // Фолбэк для локального запуска без докера
	}

	rdb := redis.NewClient(&redis.Options{
		Addr:     redisAddr,
		Password: "", // Без пароля по умолчанию
		DB:       0,  // Дефолтная база
	})

	// Проверяем соединение
	ctx := context.Background()
	if err := rdb.Ping(ctx).Err(); err != nil {
		log.Fatal("[REDIS ERROR] Ошибка подключения:", err)
	}
	log.Println("[REDIS] Успешно подключен!")

	// 3. Настраиваем Telegram бота
	token := os.Getenv("BOT_TOKEN")
	if token == "" {
		log.Fatal("BOT_TOKEN не задан в переменных окружения")
	}

	pref := tele.Settings{
		Token:  token,
		Poller: &tele.LongPoller{Timeout: 10 * time.Second},
	}
	bot, err := tele.NewBot(pref)
	if err != nil {
		log.Fatal("Ошибка создания бота:", err)
	}

	// 4. ИНИЦИАЛИЗАЦИЯ ЛОКАЛИЗАТОРА ДЛЯ ТЕЛЕГРАМА (Правильный путь!)
	telegramLoc, err := i18n.NewLocalizer("locales/telegram", "ru")
	if err != nil {
		log.Fatal("Ошибка загрузки переводов Telegram:", err)
	}

	// 5. Инициализация общих слоев (Бизнес-логика)
	repo := repository.NewPostgresRepo(db)
	gachaService := service.NewGachaService(repo, rdb)
	duelService := service.NewDuelService(repo, rdb)

	// 6. Инициализация слоя Delivery (Транспорт для Телеграма)
	h := telegram.NewHandler(repo, rdb, gachaService, duelService, telegramLoc)

	// 7. Роутинг команд
	bot.Handle("/start", h.HandleStart)
	bot.Handle("/roll", h.HandleRoll)
	bot.Handle("/profile", h.HandleProfile)
	bot.Handle("\fcards_nav", h.HandleCardsNav)
	bot.Handle("\fback_profile", h.HandleBackToProfile)

	// Топы
	bot.Handle("/top", h.HandleLocalTop)
	bot.Handle("/globaltop", h.HandleGlobalTop)
	bot.Handle("\ftop_btn", h.HandleTopCallback)

	// Помощь и язык
	bot.Handle("/help", h.HandleHelp)
	bot.Handle("\fhelp_nav", h.HandleHelpCallback)
	bot.Handle("/locale", h.HandleLocale)
	bot.Handle("\flang_set", h.HandleLanguageSetCallback)

	// Дуэли
	bot.Handle("/duel", h.HandleDuel)
	bot.Handle("\fduel_accept", h.HandleDuelCallback)
	bot.Handle("\fduel_cancel", h.HandleDuelCallback)

	// Крафт и донат
	bot.Handle("/craft", h.HandleCraft)
	bot.Handle("/shop", h.HandleDonate)
	bot.Handle("\fshop_rolls_menu", h.HandleShopMenu)
	bot.Handle("\fbuy_invoice", h.HandleSendInvoice)
	bot.Handle("\froll_again", h.HandleRollAgainCallback)
	bot.Handle(tele.OnCheckout, h.HandlePreCheckout)
	bot.Handle(tele.OnPayment, h.HandlePayment)

	// Предложка карточек
	bot.Handle("\fsuggest_start", h.HandleSuggestStart)
	bot.Handle("\fs_q1_yes", h.HandleSuggestQuiz)
	bot.Handle("\fs_q1_no", h.HandleSuggestQuiz)
	bot.Handle("\fs_q2_yes", h.HandleSuggestQuiz)
	bot.Handle("\fs_q2_no", h.HandleSuggestQuiz)
	bot.Handle("\fs_q3_43", h.HandleSuggestQuiz)
	bot.Handle("\fs_q3_34", h.HandleSuggestQuiz)
	bot.Handle("\fs_q3_11", h.HandleSuggestQuiz)
	bot.Handle("\fs_cancel", h.HandleSuggestCancel)
	bot.Handle("\fs_close", h.HandleSuggestClose)

	bot.Handle("/promo", h.HandlePromo)
	bot.Handle("/addpromo", h.HandleAddPromo) // Админская команда
	bot.Handle("/createpromo", h.HandleCreatePromo)

	// Перехватчики контента
	bot.Handle(tele.OnPhoto, h.HandleMediaSuggest)
	bot.Handle(tele.OnDocument, h.HandleMediaSuggest)
	bot.Handle(tele.OnText, h.HandleTextFallback) // Защита от дурака

	bot.Handle("/link", h.HandleLinkStart)

	// discord
	// --- НАСТРОЙКА DISCORD ---
	dsToken := os.Getenv("DISCORD_TOKEN")
	if dsToken == "" {
		log.Println("[DISCORD] Пропуск запуска: DISCORD_TOKEN не найден")
	} else {
		dg, err := discordgo.New("Bot " + dsToken)
		if err != nil {
			log.Fatal("[DISCORD ERROR] Ошибка создания сессии:", err)
		}

		dg.Identify.Intents = discordgo.IntentsGuilds | discordgo.IntentsGuildMembers | discordgo.IntentsGuildMessages | discordgo.IntentsMessageContent

		discordLoc, _ := i18n.NewLocalizer("locales/discord", "ru")

		adminChatID := int64(-5214417967) // Твой ID админского чата
		notifyAdmin := func(text string, imageURL string) {
			adminChat := &tele.Chat{ID: adminChatID}
			photo := &tele.Photo{
				File:    tele.FromURL(imageURL),
				Caption: text,
			}
			// Отправляем фото в Телеграм от имени бота
			_, err := bot.Send(adminChat, photo, tele.ModeHTML)
			if err != nil {
				log.Printf("Ошибка пересылки предложки из Discord в Telegram: %v", err)
			}
		}

		// h — это твой telegram.Handler, который мы создали выше
		dsHandler := discord.NewHandler(repo, gachaService, duelService, discordLoc, h, notifyAdmin, rdb)
		dg.AddHandler(dsHandler.HandleInteraction)
		dg.AddHandler(dsHandler.HandleComponentInteraction)
		dg.AddHandler(dsHandler.HandleMessageCreate)

		// Открываем соединение
		err = dg.Open()
		if err != nil {
			log.Fatal("[DISCORD ERROR] Ошибка подключения:", err)
		}
		defer dg.Close()
		log.Println("[DISCORD] Бот успешно подключен к шлюзу!")

		// Регистрация команд (Bulk Overwrite)
		commands := []*discordgo.ApplicationCommand{
			{
				Name:        "roll",
				Description: "Get a random card", // Базовое (английское) описание
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
			{
				Name:        "top",
				Description: "Server leaderboard",
				DescriptionLocalizations: &map[discordgo.Locale]string{
					discordgo.Russian: "Топ сервера по балансу",
				},
			},
			{
				Name:        "globaltop",
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

		// И не забудь добавить обработчик компонентов (для Select Menu)

		_, err = dg.ApplicationCommandBulkOverwrite(dg.State.User.ID, "", commands)
		if err != nil {
			log.Printf("[DISCORD ERROR] Ошибка регистрации команд: %v", err)
		} else {
			log.Println("[DISCORD] Слэш-команды зарегистрированы!")
		}
	}

	// --- ФИНАЛЬНЫЙ ЗАПУСК ТЕЛЕГРАМА ---
	log.Println("Telegram-бот успешно запущен и готов к работе!")
	bot.Start() // <--- Эта строчка всегда должна быть ПОСЛЕДНЕЙ в main()
}
