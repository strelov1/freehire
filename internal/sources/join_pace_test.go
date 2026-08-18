package sources

import (
	"testing"
	"time"
)

// joinShards is how many staggered runs sources/join.yml is split across on prod
// (freehire-ingest-join-shard@N.timer, --shard=N/5, one firing per hour so only one is ever
// crawling and the pace below is the whole fleet's rate, not each shard's). Raised from 4 to
// 5 alongside the pace drop to 1.5 req/s (issue #2094) — at 4 shards that pace misses the run
// budget below.
const joinShards = 5

// TestJoinPaceFitsTheRunBudget pins the arithmetic that picked both the pace and the shard
// count. Too fast and the refusals come back; too slow and systemd kills the run mid-crawl,
// which board_health cannot tell apart from the platform being down — the same trap the
// Teamtailor pacing hit.
func TestJoinPaceFitsTheRunBudget(t *testing.T) {
	// Measured on prod 2026-08-18: 4749 boards needing 5234 listing pages at the API's
	// hard pageSize cap of 5, and 10455 postings each costing one detail request.
	const (
		requestsPerRun = 5234 + 10455
		unitTimeout    = 3000 * time.Second // freehire-ingest@.service TimeoutStartSec
		// A run that uses its whole budget has no room for a slow page or a retry, and a
		// killed run is indistinguishable from an outage in board_health.
		headroom = 0.8
	)

	perSecond := float64(time.Second) / float64(joinRequestInterval)
	perShard := float64(requestsPerRun) / joinShards
	runtime := time.Duration(perShard / perSecond * float64(time.Second))

	if budget := time.Duration(float64(unitTimeout) * headroom); runtime > budget {
		t.Errorf("a shard takes %v at %.1f req/s (%.0f of %d requests), past the %v working "+
			"budget — the tail would be killed mid-crawl; raise joinShards or the pace",
			runtime.Round(time.Second), perSecond, perShard, requestsPerRun, budget)
	}
	// 3 req/s was refused 10% of the time against live boards; a "pace" at or above that
	// would not be a pace at all.
	if perSecond >= 3 {
		t.Errorf("%.1f req/s is at or past the rate Join began refusing", perSecond)
	}
}

// TestJoinIsPaced guards the wiring. Join has one wiring — it is not in proxiedProviders — but
// the registry is where an adapter loses its pacer during an unrelated edit, and losing it is
// silent: the crawl simply starts being refused again.
func TestJoinIsPaced(t *testing.T) {
	src, ok := All(NewClient())["join"].(join)
	if !ok {
		t.Fatal("join is not registered, or no longer the join type")
	}
	if _, paced := src.http.(rateLimitedJSONGetter); !paced {
		t.Errorf("registry join is not paced: got %T", src.http)
	}
}
