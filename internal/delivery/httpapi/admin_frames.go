package httpapi

import (
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
)

// Card frames are content, not code: gen.py composites frames/<Rarity>.png over
// the prepared art to produce cards_framed/. The bot never draws them, it just
// swaps /cards/ for /cards_framed/ in the url. To let the admin panel try a raw
// file on before the generator has run, the server hands those same PNGs out —
// so the deployment keeps them next to the binary, like the Art Guess overlay.
const defaultFramesDir = "assets/frames"

// Frame names mirror the raw_art folders (Rare, Mythical, …). Keep the charset
// tight and drop any path parts: the name reaches us straight from the URL.
var frameNameRe = regexp.MustCompile(`^[A-Za-z0-9 _-]{1,40}$`)

func framesDir() string {
	if dir := os.Getenv("CARD_FRAMES_DIR"); dir != "" {
		return dir
	}
	return defaultFramesDir
}

// GET /api/admin/frames — which frames the server has, and which rarity each one
// belongs to. The panel needs the second half because the editor knows a rarity
// id, not a frame file name.
func (s *Server) handleAdminListFrames(w http.ResponseWriter, _ *http.Request) {
	dir := framesDir()
	byRarity := map[string]string{}

	entries, err := os.ReadDir(dir)
	if err != nil {
		// A missing folder is a normal state (nothing deployed yet), not an error.
		writeJSON(w, http.StatusOK, map[string]any{"dir": dir, "frames": []string{}, "byRarity": byRarity})
		return
	}

	frames := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.EqualFold(filepath.Ext(e.Name()), ".png") {
			continue
		}
		name := strings.TrimSuffix(e.Name(), filepath.Ext(e.Name()))
		if frameNameRe.MatchString(name) {
			frames = append(frames, name)
		}
	}
	sort.Strings(frames)

	// Tie frames to rarities through the art folders the cards already use.
	if folders, err := s.repo.RarityArtFolders(); err == nil {
		for rarityID, folder := range folders {
			for _, f := range frames {
				if strings.EqualFold(f, folder) {
					byRarity[strconv.Itoa(rarityID)] = f
					break
				}
			}
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"dir": dir, "frames": frames, "byRarity": byRarity})
}

// GET /api/admin/frames/{name} — the frame PNG itself.
func (s *Server) handleAdminGetFrame(w http.ResponseWriter, r *http.Request) {
	name := filepath.Base(chi.URLParam(r, "name"))
	if !frameNameRe.MatchString(name) {
		writeErr(w, http.StatusBadRequest, "плохое имя рамки")
		return
	}

	path := filepath.Join(framesDir(), name+".png")
	if _, err := os.Stat(path); err != nil {
		writeErr(w, http.StatusNotFound, "рамка «"+name+"» не найдена на сервере")
		return
	}

	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "private, max-age=300")
	http.ServeFile(w, r, path)
}
