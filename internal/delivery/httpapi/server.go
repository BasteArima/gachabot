// Package httpapi is the web/API delivery layer: it serves the gacha-nova SPA
// and a JSON API consumed from Telegram Mini App, Discord Activity and the
// browser. Business logic stays in the service/repository layers.
package httpapi

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gachabot/internal/config"
	"gachabot/internal/repository"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/redis/go-redis/v9"
)

type Server struct {
	repo     *repository.PostgresRepo
	rdb      *redis.Client
	botToken string
	adminID  int64
	cfg      config.HTTPConfig
	discord  config.DiscordConfig
}

func NewServer(repo *repository.PostgresRepo, rdb *redis.Client, botToken string, adminID int64, cfg config.HTTPConfig, discord config.DiscordConfig) *Server {
	return &Server{repo: repo, rdb: rdb, botToken: botToken, adminID: adminID, cfg: cfg, discord: discord}
}

// Start builds the router and serves in a background goroutine.
func (s *Server) Start() {
	r := chi.NewRouter()
	r.Use(middleware.Recoverer)
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET", "POST", "OPTIONS"},
		AllowedHeaders:   []string{"Authorization", "Content-Type", "X-Telegram-Init-Data"},
		AllowCredentials: false,
		MaxAge:           300,
	}))

	r.Route("/api", func(r chi.Router) {
		r.Post("/auth/telegram", s.handleAuthTelegram)
		r.Post("/auth/discord", s.handleAuthDiscord)

		r.Group(func(r chi.Router) {
			r.Use(s.authMiddleware)
			r.Get("/me", s.handleMe)
			r.Get("/inventory", s.handleInventory)
		})
	})

	if s.cfg.StaticDir != "" {
		s.mountStatic(r)
	}

	addr := ":" + s.cfg.Port
	srv := &http.Server{Addr: addr, Handler: r, ReadHeaderTimeout: 10 * time.Second}
	go func() {
		log.Printf("[HTTP] listening on %s (static=%q, devNoAuth=%v)", addr, s.cfg.StaticDir, s.cfg.DevAllowNoAuth)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("[HTTP] server error: %v", err)
		}
	}()
}

// mountStatic serves the built SPA with client-side-routing fallback: existing
// files are served as-is, everything else returns index.html.
func (s *Server) mountStatic(r chi.Router) {
	dir := s.cfg.StaticDir
	fs := http.FileServer(http.Dir(dir))
	r.Get("/*", func(w http.ResponseWriter, req *http.Request) {
		full := filepath.Join(dir, filepath.Clean(req.URL.Path))
		if rel, err := filepath.Rel(dir, full); err != nil || strings.HasPrefix(rel, "..") {
			http.NotFound(w, req)
			return
		}
		if st, err := os.Stat(full); err == nil && !st.IsDir() {
			fs.ServeHTTP(w, req)
			return
		}
		http.ServeFile(w, req, filepath.Join(dir, "index.html"))
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
