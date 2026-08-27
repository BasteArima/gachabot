package httpapi

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"

	"gachabot/internal/service/artstore"
)

// Card art is prepared in the browser (crop to the card size, frame composited
// on a canvas, encoded with canvas.toBlob) and only stored here. The server does
// no image processing: the binary is built with CGO_ENABLED=0 and Go has no
// pure-Go lossy WebP encoder, while every browser has one built in. What the
// admin sees on the canvas is exactly the bytes that get written.
const (
	maxArtFileBytes  = 8 << 20  // one file
	maxArtUploadForm = 20 << 20 // whole request
)

// GET /api/admin/art — where uploads would go, and whether they can go at all.
func (s *Server) handleAdminArtConfig(w http.ResponseWriter, _ *http.Request) {
	folders := map[string]string{}
	if m, err := s.repo.RarityArtFolders(); err == nil {
		for rarityID, folder := range m {
			folders[strconv.Itoa(rarityID)] = folder
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"enabled":    s.art.Enabled(),
		"driver":     s.art.DriverName(),
		"publicBase": s.art.PublicBase(),
		// rarity id → the English art folder its cards already use.
		"folders": folders,
	})
}

// GET /api/admin/art/exists?folder=Rare&slug=bunny_suit — what a save would hit.
func (s *Server) handleAdminArtExists(w http.ResponseWriter, r *http.Request) {
	folder := r.URL.Query().Get("folder")
	slug := r.URL.Query().Get("slug")

	found, err := s.art.Existing(folder, slug)
	if errors.Is(err, artstore.ErrDisabled) {
		writeErr(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"taken": found})
}

// POST /api/admin/art (multipart: folder, slug, overwrite, plain, framed)
// Writes the frameless and framed files and reports the urls they are served
// under. Creating the card row is a separate call — art first, so a database
// failure leaves usable files rather than a card pointing at nothing.
func (s *Server) handleAdminArtUpload(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(maxArtUploadForm); err != nil {
		writeErr(w, http.StatusBadRequest, "не удалось прочитать форму: "+err.Error())
		return
	}
	defer r.MultipartForm.RemoveAll()

	folder := r.FormValue("folder")
	slug := r.FormValue("slug")
	overwrite := r.FormValue("overwrite") == "true"

	plain, plainExt, err := readArtFile(r, "plain")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "без рамки: "+err.Error())
		return
	}
	framed, framedExt, err := readArtFile(r, "framed")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "с рамкой: "+err.Error())
		return
	}
	// Both halves must be the same format, or the url pair would need two
	// extensions and cardart.Framed could no longer derive one from the other.
	if plainExt != framedExt {
		writeErr(w, http.StatusBadRequest, "файлы в разных форматах: "+plainExt+" и "+framedExt)
		return
	}

	plainURL, framedURL, err := s.art.Put(folder, slug, plainExt, plain, framed, overwrite)
	switch {
	case errors.Is(err, artstore.ErrDisabled):
		writeErr(w, http.StatusServiceUnavailable, err.Error())
		return
	case errors.Is(err, artstore.ErrExists):
		writeErr(w, http.StatusConflict, err.Error())
		return
	case err != nil:
		log.Printf("[ADMIN] art upload %s/%s failed: %v", folder, slug, err)
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}

	log.Printf("[ADMIN] art uploaded %s/%s%s (%d + %d bytes)", folder, slug, plainExt, len(plain), len(framed))
	writeJSON(w, http.StatusOK, map[string]any{"imageUrl": plainURL, "framedUrl": framedURL})
}

// readArtFile pulls one uploaded image out of the form and identifies it by its
// magic bytes rather than by the name or content type the client claims. Safari
// silently hands back PNG when asked for WebP, so the real format decides the
// extension the file is stored under.
func readArtFile(r *http.Request, field string) (data []byte, ext string, err error) {
	f, hdr, err := r.FormFile(field)
	if err != nil {
		return nil, "", fmt.Errorf("файл не приложен")
	}
	defer f.Close()

	if hdr.Size > maxArtFileBytes {
		return nil, "", fmt.Errorf("файл больше %d МБ", maxArtFileBytes>>20)
	}
	data, err = io.ReadAll(io.LimitReader(f, maxArtFileBytes+1))
	if err != nil {
		return nil, "", err
	}
	if len(data) > maxArtFileBytes {
		return nil, "", fmt.Errorf("файл больше %d МБ", maxArtFileBytes>>20)
	}

	switch {
	case len(data) >= 12 && bytes.Equal(data[0:4], []byte("RIFF")) && bytes.Equal(data[8:12], []byte("WEBP")):
		return data, ".webp", nil
	case bytes.HasPrefix(data, []byte("\x89PNG\r\n\x1a\n")):
		return data, ".png", nil
	default:
		return nil, "", fmt.Errorf("это не webp и не png")
	}
}
