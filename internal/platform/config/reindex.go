package config

// Reindex holds the operational guards for the reindex command (cmd/reindex). Meilisearch is
// configured via the shared Settings; this holds only the disk-safety knobs.
type Reindex struct {
	// MeiliDataDir is the path the disk guard measures free space on. It must be the volume
	// Meilisearch stores its indexes under.
	MeiliDataDir string
	// MinFreeGB is the minimum free space (GiB) on MeiliDataDir before a swap-rebuild may start.
	// A rebuild transiently holds a second copy of the index; below the floor the reindex refuses
	// rather than risk a disk-full outage. 0 disables the guard.
	MinFreeGB int
	// Dedup runs the three duplicate-marker passes (role clusters, aggregator suppression, fuzzy
	// collapse) before the index rebuild. Off by default: the passes are slow enough to hit the unit
	// timeout before the rebuild, so they run on their own schedule (REINDEX_DEDUP=1). Markers are
	// eventually consistent and best-effort, so running them less often costs freshness only.
	Dedup bool
	// DedupOnly runs the marker passes and exits, without the facet rebuild. The passes are pure
	// Postgres, so this can run on a tighter cadence than Dedup; the rebuild reads whatever markers
	// are current.
	//
	// Takes precedence over Dedup: it short-circuits before the disk guard and Meilisearch client
	// setup, and Dedup is not consulted.
	DedupOnly bool
}

// LoadReindex reads the reindex disk-guard config, all optional. MEILI_MASTER_KEY is enforced
// at the cmd/reindex call site, so this never fails.
func LoadReindex() Reindex {
	r := Reindex{
		MeiliDataDir: env("MEILI_DATA_DIR", "/var/lib/freehire/meili"),
		MinFreeGB:    envInt("REINDEX_MIN_FREE_GB", 70),
		Dedup:        envBool("REINDEX_DEDUP", false),
		DedupOnly:    envBool("REINDEX_DEDUP_ONLY", false),
	}
	// A negative floor would make `free < floor` always false, silently disabling the guard. Clamp to 0.
	if r.MinFreeGB < 0 {
		r.MinFreeGB = 0
	}
	return r
}
