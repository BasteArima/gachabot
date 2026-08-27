package httpapi

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
)

// GET /api/admin/chats — the posting registry plus which platforms can send now.
func (s *Server) handleAdminListChats(w http.ResponseWriter, _ *http.Request) {
	chats, err := s.repo.ListAllChats()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"chats":     chats,
		"platforms": s.broadcast.AvailablePlatforms(),
	})
}

// chatParams pulls the platform/chatId pair out of the URL.
func chatParams(r *http.Request) (string, int64, error) {
	platform := chi.URLParam(r, "platform")
	id, err := strconv.ParseInt(chi.URLParam(r, "chatId"), 10, 64)
	return platform, id, err
}

// PUT /api/admin/chats/{platform}/{chatId} {spawnEnabled}
func (s *Server) handleAdminUpdateChat(w http.ResponseWriter, r *http.Request) {
	platform, chatID, err := chatParams(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad chat id")
		return
	}
	var in struct {
		SpawnEnabled bool `json:"spawnEnabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "bad request")
		return
	}
	if err := s.repo.SetChatSpawnEnabled(platform, chatID, in.SpawnEnabled); err == sql.ErrNoRows {
		writeErr(w, http.StatusNotFound, "чат не найден")
		return
	} else if err != nil {
		writeErr(w, http.StatusBadRequest, "не удалось обновить чат")
		return
	}
	log.Printf("[ADMIN] chat %s:%d spawnEnabled=%v", platform, chatID, in.SpawnEnabled)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// DELETE /api/admin/chats/{platform}/{chatId} — forget the chat. The bot stays a
// member if it still is one; use the leave action to actually walk out.
func (s *Server) handleAdminDeleteChat(w http.ResponseWriter, r *http.Request) {
	platform, chatID, err := chatParams(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad chat id")
		return
	}
	if err := s.repo.DeleteChat(platform, chatID); err == sql.ErrNoRows {
		writeErr(w, http.StatusNotFound, "чат не найден")
		return
	} else if err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	log.Printf("[ADMIN] chat %s:%d removed from registry", platform, chatID)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// POST /api/admin/chats/{platform}/{chatId}/leave — bot leaves and stops posting.
func (s *Server) handleAdminLeaveChat(w http.ResponseWriter, r *http.Request) {
	platform, chatID, err := chatParams(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad chat id")
		return
	}
	if err := s.broadcast.LeaveChat(platform, chatID); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// POST /api/admin/broadcast {text, platforms, dryRun}
func (s *Server) handleAdminBroadcast(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Text      string   `json:"text"`
		Platforms []string `json:"platforms"`
		DryRun    bool     `json:"dryRun"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "bad request")
		return
	}
	rep, err := s.broadcast.Send(in.Text, in.Platforms, in.DryRun)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, rep)
}
