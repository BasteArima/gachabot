package httpapi

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"sync"
	"time"

	"gachabot/internal/cardart"
	"gachabot/internal/repository"
)

const (
	artLintWorkers   = 12
	artLintTimeout   = 8 * time.Second
	artLintTotalCap  = 3 * time.Minute
	problemNoURL     = "no_url"
	problemFrameless = "frameless"
	problemFramed    = "framed"
)

var artLintClient = &http.Client{Timeout: artLintTimeout}

type artIssue struct {
	CardID     int    `json:"cardId"`
	Name       string `json:"name"`
	RarityName string `json:"rarityName"`
	Problem    string `json:"problem"`
	URL        string `json:"url"`
	Detail     string `json:"detail"`
}

// GET /api/admin/art-lint — checks every card's art: that a URL is set, that the
// frameless art resolves, and that the framed counterpart exists (reveal surfaces
// use it, so a missing framed file shows a broken image in chat).
func (s *Server) handleAdminArtLint(w http.ResponseWriter, r *http.Request) {
	cards, err := s.repo.ListAdminCards(repository.CardFilter{})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), artLintTotalCap)
	defer cancel()

	started := time.Now()
	var (
		mu       sync.Mutex
		issues   []artIssue
		checked  int
		urlCache = map[string]string{} // url -> "" ok, otherwise the problem detail
	)

	// probe reports why a URL is unusable, or "" when it is fine. Results are
	// memoised so cards sharing an art file are only fetched once.
	probe := func(url string) string {
		mu.Lock()
		if cached, ok := urlCache[url]; ok {
			mu.Unlock()
			return cached
		}
		mu.Unlock()

		detail := fetchStatus(ctx, url)

		mu.Lock()
		urlCache[url] = detail
		mu.Unlock()
		return detail
	}

	jobs := make(chan repository.AdminCard)
	var wg sync.WaitGroup
	for i := 0; i < artLintWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for c := range jobs {
				var found []artIssue
				switch {
				case c.ImageURL == "":
					found = append(found, artIssue{c.ID, c.Name, c.RarityName, problemNoURL, "", "ссылка на арт не задана"})
				default:
					if d := probe(c.ImageURL); d != "" {
						found = append(found, artIssue{c.ID, c.Name, c.RarityName, problemFrameless, c.ImageURL, d})
					}
					if framed := cardart.Framed(c.ImageURL); framed != c.ImageURL {
						if d := probe(framed); d != "" {
							found = append(found, artIssue{c.ID, c.Name, c.RarityName, problemFramed, framed, d})
						}
					}
				}
				mu.Lock()
				issues = append(issues, found...)
				checked++
				mu.Unlock()
			}
		}()
	}
	for _, c := range cards {
		select {
		case jobs <- c:
		case <-ctx.Done():
		}
	}
	close(jobs)
	wg.Wait()

	sort.Slice(issues, func(i, j int) bool {
		if issues[i].Problem != issues[j].Problem {
			return issues[i].Problem < issues[j].Problem
		}
		return issues[i].Name < issues[j].Name
	})
	if issues == nil {
		issues = []artIssue{}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"checked":    checked,
		"total":      len(cards),
		"issues":     issues,
		"durationMs": time.Since(started).Milliseconds(),
	})
}

// fetchStatus returns "" when the URL serves an image, or a short reason. HEAD is
// tried first; hosts that reject it fall back to a 1-byte ranged GET.
func fetchStatus(ctx context.Context, url string) string {
	status, err := requestStatus(ctx, http.MethodHead, url, false)
	if err != nil {
		return err.Error()
	}
	if status == http.StatusMethodNotAllowed || status == http.StatusNotImplemented || status == http.StatusForbidden {
		status, err = requestStatus(ctx, http.MethodGet, url, true)
		if err != nil {
			return err.Error()
		}
	}
	if status >= 200 && status < 300 {
		return ""
	}
	return fmt.Sprintf("HTTP %d", status)
}

func requestStatus(ctx context.Context, method, url string, ranged bool) (int, error) {
	req, err := http.NewRequestWithContext(ctx, method, url, nil)
	if err != nil {
		return 0, fmt.Errorf("неверный URL")
	}
	if ranged {
		req.Header.Set("Range", "bytes=0-0")
	}
	resp, err := artLintClient.Do(req)
	if err != nil {
		return 0, fmt.Errorf("недоступен")
	}
	defer resp.Body.Close()
	return resp.StatusCode, nil
}
