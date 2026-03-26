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
	query := "SELECT tg_id, balance, streak_days, last_roll_time, last_streak_date FROM users WHERE tg_id=$1 LIMIT 1"
	err := r.db.QueryRow(query, tgID).Scan(
		&user.TgID,
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

func (r *PostgresRepo) CreateUserIfNotExist(tgID int64, username string) error {
	_, err := r.db.Exec("INSERT INTO users (tg_id, username) VALUES ($1, $2) ON CONFLICT DO NOTHING", tgID, username)
	return err
}

func (r *PostgresRepo) GetRarities() ([]models.Rarity, error) {
	var rarities []models.Rarity
	rows, err := r.db.Query("SELECT id, name, drop_chance, base_reward FROM rarities ORDER BY id ASC")
	if err != nil {
		return nil, err // Возвращаем ошибку!
	}
	defer rows.Close()

	for rows.Next() {
		var rarity models.Rarity
		// Обязательно проверяем ошибку Scan
		if err := rows.Scan(&rarity.ID, &rarity.Name, &rarity.DropChance, &rarity.BaseReward); err != nil {
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
