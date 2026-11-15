package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// defaultDirectApplyDailyLimit bounds on-site applications per candidate per UTC day, a guard
// against spraying employers' inboxes.
const defaultDirectApplyDailyLimit = 30

// LoadDirectApplyDailyLimit reads DIRECT_APPLY_DAILY_LIMIT. A set but non-positive-integer value
// is an error, not a fallback, so a typo cannot look like a tuned cap. Not a plan feature:
// internal/ai/plan meters AI actions only.
func LoadDirectApplyDailyLimit() (int64, error) {
	raw := strings.TrimSpace(os.Getenv("DIRECT_APPLY_DAILY_LIMIT"))
	if raw == "" {
		return defaultDirectApplyDailyLimit, nil
	}
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("DIRECT_APPLY_DAILY_LIMIT=%q: want a positive integer", raw)
	}
	return n, nil
}
