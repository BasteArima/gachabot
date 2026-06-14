package artguess

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
)

// Deep links carry the launch chat into the web app as a Telegram `startapp`
// value: <prefix><chatID>_<sig>. The signature (HMAC over the chat id, keyed by
// the bot token) stops a user from forging a chat id to post results into a chat
// they were never invited to play from. Two prefixes, both verified the same way:
//
//	artguess_ — from the "Играть" button: the frontend opens Art Guess directly.
//	agctx_    — from a roll's "Открыть приложение": just chat context, stays on hub.
const (
	deepLinkPrefix = "artguess_"
	contextPrefix  = "agctx_"
)

func (s *Service) sign(platform string, chatID int64) string {
	mac := hmac.New(sha256.New, []byte(s.secret))
	mac.Write([]byte("artguess:" + platform + ":" + strconv.FormatInt(chatID, 10)))
	return hex.EncodeToString(mac.Sum(nil))[:16]
}

// DeepLinkParam builds the signed "open Art Guess" startapp value (Play button).
func (s *Service) DeepLinkParam(platform string, chatID int64) string {
	return fmt.Sprintf("%s%d_%s", deepLinkPrefix, chatID, s.sign(platform, chatID))
}

// ContextParam builds the signed "chat context only" startapp value (roll's
// open-app button): carries the chat for attribution but doesn't open the game.
func (s *Service) ContextParam(platform string, chatID int64) string {
	return fmt.Sprintf("%s%d_%s", contextPrefix, chatID, s.sign(platform, chatID))
}

// parseLaunch verifies a startapp value (either prefix) for the given platform
// and returns the chat id it encodes.
func (s *Service) parseLaunch(platform, raw string) (int64, bool) {
	var rest string
	switch {
	case strings.HasPrefix(raw, deepLinkPrefix):
		rest = strings.TrimPrefix(raw, deepLinkPrefix)
	case strings.HasPrefix(raw, contextPrefix):
		rest = strings.TrimPrefix(raw, contextPrefix)
	default:
		return 0, false
	}
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

// dsLaunchPrefix marks an (unsigned) Discord launch context: "ds:<channelID>".
// Discord Activities can't carry our signed startapp value, so the channel comes
// from the authenticated Embedded SDK; we trust it only if it's a registered chat.
const dsLaunchPrefix = "ds:"

// resolveLaunch turns a raw launch value (Telegram signed startapp, or
// "ds:<channelID>") into the chat to attribute play to.
func (s *Service) resolveLaunch(ctx context.Context, launch string) (platform string, chatID int64, ok bool) {
	if strings.HasPrefix(launch, dsLaunchPrefix) {
		id, err := strconv.ParseInt(strings.TrimPrefix(launch, dsLaunchPrefix), 10, 64)
		if err != nil {
			return "", 0, false
		}
		if exists, _ := s.repo.ChatExists(PlatformDiscord, id); !exists {
			return "", 0, false
		}
		return PlatformDiscord, id, true
	}
	if id, valid := s.parseLaunch(PlatformTelegram, launch); valid {
		return PlatformTelegram, id, true
	}
	return "", 0, false
}
