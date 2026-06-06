package httpapi

import (
	"encoding/json"
	"net/http"
	"strconv"

	"gachabot/internal/models"
)

type playerDTO struct {
	ID         string `json:"id"`
	Username   string `json:"username"`
	AvatarURL  string `json:"avatarUrl"`
	Coins      int    `json:"coins"`
	StreakDays int    `json:"streakDays"`
	IsAdmin    bool   `json:"isAdmin"`
}

type cardDTO struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Power    int    `json:"power"`
	Rarity   string `json:"rarity"`
	ImageURL string `json:"imageUrl"`
	Quantity int    `json:"quantity"`
}

type authRequest struct {
	InitData string            `json:"initData"`
	Widget   map[string]string `json:"widget"`
}

type authResponse struct {
	Token  string    `json:"token"`
	Player playerDTO `json:"player"`
}

// POST /api/auth/telegram — verify TMA initData or Login Widget data, return a session.
func (s *Server) handleAuthTelegram(w http.ResponseWriter, r *http.Request) {
	var req authRequest
	_ = json.NewDecoder(r.Body).Decode(&req)
	if req.InitData == "" {
		req.InitData = r.Header.Get("X-Telegram-Init-Data")
	}

	var (
		tu  tgAuthUser
		err error
	)
	switch {
	case req.InitData != "":
		tu, err = validateInitData(req.InitData, s.botToken)
	case len(req.Widget) > 0:
		tu, err = validateLoginWidget(req.Widget, s.botToken)
	default:
		writeErr(w, http.StatusBadRequest, "no telegram data")
		return
	}
	if err != nil {
		writeErr(w, http.StatusUnauthorized, "telegram auth failed: "+err.Error())
		return
	}

	user, err := s.repo.GetOrCreateUserByTelegramID(tu.ID, tu.Username, tu.FirstName, tu.LastName)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	if tu.PhotoURL != "" {
		_ = s.repo.UpdateUserAvatar(user.ID, tu.PhotoURL)
		user.AvatarURL = tu.PhotoURL
	}
	token, err := s.createSession(user.ID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "session error")
		return
	}
	writeJSON(w, http.StatusOK, authResponse{Token: token, Player: s.toPlayer(user)})
}

// GET /api/me — current player.
func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	user, err := s.repo.GetUserByID(userIDFrom(r))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	writeJSON(w, http.StatusOK, s.toPlayer(user))
}

// GET /api/inventory?rarity=All|<name> — player's owned cards.
func (s *Server) handleInventory(w http.ResponseWriter, r *http.Request) {
	cards, err := s.repo.GetUserInventoryList(userIDFrom(r), r.URL.Query().Get("rarity"))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	out := make([]cardDTO, 0, len(cards))
	for _, c := range cards {
		out = append(out, cardDTO{
			ID:       strconv.Itoa(c.CardID),
			Name:     c.Name,
			Power:    c.PowerLevel,
			Rarity:   c.RarityName,
			ImageURL: c.ImageURL,
			Quantity: c.Quantity,
		})
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) toPlayer(u *models.User) playerDTO {
	name := u.FirstName
	if name == "" {
		name = u.Username
	}
	return playerDTO{
		ID:         strconv.FormatInt(u.ID, 10),
		Username:   name,
		AvatarURL:  u.AvatarURL,
		Coins:      u.Balance,
		StreakDays: u.StreakDays,
		IsAdmin:    s.isAdmin(u),
	}
}

// isAdmin reports whether the user is the configured bot admin (by Telegram id).
func (s *Server) isAdmin(u *models.User) bool {
	return u != nil && s.adminID != 0 && u.TelegramID.Valid && u.TelegramID.Int64 == s.adminID
}
