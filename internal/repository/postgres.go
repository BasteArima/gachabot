package repository

import (
	"database/sql"
	"fmt"
	"gachabot/internal/models" // Подставь свой путь из go.mod
	"log"
)

// Структура репозитория хранит подключение к БД
type PostgresRepo struct {
	db *sql.DB
}

func NewPostgresRepo(db *sql.DB) *PostgresRepo {
	return &PostgresRepo{db: db}
}

// Получаем пользователя по ВНУТРЕННЕМУ ID
func (r *PostgresRepo) GetUserByID(internalID int64) (*models.User, error) {
	user := &models.User{}
	query := `
		SELECT id, telegram_id, discord_id, username, 
           COALESCE(first_name, '') as first_name, 
           COALESCE(last_name, '') as last_name, 
           balance, streak_days, last_roll_time, last_streak_date, 
           premium_rolls, COALESCE(language_code, '')
		FROM users WHERE id=$1 LIMIT 1
	`
	err := r.db.QueryRow(query, internalID).Scan(
		&user.ID, &user.TelegramID, &user.DiscordID, &user.Username, &user.FirstName, &user.LastName,
		&user.Balance, &user.StreakDays, &user.LastRollTime, &user.LastStreakDate, &user.PremiumRolls, &user.LanguageCode,
	)
	return user, err
}

// 1. Получаем пользователя по Telegram ID
func (r *PostgresRepo) GetUserByTelegramID(tgID int64) (*models.User, error) {
	user := &models.User{}
	query := `
		SELECT id, telegram_id, discord_id, username, 
           COALESCE(first_name, '') as first_name, 
           COALESCE(last_name, '') as last_name, 
           balance, streak_days, last_roll_time, last_streak_date, 
           premium_rolls, COALESCE(language_code, '')
		FROM users WHERE telegram_id=$1 LIMIT 1
	`
	err := r.db.QueryRow(query, tgID).Scan(
		&user.ID, &user.TelegramID, &user.DiscordID, &user.Username, &user.FirstName, &user.LastName,
		&user.Balance, &user.StreakDays, &user.LastRollTime, &user.LastStreakDate, &user.PremiumRolls, &user.LanguageCode,
	)
	return user, err
}

