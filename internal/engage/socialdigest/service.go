package socialdigest

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Service builds a day's digest and dispatches it to the configured channels.
type Service struct {
	repo Repository
	now  func() time.Time
}

// New builds a Service. now is injectable because the staleness guard is a
// comparison against the clock, and a rule about time that cannot be tested at a
// chosen instant is a rule nobody has read.
func New(repo Repository, now func() time.Time) *Service {
	if now == nil {
		now = time.Now
	}
	return &Service{repo: repo, now: now}
}

// Build assembles the digest for a day.
//
// A zero requestedDay means "discover the freshest day with data" and applies the
// staleness guard; a non-zero one is used verbatim, because asking to replay an old
// day is the one case where old data is the point.
//
// An empty Digest is a normal result: the day was quiet. The errors it returns
// (ErrNoViewData, ErrStaleViewData) mean the view pipeline is broken, which is not
// the same thing and must not be published over.
func (s *Service) Build(ctx context.Context, requestedDay time.Time) (Digest, error) {
	day := truncateDay(requestedDay)
	if requestedDay.IsZero() {
		latest, ok, err := s.repo.LatestViewDay(ctx)
		if err != nil {
			return Digest{}, fmt.Errorf("latest view day: %w", err)
		}
		day, err = ResolveDay(latest, ok, s.now())
		if err != nil {
			return Digest{}, err
		}
	}

	candidates, err := s.repo.TopPageViewed(ctx, day, CandidateLimit)
	if err != nil {
		return Digest{}, fmt.Errorf("candidates for %s: %w", day.Format(DayLayout), err)
	}
	quarantined, err := s.repo.RecentlyDigested(ctx, QuarantineSince(day), day)
	if err != nil {
		return Digest{}, fmt.Errorf("quarantine set: %w", err)
	}

	return Digest{Day: day, Items: Select(candidates, quarantined)}, nil
}

// Dispatch publishes a digest to every publisher that has not already had this day,
// and records each success in the ledger.
//
// Every publisher is attempted even after one fails: a Discord outage must not cost
// us the day everywhere. The failures are joined and returned together, so the run
// exits non-zero and the log names each channel that broke rather than only the
// first.
//
// An empty digest publishes nothing anywhere and is not an error — a quiet day.
func (s *Service) Dispatch(ctx context.Context, d Digest, publishers []Publisher) error {
	if d.Empty() {
		return nil
	}

	var failures []error
	for _, p := range publishers {
		done, err := s.repo.PublishedForChannel(ctx, d.Day, p.Name())
		if err != nil {
			failures = append(failures, fmt.Errorf("%s: publish-once check: %w", p.Name(), err))
			continue
		}
		if done {
			continue
		}
		if err := p.Publish(ctx, d); err != nil {
			failures = append(failures, fmt.Errorf("%s: publish: %w", p.Name(), err))
			continue
		}
		// Written immediately after the publish, and deliberately not before it. The
		// two cannot be one transaction across an HTTP boundary, so one of the orders
		// has to lose: recording first risks a day that is silently never published,
		// recording after risks one duplicate post. A duplicate is visible and
		// recoverable; a silent gap is neither.
		if err := s.repo.RecordPublished(ctx, d.Day, p.Name(), d.Items); err != nil {
			failures = append(failures, fmt.Errorf("%s: ledger write after a SUCCESSFUL publish: %w", p.Name(), err))
		}
	}
	return errors.Join(failures...)
}

// jobURL is the public link for a posting, tagged with the channel that carried it so
// the digest's traffic is separable from every other inbound path in analytics.
func jobURL(origin, slug, utmSource string) string {
	return strings.TrimRight(origin, "/") + "/jobs/" + slug + "?utm_source=" + utmSource
}
