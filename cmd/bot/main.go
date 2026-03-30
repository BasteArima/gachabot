package main

import (
	"database/sql"
	"fmt"
	"gachabot/internal/i18n"
	"gachabot/internal/repository"
	"gachabot/internal/service"
	"gachabot/internal/telegram"
	"log"
	"os"
	"time"

	_ "gachabot/internal/i18n"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
	tele "gopkg.in/telebot.v3"
)

func main() {
	// Пытаемся загрузить .env файл.
	// Если его нет (например, переменные уже заданы в системе или в Docker), игнорируем ошибку.
	_ = godotenv.Load()

	// 1. Собираем строку подключения к БД из переменных окружения
	dbUser := os.Getenv("POSTGRES_USER")
	dbPass := os.Getenv("POSTGRES_PASSWORD")
	dbHost := os.Getenv("POSTGRES_HOST")
	dbPort := os.Getenv("POSTGRES_PORT")
	dbName := os.Getenv("POSTGRES_DB")

	// Если хост или порт не заданы, ставим дефолтные значения (полезно для локальной разработки)
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

	// 2. Настраиваем бота
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

	// Инициализация локализатора (указываем папку и язык по умолчанию)
	loc, err := i18n.NewLocalizer("locales", "ru")
	if err != nil {
		log.Fatal("Ошибка загрузки переводов:", err)
	}

	// Инициализация слоев
	repo := repository.NewPostgresRepo(db)
	gachaService := service.NewGachaService(repo)
	duelService := service.NewDuelService(repo)
	h := telegram.NewHandler(repo, gachaService, duelService, loc)

	// Роутинг
	bot.Handle("/start", h.HandleStart)
	bot.Handle("/roll", h.HandleRoll)
	bot.Handle("/profile", h.HandleProfile)
	bot.Handle("\fcards_nav", h.HandleCardsNav)
	bot.Handle("\fback_profile", h.HandleBackToProfile)
	bot.Handle("/top", h.HandleLocalTop)
	bot.Handle("/globaltop", h.HandleGlobalTop)
	bot.Handle("\ftop_btn", h.HandleTopCallback)
	bot.Handle("/help", h.HandleHelp)
	bot.Handle("\fhelp_nav", h.HandleHelpCallback)
	bot.Handle("/duel", h.HandleDuel)
	bot.Handle("\fduel_accept", h.HandleDuelCallback)
	bot.Handle("\fduel_cancel", h.HandleDuelCallback)
	bot.Handle("/craft", h.HandleCraft)
	bot.Handle("/shop", h.HandleDonate)
	// Добавь эти строки туда, где у тебя bot.Handle(...)
	// Магазин и донаты
	bot.Handle("\fshop_rolls_menu", h.HandleShopMenu)
	bot.Handle("\fbuy_invoice", h.HandleSendInvoice)
	bot.Handle("\froll_again", h.HandleRollAgainCallback)
	bot.Handle(tele.OnCheckout, h.HandlePreCheckout)
	bot.Handle(tele.OnPayment, h.HandlePayment)

	log.Println("Бот запущен...")
	bot.Start()
}
