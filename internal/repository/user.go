package repository

import (
	"database/sql"
	"fmt"
	"gachabot/internal/models"
)

func (r *PostgresRepo) GetUserByID(internalID int64) (*models.User, error) {
	user := &models.User{}
	query := `
		SELECT id, telegram_id, discord_id, username,
           COALESCE(first_name, '') as first_name,
           COALESCE(last_name, '') as last_name,
           balance, streak_days, last_roll_time, last_streak_date,
           premium_rolls, COALESCE(language_code, ''), is_adult, COALESCE(avatar_url, '')
		FROM users WHERE id=$1 LIMIT 1
	`
	err := r.db.QueryRow(query, internalID).Scan(
		&user.ID, &user.TelegramID, &user.DiscordID, &user.Username, &user.FirstName, &user.LastName,
		&user.Balance, &user.StreakDays, &user.LastRollTime, &user.LastStreakDate, &user.PremiumRolls, &user.LanguageCode, &user.IsAdult, &user.AvatarURL,
	)
	return user, err
}

// UpdateUserAvatar stores the player's avatar URL (from the auth provider).
func (r *PostgresRepo) UpdateUserAvatar(internalID int64, url string) error {
	_, err := r.db.Exec(`UPDATE users SET avatar_url = $1 WHERE id = $2`, url, internalID)
	return err
}

func (r *PostgresRepo) GetUserByTelegramID(tgID int64) (*models.User, error) {
	user := &models.User{}
	query := `
		SELECT id, telegram_id, discord_id, username, 
           COALESCE(first_name, '') as first_name, 
           COALESCE(last_name, '') as last_name, 
           balance, streak_days, last_roll_time, last_streak_date, 
           premium_rolls, COALESCE(language_code, ''), is_adult
		FROM users WHERE telegram_id=$1 LIMIT 1
	`
	err := r.db.QueryRow(query, tgID).Scan(
		&user.ID, &user.TelegramID, &user.DiscordID, &user.Username, &user.FirstName, &user.LastName,
		&user.Balance, &user.StreakDays, &user.LastRollTime, &user.LastStreakDate, &user.PremiumRolls, &user.LanguageCode, &user.IsAdult,
	)
	return user, err
}

func (r *PostgresRepo) GetOrCreateUserByTelegramID(tgID int64, username, firstName, lastName string) (*models.User, error) {
	user := &models.User{}
	query := `
		INSERT INTO users (telegram_id, username, first_name, last_name) 
		VALUES ($1, $2, $3, $4) 
		ON CONFLICT (telegram_id) DO UPDATE SET
			username = EXCLUDED.username,
			first_name = EXCLUDED.first_name,
			last_name = EXCLUDED.last_name
		RETURNING id, telegram_id, discord_id, username,
		COALESCE(first_name, '') as first_name, 
		          COALESCE(last_name, '') as last_name, 
		          balance, streak_days, last_roll_time, last_streak_date, 
		          premium_rolls, COALESCE(language_code, ''), is_adult
	`
	err := r.db.QueryRow(query, tgID, username, firstName, lastName).Scan(
		&user.ID, &user.TelegramID, &user.DiscordID, &user.Username, &user.FirstName, &user.LastName,
		&user.Balance, &user.StreakDays, &user.LastRollTime, &user.LastStreakDate, &user.PremiumRolls, &user.LanguageCode, &user.IsAdult,
	)

	return user, err
}

func (r *PostgresRepo) SetUserLanguage(internalID int64, langCode string) error {
	query := `UPDATE users SET language_code = $1 WHERE id = $2`
	_, err := r.db.Exec(query, langCode, internalID)
	return err
}

func (r *PostgresRepo) UpdateUserAfterRoll(user *models.User) error {
	query := `
       UPDATE users 
       SET balance = $1, 
           streak_days = $2, 
           last_roll_time = $3, 
           last_streak_date = $4,
           premium_rolls = $5 
       WHERE id = $6
    `
	_, err := r.db.Exec(query, user.Balance, user.StreakDays, user.LastRollTime, user.LastStreakDate, user.PremiumRolls, user.ID)
	return err
}

func (r *PostgresRepo) GetUserByUsername(username string) (*models.User, error) {
	user := &models.User{}
	query := "SELECT id, telegram_id, discord_id, balance, streak_days, last_roll_time, last_streak_date FROM users WHERE username ILIKE $1 LIMIT 1"
	err := r.db.QueryRow(query, username).Scan(
		&user.ID, &user.TelegramID, &user.DiscordID, &user.Balance, &user.StreakDays, &user.LastRollTime, &user.LastStreakDate,
	)
	return user, err
}

func (r *PostgresRepo) AddPremiumRollsByTelegramID(tgID int64, amount int) error {
	_, err := r.db.Exec("UPDATE users SET premium_rolls = premium_rolls + $1 WHERE telegram_id = $2", amount, tgID)
	return err
}

func (r *PostgresRepo) AddPremiumRolls(internalID int64, amount int) error {
	_, err := r.db.Exec("UPDATE users SET premium_rolls = premium_rolls + $1 WHERE id = $2", amount, internalID)
	return err
}

func (r *PostgresRepo) SetUserAdultStatus(internalID int64, isAdult bool) error {
	query := `UPDATE users SET is_adult = $1 WHERE id = $2`
	_, err := r.db.Exec(query, isAdult, internalID)
	return err
}

func (r *PostgresRepo) GetOrCreateUserByDiscordID(discordID int64, username string) (*models.User, error) {
	user := &models.User{}
	query := `
		INSERT INTO users (discord_id, username) 
		VALUES ($1, $2) 
		ON CONFLICT (discord_id) DO UPDATE SET
			username = EXCLUDED.username
		RETURNING id, telegram_id, discord_id, username,
		COALESCE(first_name, '') as first_name, 
		          COALESCE(last_name, '') as last_name, 
		          balance, streak_days, last_roll_time, last_streak_date, 
		          premium_rolls, COALESCE(language_code, ''), is_adult
	`
	err := r.db.QueryRow(query, discordID, username).Scan(
		&user.ID, &user.TelegramID, &user.DiscordID, &user.Username, &user.FirstName, &user.LastName,
		&user.Balance, &user.StreakDays, &user.LastRollTime, &user.LastStreakDate, &user.PremiumRolls, &user.LanguageCode, &user.IsAdult,
	)

	return user, err
}

func (r *PostgresRepo) LinkDiscordAccount(internalUserID int64, discordID int64) error {
	var existingID int64
	err := r.db.QueryRow("SELECT id FROM users WHERE discord_id = $1", discordID).Scan(&existingID)
	if err == nil && existingID != internalUserID {
		return fmt.Errorf("this Discord account is already linked to another profile.")
	}

	_, err = r.db.Exec("UPDATE users SET discord_id = $1 WHERE id = $2", discordID, internalUserID)
	return err
}

func (r *PostgresRepo) LinkAccountsOverwrite(keepID int64, deleteID int64) error {
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var delTgID, delDsID sql.NullInt64
	err = tx.QueryRow(`SELECT telegram_id, discord_id FROM users WHERE id = $1`, deleteID).Scan(&delTgID, &delDsID)
	if err != nil {
		return err
	}

	_, err = tx.Exec("DELETE FROM users WHERE id = $1", deleteID)
	if err != nil {
		return err
	}

	_, err = tx.Exec(`
		UPDATE users 
		SET 
			telegram_id = COALESCE(telegram_id, $1),
			discord_id = COALESCE(discord_id, $2)
		WHERE id = $3`,
		delTgID, delDsID, keepID)
	if err != nil {
		return err
	}

	return tx.Commit()
}
