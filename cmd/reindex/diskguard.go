package main

import (
	"fmt"
	"log"
	"syscall"
)

// guardDisk refuses a swap-rebuild when free space on dir is below minFreeGB (GiB).
// A full rebuild transiently holds a second copy of the index, so on a tight disk it
// fills the volume, aborts before the swap, and leaves an orphan rebuild index that
// eats ~index-size until dropped by hand. Aborting up front trades a stale index (the
// facet index stays fresh via incremental ingest anyway) for never risking a disk-full
// outage. minFreeGB <= 0 disables the guard. The probe is injected so the threshold
// logic is unit-tested without a real filesystem.
func guardDisk(dir string, minFreeGB int, probe func(string) (uint64, error)) error {
	if minFreeGB <= 0 {
		return nil
	}
	free, err := probe(dir)
	if err != nil {
		// Fail open: the guard is a best-effort safety net, not a correctness gate. If the
		// dir cannot be measured (e.g. it does not exist off the prod host, as in CI/dev),
		// skip the guard rather than block reindex everywhere. A misconfigured MEILI_DATA_DIR
		// on prod thus silently disables the guard, so log it loudly — the disk alert is the
		// backstop.
		log.Printf("reindex: disk-guard skipped — cannot measure free space on %s: %v", dir, err)
		return nil
	}
	required := uint64(minFreeGB) << 30
	if free < required {
		return fmt.Errorf("refusing rebuild — free %dGiB on %s is below the %dGiB floor (REINDEX_MIN_FREE_GB); a swap-rebuild would risk filling the disk",
			free>>30, dir, minFreeGB)
	}
	return nil
}

// statfsFree reports the bytes available to an unprivileged writer on the filesystem
// backing dir. It is the production probe for guardDisk.
func statfsFree(dir string) (uint64, error) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(dir, &st); err != nil {
		return 0, err
	}
	// Bavail (not Bfree) is the space usable by non-root writers — what Meilisearch
	// actually gets.
	return st.Bavail * uint64(st.Bsize), nil
}
