package httpapi

import (
	"bytes"
	"context"
	"io"
	"log"
	"net/http"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// --- Audit ---

// maxAuditPayload bounds what we copy from a request body into the audit row.
const maxAuditPayload = 500

// auditRecorder captures the status code so the audit entry can record the outcome.
type auditRecorder struct {
	http.ResponseWriter
	status int
}

func (a *auditRecorder) WriteHeader(code int) {
	a.status = code
	a.ResponseWriter.WriteHeader(code)
}

func (a *auditRecorder) Write(b []byte) (int, error) {
	if a.status == 0 {
		a.status = http.StatusOK
	}
	return a.ResponseWriter.Write(b)
}

// auditMiddleware records every mutating admin request. Reads are skipped: the
// point is attribution for actions that change money, content or chats.
func (s *Server) auditMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet || r.Method == http.MethodOptions {
			next.ServeHTTP(w, r)
			return
		}

		// Copy a bounded prefix of the body, then restore it for the handler.
		var payload string
		if r.Body != nil {
			body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
			if err == nil {
				r.Body = io.NopCloser(bytes.NewReader(body))
				payload = strings.TrimSpace(string(body))
				if len(payload) > maxAuditPayload {
					payload = payload[:maxAuditPayload] + "…"
				}
			}
		}

		rec := &auditRecorder{ResponseWriter: w}
		next.ServeHTTP(rec, r)

		if rec.status == 0 {
			rec.status = http.StatusOK
		}
		if err := s.repo.InsertAudit(userIDFrom(r), r.Method, r.URL.Path, payload, rec.status); err != nil {
			// Never fail the action because the trail could not be written.
			log.Printf("[ADMIN] audit write failed: %v", err)
		}
	})
}

// GET /api/admin/audit?limit=
func (s *Server) handleAdminAudit(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	entries, err := s.repo.ListAudit(limit)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	writeJSON(w, http.StatusOK, entries)
}

// --- Dashboard ---

// GET /api/admin/dashboard
func (s *Server) handleAdminDashboard(w http.ResponseWriter, _ *http.Request) {
	stats, err := s.repo.GetDashboard()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db error: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, stats)
}

// --- Health ---

// startedAt is process start, used for the uptime readout.
var startedAt = time.Now()

// GET /api/admin/health — liveness of the pieces the bot depends on.
func (s *Server) handleAdminHealth(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	type check struct {
		OK      bool   `json:"ok"`
		Detail  string `json:"detail"`
		Latency int64  `json:"latencyMs"`
	}

	db := check{}
	t0 := time.Now()
	if err := s.repo.DB().PingContext(ctx); err != nil {
		db.Detail = err.Error()
	} else {
		db.OK = true
		var version string
		if err := s.repo.DB().QueryRowContext(ctx, `SHOW server_version`).Scan(&version); err == nil {
			db.Detail = "PostgreSQL " + version
		}
	}
	db.Latency = time.Since(t0).Milliseconds()

	redis := check{}
	t1 := time.Now()
	if err := s.rdb.Ping(ctx).Err(); err != nil {
		redis.Detail = err.Error()
	} else {
		redis.OK = true
		if n, err := s.rdb.DBSize(ctx).Result(); err == nil {
			redis.Detail = strconv.FormatInt(n, 10) + " ключей"
		}
	}
	redis.Latency = time.Since(t1).Milliseconds()

	// Which delivery layers are actually live (empty in API-only dev mode).
	writeJSON(w, http.StatusOK, map[string]any{
		"db":            db,
		"redis":         redis,
		"uptimeSeconds": int64(time.Since(startedAt).Seconds()),
		"goroutines":    runtime.NumGoroutine(),
		"bots":          s.broadcast.AvailablePlatforms(),
		"apiOnly":       s.cfg.APIOnly,
		"staticDir":     s.cfg.StaticDir,
	})
}
