package main

import (
	"fmt"
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
		return fmt.Errorf("reindex: measure free disk on %s: %w", dir, err)
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
	return uint64(st.Bavail) * uint64(st.Bsize), nil
}
