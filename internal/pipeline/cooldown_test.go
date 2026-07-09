package pipeline

import (
	"testing"
	"time"
)

// CooldownFor is the board backoff policy: the hourly cron is the natural retry for
// the first couple failures (no cooldown), then an exponential 6h·2^(f-3) capped at
// 24h. It never returns a permanent cooldown, so a fixed board self-heals.
func TestCooldownFor(t *testing.T) {
	cases := []struct {
		failures int
		want     time.Duration
		cool     bool
	}{
		{0, 0, false},
		{1, 0, false},
		{2, 0, false}, // below threshold — hourly cron is the retry
		{3, 6 * time.Hour, true},
		{4, 12 * time.Hour, true},
		{5, 24 * time.Hour, true},  // 6h*4 = 24h, at the cap
		{6, 24 * time.Hour, true},  // 6h*8 = 48h → capped
		{20, 24 * time.Hour, true}, // never exceeds the cap
	}
	for _, c := range cases {
		got, cool := CooldownFor(c.failures)
		if cool != c.cool || got != c.want {
			t.Errorf("CooldownFor(%d) = (%v, %v), want (%v, %v)", c.failures, got, cool, c.want, c.cool)
		}
	}
}
