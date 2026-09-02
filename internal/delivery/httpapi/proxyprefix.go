package httpapi

import (
	"bytes"
	"compress/gzip"
	"crypto/rand"
	"encoding/base64"
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

// Probe assets answer questions from inside a Discord Activity, where there are
// no developer tools. Two rounds have narrowed it down: the path shape does not
// matter, and what breaks is the number of bytes on the wire — 8 KB arrives,
// 32 KB does not, while 400 KB that gzip squeezed to 481 bytes arrives fine.
//
// So the padding can be made incompressible on request: only then is the wire
// size known for certain, whatever compresses the response along the way.
const probeMaxSize = 2 << 20

func (s *Server) handleProbeAsset(w http.ResponseWriter, r *http.Request) {
	size := 1 << 10
	if strings.Contains(r.URL.Path, "large") {
		size = 400 << 10
	}
	if q := r.URL.Query().Get("size"); q != "" {
		if n, err := strconv.Atoi(q); err == nil && n > 0 && n <= probeMaxSize {
			size = n
		}
	}

	// Valid JavaScript of a known length: a comment padded to the target size, so
	// a truncated or mangled body is obvious from the byte count alone. The body
	// is deliberately repetitive, which is also what makes the gzip case a fair
	// test of "does compressing it help".
	head := "/* gachabot proxy probe, " + strconv.Itoa(size) + " bytes */\n"
	body := make([]byte, size)
	copy(body, head)
	if r.URL.Query().Get("rand") == "1" {
		// Random base64 defeats compression anywhere along the path, so the wire
		// size is exactly the size being tested — which is the thing that broke.
		raw := make([]byte, size)
		if _, err := rand.Read(raw); err == nil {
			copy(body[len(head):], base64.StdEncoding.EncodeToString(raw))
		}
	} else {
		for i := len(head); i < size; i++ {
			body[i] = '.'
		}
	}

	w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")

	if r.URL.Query().Get("gz") == "1" && strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
		var buf bytes.Buffer
		zw := gzip.NewWriter(&buf)
		_, _ = zw.Write(body)
		_ = zw.Close()

		w.Header().Set("Content-Encoding", "gzip")
		w.Header().Set("Vary", "Accept-Encoding")
		w.Header().Set("Content-Length", strconv.Itoa(buf.Len()))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(buf.Bytes())
		return
	}

	w.Header().Set("Content-Length", strconv.Itoa(size))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}
