package main

import (
	"context"
	"database/sql"
	"fmt"
	"gachabot/internal/service/backup"
	"gachabot/internal/service/duel"
	"gachabot/internal/service/gacha"
	"gachabot/internal/service/suggest"
	"log"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"gachabot/internal/delivery/discord"
	"gachabot/internal/delivery/telegram"
	"gachabot/internal/i18n"
	"gachabot/internal/repository"

	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
	"github.com/redis/go-redis/v9"
)

func main() {
	_ = godotenv.Load()

	db := initPostgres()
	defer db.Close()

	rdb := initRedis()
	defer rdb.Close()

	adminIDStr := os.Getenv("ADMIN_TELEGRAM_ID")
	adminID, _ := strconv.ParseInt(adminIDStr, 10, 64)

	cooldownStr := os.Getenv("COOLDOWN_HOURS")
	cooldownHours, _ := strconv.Atoi(cooldownStr)
	if cooldownHours == 0 {
		cooldownHours = 3 // Default value
	}
	cooldownDuration := time.Duration(cooldownHours) * time.Hour

	repo := repository.NewPostgresRepo(db)
	gachaService := gacha.NewGachaService(repo, rdb, adminID, cooldownDuration)
	duelService := duel.NewDuelService(repo, rdb)
	suggestService := suggest.NewSuggestService(repo, rdb)

	botToken := getEnvOrFatal("TELEGRAM_BOT_TOKEN")
	tgLoc, err := i18n.NewLocalizer("locales/telegram", "ru")
	if err != nil {
		log.Fatalf("failed to load telegram localization: %v", err)
	}

	tgBot, err := telegram.NewBot(botToken, repo, rdb, gachaService, duelService, suggestService, tgLoc, adminID)
	if err != nil {
		log.Fatalf("failed to create telegram bot: %v", err)
	}

	dsToken := os.Getenv("DISCORD_TOKEN")
	if dsToken != "" {
		discordLoc, err := i18n.NewLocalizer("locales/discord", "ru")
		if err != nil {
			log.Fatalf("failed to load discord localization: %v", err)
		}

		dsBot, err := discord.NewBot(dsToken, repo, rdb, gachaService, duelService, suggestService, discordLoc, tgBot, tgBot.NotifyAdmin)
		if err != nil {
			log.Fatalf("failed to create discord bot: %v", err)
		}

		if err := dsBot.Start(); err != nil {
			log.Printf("failed to start discord bot: %v", err)
		} else {
			log.Println("Discord bot started successfully")
		}
	} else {
		log.Println("DISCORD_TOKEN not found, skipping discord bot startup")
	}

	bHour, _ := strconv.Atoi(os.Getenv("BACKUP_TIME_HOUR"))
	bMinute, _ := strconv.Atoi(os.Getenv("BACKUP_TIME_MINUTE"))
	if bHour == 0 && bMinute == 0 && os.Getenv("BACKUP_TIME_HOUR") == "" {
		bHour = 3
		bMinute = 10
	}
	backupService := backup.NewBackupService(bHour, bMinute, "/backups/daily", tgBot)
	backupService.Start()

	go func() {
		log.Println("Telegram bot is running...")
		tgBot.Start()
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down gracefully...")
}

func initPostgres() *sql.DB {
	host := os.Getenv("POSTGRES_HOST")
	if host == "" {
		host = "localhost"
	}
	port := os.Getenv("POSTGRES_PORT")
	if port == "" {
		port = "5432"
	}

	connStr := fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable",
		os.Getenv("POSTGRES_USER"),
		os.Getenv("POSTGRES_PASSWORD"),
		host,
		port,
		os.Getenv("POSTGRES_DB"),
	)

	db, err := sql.Open("postgres", connStr)
	if err != nil {
		log.Fatalf("failed to initialize db driver: %v", err)
	}
	if err = db.Ping(); err != nil {
		log.Fatalf("failed to ping database: %v", err)
	}
	log.Println("PostgreSQL connected")
	return db
}

func initRedis() *redis.Client {
	addr := os.Getenv("REDIS_ADDR")
	if addr == "" {
		addr = "localhost:6379"
	}

	rdb := redis.NewClient(&redis.Options{
		Addr: addr,
		DB:   0,
	})

	if err := rdb.Ping(context.Background()).Err(); err != nil {
		log.Fatalf("failed to connect to redis: %v", err)
	}
	log.Println("Redis connected")
	return rdb
}

func getEnvOrFatal(key string) string {
	val := os.Getenv(key)
	if val == "" {
		log.Fatalf("environment variable %s is required", key)
	}
	return val
}
