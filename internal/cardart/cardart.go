// Package cardart maps a card's stored (frameless) art URL to its framed
// variant. The DB stores frameless URLs (better for inventory / Art Guess);
// framed art lives in a parallel path and is used only on "reveal" surfaces
// (roll, craft, spawns, duels). The mapping is a single path-segment
// replacement, configurable via env so it isn't hard-coded.
//
//	FRAMED_ART_FROM (default "/cards/")  -> FRAMED_ART_TO (default "/cards_framed/")
//
// e.g. https://api.baste.ru/cards/Epic/x.webp -> .../cards_framed/Epic/x.webp
package cardart

import (
	"os"
	"strings"
)

var (
	from = envOr("FRAMED_ART_FROM", "/cards/")
	to   = envOr("FRAMED_ART_TO", "/cards_framed/")
)

// Framed returns the framed-art URL for a stored frameless URL. If the URL is
// empty or doesn't contain the source segment, it's returned unchanged.
func Framed(url string) string {
	if url == "" || from == "" || !strings.Contains(url, from) {
		return url
	}
	return strings.Replace(url, from, to, 1)
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
