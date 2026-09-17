package season

import "errors"

// ErrNoSeason is returned by operations that need a running season when none is.
var ErrNoSeason = errors.New("сейчас нет активного сезона")
