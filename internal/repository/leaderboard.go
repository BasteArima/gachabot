package repository

import (
	"fmt"
	"gachabot/internal/models"
	"log"
)

func (r *PostgresRepo) TrackUserChat(userID int64, chatID int64) error {
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
		return nil, fmt.Errorf("unknown top criterion")
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
		if err := rows.Scan(&entry.DisplayName, &entry.Value, &entry.ActiveAura); err != nil {
			return nil, err
		}
		board = append(board, entry)
	}
	return board, nil
}
