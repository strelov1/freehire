package pipeline

import "time"

const (
	// cooldownThreshold is the number of consecutive board failures tolerated before
	// any cooldown applies — below it, the hourly cron re-run is the natural retry.
	cooldownThreshold = 3
	// cooldownBase is the first cooldown (at the threshold); it doubles per further
	// failure up to cooldownMax.
	cooldownBase = 6 * time.Hour
	// cooldownMax caps the backoff so even a chronically dead board retries daily and
	// can self-heal — a cooldown is never permanent.
	cooldownMax = 24 * time.Hour
)

// CooldownFor returns how long a board should be cooled down after
// consecutiveFailures failures, and whether any cooldown applies at all. Below the
// threshold it returns (0, false); at and above it, an exponential 6h·2^(f-threshold)
// capped at 24h.
func CooldownFor(consecutiveFailures int) (time.Duration, bool) {
	if consecutiveFailures < cooldownThreshold {
		return 0, false
	}
	d := cooldownBase << (consecutiveFailures - cooldownThreshold)
	if d > cooldownMax || d <= 0 { // d<=0 guards the shift overflowing on a huge count
		d = cooldownMax
	}
	return d, true
}
