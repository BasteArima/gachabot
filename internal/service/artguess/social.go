package artguess

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"strconv"
	"strings"
	"time"

	"gachabot/internal/models"
)

// Platform identifiers (match repository.chats.platform values).
const (
	PlatformTelegram = "telegram"
	PlatformDiscord  = "discord"
)

const (
	keyPartPrefix   = "artguess:part:"   // +plat:chatID:date -> hash uid->participant JSON
	keyBoardPrefix  = "artguess:board:"  // +plat:chatID:date -> scoreboard message id
	keyBoardsSet    = "artguess:boards:" // +date -> set of "plat:chatID" with a board
	keyStreakPrefix = "artguess:streak:" // +plat:chatID -> {count,last} group streak
)

// Broadcaster is implemented by each delivery layer so the engine can post and
// update the chat scoreboard without knowing platform details. Text is fully
// rendered (plain, no markup) so it reads identically on Telegram and Discord.
type Broadcaster interface {
	PostBoard(chat models.Chat, text, startParam string) (messageID int64, err error)
	EditBoard(chat models.Chat, messageID int64, text, startParam string) error
	PostSummary(chat models.Chat, text string, art *models.Card) error
}

// RegisterBroadcaster wires a delivery layer in (call before Start).
func (s *Service) RegisterBroadcaster(platform string, b Broadcaster) {
	s.broadcasters[platform] = b
}

// participant is one player's standing in a chat's daily scoreboard.
type participant struct {
	Name     string `json:"name"`
	Attempts int    `json:"attempts"`
	Solved   bool   `json:"solved"`
	Finished bool   `json:"finished"`
}

func pcd(platform string, chatID int64, date string) string {
	return platform + ":" + strconv.FormatInt(chatID, 10) + ":" + date
}

// onPlay records the player's standing in the launch chat and refreshes the
// scoreboard. Called on every guess so the board reflects "in progress" too.
func (s *Service) onPlay(ctx context.Context, platform string, chatID, uid int64, p progress) {
	date := dateKey(s.midnight())
	name := "Игрок"
	if u, err := s.repo.GetUserByID(uid); err == nil {
		name = displayName(u)
	}
	raw, _ := json.Marshal(participant{Name: name, Attempts: len(p.Guesses), Solved: p.Solved, Finished: p.Finished})

	partKey := keyPartPrefix + pcd(platform, chatID, date)
	s.rdb.HSet(ctx, partKey, strconv.FormatInt(uid, 10), raw)
	s.rdb.Expire(ctx, partKey, dayTTL)
	s.rdb.SAdd(ctx, keyBoardsSet+date, platform+":"+strconv.FormatInt(chatID, 10))
	s.rdb.Expire(ctx, keyBoardsSet+date, dayTTL)

	go s.refreshBoard(context.Background(), platform, chatID, date)
}

// ShowBoard posts (or refreshes) the chat scoreboard on demand — used by the
// /artguess command and the morning ping.
func (s *Service) ShowBoard(ctx context.Context, platform string, chatID int64) {
	cfg := s.Config()
	if !cfg.Enabled || !cfg.ChatPostEnabled {
		return
	}
	date := dateKey(s.midnight())
	s.rdb.SAdd(ctx, keyBoardsSet+date, platform+":"+strconv.FormatInt(chatID, 10))
	s.rdb.Expire(ctx, keyBoardsSet+date, dayTTL)
	s.refreshBoard(ctx, platform, chatID, date)
}

func (s *Service) loadParticipants(ctx context.Context, platform string, chatID int64, date string) []participant {
	m, err := s.rdb.HGetAll(ctx, keyPartPrefix+pcd(platform, chatID, date)).Result()
	if err != nil {
		return nil
	}
	parts := make([]participant, 0, len(m))
	for _, v := range m {
		var p participant
		if json.Unmarshal([]byte(v), &p) == nil {
			parts = append(parts, p)
		}
	}
	return parts
}

// refreshBoard renders the current scoreboard and posts or edits the chat message.
func (s *Service) refreshBoard(ctx context.Context, platform string, chatID int64, date string) {
	b := s.broadcasters[platform]
	if b == nil {
		return
	}
	cfg := s.Config()
	parts := s.loadParticipants(ctx, platform, chatID, date)
	text := renderBoard(puzzleNumber(s.midnight(), s.epoch(cfg)), cfg.MaxAttempts, parts)
	startParam := s.DeepLinkParam(platform, chatID)
	chat := models.Chat{Platform: platform, ChatID: chatID}
	boardKey := keyBoardPrefix + pcd(platform, chatID, date)

	if mid, err := s.rdb.Get(ctx, boardKey).Int64(); err == nil {
		if err := b.EditBoard(chat, mid, text, startParam); err != nil {
			log.Printf("[artguess] edit board failed (%s:%d): %v", platform, chatID, err)
		}
		return
	}
	mid, err := b.PostBoard(chat, text, startParam)
	if err != nil {
		log.Printf("[artguess] post board failed (%s:%d): %v", platform, chatID, err)
		return
	}
	s.rdb.Set(ctx, boardKey, mid, dayTTL)
}

