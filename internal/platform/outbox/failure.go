package outbox

// FailurePolicy is how one failed entry is bounded. AtFault picks the bound: the item's own
// fault spends the attempt budget (MaxAttempts), anything else waits out a dependency expected
// back (UpstreamGraceDays of queue age). The queues that use it (enrichment, email
// classification) record it in their Fail statement; whether an error is the item's fault
// stays with the queue, since only it knows its own failure classes.
type FailurePolicy struct {
	AtFault           bool
	MaxAttempts       int
	UpstreamGraceDays int
}
