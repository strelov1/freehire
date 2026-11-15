package socialdigest

import (
	"context"
	"errors"
	"fmt"
	"log"
	"slices"
	"strings"
	"time"
)

// Service builds a day's digest and dispatches it to the configured channels.
type Service struct {
	repo Repository
	now  func() time.Time
}

// New builds a Service. now is injectable so the staleness guard can be tested at a chosen
// instant.
func New(repo Repository, now func() time.Time) *Service {
	if now == nil {
		now = time.Now
	}
	return &Service{repo: repo, now: now}
}

// Build assembles the digest for a day.
//
// A zero requestedDay discovers the freshest day with data and applies the staleness guard; a
// non-zero one is used verbatim, since replaying an old day is the point.
//
// An empty Digest is normal (every posting above the floor was published recently). The errors
// (ErrNoViewData, ErrStaleViewData, ErrThinViewData) mean the view pipeline is broken or
// behind and must not be published over.
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
	if !slices.ContainsFunc(candidates, func(p Posting) bool { return p.PageUniques >= MinPageUniques }) {
		return Digest{}, fmt.Errorf("%s: %w", day.Format(DayLayout), ErrThinViewData)
	}
	quarantined, err := s.repo.RecentlyDigested(ctx, QuarantineSince(day), day)
	if err != nil {
		return Digest{}, fmt.Errorf("quarantine set: %w", err)
	}

	theme, items := SelectThemed(ThemeFor(day), candidates, quarantined)
	return Digest{Day: day, Items: items, Theme: theme}, nil
}

// Dispatch publishes a digest to every publisher that has not already had this day, and records
// each success in the ledger.
//
// Every publisher is attempted even after one fails; the failures are joined and returned
// together. An empty digest publishes nothing and is not an error.
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
			log.Printf("social-digest: %s already has %s, skipping", p.Name(), d.Day.Format(DayLayout))
			continue
		}
		if err := p.Publish(ctx, d); err != nil {
			failures = append(failures, fmt.Errorf("%s: publish: %w", p.Name(), err))
			continue
		}
		// "Selected", because a publisher may carry fewer: LinkedIn trims to its length limit.
		log.Printf("social-digest: published %s to %s (%d postings selected)", d.Day.Format(DayLayout), p.Name(), len(d.Items))
		// After the publish, deliberately not before: they cannot share a transaction across
		// HTTP. Recording first risks a silent gap; recording after risks a visible,
		// recoverable duplicate.
		if err := s.repo.RecordPublished(ctx, d.Day, p.Name(), d.Items); err != nil {
			failures = append(failures, fmt.Errorf("%s: ledger write after a SUCCESSFUL publish: %w", p.Name(), err))
		}
	}
	return errors.Join(failures...)
}

// viewsLabel is how many people opened a posting, as one short phrase. Shared by both
// publishers because it is the figure the list is ranked on. It counts page_uniques, which is
// bot-filtered; never say "views" of a number that includes crawler traffic.
func viewsLabel(p Posting) string {
	if p.PageUniques == 1 {
		return "1 view"
	}
	return fmt.Sprintf("%d views", p.PageUniques)
}

// jobURL is the public link for a posting, tagged with its channel (utm_source) for analytics.
func jobURL(origin, slug, utmSource string) string {
	return strings.TrimRight(origin, "/") + "/jobs/" + slug + "?utm_source=" + utmSource
}
