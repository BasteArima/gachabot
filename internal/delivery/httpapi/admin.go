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

// GET /api/admin/overview — quick counts for the admin dashboard.
func (s *Server) handleAdminOverview(w http.ResponseWriter, _ *http.Request) {
	users, _ := s.repo.GetUserCount()
	cards, _ := s.repo.GetTotalCardsCount()
	writeJSON(w, http.StatusOK, map[string]any{"users": users, "cards": cards})
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
