package httpapi

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

type ctxKey int

const userIDKey ctxKey = iota

// authMaxAge bounds how stale a Telegram auth payload may be at login time.
const authMaxAge = 24 * time.Hour

type tgAuthUser struct {
	ID        int64
	Username  string
	FirstName string
	LastName  string
}

// validateInitData verifies a Telegram Mini App initData string (HMAC key derived
// as HMAC_SHA256("WebAppData", botToken)) and returns the embedded user.
func validateInitData(initData, botToken string) (tgAuthUser, error) {
	var u tgAuthUser
	vals, err := url.ParseQuery(initData)
	if err != nil {
		return u, fmt.Errorf("malformed initData")
	}
	hash := vals.Get("hash")
	if hash == "" {
		return u, fmt.Errorf("missing hash")
	}

	secret := hmacSum([]byte("WebAppData"), []byte(botToken))
	if !hmacEqual(secret, dataCheckString(vals, "hash"), hash) {
		return u, fmt.Errorf("invalid signature")
	}
	if !freshAuthDate(vals.Get("auth_date")) {
		return u, fmt.Errorf("auth data expired")
	}

	userJSON := vals.Get("user")
	if userJSON == "" {
		return u, fmt.Errorf("missing user")
	}
	var p struct {
		ID        int64  `json:"id"`
		Username  string `json:"username"`
		FirstName string `json:"first_name"`
		LastName  string `json:"last_name"`
	}
	if err := json.Unmarshal([]byte(userJSON), &p); err != nil {
		return u, fmt.Errorf("bad user payload")
	}
	return tgAuthUser{ID: p.ID, Username: p.Username, FirstName: p.FirstName, LastName: p.LastName}, nil
}

// validateLoginWidget verifies Telegram Login Widget data (flat fields; HMAC key
// is SHA256(botToken)).
func validateLoginWidget(fields map[string]string, botToken string) (tgAuthUser, error) {
	var u tgAuthUser
	hash := fields["hash"]
	if hash == "" {
		return u, fmt.Errorf("missing hash")
	}
	vals := url.Values{}
	for k, v := range fields {
		vals.Set(k, v)
	}
	sum := sha256.Sum256([]byte(botToken))
	if !hmacEqual(sum[:], dataCheckString(vals, "hash"), hash) {
		return u, fmt.Errorf("invalid signature")
	}
	if !freshAuthDate(fields["auth_date"]) {
		return u, fmt.Errorf("auth data expired")
	}
	id, _ := strconv.ParseInt(fields["id"], 10, 64)
	if id == 0 {
		return u, fmt.Errorf("missing id")
	}
	return tgAuthUser{ID: id, Username: fields["username"], FirstName: fields["first_name"], LastName: fields["last_name"]}, nil
}

// dataCheckString builds Telegram's check string: all fields except omit, sorted
// by key, joined as "key=value" with newlines.
func dataCheckString(vals url.Values, omit string) string {
	keys := make([]string, 0, len(vals))
	for k := range vals {
		if k == omit {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)
	pairs := make([]string, 0, len(keys))
	for _, k := range keys {
		pairs = append(pairs, k+"="+vals.Get(k))
	}
	return strings.Join(pairs, "\n")
}

func hmacSum(key, msg []byte) []byte {
	m := hmac.New(sha256.New, key)
	m.Write(msg)
	return m.Sum(nil)
}

func hmacEqual(secret []byte, data, expectedHex string) bool {
	m := hmac.New(sha256.New, secret)
	m.Write([]byte(data))
	exp, err := hex.DecodeString(expectedHex)
	if err != nil {
		return false
	}
	return hmac.Equal(m.Sum(nil), exp)
}

func freshAuthDate(s string) bool {
	if s == "" {
		return true // some widget configs omit it; don't hard-fail
	}
	ts, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return false
	}
	return time.Since(time.Unix(ts, 0)) <= authMaxAge
}

// --- Sessions (opaque token in Redis, sent as Bearer) ---

func (s *Server) createSession(userID int64) (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	token := hex.EncodeToString(b)
	if err := s.rdb.Set(context.Background(), "session:"+token, userID, 7*24*time.Hour).Err(); err != nil {
		return "", err
	}
	return token, nil
}

func (s *Server) sessionUser(token string) (int64, bool) {
	id, err := s.rdb.Get(context.Background(), "session:"+token).Int64()
	if err != nil {
		return 0, false
	}
	return id, true
}

// authMiddleware resolves the session token into a user id. With DevAllowNoAuth,
// unauthenticated requests fall back to the admin user (local dev only).
func (s *Server) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if token := bearer(r); token != "" {
			if id, ok := s.sessionUser(token); ok {
				next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), userIDKey, id)))
				return
			}
		}
		if s.cfg.DevAllowNoAuth && s.adminID != 0 {
			if u, err := s.repo.GetOrCreateUserByTelegramID(s.adminID, "dev", "Dev", ""); err == nil {
				next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), userIDKey, u.ID)))
				return
			}
		}
		writeErr(w, http.StatusUnauthorized, "unauthorized")
	})
}

func bearer(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if strings.HasPrefix(h, "Bearer ") {
		return strings.TrimSpace(strings.TrimPrefix(h, "Bearer "))
	}
	return ""
}

func userIDFrom(r *http.Request) int64 {
	id, _ := r.Context().Value(userIDKey).(int64)
	return id
}
