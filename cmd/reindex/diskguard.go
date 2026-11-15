package main

import (
	"fmt"
	"log"
	"syscall"
)

// guardDisk refuses a swap-rebuild when free space on dir is below minFreeGB (GiB).
// A full rebuild transiently holds a second copy of the index, so on a tight disk it can fill the volume
// and leave an orphan rebuild index. Aborting up front trades a stale index (incremental ingest keeps it
// fresh) for never risking a disk-full outage. minFreeGB <= 0 disables the guard. The probe is injected
// so the threshold logic is testable without a real filesystem.
func guardDisk(dir string, minFreeGB int, probe func(string) (uint64, error)) error {
	if minFreeGB <= 0 {
		return nil
	}
	free, err := probe(dir)
	if err != nil {
		// Fail open: the guard is a best-effort safety net. If the dir cannot be measured (e.g. off the prod
		// host, as in CI/dev), skip it rather than block reindex. A misconfigured MEILI_DATA_DIR on prod thus
		// disables the guard silently, so log loudly; the disk alert is the backstop.
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
	// Bavail (not Bfree) is the space usable by non-root writers, which is what Meilisearch gets.
	return st.Bavail * uint64(st.Bsize), nil
}
