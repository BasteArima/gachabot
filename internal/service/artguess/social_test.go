package artguess

import (
	"strings"
	"testing"
)

func TestDeepLinkRoundTripAndForgery(t *testing.T) {
	s := &Service{secret: "bot-token-123"}
	chatID := int64(-1001234567890)

	param := s.DeepLinkParam(PlatformTelegram, chatID)
	if !strings.HasPrefix(param, deepLinkPrefix) {
		t.Fatalf("param missing prefix: %q", param)
	}
	got, ok := s.parseLaunch(PlatformTelegram, param)
	if !ok || got != chatID {
		t.Fatalf("round-trip failed: got (%d,%v), want (%d,true)", got, ok, chatID)
	}

	// Forged chat id (valid format, wrong signature) must be rejected.
	if _, ok := s.parseLaunch(PlatformTelegram, "artguess_-100999_deadbeefdeadbeef"); ok {
		t.Errorf("forged param accepted")
	}
	// A different platform changes the signature → reject.
	if _, ok := s.parseLaunch(PlatformDiscord, param); ok {
		t.Errorf("cross-platform param accepted")
	}
	// Garbage.
	for _, bad := range []string{"", "artguess_", "artguess_abc_def", "nope_1_2"} {
		if _, ok := s.parseLaunch(PlatformTelegram, bad); ok {
			t.Errorf("garbage %q accepted", bad)
		}
	}
}

func TestSortPartsOrder(t *testing.T) {
	parts := []participant{
		{Name: "failed", Finished: true, Solved: false},
		{Name: "slow", Finished: true, Solved: true, Attempts: 5},
		{Name: "playing", Finished: false},
		{Name: "fast", Finished: true, Solved: true, Attempts: 2},
	}
	sortParts(parts)
	order := []string{parts[0].Name, parts[1].Name, parts[2].Name, parts[3].Name}
	want := []string{"fast", "slow", "playing", "failed"}
	for i := range want {
		if order[i] != want[i] {
			t.Fatalf("order = %v, want %v", order, want)
		}
	}
}

func TestRenderBoard(t *testing.T) {
	empty := renderBoard(7, 6, nil)
	if !strings.Contains(empty, "#7") || !strings.Contains(empty, "никто") {
		t.Errorf("empty board unexpected: %q", empty)
	}
	parts := []participant{
		{Name: "Alice", Finished: true, Solved: true, Attempts: 3},
		{Name: "Bob", Finished: false},
	}
	board := renderBoard(7, 6, parts)
	if !strings.Contains(board, "🥇 Alice — 3/6") {
		t.Errorf("missing winner line: %q", board)
	}
	if !strings.Contains(board, "⏳ Bob — играет") {
		t.Errorf("missing in-progress line: %q", board)
	}
	if !strings.Contains(board, "Сыграли: 2 · угадали: 1") {
		t.Errorf("missing counts: %q", board)
	}
}
