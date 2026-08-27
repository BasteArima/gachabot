package httpapi

import (
	"database/sql"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"gachabot/internal/repository"

	"github.com/go-chi/chi/v5"
)

// GET /api/admin/suggestions?status=pending
func (s *Server) handleAdminListSuggestions(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")
	list, err := s.repo.ListSuggestions(status, 100)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	pending, _ := s.repo.CountPendingSuggestions()
	writeJSON(w, http.StatusOK, map[string]any{"suggestions": list, "pending": pending})
}

// GET /api/admin/suggestions/{id}/image — streams the submitted image.
// Telegram file links embed the bot token and expire, so the id is resolved here
// and the bytes are proxied; Discord submissions are redirected to their CDN url.
func (s *Server) handleAdminSuggestionImage(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad id")
		return
	}
	sug, err := s.repo.GetSuggestion(id)
	if err == sql.ErrNoRows {
		writeErr(w, http.StatusNotFound, "заявка не найдена")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}

	if sug.FileID == "" {
		if sug.ImageURL == "" {
			writeErr(w, http.StatusNotFound, "у заявки нет изображения")
			return
		}
		http.Redirect(w, r, sug.ImageURL, http.StatusFound)
		return
	}

	var file struct {
		FilePath string `json:"file_path"`
	}
	if err := s.tgCall("getFile", url.Values{"file_id": {sug.FileID}}, &file); err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	resp, err := tgAPIClient.Get("https://api.telegram.org/file/bot" + s.botToken + "/" + file.FilePath)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "не удалось скачать файл")
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		writeErr(w, http.StatusBadGateway, "Telegram вернул "+resp.Status)
		return
	}

	if ct := resp.Header.Get("Content-Type"); ct != "" {
		w.Header().Set("Content-Type", ct)
	}
	w.Header().Set("Cache-Control", "private, max-age=300")
	if _, err := io.Copy(w, resp.Body); err != nil {
		log.Printf("[ADMIN] suggestion image stream failed: %v", err)
	}
}

// POST /api/admin/suggestions/{id}/approve
// Approving records the decision and optionally creates the card right away.
// The submitted image is not the final art (it still has to go through the art
// pipeline and be uploaded), so imageUrl is entered by the admin and may be left
// empty — the card then shows up in the "без арта" filter and the art linter.
func (s *Server) handleAdminApproveSuggestion(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad id")
		return
	}
	var in struct {
		CreateCard bool   `json:"createCard"`
		Name       string `json:"name"`
		RarityID   int    `json:"rarityId"`
		PowerLevel int    `json:"powerLevel"`
		ImageURL   string `json:"imageUrl"`
		SetID      *int   `json:"setId"`
		Note       string `json:"note"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "bad request")
		return
	}

	var cardID *int
	if in.CreateCard {
		name := strings.TrimSpace(in.Name)
		if name == "" {
			writeErr(w, http.StatusBadRequest, "укажи название карты")
			return
		}
		if in.RarityID <= 0 {
			writeErr(w, http.StatusBadRequest, "выбери редкость")
			return
		}
		if in.SetID != nil && *in.SetID <= 0 {
			in.SetID = nil
		}
		newID, err := s.repo.CreateCard(name, in.RarityID, strings.TrimSpace(in.ImageURL), in.PowerLevel, in.SetID)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		cardID = &newID
	}

	if err := s.repo.ReviewSuggestion(id, repository.SuggestionApproved, in.Note, cardID, false); err == sql.ErrNoRows {
		writeErr(w, http.StatusBadRequest, "заявка уже обработана")
		return
	} else if err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}

	log.Printf("[ADMIN] suggestion %d approved (card=%v)", id, cardID)
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "cardId": cardID})
}

// POST /api/admin/suggestions/{id}/reject {note, refund}
// The player paid for the submission, so a rejection can hand the coins back.
func (s *Server) handleAdminRejectSuggestion(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad id")
		return
	}
	var in struct {
		Note   string `json:"note"`
		Refund bool   `json:"refund"`
		Amount int    `json:"amount"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "bad request")
		return
	}

	sug, err := s.repo.GetSuggestion(id)
	if err == sql.ErrNoRows {
		writeErr(w, http.StatusNotFound, "заявка не найдена")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}

	refunded := false
	if in.Refund && sug.UserID != nil {
		amount := in.Amount
		if amount <= 0 {
			amount = suggestionPrice
		}
		if _, err := s.repo.AdjustBalance(*sug.UserID, amount); err != nil {
			writeErr(w, http.StatusBadRequest, "не удалось вернуть монеты")
			return
		}
		refunded = true
	}

	if err := s.repo.ReviewSuggestion(id, repository.SuggestionRejected, in.Note, nil, refunded); err == sql.ErrNoRows {
		writeErr(w, http.StatusBadRequest, "заявка уже обработана")
		return
	} else if err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}

	log.Printf("[ADMIN] suggestion %d rejected (refunded=%v)", id, refunded)
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "refunded": refunded})
}

// suggestionPrice mirrors what suggest.SubmitSuggestion charges the player.
const suggestionPrice = 1000
