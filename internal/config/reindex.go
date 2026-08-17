package config

// Reindex holds the operational guards for the reindex command (cmd/reindex).
// Meilisearch itself is configured via the shared Settings (MEILI_URL/
// MEILI_MASTER_KEY); this holds only the disk-safety knobs, mirroring the tuning
// half of config.Embed.
type Reindex struct {
	// MeiliDataDir is the filesystem path the reindex disk-guard measures free space
	// on before building a swap-rebuild index. It must be the volume Meilisearch
	// stores its indexes under, so the guard sees the space the 2nd copy will consume.
	MeiliDataDir string
	// MinFreeGB is the minimum free space (GiB) required on MeiliDataDir before a
	// swap-rebuild is allowed to start. A full rebuild transiently holds a second
	// copy of the index (~index-size), so a chronically tight disk fills and leaves an
	// orphan rebuild index. Below this floor the reindex refuses rather than risk a
	// disk-full outage. 0 disables the guard.
	MinFreeGB int
	// Dedup runs the three duplicate-marker passes (role clusters, aggregator
	// suppression, fuzzy collapse) before the index rebuild. OFF by default, which is
	// a deliberate inversion of how this worker behaved until 2026-08-16.
	//
	// Those passes are not what `reindex` is for, and they had grown to where they
	// prevented it from doing its actual job: aggregator suppression alone measured
	// ~23h over 306 batches on prod, against a 12h unit timeout, so the run was
	// cancelled mid-dedup and NEVER REACHED the rebuild — zero successful reindexes in
	// the three days before this was found, while the incremental search-drain quietly
	// kept the index serving.
	//
	// Splitting them apart means the index rebuild happens on its own schedule again,
	// and the markers refresh on theirs (a separate, rarer invocation with
	// REINDEX_DEDUP=1). The markers are eventually-consistent by design — every pass
	// is best-effort and degrades to the prior markers — so running them less often
	// costs freshness, not correctness.
	Dedup bool
	// DedupOnly runs the marker passes and exits, WITHOUT the facet rebuild that
	// Dedup alone runs afterward. The marker passes are pure Postgres (no Meilisearch,
	// no disk guard, no search-drain pause needed); the facet rebuild is the expensive,
	// Meilisearch-bound half, and it already runs on its own schedule via the plain
	// (Dedup=false) invocation, reading whatever markers are current at that moment.
	// So a DedupOnly run can afford a tighter cadence than a Dedup run — the freshness
	// a repost needs to disappear from search is bounded by MIN(next DedupOnly run,
	// next plain rebuild), not by the full marker+rebuild round trip.
	//
	// Takes precedence over Dedup when both are set — a single invocation is never
	// meant to run the markers and then skip the rebuild it just paid the disk guard
	// and Meilisearch client setup for, so DedupOnly short-circuits before either
	// happens and Dedup is not consulted at all.
	DedupOnly bool
}

// LoadReindex reads the reindex disk-guard config from the environment, all optional
// with defaults. There is no required field — the MEILI_MASTER_KEY requirement is
// enforced at the cmd/reindex call site, so this never fails.
func LoadReindex() Reindex {
	r := Reindex{
		MeiliDataDir: env("MEILI_DATA_DIR", "/var/lib/freehire/meili"),
		MinFreeGB:    envInt("REINDEX_MIN_FREE_GB", 70),
		Dedup:        envBool("REINDEX_DEDUP", false),
		DedupOnly:    envBool("REINDEX_DEDUP_ONLY", false),
	}
	// A negative floor would make the guard's `free < floor` comparison always false,
	// silently disabling it in a way that reads like a real threshold. Clamp to 0, the
	// explicit "guard off" value.
	if r.MinFreeGB < 0 {
		r.MinFreeGB = 0
	}
	return r
}
