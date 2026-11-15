package embedprogress

import (
	"math"
	"time"
)

// Progress is the document the JSON endpoint and the stream serve. Every field is a pointer
// because unmeasured must reach the wire as null: a 0 would be read as a measurement.
type Progress struct {
	Eligible        *int64     `json:"eligible"`
	Covered         *int64     `json:"covered"`
	SnapshotCovered *int64     `json:"snapshot_covered"`
	Queued          *int64     `json:"queued"`
	RatePerMinute   *float64   `json:"rate_per_minute"`
	ETASeconds      *int64     `json:"eta_seconds"`
	SnapshotAt      *time.Time `json:"snapshot_at"`
	MeasuredAt      *time.Time `json:"measured_at"`
}

// Build combines what is known into one document. covered is the snapshot's measured count moved
// by how much the queue has drained since, clamped to [0, eligible]. It is not eligible - queued:
// the queue also carries ineligible rows (closed, duplicate, private) and can outnumber the eligible
// postings. Those rows sit in both depths and cancel, except a closed row leaving the queue,
// which counts as progress. A snapshot without a recorded depth leaves the measured count as is.
func Build(snap *Snapshot, latest *Sample, ratePerMinute *float64) Progress {
	var p Progress
	if snap != nil {
		p.Eligible = &snap.Eligible
		p.SnapshotCovered = &snap.Covered
		p.SnapshotAt = &snap.MeasuredAt
	}
	if latest != nil {
		p.Queued = &latest.Depth
		p.MeasuredAt = &latest.At
	}
	if snap != nil && latest != nil {
		covered := snap.Covered
		if snap.QueueDepth != nil {
			covered += *snap.QueueDepth - latest.Depth
		}
		covered = min(max(covered, 0), snap.Eligible)
		p.Covered = &covered
	}
	if ratePerMinute != nil {
		p.RatePerMinute = ratePerMinute
		if latest != nil && *ratePerMinute > 0 {
			eta := int64(math.Round(float64(latest.Depth) / *ratePerMinute * 60))
			p.ETASeconds = &eta
		}
	}
	return p
}
