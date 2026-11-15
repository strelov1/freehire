package notify

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"slices"
	"strconv"
)

// kindSubscriptionDigest is the kind a digest's notification-center row is filed under and the
// prefix of its dedup key. One constant, since /my/notifications renders on it and the key that
// namespaces it must not drift from it.
const kindSubscriptionDigest = "subscription_digest"

// digestDedupKey names the job-match EVENT a digest records, so the notification centre holds one
// row for it however many channels carried it.
//
// `subscriptions` is keyed (saved_search_id, channel), so one saved search with Telegram, email
// and push is three deliveries. They share the saved search and the job set, not the
// subscription id, channel or send time. Both halves are load-bearing:
//
//   - The saved search, because two saved searches of one person can match the same job in the
//     same pass, and those are two events with two names to show.
//
//   - The job set, because that is what a digest IS: a saved search delivers many digests and
//     each needs its own row. When a send succeeded but MarkMatchesNotified failed, the next pass
//     re-delivers the same set, meets the same key and JOINS the old row, which is right: the
//     history says the person was told once.
//
// The key is exact-set equality, so channels share a row only while they carry the same jobs.
// Two channels out of step (one soft-skipped while a later job matched) record two rows; keying
// on the saved search and a time bucket would file a job under an event that never carried it.
//
// The ids are sorted on a COPY, since jobIDs is the live claim list the caller hands on to
// MarkMatchesNotified and ReleaseMatchClaim, and claims of the same jobs can come back in
// different orders. They are hashed because a digest carries up to SnapshotCap (200) of them
// and this is a text column under a unique index. SHA-256 because a collision would silently
// drop somebody's notification into an unrelated event's row.
func digestDedupKey(savedSearchID int64, jobIDs []int64) string {
	sorted := slices.Clone(jobIDs)
	slices.Sort(sorted)
	h := sha256.New()
	var buf [8]byte
	for _, id := range sorted {
		binary.BigEndian.PutUint64(buf[:], uint64(id))
		h.Write(buf[:])
	}
	return kindSubscriptionDigest + ":" + strconv.FormatInt(savedSearchID, 10) + ":" + hex.EncodeToString(h.Sum(nil))
}
