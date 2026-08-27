package httpapi

import (
	"io"
	"net/http"
)

// adminMiddleware allows only the configured bot admin (runs after authMiddleware,
// so the user id is already in context).
func (s *Server) adminMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, err := s.repo.GetUserByID(userIDFrom(r))
		if err != nil || !s.isAdmin(user) {
			writeErr(w, http.StatusForbidden, "forbidden")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// GET /api/admin/overview — dashboard counters.
func (s *Server) handleAdminOverview(w http.ResponseWriter, _ *http.Request) {
	stats, err := s.repo.GetAdminStats()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	pending, _ := s.repo.CountPendingSuggestions()
	writeJSON(w, http.StatusOK, map[string]any{
		"users": stats.Users, "usersActive": stats.UsersActive,
		"cards": stats.Cards, "cardsNoArt": stats.CardsNoArt,
		"rarities": stats.Rarities, "sets": stats.Sets,
		"chats": stats.Chats, "chatsEnabled": stats.ChatsEnabled,
		"pendingSuggestions": pending,
	})
}

// GET /api/admin/settings — read-only view of the env-configured game settings
// (they are deliberately not editable from the web: changing them needs a redeploy).
func (s *Server) handleAdminSettings(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"duplicatesEnabled": s.gacha.DuplicatesEnabled(),
		"craftEnabled":      s.gacha.CraftEnabled(),
		"cooldownHours":     int(s.game.CooldownDuration.Hours()),
		"require18Plus":     s.require18Plus,
		"webAppURL":         s.cfg.WebAppURL,
		"discordConfigured": s.discord.ClientID != "",
	})
}

// GET /api/admin/spawn-config — current spawn config as JSON.
func (s *Server) handleGetSpawnConfig(w http.ResponseWriter, _ *http.Request) {
	data, err := s.spawn.CurrentConfigJSON()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "config error")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

// GET /api/admin/artguess-config — current Art Guess config as JSON.
func (s *Server) handleGetArtGuessConfig(w http.ResponseWriter, _ *http.Request) {
	data, err := s.artguess.CurrentConfigJSON()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "config error")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

// PUT /api/admin/artguess-config — validate and persist a new Art Guess config.
func (s *Server) handlePutArtGuessConfig(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "read error")
		return
	}
	if _, err := s.artguess.SaveConfigJSON(body); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// PUT /api/admin/spawn-config — validate and persist a new spawn config (raw JSON body).
func (s *Server) handlePutSpawnConfig(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "read error")
		return
	}
	if _, err := s.spawn.SaveConfigJSON(body); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
