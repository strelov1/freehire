package sweep

import (
	"fmt"
	"strconv"
	"time"
)

// BodyRefresh is the resolved "re-read a stale body" policy: how old a stored body may be
// before a hydrating crawl fetches it again, and which slice of the catalogue this run takes.
// See dbStore.ExistingExternalIDs and migration 0144.
type BodyRefresh struct {
	// Cutoff is the age boundary: a row hydrated before it is stale. Meaningless while
	// disabled, since slot then matches nothing.
	Cutoff time.Time
	// Slices is how many runs a full sweep of the catalogue takes; always positive, because
	// it is a modulus.
	Slices int64
	// Slot is the slice this run takes, or BodyRefreshDisabledSlot.
	Slot int64
}

// BodyRefreshDisabledSlot is a slot no row can hash to: the predicate computes
// abs(hashtext(external_id)) % slices, which is never negative. Disabling the feature is
// therefore a parameter value rather than a second query, so there is one predicate to reason
// about and no branch that could disagree with the other.
const BodyRefreshDisabledSlot = -1

// bodyRefreshDefaultSlices is how many runs a full sweep takes when BODY_REFRESH_SLICE is unset.
const bodyRefreshDefaultSlices = 30

func (b BodyRefresh) Enabled() bool { return b.Slot != BodyRefreshDisabledSlot }

// BodyRefreshFor resolves BODY_REFRESH_DAYS and BODY_REFRESH_SLICE.
//
// It exists because a hydrating adapter fetches a posting's detail only while the catalogue
// does not already hold it: once stored, a posting's body is never read again, so an employer
// editing their own posting — adding "this position is 100% on-site", moving the location,
// stating a salary — reaches us never (freehire#2555). BODY_REFRESH_DAYS is how old a stored
// body may get before the next crawl re-reads it.
//
// Unset means disabled, and that is the shipped default: turning this on costs one detail
// request per re-read posting against a host whose crawl fleet is already its own bottleneck,
// so it is enabled deliberately, one provider at a time, with the cost watched. A set value
// must be a positive number of days — an unparseable one fails the run rather than quietly
// crawling as before, which is the same contract every other hand-set knob here has.
//
// BODY_REFRESH_SLICE bounds one run: a posting belongs to one slice for its whole life, and a
// run takes the slice named by the day of the year, so a full sweep takes SLICE days and no
// single run faces a provider's entire catalogue (1.27M postings on workday). Setting it
// without BODY_REFRESH_DAYS is an error rather than a no-op: a knob that silently does nothing
// reads exactly like a refresh that found nothing to refresh.
func BodyRefreshFor(days, slice string, now time.Time) (BodyRefresh, error) {
	if days == "" {
		if slice != "" {
			return BodyRefresh{}, fmt.Errorf("BODY_REFRESH_SLICE=%q has no effect without BODY_REFRESH_DAYS", slice)
		}
		return BodyRefresh{Slices: 1, Slot: BodyRefreshDisabledSlot}, nil
	}
	n, err := strconv.Atoi(days)
	if err != nil || n <= 0 {
		return BodyRefresh{}, fmt.Errorf("BODY_REFRESH_DAYS must be a positive number of days, got %q", days)
	}
	slices := int64(bodyRefreshDefaultSlices)
	if slice != "" {
		s, err := strconv.Atoi(slice)
		if err != nil || s <= 0 {
			return BodyRefresh{}, fmt.Errorf("BODY_REFRESH_SLICE must be a positive number of runs, got %q", slice)
		}
		slices = int64(s)
	}
	return BodyRefresh{
		Cutoff: now.AddDate(0, 0, -n),
		Slices: slices,
		Slot:   sliceOfDay(now, slices),
	}, nil
}

// deadlineReadLive is the day the application deadline read by freehire#4052 was live.
var deadlineReadLive = time.Date(2026, 11, 4, 0, 0, 0, 0, time.UTC)

// detailFieldAdded names the hydrating providers whose adapter learned to read a new field from
// the posting's DETAIL, with the day that reading was live. Such a fix reaches new postings on
// the next crawl and stored ones never, because a stored posting's detail is not fetched again:
// freehire#4052 taught these five to read the application deadline, and every posting they
// already held went on showing none (freehire#4124).
//
// Add an entry when an adapter fix changes what its detail yields, having checked that a
// seventh of the provider's stored postings fits its bound (see detailRereadSlices). The write
// stamps hydrated_at past the date, so a posting read once leaves the re-read whether or not the
// source states the field. An entry past detailRereadWindow does nothing and can be deleted. A
// fix to what the LISTING yields is not repaired this way; that remains INGEST_REFETCH_ALL's job.
var detailFieldAdded = map[string]time.Time{
	"ess":      deadlineReadLive,
	"infostud": deadlineReadLive,
	"mojposao": deadlineReadLive,
	"nsz":      deadlineReadLive,
	"optius":   deadlineReadLive,
}

// detailRereadSlices spreads a detailFieldAdded re-read over a week. The slice is what keeps the
// re-read from closing live postings: a withheld row is offered as new, and one an adapter leaves
// unread past its maxNewPerRun gets no liveness refresh that run, so the backlog of a run must fit
// the bound. Open postings against the bound, measured 2026-11-04: ess 4,212 / 1,200, infostud
// 3,762 / 1,000, mojposao 2,348 / 1,200, optius 1,342 / 400, nsz 590 / 1,200. A seventh of each
// fits, and a row outside today's slice is refreshed as usual.
const detailRereadSlices = 7

// detailRereadWindow ends a re-read after three passes over the catalogue. Not every row can
// leave by being read: a detail that keeps failing, or a posting the listing gate turns away, is
// never stamped and would otherwise come back every week for as long as the source lists it.
const detailRereadWindow = 3 * detailRereadSlices * 24 * time.Hour

// WithDetailReread applies provider's detailFieldAdded entry while it is inside
// detailRereadWindow. A provider with BODY_REFRESH_DAYS set keeps that policy: it re-reads the
// same rows on the operator's own cadence.
func (b BodyRefresh) WithDetailReread(provider string, now time.Time) BodyRefresh {
	added, ok := detailFieldAdded[provider]
	if !ok || b.Enabled() || now.After(added.Add(detailRereadWindow)) {
		return b
	}
	return BodyRefresh{
		Cutoff: added,
		Slices: detailRereadSlices,
		Slot:   sliceOfDay(now, detailRereadSlices),
	}
}

// sliceOfDay is the slice a run takes: the one named by the day of the year.
func sliceOfDay(now time.Time, slices int64) int64 {
	return int64(now.YearDay()) % slices
}
