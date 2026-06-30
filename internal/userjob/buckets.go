package userjob

// StageCount is one (stage, count) group from the per-user application aggregate.
// An empty Stage is an applied row carrying no explicit stage.
type StageCount struct {
	Stage string
	Count int64
}

// BucketCounts is the snapshot distribution of a user's applications across the
// seven pipeline buckets. Bucket names are derived from the stage vocabulary and
// are the wire shape returned by the pipeline endpoint.
type BucketCounts struct {
	NoAnswer     int64 `json:"no_answer"`
	InProgress   int64 `json:"in_progress"`
	Interviewing int64 `json:"interviewing"`
	Offer        int64 `json:"offer"`
	Accepted     int64 `json:"accepted"`
	Rejected     int64 `json:"rejected"`
	Declined     int64 `json:"declined"`
}

// Pipeline is the application-pipeline snapshot: the total application count and
// its distribution across buckets. Buckets always sum to Applications.
type Pipeline struct {
	Applications int64        `json:"applications"`
	Buckets      BucketCounts `json:"buckets"`
}

// Aggregate folds per-stage counts into the pipeline snapshot. The stage→bucket
// mapping is the single source of truth: screening/responded collapse into
// in_progress, withdrawn is declined, and an empty or unrecognized stage falls to
// no_answer so every counted application lands in exactly one bucket.
func Aggregate(counts []StageCount) Pipeline {
	var p Pipeline
	for _, c := range counts {
		p.Applications += c.Count
		switch c.Stage {
		case "screening", "responded":
			p.Buckets.InProgress += c.Count
		case "interview":
			p.Buckets.Interviewing += c.Count
		case "offer":
			p.Buckets.Offer += c.Count
		case "accepted":
			p.Buckets.Accepted += c.Count
		case "rejected":
			p.Buckets.Rejected += c.Count
		case "withdrawn":
			p.Buckets.Declined += c.Count
		default: // "applied", "", and any unknown stage
			p.Buckets.NoAnswer += c.Count
		}
	}
	return p
}
