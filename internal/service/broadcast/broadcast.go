// Package broadcast sends one-off admin announcements to the registered chats
// and performs chat-registry maintenance that needs the bots (leaving a chat).
// Delivery layers register themselves as Senders, so the web/API layer can reach
// Telegram and Discord without importing them.
package broadcast

import (
	"fmt"
	"log"
	"sort"
	"strings"

	"gachabot/internal/models"
	"gachabot/internal/repository"
)

// Platform identifiers (match repository.chats.platform values).
const (
	PlatformTelegram = "telegram"
	PlatformDiscord  = "discord"
)

// Sender is implemented by each delivery layer.
type Sender interface {
	// SendChatMessage posts a plain announcement (HTML on Telegram, markdown on
	// Discord — both render the same simple formatting we allow).
	SendChatMessage(chatID int64, text string) error
	// LeaveChat makes the bot leave/abandon the chat.
	LeaveChat(chatID int64) error
}

type Service struct {
	repo    *repository.PostgresRepo
	senders map[string]Sender
}

func New(repo *repository.PostgresRepo) *Service {
	return &Service{repo: repo, senders: make(map[string]Sender)}
}

// RegisterSender wires a delivery layer in (call before use).
func (s *Service) RegisterSender(platform string, snd Sender) {
	s.senders[platform] = snd
}

// AvailablePlatforms lists platforms that can currently send (a bot is running).
func (s *Service) AvailablePlatforms() []string {
	out := make([]string, 0, len(s.senders))
	for p := range s.senders {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

// Result is one chat's delivery outcome.
type Result struct {
	Platform string `json:"platform"`
	ChatID   int64  `json:"chatId,string"`
	Title    string `json:"title"`
	OK       bool   `json:"ok"`
	Error    string `json:"error,omitempty"`
}

// Report summarises a broadcast (or its dry run).
type Report struct {
	DryRun    bool     `json:"dryRun"`
	Delivered int      `json:"delivered"`
	Failed    int      `json:"failed"`
	Targets   []Result `json:"targets"`
}

// targets returns the chats a broadcast would reach on the given platforms.
// Only chats with spawns enabled are used: that flag doubles as "the bot posts
// here", the same set the spawn engine and Art Guess boards use.
func (s *Service) targets(platforms []string) ([]models.Chat, error) {
	chats, err := s.repo.GetSpawnChats()
	if err != nil {
		return nil, err
	}
	allow := map[string]bool{}
	for _, p := range platforms {
		allow[p] = true
	}
	out := make([]models.Chat, 0, len(chats))
	for _, c := range chats {
		if len(allow) > 0 && !allow[c.Platform] {
			continue
		}
		out = append(out, c)
	}
	return out, nil
}

// Send delivers text to every matching chat. With dryRun it only reports the
// targets — nothing is sent, which is how the admin UI previews a broadcast.
func (s *Service) Send(text string, platforms []string, dryRun bool) (Report, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return Report{}, fmt.Errorf("текст рассылки пуст")
	}
	chats, err := s.targets(platforms)
	if err != nil {
		return Report{}, err
	}

	rep := Report{DryRun: dryRun, Targets: make([]Result, 0, len(chats))}
	for _, c := range chats {
		res := Result{Platform: c.Platform, ChatID: c.ChatID, Title: c.Title}
		sender := s.senders[c.Platform]
		switch {
		case sender == nil:
			res.Error = "бот этой платформы не запущен"
		case dryRun:
			res.OK = true
		default:
			if err := sender.SendChatMessage(c.ChatID, text); err != nil {
				res.Error = err.Error()
			} else {
				res.OK = true
			}
		}
		if res.OK {
			rep.Delivered++
		} else {
			rep.Failed++
		}
		rep.Targets = append(rep.Targets, res)
	}
	if !dryRun {
		log.Printf("[ADMIN] broadcast: delivered=%d failed=%d platforms=%v", rep.Delivered, rep.Failed, platforms)
	}
	return rep, nil
}

// LeaveChat makes the bot leave a chat and drops it from the posting registry, so
// the spawn engine and Art Guess stop targeting it.
func (s *Service) LeaveChat(platform string, chatID int64) error {
	sender := s.senders[platform]
	if sender == nil {
		return fmt.Errorf("бот платформы %q не запущен", platform)
	}
	if err := sender.LeaveChat(chatID); err != nil {
		return err
	}
	if err := s.repo.SetChatSpawnEnabled(platform, chatID, false); err != nil {
		return err
	}
	log.Printf("[ADMIN] left chat %s:%d", platform, chatID)
	return nil
}
