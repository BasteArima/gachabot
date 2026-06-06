package config

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds every runtime setting, loaded and validated once at startup.
// Nothing else in the codebase should read os.Getenv directly — pass the relevant
// fields down instead, so configuration has a single source of truth.
type Config struct {
	Telegram  TelegramConfig
	Discord   DiscordConfig
	Postgres  PostgresConfig
	RedisAddr string
	Game      GameConfig
	Backup    BackupConfig
	HTTP      HTTPConfig
}

type TelegramConfig struct {
	Token           string
	AdminID         int64
	SuggestsGroupID int64
	BackupGroupID   int64
	Require18Plus   bool
	GroupLink       string
	StartBannerURL  string
}

type DiscordConfig struct {
	Token string
	// OAuth/Activity credentials (used by the web layer, not the bot gateway).
	ClientID      string
	ClientSecret  string
	OAuthRedirect string
}

// HTTPConfig configures the web/API delivery layer (TMA, Discord Activity, browser).
type HTTPConfig struct {
	Port      string // listen port, e.g. "8080"
	StaticDir string // built SPA directory to serve; empty = API only
	WebAppURL string // public HTTPS URL of the web app (for buttons/redirects)
	// DevAllowNoAuth lets unauthenticated requests act as the admin user — local dev only.
	DevAllowNoAuth bool
}

type PostgresConfig struct {
	User     string
	Password string
	Host     string
	Port     string
	DB       string
}

// PostgresFromEnv reads only the database settings from the environment, applying
// defaults. Useful for tools (e.g. the theme seeder) that need the DB but not the
// full bot configuration.
func PostgresFromEnv() PostgresConfig {
	return PostgresConfig{
		User:     os.Getenv("POSTGRES_USER"),
		Password: os.Getenv("POSTGRES_PASSWORD"),
		Host:     getEnv("POSTGRES_HOST", "localhost"),
		Port:     getEnv("POSTGRES_PORT", "5432"),
		DB:       os.Getenv("POSTGRES_DB"),
	}
}

// DSN builds the libpq connection string.
func (p PostgresConfig) DSN() string {
	return fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable",
		p.User, p.Password, p.Host, p.Port, p.DB)
}

type GameConfig struct {
	CooldownDuration time.Duration
	// DuplicatesEnabled: if false, players never receive a card they already own —
	// rolls yield only unowned cards (or points once everything is collected), and
	// duplicate-based mechanics (crafting, duplicate counters) are disabled.
	DuplicatesEnabled bool
}

type BackupConfig struct {
	Hour   int
	Minute int
}

// Load reads configuration from the environment, applies defaults, validates
// required values and returns an error if a required value is missing. Malformed
// optional values are logged and fall back to their defaults rather than aborting.
func Load() (*Config, error) {
	cfg := &Config{
		Telegram: TelegramConfig{
			Token:           os.Getenv("TELEGRAM_BOT_TOKEN"),
			AdminID:         getEnvInt64("ADMIN_TELEGRAM_ID", 0),
			SuggestsGroupID: getEnvInt64("ADMIN_TG_SUGGESTS_GROUP_ID", 0),
			BackupGroupID:   getEnvInt64("ADMIN_TG_BACKUP_GROUP_ID", 0),
			Require18Plus:   os.Getenv("REQUIRE_18_PLUS_CONFIRM") == "true",
			GroupLink:       os.Getenv("ADD_BOT_TO_GROUP_LINK"),
			StartBannerURL:  os.Getenv("START_BANNER_URL"),
		},
		Discord: DiscordConfig{
			Token:         os.Getenv("DISCORD_TOKEN"),
			ClientID:      os.Getenv("DISCORD_CLIENT_ID"),
			ClientSecret:  os.Getenv("DISCORD_CLIENT_SECRET"),
			OAuthRedirect: os.Getenv("DISCORD_OAUTH_REDIRECT"),
		},
		Postgres:  PostgresFromEnv(),
		RedisAddr: getEnv("REDIS_ADDR", "localhost:6379"),
		Game: GameConfig{
			CooldownDuration:  time.Duration(getEnvInt("COOLDOWN_HOURS", 3)) * time.Hour,
			DuplicatesEnabled: getEnvBool("ENABLE_DUPLICATES", true),
		},
		Backup: BackupConfig{
			Hour:   getEnvInt("BACKUP_TIME_HOUR", 3),
			Minute: getEnvInt("BACKUP_TIME_MINUTE", 10),
		},
		HTTP: HTTPConfig{
			Port:           getEnv("HTTP_PORT", "8080"),
			StaticDir:      os.Getenv("HTTP_STATIC_DIR"),
			WebAppURL:      os.Getenv("WEB_APP_URL"),
			DevAllowNoAuth: getEnvBool("DEV_ALLOW_NO_AUTH", false),
		},
	}

	if cfg.Telegram.Token == "" {
		return nil, fmt.Errorf("TELEGRAM_BOT_TOKEN is required")
	}
	if cfg.Postgres.User == "" || cfg.Postgres.DB == "" {
		return nil, fmt.Errorf("POSTGRES_USER and POSTGRES_DB are required")
	}

	return cfg, nil
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	raw := os.Getenv(key)
	if raw == "" {
		return fallback
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		log.Printf("Warning: %s=%q is not a valid integer, using default %d", key, raw, fallback)
		return fallback
	}
	return v
}

func getEnvBool(key string, fallback bool) bool {
	raw := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
	if raw == "" {
		return fallback
	}
	return raw == "true" || raw == "1" || raw == "yes" || raw == "on"
}

func getEnvInt64(key string, fallback int64) int64 {
	raw := os.Getenv(key)
	if raw == "" {
		return fallback
	}
	v, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		log.Printf("Warning: %s=%q is not a valid integer, using default %d", key, raw, fallback)
		return fallback
	}
	return v
}
