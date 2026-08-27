package httpapi

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"gachabot/internal/theme"
)

// maxThemeBytes bounds an uploaded theme document (the live theme is ~100 KB).
const maxThemeBytes = 8 << 20

// GET /api/admin/theme/export — the current content as a theme.json download.
func (s *Server) handleAdminThemeExport(w http.ResponseWriter, _ *http.Request) {
	t, err := theme.Export(s.repo.DB())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "export failed: "+err.Error())
		return
	}
	data, err := json.MarshalIndent(t, "", "  ")
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "encode failed")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", `attachment; filename="theme.json"`)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

// themePreviewDTO is what the import screen shows before anything is written.
type themePreviewDTO struct {
	Rarities      int      `json:"rarities"`
	Sets          int      `json:"sets"`
	Cards         int      `json:"cards"`
	CardsInserted int      `json:"cardsInserted"`
	CardsUpdated  int      `json:"cardsUpdated"`
	Warnings      []string `json:"warnings"`
}

// parseThemeBody reads, parses and validates an uploaded theme document.
func parseThemeBody(r *http.Request) (*theme.Theme, []string, error) {
	body, err := io.ReadAll(io.LimitReader(r.Body, maxThemeBytes))
	if err != nil {
		return nil, nil, fmt.Errorf("не удалось прочитать файл")
	}
	t, err := theme.Parse(body)
	if err != nil {
		return nil, nil, fmt.Errorf("неверный JSON: %w", err)
	}
	warnings, err := t.Validate()
	if err != nil {
		return nil, warnings, fmt.Errorf("невалидная тема: %w", err)
	}
	return t, warnings, nil
}

// POST /api/admin/theme/preview — dry-run: report what an import would change.
func (s *Server) handleAdminThemePreview(w http.ResponseWriter, r *http.Request) {
	t, warnings, err := parseThemeBody(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	rep, err := theme.Import(s.repo.DB(), t, theme.Options{DryRun: true})
	if err != nil {
		writeErr(w, http.StatusBadRequest, "проверка не удалась: "+err.Error())
		return
	}
	if warnings == nil {
		warnings = []string{}
	}
	writeJSON(w, http.StatusOK, themePreviewDTO{
		Rarities:      len(t.Rarities),
		Sets:          len(t.Sets),
		Cards:         len(t.Cards),
		CardsInserted: rep.CardsInserted,
		CardsUpdated:  rep.CardsUpdated,
		Warnings:      warnings,
	})
}

// POST /api/admin/theme/apply — write the theme. Additive by design: existing
// rows are upserted and card ids are preserved, so player collections survive.
// theme.Options.Reset (which wipes collections) is deliberately never exposed here.
func (s *Server) handleAdminThemeApply(w http.ResponseWriter, r *http.Request) {
	t, warnings, err := parseThemeBody(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	rep, err := theme.Import(s.repo.DB(), t, theme.Options{})
	if err != nil {
		writeErr(w, http.StatusBadRequest, "импорт не удался: "+err.Error())
		return
	}
	if warnings == nil {
		warnings = []string{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"raritiesUpserted": rep.RaritiesUpserted,
		"setsUpserted":     rep.SetsUpserted,
		"cardsInserted":    rep.CardsInserted,
		"cardsUpdated":     rep.CardsUpdated,
		"warnings":         warnings,
	})
}
