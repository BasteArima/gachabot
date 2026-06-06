package main

import (
	"context"
	"database/sql"
	"log"
	"os"
	"os/signal"
	"syscall"

	"gachabot/internal/config"
	"gachabot/internal/delivery/discord"
	httpapi "gachabot/internal/delivery/httpapi"
	"gachabot/internal/delivery/telegram"
	"gachabot/internal/i18n"
	"gachabot/internal/migrations"
	"gachabot/internal/repository"
	"gachabot/internal/service/backup"
	"gachabot/internal/service/duel"
	"gachabot/internal/service/gacha"
	"gachabot/internal/service/spawn"
	"gachabot/internal/service/suggest"

	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
	"github.com/redis/go-redis/v9"
)

func main() {
	_ = godotenv.Load()

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	db := initPostgres(cfg.Postgres)
	defer db.Close()

	if err := migrations.Run(db); err != nil {
		log.Fatalf("failed to run migrations: %v", err)
	}

	rdb := initRedis(cfg.RedisAddr)
	defer rdb.Close()

	repo := repository.NewPostgresRepo(db)
	gachaService := gacha.NewGachaService(repo, rdb, cfg.Telegram.AdminID, cfg.Game.CooldownDuration, cfg.Game.DuplicatesEnabled)
	duelService := duel.NewDuelService(repo, rdb)
	suggestService := suggest.NewSuggestService(repo, rdb)
	spawnService := spawn.NewSpawnService(repo, rdb, gachaService)

	tgLoc, err := i18n.NewLocalizer("locales/base", "locales/telegram", "ru")
	if err != nil {
		log.Fatalf("failed to load telegram localization: %v", err)
	}

	tgBot, err := telegram.NewBot(repo, rdb, gachaService, duelService, suggestService, spawnService, tgLoc, cfg.Telegram)
	if err != nil {
		log.Fatalf("failed to create telegram bot: %v", err)
	}
	spawnService.RegisterSpawner(spawn.PlatformTelegram, tgBot)

	if cfg.Discord.Token != "" {
		discordLoc, err := i18n.NewLocalizer("locales/base", "locales/discord", "ru")
		if err != nil {
			log.Fatalf("failed to load discord localization: %v", err)
		}

		dsBot, err := discord.NewBot(cfg.Discord.Token, repo, rdb, gachaService, duelService, suggestService, spawnService, discordLoc, tgBot, tgBot.NotifyAdmin)
		if err != nil {
			log.Fatalf("failed to create discord bot: %v", err)
		}
		spawnService.RegisterSpawner(spawn.PlatformDiscord, dsBot)

		if err := dsBot.Start(); err != nil {
			log.Printf("failed to start discord bot: %v", err)
		} else {
			log.Println("Discord bot started successfully")
		}
	} else {
		log.Println("DISCORD_TOKEN not found, skipping discord bot startup")
	}

	backupService := backup.NewBackupService(cfg.Backup.Hour, cfg.Backup.Minute, "/backups/daily", tgBot)
	backupService.Start()

	go func() {
		log.Println("Telegram bot is running...")
		tgBot.Start()
	}()

	spawnService.Start()

	webServer := httpapi.NewServer(repo, rdb, cfg.Telegram.Token, cfg.Telegram.AdminID, cfg.HTTP, cfg.Discord)
	webServer.Start()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down gracefully...")
}

func initPostgres(cfg config.PostgresConfig) *sql.DB {
	db, err := sql.Open("postgres", cfg.DSN())
	if err != nil {
		log.Fatalf("failed to initialize db driver: %v", err)
	}
	if err = db.Ping(); err != nil {
		log.Fatalf("failed to ping database: %v", err)
	}
	log.Println("PostgreSQL connected")
	return db
}

func initRedis(addr string) *redis.Client {
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
