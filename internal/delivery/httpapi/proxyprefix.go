package httpapi

import (
	"net/http"
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
