package artguess

import (
	"hash/fnv"
	"time"
)

// Arrow feedback values returned per guess (compared against the answer).
const (
	ArrowUp    = "up"    // the answer's value is higher than the guess
	ArrowDown  = "down"  // the answer's value is lower than the guess
	ArrowEqual = "equal" // same value
)

// arrow reports how the answer's numeric attribute compares to the guess's.
func arrow(answer, guess int) string {
	switch {
	case answer > guess:
		return ArrowUp
	case answer < guess:
		return ArrowDown
	default:
		return ArrowEqual
	}
}

// revealLevel maps how many guesses have been spent to the blur level the player
// is allowed to see. Level 0 is the most hidden; maxAttempts is fully revealed.
// A finished game (solved or exhausted) reveals everything.
func revealLevel(attemptsUsed, maxAttempts int, finished bool) int {
	if finished {
		return maxAttempts
	}
	if attemptsUsed > maxAttempts {
		return maxAttempts
	}
	if attemptsUsed < 0 {
		return 0
	}
	return attemptsUsed
}

// coinReward is the payout for solving: a base plus a bonus for every unused
// attempt, so faster solves pay more.
func coinReward(cfg Config, attemptsUsed int) int {
	unused := cfg.MaxAttempts - attemptsUsed
	if unused < 0 {
		unused = 0
	}
	return cfg.RewardBase + unused*cfg.RewardPerStep
}

// puzzleNumber is the player-visible "#N": days since the configured epoch, +1
// (epoch day itself is #1). Both arguments must be normalised to midnight MSK.
func puzzleNumber(today, epoch time.Time) int {
	days := int(today.Sub(epoch).Hours() / 24)
	if days < 0 {
		days = 0
	}
	return days + 1
}

// pickIndex deterministically maps a date string to an index in [0,n). Used so
// concurrent first-requests of the day converge on the same card of the day even
// before the choice is cached in Redis.
func pickIndex(dateStr string, n int) int {
	if n <= 0 {
		return 0
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(dateStr))
	return int(h.Sum32() % uint32(n))
}
