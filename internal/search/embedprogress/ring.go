package embedprogress

import "time"

// Sample is one reading of the claimable queue depth.
type Sample struct {
	Depth int64
	At    time.Time
}

// rateWindow is how far back the rate looks, and minRateSpan how much of it must be
// covered before a rate is reported at all: a rate from two samples is noise.
const (
	rateWindow  = 10 * time.Minute
	minRateSpan = 5 * time.Minute
)

// Ring holds the samples from the last rateWindow. It is not safe for concurrent use;
// Sampler guards it.
type Ring struct {
	samples []Sample
}

func NewRing() *Ring { return &Ring{} }

// Add appends a sample and forgets those older than the window, measured from it.
func (r *Ring) Add(s Sample) {
	r.samples = append(r.samples, s)
	cutoff := s.At.Add(-rateWindow)
	keep := 0
	for keep < len(r.samples) && r.samples[keep].At.Before(cutoff) {
		keep++
	}
	r.samples = r.samples[keep:]
}

// Latest is the most recent sample.
func (r *Ring) Latest() (Sample, bool) {
	if len(r.samples) == 0 {
		return Sample{}, false
	}
	return r.samples[len(r.samples)-1], true
}

// DrainRatePerMinute is how many postings left the queue per minute over the window.
// It sums only the decreases between consecutive samples: the hourly enqueue adds an hour of
// postings in one step, so (first - last) / span would go negative after every enqueue.
func (r *Ring) DrainRatePerMinute() (float64, bool) {
	if len(r.samples) < 2 {
		return 0, false
	}
	span := r.samples[len(r.samples)-1].At.Sub(r.samples[0].At)
	if span < minRateSpan {
		return 0, false
	}
	var drained int64
	for i := 1; i < len(r.samples); i++ {
		if d := r.samples[i-1].Depth - r.samples[i].Depth; d > 0 {
			drained += d
		}
	}
	return float64(drained) / span.Minutes(), true
}
