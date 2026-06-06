package repository

import (
	"gachabot/internal/models"
)

// UpsertChatEnabled registers a chat (or re-enables it) — used when the bot is
// added to a group / a Discord main channel is (re)assigned. Forces spawns on.
func (r *PostgresRepo) UpsertChatEnabled(platform string, chatID int64, guildID *int64, title string) error {
	_, err := r.db.Exec(`
		INSERT INTO chats (platform, chat_id, guild_id, title, spawn_enabled)
		VALUES ($1, $2, $3, $4, TRUE)
		ON CONFLICT (platform, chat_id) DO UPDATE SET
			guild_id = EXCLUDED.guild_id,
			title = EXCLUDED.title,
			spawn_enabled = TRUE`,
		platform, chatID, guildID, title)
	return err
}

// EnsureChat lazily records a chat seen via normal activity. Unlike
// UpsertChatEnabled it never flips spawn_enabled, so an admin's manual disable
// is preserved; it only refreshes the title.
func (r *PostgresRepo) EnsureChat(platform string, chatID int64, guildID *int64, title string) error {
	_, err := r.db.Exec(`
		INSERT INTO chats (platform, chat_id, guild_id, title)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (platform, chat_id) DO UPDATE SET
			title = EXCLUDED.title`,
		platform, chatID, guildID, title)
	return err
}

// SetChatSpawnEnabled toggles spawns for a single chat.
func (r *PostgresRepo) SetChatSpawnEnabled(platform string, chatID int64, enabled bool) error {
	_, err := r.db.Exec(
		`UPDATE chats SET spawn_enabled = $1 WHERE platform = $2 AND chat_id = $3`,
		enabled, platform, chatID)
	return err
}

// DisableGuildChats turns off spawns for every channel of a Discord guild
// (used when the bot is removed from the guild).
func (r *PostgresRepo) DisableGuildChats(guildID int64) error {
	_, err := r.db.Exec(
		`UPDATE chats SET spawn_enabled = FALSE WHERE platform = 'discord' AND guild_id = $1`,
		guildID)
	return err
}

// GuildHasChat reports whether a Discord guild already has a registered channel,
// so GuildCreate (which fires on every reconnect) won't clobber an admin's choice.
func (r *PostgresRepo) GuildHasChat(guildID int64) (bool, error) {
	var exists bool
	err := r.db.QueryRow(
		`SELECT EXISTS(SELECT 1 FROM chats WHERE platform = 'discord' AND guild_id = $1)`,
		guildID).Scan(&exists)
	return exists, err
}

// SetDiscordMainChannel sets the single posting channel for a guild, replacing
// any previously registered channel for that guild.
func (r *PostgresRepo) SetDiscordMainChannel(guildID, channelID int64, title string) error {
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(
		`DELETE FROM chats WHERE platform = 'discord' AND guild_id = $1`, guildID); err != nil {
		return err
	}
	if _, err := tx.Exec(`
		INSERT INTO chats (platform, chat_id, guild_id, title, spawn_enabled)
		VALUES ('discord', $1, $2, $3, TRUE)`,
		channelID, guildID, title); err != nil {
		return err
	}
	return tx.Commit()
}

// GetSpawnChats returns every chat with spawns enabled (used by the spawn engine).
func (r *PostgresRepo) GetSpawnChats() ([]models.Chat, error) {
	rows, err := r.db.Query(
		`SELECT platform, chat_id, guild_id, COALESCE(title, ''), spawn_enabled
		 FROM chats WHERE spawn_enabled = TRUE`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var chats []models.Chat
	for rows.Next() {
		var c models.Chat
		if err := rows.Scan(&c.Platform, &c.ChatID, &c.GuildID, &c.Title, &c.SpawnEnabled); err != nil {
			return nil, err
		}
		chats = append(chats, c)
	}
	return chats, rows.Err()
}
