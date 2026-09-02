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

	if s.art != nil && s.art.Enabled() {
		// 302 rather than 301: the art host is a deployment detail, and a
		// permanent redirect would be cached by clients long after it changed.
		http.Redirect(w, r, s.art.PublicBase()+rest, http.StatusFound)
		return
	}

	// No art host: serve the file from the local build. "/cdn/app/assets/x.js"
	// is "<static>/assets/x.js" — the /app prefix only exists remotely, to keep
	// the app's files apart from the card art in the same root.
	if s.cfg.StaticDir == "" {
		http.NotFound(w, r)
		return
	}
	local := strings.TrimPrefix(rest, "/app")
	full := filepath.Join(s.cfg.StaticDir, filepath.Clean(local))
	if relPath, err := filepath.Rel(s.cfg.StaticDir, full); err != nil || strings.HasPrefix(relPath, "..") {
		http.NotFound(w, r)
		return
	}
	if st, err := os.Stat(full); err != nil || st.IsDir() {
		http.NotFound(w, r)
		return
	}
	http.ServeFile(w, r, full)
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
