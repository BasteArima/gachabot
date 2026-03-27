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

func (r *PostgresRepo) GetUser(tgID int64) (*models.User, error) {
	user := &models.User{}
	query := "SELECT tg_id, username, first_name, last_name, balance, streak_days, last_roll_time, last_streak_date FROM users WHERE tg_id=$1 LIMIT 1"
	err := r.db.QueryRow(query, tgID).Scan(
		&user.TgID,
		&user.Username,
		&user.FirstName,
		&user.LastName,
		&user.Balance,
		&user.StreakDays,
		&user.LastRollTime,
		&user.LastStreakDate,
	)
	if err != nil {
		return nil, err
	}
	return user, nil
}

func (r *PostgresRepo) CreateUserIfNotExist(tgID int64, username, firstName, lastName string) error {
	query := `
        INSERT INTO users (tg_id, username, first_name, last_name) VALUES ($1, $2, $3, $4) 
        ON CONFLICT (tg_id) DO UPDATE SET
            username = EXCLUDED.username,
            first_name = EXCLUDED.first_name,
            last_name = EXCLUDED.last_name
    `
	_, err := r.db.Exec(query, tgID, username, firstName, lastName)
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

// UpdateUserAfterRoll сохраняет обновленные данные пользователя одним запросом
func (r *PostgresRepo) UpdateUserAfterRoll(user *models.User) error {
	query := `
		UPDATE users 
		SET balance = $1, 
		    streak_days = $2, 
		    last_roll_time = $3, 
		    last_streak_date = $4
		WHERE tg_id = $5
	`
	// Данные берем напрямую из структуры, которую передадим из бизнес-логики
	_, err := r.db.Exec(query, user.Balance, user.StreakDays, user.LastRollTime, user.LastStreakDate, user.TgID)
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
	card := &models.UserCardView{}
	// Магия SQL: объединяем (JOIN) три таблицы, чтобы получить и имя карты, и название редкости
	query := `
		SELECT c.name, r.name, c.image_url, ui.quantity 
		FROM user_inventory ui
		JOIN cards c ON ui.card_id = c.id
		JOIN rarities r ON c.rarity_id = r.id
		WHERE ui.user_id = $1
		ORDER BY r.id DESC, c.id ASC
		OFFSET $2 LIMIT 1
	`
	err := r.db.QueryRow(query, userID, offset).Scan(
		&card.CardName, &card.RarityName, &card.ImageURL, &card.Quantity,
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
		joinChat = "JOIN user_chats uc ON u.tg_id = uc.user_id"
		whereChat = "WHERE uc.chat_id = $1"
		args = append(args, chatID)
	}

	// Склеиваем имя и фамилию
	//nameFormat := "TRIM(COALESCE(u.first_name, '') || ' ' || COALESCE(u.last_name, ''))"
	nameFormat := "COALESCE(u.first_name)"
	// ORDER BY: сначала по основному показателю (DESC),
	// затем по времени последнего ролла (ASC) — кто раньше крутил, тот выше при равенстве.
	switch criteria {
	case "balance":
		query = fmt.Sprintf(`SELECT %s, u.balance FROM users u %s %s ORDER BY u.balance DESC, u.last_roll_time ASC LIMIT 10`, nameFormat, joinChat, whereChat)
	case "streak":
		query = fmt.Sprintf(`SELECT %s, u.streak_days FROM users u %s %s ORDER BY u.streak_days DESC, u.last_roll_time ASC LIMIT 10`, nameFormat, joinChat, whereChat)
	case "cards":
		query = fmt.Sprintf(`
          SELECT %s, COUNT(DISTINCT ui.card_id) as val 
          FROM users u 
          %s 
          JOIN user_inventory ui ON u.tg_id = ui.user_id 
          %s 
          GROUP BY u.tg_id, u.first_name, u.last_name 
          ORDER BY val DESC, MIN(u.last_roll_time) ASC LIMIT 10`, nameFormat, joinChat, whereChat)
	default:
		return nil, fmt.Errorf("неизвестный критерий топа")
	}

	rows, err := r.db.Query(query, args...)
	if err != nil {
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

// Ищем пользователя по юзернейму (без учета символа @)
func (r *PostgresRepo) GetUserByUsername(username string) (*models.User, error) {
	user := &models.User{}
	// Используем ILIKE для поиска без учета регистра
	query := "SELECT tg_id, balance, streak_days, last_roll_time, last_streak_date FROM users WHERE username ILIKE $1 LIMIT 1"
	err := r.db.QueryRow(query, username).Scan(
		&user.TgID, &user.Balance, &user.StreakDays, &user.LastRollTime, &user.LastStreakDate,
	)
	return user, err
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

// Находит первую попавшуюся редкость, у которой у юзера есть 5+ дубликатов (quantity > 1)
func (r *PostgresRepo) GetRarityWithDuplicates(userID int64, required int) (int, error) {
	var rarityID int
	query := `
		SELECT c.rarity_id 
		FROM user_inventory ui
		JOIN cards c ON ui.card_id = c.id
		WHERE ui.user_id = $1 AND ui.quantity > 1
		GROUP BY c.rarity_id
		HAVING SUM(ui.quantity - 1) >= $2
		LIMIT 1
	`
	err := r.db.QueryRow(query, userID, required).Scan(&rarityID)
	return rarityID, err
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
