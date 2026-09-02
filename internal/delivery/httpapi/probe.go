package httpapi

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// TEMPORARY DIAGNOSTIC. A Discord Activity is a webview with no developer tools,
// and its screen has been black — so the page cannot be asked to show anything
// useful. Instead the probe in index.html posts what it measured here, and the
// results are read back over plain HTTP from any browser.
//
// Both endpoints are unauthenticated on purpose: inside the Activity there is no
// session yet (that is downstream of the bundle that fails to load), and the
// report holds nothing private — urls, byte counts, timings, user agent. Remove
// this file once the Activity works.
const (
	probeMaxReports = 20
	probeMaxBody    = 8 << 10
)

type probeReport struct {
	At    time.Time
	Agent string
	Href  string
	Build string
	Lines []string
}

var probeLog struct {
	sync.Mutex
	reports []probeReport
}

// POST /api/probe — the Activity reports what it measured.
func (s *Server) handleProbeReport(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, probeMaxBody))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad body")
		return
	}

	var in struct {
		Href  string   `json:"href"`
		Build string   `json:"build"`
		Lines []string `json:"lines"`
	}
	if err := json.Unmarshal(body, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "bad json")
		return
	}
	if len(in.Lines) > 40 {
		in.Lines = in.Lines[:40]
	}

	rep := probeReport{
		At:    time.Now(),
		Agent: r.UserAgent(),
		Href:  in.Href,
		Build: in.Build,
		Lines: in.Lines,
	}

	probeLog.Lock()
	probeLog.reports = append(probeLog.reports, rep)
	if len(probeLog.reports) > probeMaxReports {
		probeLog.reports = probeLog.reports[len(probeLog.reports)-probeMaxReports:]
	}
	probeLog.Unlock()

	// Also to the container log, so it can be read with `docker logs` too.
	for _, l := range rep.Lines {
		fmt.Printf("[PROBE] %s\n", l)
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// GET /api/probe — read the reports as plain text in any browser.
func (s *Server) handleProbeRead(w http.ResponseWriter, _ *http.Request) {
	probeLog.Lock()
	reports := append([]probeReport(nil), probeLog.reports...)
	probeLog.Unlock()

	var b strings.Builder
	if len(reports) == 0 {
		b.WriteString("отчётов пока нет — открой Activity в Discord и обнови эту страницу\n")
	}
	for i := len(reports) - 1; i >= 0; i-- { // newest first
		rep := reports[i]
		fmt.Fprintf(&b, "=== %s · сборка %s\n%s\n%s\n",
			rep.At.Format("2006-01-02 15:04:05"), rep.Build, rep.Href, rep.Agent)
		for _, l := range rep.Lines {
			fmt.Fprintf(&b, "  %s\n", l)
		}
		b.WriteString("\n")
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = io.WriteString(w, b.String())
}