// --- Group streak ---

type streakState struct {
	Count int    `json:"count"`
	Last  string `json:"last"` // last date (YYYY-MM-DD) the chat had participants
}

// bumpStreak advances the chat's consecutive-days streak for the given (just
// ended) date and returns the new count.
func (s *Service) bumpStreak(ctx context.Context, platform string, chatID int64, date string) int {
	key := keyStreakPrefix + platform + ":" + strconv.FormatInt(chatID, 10)
	var st streakState
	if raw, err := s.rdb.Get(ctx, key).Bytes(); err == nil {
		_ = json.Unmarshal(raw, &st)
	}
	if st.Last == s.dayBefore(date) {
		st.Count++
	} else {
		st.Count = 1
	}
	st.Last = date
	raw, _ := json.Marshal(st)
	s.rdb.Set(ctx, key, raw, 30*24*time.Hour)
	return st.Count
}

func (s *Service) dayBefore(date string) string {
	t, err := time.ParseInLocation("2006-01-02", date, s.loc)
	if err != nil {
		return ""
	}
	return t.Add(-24 * time.Hour).Format("2006-01-02")
}

// puzzleNumberFor computes the player-visible #N for a given date string.
func (s *Service) puzzleNumberFor(date string, cfg Config) int {
	t, err := time.ParseInLocation("2006-01-02", date, s.loc)
	if err != nil {
		return 0
	}
	return puzzleNumber(t, s.epoch(cfg))
}

// --- Rendering (plain text; identical on both platforms) ---

func renderBoard(number, maxAttempts int, parts []participant) string {
	var b strings.Builder
	fmt.Fprintf(&b, "🖼 Карта дня #%d\n", number)
	b.WriteString("Угадай карту по арту — арт открывается с каждой попыткой.\n\n")
	if len(parts) == 0 {
		b.WriteString("Пока никто не сыграл. Будь первым!")
		return b.String()
	}
	fmt.Fprintf(&b, "Сыграли: %d · угадали: %d\n", len(parts), countSolved(parts))
	writeLines(&b, parts, maxAttempts)
	return b.String()
}

func renderSummary(number, maxAttempts int, parts []participant, cardName, rarity string, streak int) string {
	var b strings.Builder
	fmt.Fprintf(&b, "🖼 Итоги «Карты дня» #%d\n", number)
	fmt.Fprintf(&b, "Ответ: %s (%s)\n", cardName, rarity)
	if streak > 1 {
		fmt.Fprintf(&b, "🔥 Чат играет %d дней подряд!\n", streak)
	}
	fmt.Fprintf(&b, "\nСыграли: %d · угадали: %d\n", len(parts), countSolved(parts))
	writeLines(&b, parts, maxAttempts)
	return b.String()
}

func writeLines(b *strings.Builder, parts []participant, maxAttempts int) {
	sortParts(parts)
	firstSolved := true
	for _, p := range parts {
		icon, score := lineFor(p, maxAttempts, &firstSolved)
		fmt.Fprintf(b, "%s %s — %s\n", icon, p.Name, score)
	}
}

func lineFor(p participant, maxAttempts int, firstSolved *bool) (icon, score string) {
	if !p.Finished {
		return "⏳", "играет"
	}
	if p.Solved {
		if *firstSolved {
			*firstSolved = false
			return "🥇", fmt.Sprintf("%d/%d", p.Attempts, maxAttempts)
		}
		return "✅", fmt.Sprintf("%d/%d", p.Attempts, maxAttempts)
	}
	return "❌", fmt.Sprintf("X/%d", maxAttempts)
}

// sortParts orders solvers first (fewest attempts), then in-progress, then failed.
func sortParts(parts []participant) {
	rank := func(p participant) int {
		switch {
		case p.Finished && p.Solved:
			return 0
		case !p.Finished:
			return 1
		default:
			return 2
		}
	}
	sort.SliceStable(parts, func(i, j int) bool {
		ri, rj := rank(parts[i]), rank(parts[j])
		if ri != rj {
			return ri < rj
		}
		if ri == 0 {
			return parts[i].Attempts < parts[j].Attempts
		}
		return false
	})
}

func countSolved(parts []participant) int {
	n := 0
	for _, p := range parts {
		if p.Solved {
			n++
		}
	}
	return n
}

func displayName(u *models.User) string {
	if u.FirstName != "" {
		return u.FirstName
	}
	if u.Username != "" {
		return "@" + u.Username
	}
	return "Игрок"
}

// parseBoardMember splits a "plat:chatID" board-set member.
func parseBoardMember(m string) (platform string, chatID int64, ok bool) {
	i := strings.Index(m, ":")
	if i <= 0 {
		return "", 0, false
	}
	id, err := strconv.ParseInt(m[i+1:], 10, 64)
	if err != nil {
		return "", 0, false
	}
	return m[:i], id, true
}
