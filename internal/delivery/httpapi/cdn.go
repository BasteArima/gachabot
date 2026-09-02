package httpapi

import (
	"log"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"

	"gachabot/internal/service/artstore"
)

// The app's own static files are served from the art host rather than from here.
// The reason is measured, not theoretical: this server cannot deliver responses
// much over 27 KB to some networks. Telegram hit it first (card art stopped
// loading, which is why the art moved to a separate host), and a Discord
// Activity hits it with the 345 KB bundle — the request arrives, this server
// answers 200 with the whole body and logs no error, and the bytes never reach
// the client. The same files served from the art host arrive fine.
//
// So the built asset urls point at /cdn/app/…, and who resolves that depends on
// the caller:
//
//	Discord Activity — a proxy path mapping in the Developer Portal sends /cdn
//	                   straight to the art host, so those bytes never touch here.
//	browser, Telegram — this handler redirects there. A redirect is a couple of
//	                   hundred bytes, well under what this server can deliver.
//	no art host configured — the files are served from disk, so a self-hosted
//	                   copy with no external storage still works.
const (
	cdnPrefix    = "/cdn"
	assetsRemote = "app/assets"
)

func (s *Server) handleCDN(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, cdnPrefix)
	if rest == "" || rest == "/" {
		http.NotFound(w, r)
		return
	}
	// Refused outright rather than forwarded: a redirect would hand the climb to
	// the art host, and its idea of the path is not ours to guess.
	if strings.Contains(rest, "..") {
		http.NotFound(w, r)
		return
	}

	// Own file first, redirect second. Only a Discord Activity has to reach the
	// art host, and it never asks this server — its proxy mapping goes there
	// directly. Everyone else is a browser, which this server can serve
	// perfectly well, so serving locally when the file is here removes a whole
	// failure mode: a publish that did not land would otherwise point every
	// client at a file the art host does not have, breaking the app everywhere
	// rather than only inside Discord.
	if full, ok := s.localCDNFile(rest); ok {
		http.ServeFile(w, r, full)
		return
	}

	// Not ours — card art, or an asset this deployment does not carry. Knowing
	// where those are served from is enough to send the caller there.
	if s.art != nil && s.art.PublicBase() != "" {
		// 302 rather than 301: the art host is a deployment detail, and a
		// permanent redirect would be cached long after it changed.
		http.Redirect(w, r, s.art.PublicBase()+rest, http.StatusFound)
		return
	}
	http.NotFound(w, r)
}

// localCDNFile maps "/app/assets/x.js" onto "<static>/assets/x.js" — the /app
// prefix exists only on the art host, to keep the app's files apart from the
// card art sharing that root — and reports whether it is actually there.
func (s *Server) localCDNFile(rest string) (string, bool) {
	if s.cfg.StaticDir == "" || !strings.HasPrefix(rest, "/app/") {
		return "", false
	}
	full := filepath.Join(s.cfg.StaticDir, filepath.Clean(strings.TrimPrefix(rest, "/app")))
	if relPath, err := filepath.Rel(s.cfg.StaticDir, full); err != nil || strings.HasPrefix(relPath, "..") {
		return "", false
	}
	if st, err := os.Stat(full); err != nil || st.IsDir() {
		return "", false
	}
	return full, true
}

// PublishAssets copies the built asset files to the art host, skipping the ones
// already there. Names carry a content hash, so a redeploy uploads only what
// changed and an unchanged deploy uploads nothing.
//
// Failure is not fatal: the app still serves everything itself, which works
// everywhere except inside Discord.
func PublishAssets(art *artstore.Service, staticDir string) {
	if art == nil || !art.Enabled() || staticDir == "" {
		return
	}
	dir := filepath.Join(staticDir, "assets")
	if st, err := os.Stat(dir); err != nil || !st.IsDir() {
		log.Printf("[CDN] нечего выкладывать: %s не найдена", dir)
		return
	}

	uploaded, skipped, err := art.SyncDir(dir, path.Clean(assetsRemote))
	if err != nil {
		log.Printf("[CDN] выкладка на %s не удалась: %v (файлы по-прежнему отдаются локально)", art.PublicBase(), err)
		return
	}
	log.Printf("[CDN] выложено на %s: новых %d, уже было %d", art.PublicBase()+"/"+assetsRemote, uploaded, skipped)
}

// GET /api/config — the handful of deployment facts the frontend needs before it
// can render. Public: it holds no secrets, and the art host has to be known
// before the first card image is drawn, which happens on some screens before a
// session exists.
func (s *Server) handleConfig(w http.ResponseWriter, _ *http.Request) {
	base := ""
	if s.art != nil {
		base = s.art.PublicBase()
	}
	writeJSON(w, http.StatusOK, map[string]string{"artBase": base})
}
