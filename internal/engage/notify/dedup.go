package notify

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"slices"
	"strconv"
)

// kindSubscriptionDigest is what a digest's notification-center row is filed
// under, and the prefix its dedup key carries. One constant rather than two
// literals: the kind is the row's public vocabulary — /my/notifications renders
// on it — so the value and the key that namespaces it must not be able to drift
// apart in the one package that writes both.
const kindSubscriptionDigest = "subscription_digest"

// digestDedupKey names the job-match EVENT a digest records, so the notification
// centre holds one row for it however many channels carried it.
//
// `subscriptions` is keyed (saved_search_id, channel), so one saved search with
// Telegram, email and push enabled is three rows, three deliveries, and — before
// this key — three indistinguishable history entries for one match
// (freehire#3020). What those three deliveries share is the saved search and the
// set of jobs; what they do not share is the subscription id, the channel, or
// the instant they were sent.
//
// Both halves are load-bearing:
//
//   - The saved search, because two different saved searches of the same person
//     can match the same job in the same pass, and those are two events with two
//     different names to show.
//
//   - The job set, because that is what a digest IS. A saved search delivers
//     many digests over its life and each must get its own row; nothing else
//     about a delivery distinguishes one from the next. It barely repeats by
//     accident either: a delivered match is stamped notified and does not
//     re-enter a digest — except on the one path where the send succeeded and
//     MarkMatchesNotified then failed, where the next pass re-delivers the same
//     set, meets the same key and JOINS the old row rather than recording the
//     re-delivery. That is the right outcome of the two available: the person
//     was told about these jobs once as far as the history is concerned, and the
//     alternative is a second entry saying the same thing.
//
// The key is exact-set equality, which is what makes it honest rather than
// merely quiet. Channels share a row only while they carry the same jobs, so
// two channels that fall out of step — one soft-skipped while a later job
// matched, say — record two rows describing two genuinely different sets, not
// one row standing for both. Collapsing those (keying on the saved search and a
// time bucket) would file a job under an event that never carried it.
//
// The ids are sorted first because the set is what matters and each channel
// claims its own rows: two claims of the same jobs may come back in different
// orders, and an order-sensitive key would miss the duplicate it exists to
// catch. Sorted on a COPY — jobIDs is the live claim list the caller goes on to
// hand MarkMatchesNotified and ReleaseMatchClaim, and reordering it underneath
// them would be a side effect nobody asked this function for. They are then
// hashed rather than listed, because a digest carries up to
// SnapshotCap (200) of them and this value is a text column under a unique
// index. SHA-256 rather than a short non-cryptographic hash: the column is
// nowhere near a size worth economising on, while a collision would silently
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
