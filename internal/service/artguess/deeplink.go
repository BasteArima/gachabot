package artguess

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
)

// Deep links carry the launch chat into the web app as a Telegram `startapp`
// value: artguess_<chatID>_<sig>. The signature (HMAC over the chat id, keyed by
// the bot token) stops a user from forging a chat id to post results into a chat
// they were never invited to play from.
const deepLinkPrefix = "artguess_"

func (s *Service) sign(platform string, chatID int64) string {
	mac := hmac.New(sha256.New, []byte(s.secret))
	mac.Write([]byte("artguess:" + platform + ":" + strconv.FormatInt(chatID, 10)))
	return hex.EncodeToString(mac.Sum(nil))[:16]
}

// DeepLinkParam builds the signed startapp value for a chat.
func (s *Service) DeepLinkParam(platform string, chatID int64) string {
	return fmt.Sprintf("%s%d_%s", deepLinkPrefix, chatID, s.sign(platform, chatID))
}

// parseLaunch verifies a startapp value for the given platform and returns the
// chat id it encodes.
func (s *Service) parseLaunch(platform, raw string) (int64, bool) {
	if !strings.HasPrefix(raw, deepLinkPrefix) {
		return 0, false
	}
	rest := strings.TrimPrefix(raw, deepLinkPrefix)
	sep := strings.LastIndex(rest, "_")
	if sep <= 0 || sep == len(rest)-1 {
		return 0, false
	}
	chatID, err := strconv.ParseInt(rest[:sep], 10, 64)
	if err != nil {
		return 0, false
	}
	if !hmac.Equal([]byte(rest[sep+1:]), []byte(s.sign(platform, chatID))) {
		return 0, false
	}
	return chatID, true
}
