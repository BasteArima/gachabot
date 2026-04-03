package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"

	"gachabot/internal/delivery/discord"
	"gachabot/internal/delivery/telegram"
	"gachabot/internal/i18n"
	"gachabot/internal/repository"
	"gachabot/internal/service"

	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
	"github.com/redis/go-redis/v9"
)

func main() {
	_ = godotenv.Load()

	// 1. ПОДКЛЮЧЕНИЕ К POSTGRESQL
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

	// 2. ПОДКЛЮЧЕНИЕ К REDIS
	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		redisAddr = "localhost:6379"
	}

	rdb := redis.NewClient(&redis.Options{
		Addr:     redisAddr,
		Password: "",
		DB:       0,
	})

	ctx := context.Background()
	if err := rdb.Ping(ctx).Err(); err != nil {
		log.Fatal("[REDIS ERROR] Ошибка подключения:", err)
	}
	log.Println("[REDIS] Успешно подключен!")

	// 3. ИНИЦИАЛИЗАЦИЯ БИЗНЕС-ЛОГИКИ
	repo := repository.NewPostgresRepo(db)
	gachaService := service.NewGachaService(repo, rdb)
	duelService := service.NewDuelService(repo, rdb)

	// 4. ИНИЦИАЛИЗАЦИЯ TELEGRAM
	botToken := os.Getenv("BOT_TOKEN")
	tgLoc, _ := i18n.NewLocalizer("locales/telegram", "ru")

	tgBot, err := telegram.NewBot(botToken, repo, rdb, gachaService, duelService, tgLoc)
	if err != nil {
		log.Fatal("Ошибка создания Telegram бота:", err)
	}

	// 5. ИНИЦИАЛИЗАЦИЯ DISCORD
	dsToken := os.Getenv("DISCORD_TOKEN")
	if dsToken != "" {
		discordLoc, _ := i18n.NewLocalizer("locales/discord", "ru")

		// Передаем tgBot как LinkProvider и его метод NotifyAdmin
		dsBot, err := discord.NewBot(dsToken, repo, rdb, gachaService, duelService, discordLoc, tgBot, tgBot.NotifyAdmin)
		if err != nil {
			log.Fatal("Ошибка создания Discord бота:", err)
		}

		// Метод Start() для Дискорда (b.session.Open) не блокирует поток,
		// поэтому мы можем вызвать его до телеграма без горутин.
		err = dsBot.Start()
		if err != nil {
			log.Printf("[DISCORD ERROR] Не удалось запустить бота: %v", err)
		}
	} else {
		log.Println("[DISCORD] Пропуск запуска: DISCORD_TOKEN не найден")
	}

	// 6. ЗАПУСК TELEGRAM (Блокирующий вызов, держит программу включенной)
	log.Println("Бэкенд запущен! Ожидание событий...")
	tgBot.Start()
}
