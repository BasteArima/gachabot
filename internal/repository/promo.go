package repository

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"gachabot/internal/models"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/lib/pq"
)

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

	// Проверяем существование, лимиты и ВРЕМЯ
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

	// Проверка не истекшее время
	if expiresAt.Valid && time.Now().UTC().After(expiresAt.Time.UTC()) {
		return nil, nil, fmt.Errorf("expired")
	}

	// Проверяем, не использовал ли юзер код ранее
	_, err = tx.Exec(`INSERT INTO promocode_usages (user_id, promocode) VALUES ($1, $2)`, userID, code)
	if err != nil {
		if pqErr, ok := err.(*pq.Error); ok && pqErr.Code == "23505" { // Unique Violation
			return nil, nil, fmt.Errorf("already_used")
		}
		return nil, nil, err
	}

	// Распаковываем награду
	var reward models.PromoReward
	if err := json.Unmarshal(rewardJSON, &reward); err != nil {
		return nil, nil, err
	}

	// Начисляем очки и крутки
	if reward.Points > 0 || reward.PremiumRolls > 0 {
		_, err = tx.Exec(`UPDATE users SET balance = balance + $1, premium_rolls = premium_rolls + $2 WHERE id = $3`, reward.Points, reward.PremiumRolls, userID)
		if err != nil {
			return nil, nil, err
		}
	}

	// Выдаем конкретные карты, если есть
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
			_ = tx.QueryRow(`SELECT id, name, power_level, image_url, set_id FROM cards WHERE id = $1`, cid).
				Scan(&c.ID, &c.Name, &c.PowerLevel, &c.ImageURL, &c.SetID)
			grantedCards = append(grantedCards, c)
		}
	}

	// Выдаем случайные карты заданной редкости
	if len(reward.RandomCards) > 0 {
		for rarityStr, count := range reward.RandomCards {
			rarityLevel, err := strconv.Atoi(rarityStr)
			if err != nil {
				continue
			}

			for i := 0; i < count; i++ {
				var c models.Card
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
					log.Printf("[PROMO WARN] Failed to issue random rarity card %d: %v", rarityLevel, err)
				}
			}
		}
	}

	// Увеличиваем счетчик использований
	_, err = tx.Exec(`UPDATE promocodes SET current_uses = current_uses + 1 WHERE code = $1`, code)
	if err != nil {
		return nil, nil, err
	}

	err = tx.Commit()
	return &reward, grantedCards, err
}

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
