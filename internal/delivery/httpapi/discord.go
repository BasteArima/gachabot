package httpapi

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

var discordHTTP = &http.Client{Timeout: 10 * time.Second}

type discordAuthRequest struct {
	Code        string `json:"code"`
	RedirectURI string `json:"redirectUri"`
}

// POST /api/auth/discord — exchange an OAuth2 authorization code (browser flow or
// Embedded App SDK) for a session. The code is exchanged server-side (needs the
// client secret), then /users/@me identifies the player.
func (s *Server) handleAuthDiscord(w http.ResponseWriter, r *http.Request) {
	if s.discord.ClientID == "" || s.discord.ClientSecret == "" {
		writeErr(w, http.StatusServiceUnavailable, "discord auth not configured")
		return
	}
	var req discordAuthRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Code == "" {
		writeErr(w, http.StatusBadRequest, "missing code")
		return
	}
	redirect := req.RedirectURI
	if redirect == "" {
		redirect = s.discord.OAuthRedirect
	}

	accessToken, err := s.discordExchangeCode(req.Code, redirect)
	if err != nil {
		writeErr(w, http.StatusUnauthorized, "discord token exchange failed: "+err.Error())
		return
	}
	du, err := discordFetchUser(accessToken)
	if err != nil {
		writeErr(w, http.StatusUnauthorized, "discord user fetch failed: "+err.Error())
		return
	}

	id, _ := strconv.ParseInt(du.ID, 10, 64)
	if id == 0 {
		writeErr(w, http.StatusUnauthorized, "bad discord id")
		return
	}
	name := du.GlobalName
	if name == "" {
		name = du.Username
	}
	user, err := s.repo.GetOrCreateUserByDiscordID(id, name)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	if du.Avatar != "" {
		avatarURL := fmt.Sprintf("https://cdn.discordapp.com/avatars/%s/%s.png", du.ID, du.Avatar)
		_ = s.repo.UpdateUserAvatar(user.ID, avatarURL)
		user.AvatarURL = avatarURL
	}
	token, err := s.createSession(user.ID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "session error")
		return
	}
	writeJSON(w, http.StatusOK, authResponse{Token: token, Player: s.toPlayer(user)})
}

func (s *Server) discordExchangeCode(code, redirect string) (string, error) {
	form := url.Values{}
	form.Set("client_id", s.discord.ClientID)
	form.Set("client_secret", s.discord.ClientSecret)
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("redirect_uri", redirect)

	resp, err := discordHTTP.Post(
		"https://discord.com/api/oauth2/token",
		"application/x-www-form-urlencoded",
		strings.NewReader(form.Encode()),
	)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("status %d: %s", resp.StatusCode, string(body))
	}
	var tr struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(body, &tr); err != nil {
		return "", err
	}
	if tr.AccessToken == "" {
		return "", fmt.Errorf("no access_token in response")
	}
	return tr.AccessToken, nil
}

type discordUser struct {
	ID         string `json:"id"`
	Username   string `json:"username"`
	GlobalName string `json:"global_name"`
	Avatar     string `json:"avatar"`
}

func discordFetchUser(accessToken string) (*discordUser, error) {
	req, _ := http.NewRequest(http.MethodGet, "https://discord.com/api/users/@me", nil)
	req.Header.Set("Authorization", "Bearer "+accessToken)
	resp, err := discordHTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %d: %s", resp.StatusCode, string(body))
	}
	var u discordUser
	if err := json.Unmarshal(body, &u); err != nil {
		return nil, err
	}
	return &u, nil
}
