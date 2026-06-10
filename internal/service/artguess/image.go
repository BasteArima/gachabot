package artguess

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"io"
	"net/http"
	"time"

	"github.com/disintegration/imaging"
	_ "golang.org/x/image/webp" // register webp decoder for image.Decode
)

const (
	keySrcPrefix = "artguess:src:" // +cardID -> original art bytes
	keyImgPrefix = "artguess:img:" // +cardID:level -> rendered JPEG bytes
	srcTTL       = 7 * 24 * time.Hour
	maxBlur      = 14.0 // gaussian sigma at the most-hidden level
	minPixels    = 16   // smallest pixelation grid (level 0)
)

var imgClient = &http.Client{Timeout: 15 * time.Second}

// ImageData returns the JPEG bytes of today's card at the requested reveal level,
// clamped to what the player has earned (they can never fetch a clearer image
// than their progress allows). Results are cached per (card, level) in Redis.
func (s *Service) ImageData(ctx context.Context, uid int64, reqLevel int) ([]byte, error) {
	cfg := s.Config()
	dateStr := dateKey(s.midnight())
	rmap, err := s.rarityMap()
	if err != nil {
		return nil, err
	}
	targetID, err := s.dailyCardID(ctx, cfg, dateStr, rmap)
	if err != nil {
		return nil, err
	}

	p, err := s.loadProgress(ctx, uid, dateStr)
	if err != nil {
		return nil, err
	}
	allowed := revealLevel(len(p.Guesses), cfg.MaxAttempts, p.Finished)
	if reqLevel < 0 {
		reqLevel = 0
	}
	if reqLevel > allowed {
		reqLevel = allowed
	}

	imgKey := fmt.Sprintf("%s%d:%d", keyImgPrefix, targetID, reqLevel)
	if b, err := s.rdb.Get(ctx, imgKey).Bytes(); err == nil {
		return b, nil
	}

	src, err := s.sourceBytes(ctx, targetID)
	if err != nil {
		return nil, err
	}
	img, err := imaging.Decode(bytes.NewReader(src))
	if err != nil {
		return nil, fmt.Errorf("decode art: %w", err)
	}

	out := renderLevel(img, reqLevel, cfg.MaxAttempts)
	var buf bytes.Buffer
	if err := imaging.Encode(&buf, out, imaging.JPEG, imaging.JPEGQuality(82)); err != nil {
		return nil, err
	}
	_ = s.rdb.Set(ctx, imgKey, buf.Bytes(), dayTTL).Err()
	return buf.Bytes(), nil
}

// sourceBytes returns the original art bytes for a card, caching the download.
func (s *Service) sourceBytes(ctx context.Context, cardID int) ([]byte, error) {
	key := fmt.Sprintf("%s%d", keySrcPrefix, cardID)
	if b, err := s.rdb.Get(ctx, key).Bytes(); err == nil {
		return b, nil
	}
	card, err := s.repo.GetCardByID(cardID)
	if err != nil {
		return nil, err
	}
	if card.ImageURL == "" {
		return nil, fmt.Errorf("card %d has no art", cardID)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, card.ImageURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := imgClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch art: status %d", resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 20<<20)) // 20 MiB cap
	if err != nil {
		return nil, err
	}
	_ = s.rdb.Set(ctx, key, b, srcTTL).Err()
	return b, nil
}

// renderLevel hides the art by pixelation plus a gaussian blur, both easing off
// as the level rises. Level 0 is the most hidden; at maxLevel the original is
// returned untouched.
func renderLevel(img image.Image, level, maxLevel int) image.Image {
	if maxLevel <= 0 || level >= maxLevel {
		return img
	}
	w := img.Bounds().Dx()
	if w <= 0 {
		return img
	}
	frac := float64(level) / float64(maxLevel) // 0 at most hidden, ->1 near reveal

	// Pixelation: downscale then nearest-neighbour upscale. The grid grows
	// quadratically so early levels stay strongly blocky.
	small := minPixels + int(frac*frac*float64(w-minPixels))
	if small < 8 {
		small = 8
	}
	out := imaging.Resize(img, small, 0, imaging.NearestNeighbor)
	out = imaging.Resize(out, w, 0, imaging.NearestNeighbor)

	if sigma := (1 - frac) * maxBlur; sigma > 0.5 {
		out = imaging.Blur(out, sigma)
	}
	return out
}
