package repository

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"gachabot/internal/models" // Подставь свой путь из go.mod
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/lib/pq"
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
           premium_rolls, COALESCE(language_code, ''), is_adult
		FROM users WHERE id=$1 LIMIT 1
	`
	err := r.db.QueryRow(query, internalID).Scan(
		&user.ID, &user.TelegramID, &user.DiscordID, &user.Username, &user.FirstName, &user.LastName,
		&user.Balance, &user.StreakDays, &user.LastRollTime, &user.LastStreakDate, &user.PremiumRolls, &user.LanguageCode, &user.IsAdult,
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
           premium_rolls, COALESCE(language_code, ''), is_adult
		FROM users WHERE telegram_id=$1 LIMIT 1
	`
	err := r.db.QueryRow(query, tgID).Scan(
		&user.ID, &user.TelegramID, &user.DiscordID, &user.Username, &user.FirstName, &user.LastName,
		&user.Balance, &user.StreakDays, &user.LastRollTime, &user.LastStreakDate, &user.PremiumRolls, &user.LanguageCode, &user.IsAdult,
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
		          premium_rolls, COALESCE(language_code, ''), is_adult
	`
	err := r.db.QueryRow(query, tgID, username, firstName, lastName).Scan(
		&user.ID, &user.TelegramID, &user.DiscordID, &user.Username, &user.FirstName, &user.LastName,
		&user.Balance, &user.StreakDays, &user.LastRollTime, &user.LastStreakDate, &user.PremiumRolls, &user.LanguageCode, &user.IsAdult,
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
	query := `SELECT id, name, rarity_id, image_url, power_level, set_id FROM cards WHERE rarity_id = $1 ORDER BY RANDOM() LIMIT 1`
	var c models.Card

	err := r.db.QueryRow(query, rarityID).Scan(&c.ID, &c.Name, &c.RarityID, &c.ImageURL, &c.PowerLevel, &c.SetID)
	if err != nil {
		return nil, err
	}
	return &c, nil
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
func (r *PostgresRepo) GetUserCard(userID int64, offset int) (*models.UserCardView, error) {
	query := `
		SELECT c.name, r.name, c.image_url, i.quantity, c.power_level,
		       COALESCE(cs.name, '') as set_name
		FROM user_inventory i
		JOIN cards c ON i.card_id = c.id
		JOIN rarities r ON c.rarity_id = r.id
		LEFT JOIN card_sets cs ON c.set_id = cs.id
		WHERE i.user_id = $1
		ORDER BY c.rarity_id DESC, c.power_level DESC, c.id ASC
		LIMIT 1 OFFSET $2
	`
	var card models.UserCardView
	err := r.db.QueryRow(query, userID, offset).Scan(
		&card.CardName, &card.RarityName, &card.ImageURL,
		&card.Quantity, &card.PowerLevel, &card.SetName, // <-- Добавили &card.SetName
	)
	if err != nil {
		return nil, err
	}
	return &card, nil
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
		joinChat = "JOIN user_chats uc ON u.id = uc.user_id"
		whereChat = "WHERE uc.chat_id = $1"
		args = append(args, chatID)
	}

	// Подключаем таблицу сетов для получения активной ауры
	joinAura := "LEFT JOIN card_sets cs ON u.active_set_id = cs.id"
	nameFormat := "COALESCE(NULLIF(u.first_name, ''), u.username)"

	switch criteria {
	case "balance":
		query = fmt.Sprintf(`SELECT %s, u.balance, COALESCE(cs.name, '') FROM users u %s %s %s ORDER BY u.balance DESC, u.last_roll_time ASC LIMIT 10`, nameFormat, joinChat, joinAura, whereChat)
	case "streak":
		query = fmt.Sprintf(`SELECT %s, u.streak_days, COALESCE(cs.name, '') FROM users u %s %s %s ORDER BY u.streak_days DESC, u.last_roll_time ASC LIMIT 10`, nameFormat, joinChat, joinAura, whereChat)
	case "cards":
		query = fmt.Sprintf(`
			SELECT %s, COUNT(DISTINCT ui.card_id) as val, COALESCE(cs.name, '')
			FROM users u 
			%s %s
			JOIN user_inventory ui ON u.id = ui.user_id 
			%s 
			GROUP BY u.id, u.first_name, u.username, cs.name 
			ORDER BY val DESC, MIN(u.last_roll_time) ASC LIMIT 10`, nameFormat, joinChat, joinAura, whereChat)
	default:
		return nil, fmt.Errorf("неизвестный критерий топа")
	}

	rows, err := r.db.Query(query, args...)
	if err != nil {
		log.Printf("[DB ERROR] Leaderboard query failed: %v", err)
		return nil, err
	}
	defer rows.Close()

	var board []models.LeaderboardEntry
	for rows.Next() {
		var entry models.LeaderboardEntry
		// Считываем ауру 3-им параметром
		if err := rows.Scan(&entry.DisplayName, &entry.Value, &entry.ActiveAura); err != nil {
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
		          premium_rolls, COALESCE(language_code, ''), is_adult
	`
	err := r.db.QueryRow(query, discordID, username).Scan(
		&user.ID, &user.TelegramID, &user.DiscordID, &user.Username, &user.FirstName, &user.LastName,
		&user.Balance, &user.StreakDays, &user.LastRollTime, &user.LastStreakDate, &user.PremiumRolls, &user.LanguageCode, &user.IsAdult,
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

// Создание промокода (для админ-панели)
func (r *PostgresRepo) CreatePromoCode(code string, reward models.PromoReward, maxUses *int, expiresAt *time.Time) error {
	code = strings.ToUpper(strings.TrimSpace(code))
	rewardJSON, err := json.Marshal(reward)
	if err != nil {
		return err
	}

	_, err = r.db.Exec(`
		INSERT INTO promocodes (code, reward_json, max_uses, expires_at) 
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (code) DO UPDATE SET reward_json = $2, max_uses = $3, expires_at = $4`,
		code, rewardJSON, maxUses, expiresAt)
	return err
}

// Активация промокода (Транзакция)
func (r *PostgresRepo) RedeemPromo(userID int64, code string) (*models.PromoReward, []models.Card, error) {
	code = strings.ToUpper(strings.TrimSpace(code))
	tx, err := r.db.Begin()
	if err != nil {
		return nil, nil, err
	}
	defer tx.Rollback()

	var rewardJSON []byte
	var maxUses sql.NullInt64
	var currentUses int
	var expiresAt sql.NullTime

	// 1. Проверяем существование, лимиты и ВРЕМЯ
	err = tx.QueryRow(`
		SELECT reward_json, max_uses, current_uses, expires_at 
		FROM promocodes WHERE code = $1 FOR UPDATE`, code).
		Scan(&rewardJSON, &maxUses, &currentUses, &expiresAt)

	if err == sql.ErrNoRows {
		return nil, nil, fmt.Errorf("not_found")
	}
	if err != nil {
		return nil, nil, err
	}

	if maxUses.Valid && currentUses >= int(maxUses.Int64) {
		return nil, nil, fmt.Errorf("limit_reached")
	}

	// ПРОВЕРКА НА ИСТЕКШЕЕ ВРЕМЯ
	if expiresAt.Valid && time.Now().UTC().After(expiresAt.Time.UTC()) {
		return nil, nil, fmt.Errorf("expired")
	}

	// 2. Проверяем, не использовал ли юзер код ранее
	_, err = tx.Exec(`INSERT INTO promocode_usages (user_id, promocode) VALUES ($1, $2)`, userID, code)
	if err != nil {
		if pqErr, ok := err.(*pq.Error); ok && pqErr.Code == "23505" { // Unique Violation
			return nil, nil, fmt.Errorf("already_used")
		}
		return nil, nil, err
	}

	// 3. Распаковываем награду
	var reward models.PromoReward
	if err := json.Unmarshal(rewardJSON, &reward); err != nil {
		return nil, nil, err
	}

	// 4. Начисляем очки и крутки
	if reward.Points > 0 || reward.PremiumRolls > 0 {
		_, err = tx.Exec(`UPDATE users SET balance = balance + $1, premium_rolls = premium_rolls + $2 WHERE id = $3`, reward.Points, reward.PremiumRolls, userID)
		if err != nil {
			return nil, nil, err
		}
	}

	// 5. Выдаем КОНКРЕТНЫЕ карты, если есть
	var grantedCards []models.Card
	if len(reward.Cards) > 0 {
		for _, cid := range reward.Cards {
			_, err = tx.Exec(`
				INSERT INTO user_inventory (user_id, card_id, quantity) VALUES ($1, $2, 1)
				ON CONFLICT (user_id, card_id) DO UPDATE SET quantity = user_inventory.quantity + 1`, userID, cid)
			if err != nil {
				return nil, nil, err
			}

			var c models.Card
			// ИСПРАВЛЕНИЕ: Добавили image_url И set_id в SELECT и Scan
			_ = tx.QueryRow(`SELECT id, name, power_level, image_url, set_id FROM cards WHERE id = $1`, cid).
				Scan(&c.ID, &c.Name, &c.PowerLevel, &c.ImageURL, &c.SetID)
			grantedCards = append(grantedCards, c)
		}
	}

	// 5.1 Выдаем СЛУЧАЙНЫЕ карты заданной редкости
	if len(reward.RandomCards) > 0 {
		for rarityStr, count := range reward.RandomCards {
			rarityLevel, err := strconv.Atoi(rarityStr)
			if err != nil {
				continue
			}

			for i := 0; i < count; i++ {
				var c models.Card
				// ИСПРАВЛЕНИЕ: Добавили image_url И set_id в SELECT и Scan
				err = tx.QueryRow(`
                SELECT id, name, power_level, image_url, set_id 
                FROM cards WHERE rarity_id = $1 
                ORDER BY RANDOM() LIMIT 1`, rarityLevel).
					Scan(&c.ID, &c.Name, &c.PowerLevel, &c.ImageURL, &c.SetID)

				if err == nil {
					_, err = tx.Exec(`
						INSERT INTO user_inventory (user_id, card_id, quantity) VALUES ($1, $2, 1)
						ON CONFLICT (user_id, card_id) DO UPDATE SET quantity = user_inventory.quantity + 1`, userID, c.ID)
					if err == nil {
						grantedCards = append(grantedCards, c)
					} else {
						return nil, nil, err
					}
				} else {
					log.Printf("[PROMO WARN] Не удалось выдать случайную карту редкости %d: %v", rarityLevel, err)
				}
			}
		}
	}

	// 6. Увеличиваем счетчик использований
	_, err = tx.Exec(`UPDATE promocodes SET current_uses = current_uses + 1 WHERE code = $1`, code)
	if err != nil {
		return nil, nil, err
	}

	err = tx.Commit()
	return &reward, grantedCards, err
}

// ==========================================
// БЛОК МЕТОДОВ ДЛЯ РАБОТЫ С КОЛЛЕКЦИЯМИ (СЕТАМИ)
// ==========================================

// GetUserSetsProgress возвращает список ВСЕХ сетов в игре и прогресс юзера по каждому из них
func (r *PostgresRepo) GetUserSetsProgress(userID int64) ([]models.UserSetProgress, error) {
	// 1. Берем все сеты и считаем их размер (TotalSetCards)
	// 2. Считаем, сколько карт из сетов есть у юзера (UserSetCards)
	// 3. Соединяем всё вместе. Если у юзера 0 карт, COALESCE вернет 0.
	query := `
		WITH TotalSetCards AS (
			SELECT set_id, COUNT(id) as total_cards
			FROM cards
			WHERE set_id IS NOT NULL
			GROUP BY set_id
		),
		UserSetCards AS (
			SELECT c.set_id, COUNT(DISTINCT c.id) as collected_cards
			FROM user_inventory i
			JOIN cards c ON i.card_id = c.id
			WHERE i.user_id = $1 AND c.set_id IS NOT NULL
			GROUP BY c.set_id
		)
		SELECT 
			cs.id, cs.name, cs.buff_type, cs.buff_value, cs.reward_points,
			COALESCE(usc.collected_cards, 0) as collected_cards,
			tsc.total_cards,
			COALESCE(uus.is_completed, false) as is_completed,
			COALESCE((u.active_set_id = cs.id), false) as is_active
		FROM TotalSetCards tsc
		JOIN card_sets cs ON cs.id = tsc.set_id
		LEFT JOIN UserSetCards usc ON tsc.set_id = usc.set_id
		LEFT JOIN user_unlocked_sets uus ON uus.user_id = $1 AND uus.set_id = cs.id
		LEFT JOIN users u ON u.id = $1
		ORDER BY cs.id;
	`
	rows, err := r.db.Query(query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var progress []models.UserSetProgress
	for rows.Next() {
		var p models.UserSetProgress
		if err := rows.Scan(&p.SetID, &p.SetName, &p.BuffType, &p.BuffValue, &p.RewardPoints, &p.CollectedCards, &p.TotalCards, &p.IsCompleted, &p.IsActive); err != nil {
			return nil, err
		}
		progress = append(progress, p)
	}
	return progress, nil
}

// GetSetCards возвращает все карты конкретного сета. Ненайденные карты маскируются.
func (r *PostgresRepo) GetSetCards(userID int64, setID int) ([]models.Card, error) {
	query := `
		SELECT c.id, c.name, c.rarity_id, c.power_level, c.image_url, 
		       (MAX(i.user_id) IS NOT NULL) as is_owned
		FROM cards c
		LEFT JOIN user_inventory i ON c.id = i.card_id AND i.user_id = $1
		WHERE c.set_id = $2
		GROUP BY c.id, c.name, c.rarity_id, c.power_level, c.image_url
		ORDER BY c.rarity_id, c.id;
	`
	rows, err := r.db.Query(query, userID, setID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var cards []models.Card
	for rows.Next() {
		var c models.Card
		var isOwned bool

		// Исправлено: используем rarity_id и c.RarityID
		if err := rows.Scan(&c.ID, &c.Name, &c.RarityID, &c.PowerLevel, &c.ImageURL, &isOwned); err != nil {
			return nil, err
		}

		// Если карты нет в инвентаре, затираем данные (сигнал для фронтенда)
		if !isOwned {
			c.Name = "" // Пустая строка - признак скрытой карты
			c.ImageURL = ""
			c.PowerLevel = 0
		}
		cards = append(cards, c)
	}
	return cards, nil
}

// CheckSetCompletion проверяет, собрал ли юзер все карты в сете
func (r *PostgresRepo) CheckSetCompletion(userID int64, setID int) (bool, error) {
	query := `
		SELECT 
			(SELECT COUNT(id) FROM cards WHERE set_id = $2) as total_cards,
			(SELECT COUNT(DISTINCT c.id) 
			 FROM user_inventory i 
			 JOIN cards c ON i.card_id = c.id 
			 WHERE i.user_id = $1 AND c.set_id = $2) as collected_cards
	`
	var total, collected int
	err := r.db.QueryRow(query, userID, setID).Scan(&total, &collected)
	if err != nil {
		return false, err
	}
	// Защита от деления на ноль (если сет пустой)
	if total == 0 {
		return false, nil
	}
	return total == collected, nil
}

// MarkSetCompleted помечает сет как собранный, чтобы больше не выдавать за него награду
func (r *PostgresRepo) MarkSetCompleted(userID int64, setID int) error {
	query := `
		INSERT INTO user_unlocked_sets (user_id, set_id, is_completed)
		VALUES ($1, $2, true)
		ON CONFLICT (user_id, set_id) DO UPDATE SET is_completed = true;
	`
	_, err := r.db.Exec(query, userID, setID)
	return err
}

// EquipSetAura обновляет активную ауру пользователя (setID может быть nil, чтобы снять ауру)
func (r *PostgresRepo) EquipSetAura(userID int64, setID *int) error {
	query := `UPDATE users SET active_set_id = $1 WHERE id = $2`
	_, err := r.db.Exec(query, setID, userID)
	return err
}

// GetActiveUserAura возвращает активный сет юзера для применения баффов в дуэли
func (r *PostgresRepo) GetActiveUserAura(userID int64) (*models.CardSet, error) {
	query := `
		SELECT cs.id, cs.name, cs.buff_type, cs.buff_value, cs.reward_points
		FROM users u
		JOIN card_sets cs ON u.active_set_id = cs.id
		WHERE u.id = $1
	`
	var cs models.CardSet
	err := r.db.QueryRow(query, userID).Scan(&cs.ID, &cs.Name, &cs.BuffType, &cs.BuffValue, &cs.RewardPoints)

	if err == sql.ErrNoRows {
		return nil, nil // У юзера не экипирована аура, это нормально
	}
	if err != nil {
		return nil, err
	}
	return &cs, nil
}

// GetCardSetByID вытягивает информацию о самом сете
func (r *PostgresRepo) GetCardSetByID(setID int) (*models.CardSet, error) {
	query := `SELECT id, name, buff_type, buff_value, reward_points FROM card_sets WHERE id = $1`
	var cs models.CardSet
	err := r.db.QueryRow(query, setID).Scan(&cs.ID, &cs.Name, &cs.BuffType, &cs.BuffValue, &cs.RewardPoints)
	if err != nil {
		return nil, err
	}
	return &cs, nil
}

// IsSetRewardClaimed проверяет, забирал ли юзер награду за этот сет
func (r *PostgresRepo) IsSetRewardClaimed(userID int64, setID int) (bool, error) {
	var isCompleted bool
	err := r.db.QueryRow(`SELECT is_completed FROM user_unlocked_sets WHERE user_id = $1 AND set_id = $2`, userID, setID).Scan(&isCompleted)
	if err != nil {
		return false, nil // Если записи нет, значит точно не забирал
	}
	return isCompleted, nil
}

// GetUserSetsStats возвращает количество полностью собранных сетов и общее количество сетов
func (r *PostgresRepo) GetUserSetsStats(userID int64) (int, int, error) {
	query := `
		SELECT 
			(SELECT COUNT(set_id) FROM user_unlocked_sets WHERE user_id = $1 AND is_completed = true) as completed_sets,
			(SELECT COUNT(id) FROM card_sets) as total_sets
	`
	var completed, total int
	err := r.db.QueryRow(query, userID).Scan(&completed, &total)
	if err != nil {
		return 0, 0, err
	}
	return completed, total, nil
}

// Устанавливает статус 18+ для пользователя
func (r *PostgresRepo) SetUserAdultStatus(internalID int64, isAdult bool) error {
	query := `UPDATE users SET is_adult = $1 WHERE id = $2`
	_, err := r.db.Exec(query, isAdult, internalID)
	return err
}
