package httpapi

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
)

// Discord Activities run inside an iframe on https://<app_id>.discordsays.com and
// everything they request is fetched by Discord's proxy from the mapped target.
// Discord's own convention prefixes those requests with /.proxy — so the app may
// ask for /.proxy/assets/index.js or /.proxy/api/me, and both must land on the
// same handlers as the unprefixed paths.
//
// Stripping the prefix here (rather than duplicating routes) keeps one build
// working everywhere: the browser and the Telegram Mini App can use either form.
const proxyPrefix = "/.proxy"

func stripProxyPrefix(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == proxyPrefix || strings.HasPrefix(r.URL.Path, proxyPrefix+"/") {
			trimmed := strings.TrimPrefix(r.URL.Path, proxyPrefix)
			if trimmed == "" {
				trimmed = "/"
			}
			r.URL.Path = trimmed
			// chi routes on its own copy of the path once a route context exists.
			if rctx := chi.RouteContext(r.Context()); rctx != nil && rctx.RoutePath != "" {
				rctx.RoutePath = trimmed
			}
		}
		next.ServeHTTP(w, r)
	})
}

// Probe assets exist to answer one question from inside a Discord Activity,
// where no developer tools are available: does Discord's proxy break on the path
// shape (/assets vs /.proxy/assets) or on the response size? They are served
// from memory and are harmless anywhere else.
const (
	probeTinyBytes  = 1 << 10
	probeLargeBytes = 400 << 10
)

func (s *Server) handleProbeAsset(w http.ResponseWriter, r *http.Request) {
	size := probeTinyBytes
	if strings.Contains(r.URL.Path, "large") {
		size = probeLargeBytes
	}

	// Valid JavaScript of a known length: a comment padded to the target size,
	// so a truncated or mangled body is obvious from the byte count alone.
	head := "/* gachabot proxy probe, " + strconv.Itoa(size) + " bytes */\n"
	body := make([]byte, size)
	copy(body, head)
	for i := len(head); i < size; i++ {
		body[i] = '.'
	}

	w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Length", strconv.Itoa(size))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}
