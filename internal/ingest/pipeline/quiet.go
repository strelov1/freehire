package pipeline

import (
	"crypto/sha256"
	"encoding/hex"
	"slices"
	"time"
)

const (
	// quietBase is the first quiet pause, after one complete crawl that found the board's listing
	// unchanged; it doubles per further unchanged crawl up to quietMax.
	quietBase = 4 * time.Hour
	// quietMax caps how long a quiet board waits between crawls. It stays far below the safety
	// nets' windows (close-chronic-boards: 30 days without a yield, 60 without a success), so a
	// quiet board still succeeds often enough to keep both honest.
	quietMax = 72 * time.Hour
)

// QuietFor returns how long a board whose listing has been unchanged for streak complete crawls
// in a row should wait before its next crawl, and whether it waits at all: none at streak 0, then
// 4h·2^(streak-1) capped at 72h.
func QuietFor(streak int) (time.Duration, bool) {
	if streak <= 0 {
		return 0, false
	}
	d := quietBase << (streak - 1)
	if d > quietMax || d <= 0 { // d<=0 guards the shift overflowing on a huge streak
		d = quietMax
	}
	return d, true
}

// listingFingerprint identifies the SET of postings a board listed: the sha256 of its distinct
// external ids in sorted order, each terminated by a NUL so two ids can never run together. Order
// and repeats do not change it; a posting appearing or disappearing does. An empty listing has a
// fingerprint of its own, so an empty board that stays empty counts as quiet.
func listingFingerprint(ids []string) string {
	sorted := slices.Clone(ids)
	slices.Sort(sorted)
	sorted = slices.Compact(sorted)
	h := sha256.New()
	for _, id := range sorted {
		h.Write([]byte(id))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}
