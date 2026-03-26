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
	service := service.NewGachaService(repo)
	h := telegram.NewHandler(repo, service)

	bot.Handle("/start", h.HandleStart)
	bot.Handle("/roll", h.HandleRoll)
	bot.Handle("/profile", h.HandleProfile)

	log.Println("Бот запущен...")
	bot.Start()
}
