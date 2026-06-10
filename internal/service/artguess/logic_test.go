package artguess

import (
	"testing"
	"time"
)

func TestArrow(t *testing.T) {
	if got := arrow(5, 3); got != ArrowUp {
		t.Errorf("answer higher: want up, got %s", got)
	}
	if got := arrow(2, 9); got != ArrowDown {
		t.Errorf("answer lower: want down, got %s", got)
	}
	if got := arrow(4, 4); got != ArrowEqual {
		t.Errorf("equal: want equal, got %s", got)
	}
}

func TestRevealLevel(t *testing.T) {
	cases := []struct {
		used, max int
		finished  bool
		want      int
	}{
		{0, 6, false, 0},  // nothing guessed -> most hidden
		{3, 6, false, 3},  // one step per guess
		{6, 6, false, 6},  // out of guesses -> clamped
		{9, 6, false, 6},  // never exceed max
		{2, 6, true, 6},   // finished reveals fully regardless of guesses
		{-1, 6, false, 0}, // defensive clamp
	}
	for _, c := range cases {
		if got := revealLevel(c.used, c.max, c.finished); got != c.want {
			t.Errorf("revealLevel(%d,%d,%v)=%d want %d", c.used, c.max, c.finished, got, c.want)
		}
	}
}

func TestCoinReward(t *testing.T) {
	cfg := Config{MaxAttempts: 6, RewardBase: 50, RewardPerStep: 10}
	if got := coinReward(cfg, 1); got != 50+5*10 { // solved first try -> 5 unused
		t.Errorf("first-try reward = %d, want %d", got, 50+5*10)
	}
	if got := coinReward(cfg, 6); got != 50 { // solved on last attempt -> 0 unused
		t.Errorf("last-attempt reward = %d, want 50", got)
	}
	if got := coinReward(cfg, 8); got != 50 { // over budget never goes negative
		t.Errorf("over-budget reward = %d, want 50", got)
	}
}

func TestPuzzleNumber(t *testing.T) {
	loc := time.FixedZone("MSK", 3*60*60)
	epoch := time.Date(2026, 6, 10, 0, 0, 0, 0, loc)
	if got := puzzleNumber(epoch, epoch); got != 1 {
		t.Errorf("epoch day = #%d, want #1", got)
	}
	day12 := epoch.Add(11 * 24 * time.Hour)
	if got := puzzleNumber(day12, epoch); got != 12 {
		t.Errorf("11 days later = #%d, want #12", got)
	}
	before := epoch.Add(-48 * time.Hour)
	if got := puzzleNumber(before, epoch); got != 1 {
		t.Errorf("before epoch = #%d, want clamped #1", got)
	}
}

func TestPickIndexDeterministicAndInRange(t *testing.T) {
	const n = 313
	a := pickIndex("2026-06-10", n)
	b := pickIndex("2026-06-10", n)
	if a != b {
		t.Errorf("pickIndex not deterministic: %d vs %d", a, b)
	}
	if a < 0 || a >= n {
		t.Errorf("pickIndex out of range: %d", a)
	}
	if pickIndex("anything", 0) != 0 {
		t.Errorf("pickIndex with n=0 must be 0")
	}
	// Different dates should not all collapse to one index.
	if pickIndex("2026-06-10", n) == pickIndex("2026-06-11", n) &&
		pickIndex("2026-06-11", n) == pickIndex("2026-06-12", n) {
		t.Errorf("pickIndex looks constant across dates")
	}
}

func TestConfigValidate(t *testing.T) {
	if err := DefaultConfig().Validate(); err != nil {
		t.Errorf("default config invalid: %v", err)
	}
	bad := []Config{
		{MaxAttempts: 0},
		{MaxAttempts: 13},
		{MaxAttempts: 6, RewardBase: -1},
		{MaxAttempts: 6, EpochDate: "10-06-2026"},
	}
	for i, c := range bad {
		if err := c.Validate(); err == nil {
			t.Errorf("case %d: expected validation error", i)
		}
	}
}

func TestParseConfig(t *testing.T) {
	c, err := ParseConfig([]byte(`{"enabled":true,"max_attempts":5,"reward_base":40,"reward_per_step":8,"epoch_date":"2026-06-10"}`))
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}
	if c.MaxAttempts != 5 || c.RewardBase != 40 {
		t.Errorf("parsed config wrong: %+v", c)
	}
	if _, err := ParseConfig([]byte(`{"max_attempts":99}`)); err == nil {
		t.Errorf("expected error for invalid max_attempts")
	}
}
