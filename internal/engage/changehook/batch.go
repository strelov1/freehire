package changehook

import (
	"fmt"
	"time"

	"github.com/strelov1/freehire/internal/platform/db"
)

// pageSize is how many feed rows one delivery step reads, and so the most changes one POST
// carries.
const pageSize = 100

// Event types, in the X-Freehire-Event header and the body's `type`.
const (
	EventChanges = "job.changes"
	EventTest    = "webhook.test"
)

// Change is one transition in a delivery: the feed's JobChange plus the company it belongs
// to. Apply it only when Seq is greater than the last Seq you applied for that PublicSlug.
type Change struct {
	PublicSlug   string    `json:"public_slug"`
	CompanySlug  string    `json:"company_slug"`
	Seq          int64     `json:"seq"`
	Kind         string    `json:"kind"`
	At           time.Time `json:"at"`
	ClosedReason string    `json:"closed_reason,omitempty"`
}

// Event is the JSON body of one delivery. ID is the same for a retry of the same batch.
type Event struct {
	ID      string   `json:"id"`
	Type    string   `json:"type"`
	Changes []Change `json:"changes"`
}

// Filter is what a subscription wants: the kinds (never empty) and, when CompanySlugs is
// not empty, only these companies.
type Filter struct {
	CompanySlugs []string
	Kinds        []string
}

func (f Filter) match(r db.ListJobChangesForWebhookRow) bool {
	return contains(f.Kinds, r.Kind) && (len(f.CompanySlugs) == 0 || contains(f.CompanySlugs, r.CompanySlug))
}

func contains(set []string, s string) bool {
	for _, v := range set {
		if v == s {
			return true
		}
	}
	return false
}

// Batch is the changes of one feed page the subscription wants, in feed order. Rows it does
// not want still move its position, which is the caller's business.
func (f Filter) Batch(rows []db.ListJobChangesForWebhookRow) []Change {
	var out []Change
	for _, r := range rows {
		if f.match(r) {
			out = append(out, Change{PublicSlug: r.PublicSlug, CompanySlug: r.CompanySlug, Seq: r.ID,
				Kind: r.Kind, At: r.At.Time.UTC(), ClosedReason: r.Reason})
		}
	}
	return out
}

// deliveryID names a batch by its subscription and the feed position it ends at, so a retry
// of the same batch carries the same id.
func deliveryID(sub int64, last db.ListJobChangesForWebhookRow) string {
	return fmt.Sprintf("%d-%d-%d", sub, last.Xid, last.ID)
}

// backoff is the wait after the nth consecutive failure (0-based): 30 s doubling to 1 h.
func backoff(failures int64) time.Duration {
	const base, ceiling = 30 * time.Second, time.Hour
	if failures >= 7 { // 30s * 2^7 is past the ceiling
		return ceiling
	}
	return min(base<<failures, ceiling)
}
