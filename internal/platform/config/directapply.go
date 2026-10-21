package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// defaultDirectApplyDailyLimit bounds how many on-site applications one candidate sends in
// a UTC day. It is a guard against someone spraying employers' inboxes, set well above
// what a person applying by hand reaches.
const defaultDirectApplyDailyLimit = 30

// LoadDirectApplyDailyLimit reads DIRECT_APPLY_DAILY_LIMIT. A value that is set but not a
// positive integer is an error rather than a fallback: a mistyped cap would otherwise look
// exactly like a tuned one. It is not a plan feature — internal/ai/plan meters AI actions,
// and an application is not one.
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
