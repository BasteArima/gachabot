package main

import (
	"database/sql"
	"gachabot/internal/repository"
	"gachabot/internal/service"
	"gachabot/internal/telegram"
	"log"
	"os"
	"time"

	// Подключаем драйвер анонимно (через _), чтобы инициализировать его под капотом
	_ "github.com/lib/pq"
	// Импортируем скачанную библиотеку и даем ей удобное имя "tele"
	"github.com/joho/godotenv"
	tele "gopkg.in/telebot.v3"
)

func main() {
	_ = godotenv.Load() // Читаем .env

	connStr := "postgres://root:secretpassword@localhost:5432/gachabot?sslmode=disable"
	db, err := sql.Open("postgres", connStr)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	if err = db.Ping(); err != nil {
		log.Fatal("Нет связи с БД:", err)
	}

	// 2. Настраиваем бота
	pref := tele.Settings{
		Token:  os.Getenv("BOT_TOKEN"),
		Poller: &tele.LongPoller{Timeout: 10 * time.Second},
	}
	bot, err := tele.NewBot(pref)
	if err != nil {
		log.Fatal(err)
	}

	repo := repository.NewPostgresRepo(db)
	gachaService := service.NewGachaService(repo)
	duelService := service.NewDuelService(repo)
	h := telegram.NewHandler(repo, gachaService, duelService)

	bot.Handle("/start", h.HandleStart)
	bot.Handle("/roll", h.HandleRoll)
	bot.Handle("/profile", h.HandleProfile)
	// \f означает, что мы ловим Callback (нажатие кнопки), у которой ID = cards_nav
	bot.Handle("\fcards_nav", h.HandleCardsNav)
	bot.Handle("\fback_profile", h.HandleBackToProfile)
	bot.Handle("/top", h.HandleLocalTop)
	bot.Handle("/globaltop", h.HandleGlobalTop)
	bot.Handle("\ftop_btn", h.HandleTopCallback) // \f ловит нажатия Inline кнопок
	bot.Handle("/help", h.HandleHelp)
	bot.Handle("\fhelp_nav", h.HandleHelpCallback)
	bot.Handle("/duel", h.HandleDuel)
	bot.Handle("\fduel_accept", h.HandleDuelCallback)
	bot.Handle("\fduel_cancel", h.HandleDuelCallback)

	log.Println("Бот запущен...")
	bot.Start()
}
