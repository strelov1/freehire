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
}

// LoadReindex reads the reindex disk-guard config from the environment, all optional
// with defaults. There is no required field — the MEILI_MASTER_KEY requirement is
// enforced at the cmd/reindex call site, so this never fails.
func LoadReindex() Reindex {
	r := Reindex{
		MeiliDataDir: env("MEILI_DATA_DIR", "/var/lib/freehire/meili"),
		MinFreeGB:    envInt("REINDEX_MIN_FREE_GB", 70),
	}
	// A negative floor would make the guard's `free < floor` comparison always false,
	// silently disabling it in a way that reads like a real threshold. Clamp to 0, the
	// explicit "guard off" value.
	if r.MinFreeGB < 0 {
		r.MinFreeGB = 0
	}
	return r
}
