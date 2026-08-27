package httpapi

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Telegram Stars admin tooling: list the bot's star transactions and refund one.
// Both talk to the Bot API directly — there is no local record of charge ids, so
// the transaction list is the source of truth for refunds.

var tgAPIClient = &http.Client{Timeout: 15 * time.Second}

// tgCall performs a Bot API method and decodes result into out.
func (s *Server) tgCall(method string, params url.Values, out any) error {
	if s.botToken == "" {
		return fmt.Errorf("токен бота не задан")
	}
	endpoint := "https://api.telegram.org/bot" + s.botToken + "/" + method
	resp, err := tgAPIClient.PostForm(endpoint, params)
	if err != nil {
		return fmt.Errorf("Telegram недоступен")
	}
	defer resp.Body.Close()

	var envelope struct {
		OK          bool            `json:"ok"`
		Description string          `json:"description"`
		Result      json.RawMessage `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		return fmt.Errorf("некорректный ответ Telegram")
	}
	if !envelope.OK {
		return fmt.Errorf("Telegram: %s", envelope.Description)
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(envelope.Result, out)
}

// starTxDTO is one transaction, flattened for the admin table.
type starTxDTO struct {
	ID          string `json:"id"`
	Amount      int    `json:"amount"`
	Date        int64  `json:"date"`
	Direction   string `json:"direction"` // "in" (player paid) | "out" (refund/payout)
	TelegramID  int64  `json:"telegramId"`
	Username    string `json:"username"`
	DisplayName string `json:"displayName"`
	PlayerID    int64  `json:"playerId"` // internal users.id, 0 when unknown
	Refundable  bool   `json:"refundable"`
}

// GET /api/admin/stars?offset=&limit=
func (s *Server) handleAdminStarTransactions(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	offset, _ := strconv.Atoi(q.Get("offset"))
	limit, _ := strconv.Atoi(q.Get("limit"))
	if limit <= 0 || limit > 100 {
		limit = 50
	}

	var result struct {
		Transactions []struct {
			ID     string `json:"id"`
			Amount int    `json:"amount"`
			Date   int64  `json:"date"`
			Source *struct {
				Type string `json:"type"`
				User *struct {
					ID        int64  `json:"id"`
					Username  string `json:"username"`
					FirstName string `json:"first_name"`
				} `json:"user"`
			} `json:"source"`
			Receiver *struct {
				Type string `json:"type"`
				User *struct {
					ID        int64  `json:"id"`
					Username  string `json:"username"`
					FirstName string `json:"first_name"`
				} `json:"user"`
			} `json:"receiver"`
		} `json:"transactions"`
	}

	params := url.Values{}
	params.Set("offset", strconv.Itoa(offset))
	params.Set("limit", strconv.Itoa(limit))
	if err := s.tgCall("getStarTransactions", params, &result); err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}

	out := make([]starTxDTO, 0, len(result.Transactions))
	for _, t := range result.Transactions {
		dto := starTxDTO{ID: t.ID, Amount: t.Amount, Date: t.Date}
		// A transaction has either a source (incoming payment) or a receiver
		// (money leaving the bot, e.g. an earlier refund).
		user := (*struct {
			ID        int64  `json:"id"`
			Username  string `json:"username"`
			FirstName string `json:"first_name"`
		})(nil)
		if t.Source != nil && t.Source.User != nil {
			dto.Direction = "in"
			user = t.Source.User
		} else if t.Receiver != nil && t.Receiver.User != nil {
			dto.Direction = "out"
			user = t.Receiver.User
		}
		if user != nil {
			dto.TelegramID = user.ID
			dto.Username = user.Username
			dto.DisplayName = strings.TrimSpace(user.FirstName)
			if dto.DisplayName == "" {
				dto.DisplayName = user.Username
			}
			if u, err := s.repo.GetUserByTelegramID(user.ID); err == nil && u != nil {
				dto.PlayerID = u.ID
			}
		}
		dto.Refundable = dto.Direction == "in" && dto.TelegramID != 0
		out = append(out, dto)
	}
	writeJSON(w, http.StatusOK, map[string]any{"transactions": out, "offset": offset, "limit": limit})
}

// POST /api/admin/stars/refund {telegramId, chargeId}
// Refunding only reverses the Stars payment — anything the purchase granted in
// game (premium rolls, bonus fragments) stays with the player and must be taken
// back separately via the players tool if needed.
func (s *Server) handleAdminStarRefund(w http.ResponseWriter, r *http.Request) {
	var in struct {
		TelegramID int64  `json:"telegramId"`
		ChargeID   string `json:"chargeId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "bad request")
		return
	}
	if in.TelegramID == 0 || strings.TrimSpace(in.ChargeID) == "" {
		writeErr(w, http.StatusBadRequest, "нужны telegramId и chargeId")
		return
	}

	params := url.Values{}
	params.Set("user_id", strconv.FormatInt(in.TelegramID, 10))
	params.Set("telegram_payment_charge_id", strings.TrimSpace(in.ChargeID))
	if err := s.tgCall("refundStarPayment", params, nil); err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}

	log.Printf("[ADMIN] star refund: tg=%d charge=%s", in.TelegramID, in.ChargeID)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