// 2. Умная функция: ищет юзера по ТГ, а если нет — создает и возвращает полный профиль с внутренним ID
func (r *PostgresRepo) GetOrCreateUserByTelegramID(tgID int64, username, firstName, lastName string) (*models.User, error) {
	user := &models.User{}

	// Пытаемся вставить, если конфликтует по telegram_id - обновляем ники
	// RETURNING возвращает нам все поля, включая свежесгенерированный внутренний id!
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
		          premium_rolls, COALESCE(language_code, '')
	`
	err := r.db.QueryRow(query, tgID, username, firstName, lastName).Scan(
		&user.ID, &user.TelegramID, &user.DiscordID, &user.Username, &user.FirstName, &user.LastName,
		&user.Balance, &user.StreakDays, &user.LastRollTime, &user.LastStreakDate, &user.PremiumRolls, &user.LanguageCode,
	)

	return user, err
}

// 3. Сохранение языка теперь привязано к внутреннему ID
func (r *PostgresRepo) SetUserLanguage(internalID int64, langCode string) error {
	query := `UPDATE users SET language_code = $1 WHERE id = $2`
	_, err := r.db.Exec(query, langCode, internalID)
	return err
}

// 4. Обновление прогресса теперь ИСКЛЮЧИТЕЛЬНО по внутреннему ID
func (r *PostgresRepo) UpdateUserAfterRoll(user *models.User) error {
	query := `
       UPDATE users 
       SET balance = $1, 
           streak_days = $2, 
           last_roll_time = $3, 
           last_streak_date = $4,
           premium_rolls = $5 
       WHERE id = $6 -- <--- ИСПОЛЬЗУЕМ ВНУТРЕННИЙ ID
    `
	_, err := r.db.Exec(query, user.Balance, user.StreakDays, user.LastRollTime, user.LastStreakDate, user.PremiumRolls, user.ID)
	return err
}

// 5. Поиск по юзернейму (для дуэлей). Теперь тоже достает внутренний ID
func (r *PostgresRepo) GetUserByUsername(username string) (*models.User, error) {
	user := &models.User{}
	query := "SELECT id, telegram_id, discord_id, balance, streak_days, last_roll_time, last_streak_date FROM users WHERE username ILIKE $1 LIMIT 1"
	err := r.db.QueryRow(query, username).Scan(
		&user.ID, &user.TelegramID, &user.DiscordID, &user.Balance, &user.StreakDays, &user.LastRollTime, &user.LastStreakDate,
	)
	return user, err
}

// 6. Выдача доната из ТГ
func (r *PostgresRepo) AddPremiumRollsByTelegramID(tgID int64, amount int) error {
	_, err := r.db.Exec("UPDATE users SET premium_rolls = premium_rolls + $1 WHERE telegram_id = $2", amount, tgID)
	return err
}

func (r *PostgresRepo) GetRarities() ([]models.Rarity, error) {
	var rarities []models.Rarity
	rows, err := r.db.Query("SELECT id, name, drop_chance, base_reward, pity_threshold FROM rarities ORDER BY id ASC")
	if err != nil {
		return nil, err // Возвращаем ошибку!
	}
	defer rows.Close()

	for rows.Next() {
		var rarity models.Rarity
		// Обязательно проверяем ошибку Scan
		if err := rows.Scan(&rarity.ID, &rarity.Name, &rarity.DropChance, &rarity.BaseReward, &rarity.PityThreshold); err != nil {
			return nil, err
		}
		// append динамически увеличивает массив! Нам не важно, сколько редкостей в БД.
		rarities = append(rarities, rarity)
	}
	return rarities, nil
}

func (r *PostgresRepo) GetRandomCard(rarityID int) (*models.Card, error) {
	card := &models.Card{}
	query := "SELECT id, name, rarity_id, image_url, power_level FROM cards WHERE rarity_id=$1 ORDER BY RANDOM() LIMIT 1"
	err := r.db.QueryRow(query, rarityID).Scan(
		&card.ID,
		&card.Name,
		&card.RarityID,
		&card.ImageURL,
		&card.PowerLevel,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			log.Println("Карта не найдена")
			return nil, fmt.Errorf("Карта не найдена: %w", err)
		}
		log.Printf("Ошибка БД: %v", err)
		return nil, fmt.Errorf("Ошибка БД: %w", err)
	}
	return card, nil
}

// AddCardToInventory добавляет новую карту или увеличивает счетчик существующей
func (r *PostgresRepo) AddCardToInventory(userID int64, cardID int) error {
	query := `
		INSERT INTO user_inventory (user_id, card_id, quantity) 
		VALUES ($1, $2, 1) 
		ON CONFLICT (user_id, card_id) 
		DO UPDATE SET quantity = user_inventory.quantity + 1
	`
	_, err := r.db.Exec(query, userID, cardID)
	return err
}

// AddFragment добавляет осколок и сразу возвращает их новое количество
func (r *PostgresRepo) AddFragment(userID int64, cardID int) (int, error) {
	var currentFragments int
	query := `
		INSERT INTO user_fragments (user_id, card_id, quantity) 
		VALUES ($1, $2, 1) 
		ON CONFLICT (user_id, card_id) 
		DO UPDATE SET quantity = user_fragments.quantity + 1
		RETURNING quantity
	`
	err := r.db.QueryRow(query, userID, cardID).Scan(&currentFragments)
	return currentFragments, err
}

// ClearFragments удаляет записи об осколках конкретной карты у пользователя
func (r *PostgresRepo) ClearFragments(userID int64, cardID int) error {
	query := "DELETE FROM user_fragments WHERE user_id=$1 AND card_id=$2"
	_, err := r.db.Exec(query, userID, cardID)
	return err
}

// Получаем общее количество существующих карт
func (r *PostgresRepo) GetTotalCardsCount() (int, error) {
	var count int
	err := r.db.QueryRow("SELECT COUNT(*) FROM cards").Scan(&count)
	return count, err
}

// Получаем количество уникальных собранных карт у игрока
func (r *PostgresRepo) GetUserUniqueCardsCount(userID int64) (int, error) {
	var count int
	// DISTINCT нужен, чтобы не считать дубликаты одной и той же карты
	err := r.db.QueryRow("SELECT COUNT(DISTINCT card_id) FROM user_inventory WHERE user_id = $1", userID).Scan(&count)
	return count, err
}

// Получаем одну карточку пользователя с учетом сдвига (пагинация)
// Получаем одну карточку пользователя с учетом сдвига (пагинация)
func (r *PostgresRepo) GetUserCard(userID int64, offset int) (*models.UserCardView, error) {
	card := &models.UserCardView{}
	// ДОБАВЛЕН c.power_level В SELECT
	query := `
		SELECT c.name, r.name, c.image_url, ui.quantity, c.power_level 
		FROM user_inventory ui
		JOIN cards c ON ui.card_id = c.id
		JOIN rarities r ON c.rarity_id = r.id
		WHERE ui.user_id = $1
		ORDER BY r.id DESC, c.id ASC
		OFFSET $2 LIMIT 1
	`
	// ДОБАВЛЕН &card.PowerLevel В Scan
	err := r.db.QueryRow(query, userID, offset).Scan(
		&card.CardName, &card.RarityName, &card.ImageURL, &card.Quantity, &card.PowerLevel,
	)
	return card, err
}

// Получаем все счетчики гарантов пользователя
func (r *PostgresRepo) GetUserPities(userID int64) (map[int]int, error) {
	pities := make(map[int]int)
	rows, err := r.db.Query("SELECT rarity_id, counter FROM user_pity WHERE user_id = $1", userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var rarityID, counter int
		if err := rows.Scan(&rarityID, &counter); err != nil {
			return nil, err
		}
		pities[rarityID] = counter
	}
	return pities, nil
}

// Обновляем счетчики. Используем новую фишку EXCLUDED!
func (r *PostgresRepo) UpdateUserPities(userID int64, pities map[int]int) error {
	// EXCLUDED.counter берет значение, которое мы пытались вставить, и использует его для обновления
	query := `
		INSERT INTO user_pity (user_id, rarity_id, counter) 
		VALUES ($1, $2, $3)
		ON CONFLICT (user_id, rarity_id) 
		DO UPDATE SET counter = EXCLUDED.counter
	`
	for rarityID, counter := range pities {
		if _, err := r.db.Exec(query, userID, rarityID, counter); err != nil {
			return err
		}
	}
	return nil
}

// Запоминает, что юзер активен в конкретном чате
func (r *PostgresRepo) TrackUserChat(userID int64, chatID int64) error {
	// Если это личные сообщения (ID чата совпадает с ID юзера), нам это не нужно в локальном топе
	if userID == chatID {
		return nil
	}
	_, err := r.db.Exec("INSERT INTO user_chats (user_id, chat_id) VALUES ($1, $2) ON CONFLICT DO NOTHING", userID, chatID)
	return err
}

func (r *PostgresRepo) GetLeaderboard(criteria string, chatID int64) ([]models.LeaderboardEntry, error) {
	var query string
	var args []interface{}

	joinChat := ""
	whereChat := ""
	if chatID != 0 {
		// ИСПРАВЛЕНО: соединяем по внутреннему id (u.id), а не по tg_id
		joinChat = "JOIN user_chats uc ON u.id = uc.user_id"
		whereChat = "WHERE uc.chat_id = $1"
		args = append(args, chatID)
	}

	// ИСПРАВЛЕНО: для Дискорд-юзеров first_name может быть пустым,
	// поэтому берем username как запасной вариант
	nameFormat := "COALESCE(NULLIF(u.first_name, ''), u.username)"

	switch criteria {
	case "balance":
		query = fmt.Sprintf(`SELECT %s, u.balance FROM users u %s %s ORDER BY u.balance DESC, u.last_roll_time ASC LIMIT 10`, nameFormat, joinChat, whereChat)
	case "streak":
		query = fmt.Sprintf(`SELECT %s, u.streak_days FROM users u %s %s ORDER BY u.streak_days DESC, u.last_roll_time ASC LIMIT 10`, nameFormat, joinChat, whereChat)
	case "cards":
		// Если есть chatID ($1), он должен быть передан в Query.
		// В блоке ниже важно сохранить порядок аргументов.
		query = fmt.Sprintf(`
          SELECT %s, COUNT(DISTINCT ui.card_id) as val 
          FROM users u 
          %s 
          JOIN user_inventory ui ON u.id = ui.user_id 
          %s 
          GROUP BY u.id, u.first_name, u.username 
          ORDER BY val DESC, MIN(u.last_roll_time) ASC LIMIT 10`, nameFormat, joinChat, whereChat)
	default:
		return nil, fmt.Errorf("неизвестный критерий топа")
	}

	rows, err := r.db.Query(query, args...)
	if err != nil {
		log.Printf("[DB ERROR] Leaderboard query failed: %v", err) // Поможет увидеть ошибку в консоли
		return nil, err
	}
	defer rows.Close()

	var board []models.LeaderboardEntry
	for rows.Next() {
		var entry models.LeaderboardEntry
		if err := rows.Scan(&entry.DisplayName, &entry.Value); err != nil {
			return nil, err
		}
		board = append(board, entry)
	}
	return board, nil
}

// Достаем одну случайную карту из инвентаря юзера
func (r *PostgresRepo) GetRandomUserCard(userID int64) (*models.Card, error) {
	card := &models.Card{}
	// JOIN, чтобы вытащить всю инфу о карте из инвентаря. ORDER BY RANDOM() LIMIT 1 делает всю магию.
	query := `
		SELECT c.id, c.name, c.rarity_id, c.image_url, c.power_level 
		FROM user_inventory ui
		JOIN cards c ON ui.card_id = c.id
		WHERE ui.user_id = $1
		ORDER BY RANDOM() LIMIT 1
	`
	err := r.db.QueryRow(query, userID).Scan(
		&card.ID, &card.Name, &card.RarityID, &card.ImageURL, &card.PowerLevel,
	)
	return card, err
}

// Получает количество доступных дубликатов для каждой редкости
func (r *PostgresRepo) GetAvailableCrafts(userID int64) (map[int]int, error) {
	counts := make(map[int]int)
	query := `
       SELECT c.rarity_id, SUM(ui.quantity - 1) 
       FROM user_inventory ui
       JOIN cards c ON ui.card_id = c.id
       WHERE ui.user_id = $1 AND ui.quantity > 1
       GROUP BY c.rarity_id
    `
	rows, err := r.db.Query(query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var rarityID, dupCount int
		if err := rows.Scan(&rarityID, &dupCount); err == nil {
			counts[rarityID] = dupCount
		}
	}
	return counts, nil
}

// "Сжигает" 5 лишних копий карт определенной редкости
func (r *PostgresRepo) ConsumeDuplicates(userID int64, rarityID int, amount int) error {
	// Достаем список карт этой редкости, где количество > 1
	rows, err := r.db.Query(`
		SELECT card_id, quantity FROM user_inventory ui
		JOIN cards c ON ui.card_id = c.id
		WHERE ui.user_id = $1 AND c.rarity_id = $2 AND ui.quantity > 1
	`, userID, rarityID)
	if err != nil {
		return err
	}
	defer rows.Close()

	toBurn := amount
	for rows.Next() && toBurn > 0 {
		var cardID, quantity int
		rows.Scan(&cardID, &quantity)

		canTake := quantity - 1 // Оставляем всегда 1 оригинал
		if canTake > toBurn {
			canTake = toBurn
		}

		_, _ = r.db.Exec("UPDATE user_inventory SET quantity = quantity - $1 WHERE user_id = $2 AND card_id = $3",
			canTake, userID, cardID)

		toBurn -= canTake
	}
	return nil
}

// Получаем общее количество дубликатов (все копии минус 1 для каждой карты)
func (r *PostgresRepo) GetTotalDuplicatesCount(userID int64) (int, error) {
	var count int
	// Суммируем (количество - 1) для всех записей, где больше 1 карты
	query := "SELECT COALESCE(SUM(quantity - 1), 0) FROM user_inventory WHERE user_id = $1 AND quantity > 1"
	err := r.db.QueryRow(query, userID).Scan(&count)
	return count, err
}

// Добавляем премиум-крутки после доната
func (r *PostgresRepo) AddPremiumRolls(internalID int64, amount int) error {
	_, err := r.db.Exec("UPDATE users SET premium_rolls = premium_rolls + $1 WHERE id = $2", amount, internalID)
	return err
}

// --- DISCORD И ПРИВЯЗКА АККАУНТОВ ---

// Получаем или создаем юзера по Discord ID
func (r *PostgresRepo) GetOrCreateUserByDiscordID(discordID int64, username string) (*models.User, error) {
	user := &models.User{}

	// Точно такая же логика, как для ТГ, но для discord_id
	query := `
		INSERT INTO users (discord_id, username) 
		VALUES ($1, $2) 
		ON CONFLICT (discord_id) DO UPDATE SET
			username = EXCLUDED.username
		RETURNING id, telegram_id, discord_id, username,
		COALESCE(first_name, '') as first_name, 
		          COALESCE(last_name, '') as last_name, 
		          balance, streak_days, last_roll_time, last_streak_date, 
		          premium_rolls, COALESCE(language_code, '')
	`
	err := r.db.QueryRow(query, discordID, username).Scan(
		&user.ID, &user.TelegramID, &user.DiscordID, &user.Username, &user.FirstName, &user.LastName,
		&user.Balance, &user.StreakDays, &user.LastRollTime, &user.LastStreakDate, &user.PremiumRolls, &user.LanguageCode,
	)

	return user, err
}

// Привязка Discord к существующему внутреннему ID
func (r *PostgresRepo) LinkDiscordAccount(internalUserID int64, discordID int64) error {
	// Проверяем, не привязан ли уже этот discordID к кому-то еще
	var existingID int64
	err := r.db.QueryRow("SELECT id FROM users WHERE discord_id = $1", discordID).Scan(&existingID)
	if err == nil && existingID != internalUserID {
		return fmt.Errorf("этот Discord аккаунт уже привязан к другому профилю")
	}

	// Если всё ок, обновляем запись
	_, err = r.db.Exec("UPDATE users SET discord_id = $1 WHERE id = $2", discordID, internalUserID)
	return err
}

// LinkAccountsOverwrite переносит привязки на аккаунт keepID, а deleteID полностью удаляет
func (r *PostgresRepo) LinkAccountsOverwrite(keepID int64, deleteID int64) error {
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// 1. Достаем telegram_id и discord_id у удаляемого аккаунта в память программы
	var delTgID, delDsID sql.NullInt64
	err = tx.QueryRow(`SELECT telegram_id, discord_id FROM users WHERE id = $1`, deleteID).Scan(&delTgID, &delDsID)
	if err != nil {
		return err
	}

	// 2. СНАЧАЛА удаляем аккаунт-донор.
	// Это освобождает уникальные discord_id и telegram_id для дальнейшего использования.
	// Каскадное удаление (ON DELETE CASCADE) автоматически вычистит все его карты и гарант.
	_, err = tx.Exec("DELETE FROM users WHERE id = $1", deleteID)
	if err != nil {
		return err
	}

	// 3. ТЕПЕРЬ спокойно обновляем сохраняемый аккаунт. Конфликта уникальности больше нет!
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
