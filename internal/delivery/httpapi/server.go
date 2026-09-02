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
	"gachabot/internal/service/artguess"
	"gachabot/internal/service/artstore"
	"gachabot/internal/service/broadcast"
	"gachabot/internal/service/gacha"
	"gachabot/internal/service/spawn"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/redis/go-redis/v9"
)

type Server struct {
	repo      *repository.PostgresRepo
	rdb       *redis.Client
	gacha     *gacha.GachaService
	spawn     *spawn.SpawnService
	artguess  *artguess.Service
	broadcast *broadcast.Service
	art       *artstore.Service
	botToken  string
	adminID   int64
	cfg       config.HTTPConfig
	discord   config.DiscordConfig
	game      config.GameConfig
	// require18Plus mirrors the Telegram-side age gate, shown in admin settings.
	require18Plus bool
}

func NewServer(repo *repository.PostgresRepo, rdb *redis.Client, gs *gacha.GachaService, sp *spawn.SpawnService, ag *artguess.Service, bc *broadcast.Service, art *artstore.Service, botToken string, adminID int64, cfg config.HTTPConfig, discord config.DiscordConfig, game config.GameConfig, require18Plus bool) *Server {
	return &Server{repo: repo, rdb: rdb, gacha: gs, spawn: sp, artguess: ag, broadcast: bc, art: art, botToken: botToken, adminID: adminID, cfg: cfg, discord: discord, game: game, require18Plus: require18Plus}
}

// Start builds the router and serves in a background goroutine.
func (s *Server) Start() {
	r := chi.NewRouter()
	r.Use(middleware.Recoverer)
	// Must run before routing: inside a Discord Activity every path may arrive
	// with the /.proxy prefix.
	r.Use(stripProxyPrefix)
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Authorization", "Content-Type", "X-Telegram-Init-Data"},
		AllowCredentials: false,
		MaxAge:           300,
	}))

	// Probe assets for diagnosing Discord's proxy from inside an Activity.
	r.Get("/probe/tiny.js", s.handleProbeAsset)
	r.Get("/probe/large.js", s.handleProbeAsset)
	r.Get("/probe/asset.js", s.handleProbeAsset)

	r.Route("/api", func(r chi.Router) {
		// Temporary, unauthenticated: the Activity has no session before its
		// bundle loads, which is exactly what is being diagnosed.
		r.Post("/probe", s.handleProbeReport)
		r.Get("/probe", s.handleProbeRead)

		r.Post("/auth/telegram", s.handleAuthTelegram)
		r.Post("/auth/discord", s.handleAuthDiscord)

		r.Group(func(r chi.Router) {
			r.Use(s.authMiddleware)
			r.Get("/me", s.handleMe)
			r.Get("/inventory", s.handleInventory)
			r.Get("/rarities", s.handleRarities)
			r.Get("/daily-hub", s.handleDailyHub)
			r.Get("/leaderboard", s.handleLeaderboard)
			r.Get("/cards", s.handleCards)
			r.Post("/actions/roll", s.handleRoll)
			r.Post("/actions/craft", s.handleCraft)

			r.Get("/artguess", s.handleArtGuess)
			r.Post("/artguess/guess", s.handleArtGuessGuess)
			r.Get("/artguess/image", s.handleArtGuessImage)
			r.Post("/artguess/reset", s.handleArtGuessResetMe) // owner-only (checked in handler)

			r.Group(func(r chi.Router) {
				r.Use(s.adminMiddleware)
				r.Use(s.auditMiddleware)
				r.Get("/admin/overview", s.handleAdminOverview)
				r.Get("/admin/settings", s.handleAdminSettings)
				r.Get("/admin/spawn-config", s.handleGetSpawnConfig)
				r.Put("/admin/spawn-config", s.handlePutSpawnConfig)
				r.Get("/admin/artguess-config", s.handleGetArtGuessConfig)
				r.Put("/admin/artguess-config", s.handlePutArtGuessConfig)
				r.Post("/admin/artguess/reset-all", s.handleAdminArtGuessResetAll)
				r.Post("/admin/artguess/reset-user", s.handleAdminArtGuessResetUser)
				r.Post("/admin/artguess/reroll", s.handleAdminArtGuessReroll)

				// Content editing (cards / rarities / sets).
				r.Get("/admin/cards", s.handleAdminListCards)
				r.Post("/admin/cards", s.handleAdminCreateCard)
				r.Put("/admin/cards/{id}", s.handleAdminUpdateCard)
				r.Get("/admin/rarities", s.handleAdminListRarities)
				r.Put("/admin/rarities/{id}", s.handleAdminUpdateRarity)
				r.Get("/admin/theme/export", s.handleAdminThemeExport)
				r.Post("/admin/theme/preview", s.handleAdminThemePreview)
				r.Post("/admin/theme/apply", s.handleAdminThemeApply)
				r.Get("/admin/art-lint", s.handleAdminArtLint)
				r.Get("/admin/art", s.handleAdminArtConfig)
				r.Get("/admin/art/exists", s.handleAdminArtExists)
				r.Post("/admin/art", s.handleAdminArtUpload)
				r.Get("/admin/players", s.handleAdminSearchPlayers)
				r.Get("/admin/players/{id}", s.handleAdminGetPlayer)
				r.Post("/admin/players/{id}/action", s.handleAdminPlayerAction)
				r.Get("/admin/stars", s.handleAdminStarTransactions)
				r.Post("/admin/stars/refund", s.handleAdminStarRefund)
				r.Get("/admin/promo", s.handleAdminListPromo)
				r.Post("/admin/promo", s.handleAdminCreatePromo)
				r.Delete("/admin/promo/{code}", s.handleAdminDeletePromo)
				r.Get("/admin/chats", s.handleAdminListChats)
				r.Put("/admin/chats/{platform}/{chatId}", s.handleAdminUpdateChat)
				r.Delete("/admin/chats/{platform}/{chatId}", s.handleAdminDeleteChat)
				r.Post("/admin/chats/{platform}/{chatId}/leave", s.handleAdminLeaveChat)
				r.Post("/admin/broadcast", s.handleAdminBroadcast)
				r.Get("/admin/dashboard", s.handleAdminDashboard)
				r.Get("/admin/audit", s.handleAdminAudit)
				r.Get("/admin/health", s.handleAdminHealth)
				r.Get("/admin/suggestions", s.handleAdminListSuggestions)
				r.Get("/admin/suggestions/{id}/image", s.handleAdminSuggestionImage)
				r.Post("/admin/suggestions/{id}/approve", s.handleAdminApproveSuggestion)
				r.Post("/admin/suggestions/{id}/reject", s.handleAdminRejectSuggestion)
				r.Get("/admin/sets", s.handleAdminListSets)
				r.Post("/admin/sets", s.handleAdminCreateSet)
				r.Put("/admin/sets/{id}", s.handleAdminUpdateSet)
			})
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
		// The shell carries the hashed asset urls, so a stale copy pins the app
		// to an old build — and Discord's proxy caches aggressively.
		w.Header().Set("Cache-Control", "no-store, must-revalidate")
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
