package main

import (
	"fmt"
	"strconv"
	"time"

	"github.com/strelov1/freehire/internal/ingest/pipeline"
)

// Parsers for the environment knobs run reads beyond the run budget.

// hydrationRetryWindowFor resolves how long a stored row with no description keeps being
// re-offered to a hydrating adapter for another detail fetch (see pipeline.SeenLookup). An
// empty value means the default, pipeline.HydrationRetryWindow.
//
// HYDRATION_RETRY_DAYS widens it for a repair run: the default window is measured from
// created_at, so an older backlog is never re-fetched. Expect one detail request per body-less
// row; a run that hits its timeout keeps what it hydrated, so repeating it shrinks the backlog.
//
// An unparseable or non-positive value is an error, not a fallback: a hand-set one-off must not
// silently look like a repair run that found nothing.
func hydrationRetryWindowFor(env string) (time.Duration, error) {
	if env == "" {
		return pipeline.HydrationRetryWindow, nil
	}
	days, err := strconv.Atoi(env)
	if err != nil || days <= 0 {
		return 0, fmt.Errorf("HYDRATION_RETRY_DAYS must be a positive number of days, got %q", env)
	}
	return time.Duration(days) * 24 * time.Hour, nil
}

// refetchAllFor resolves INGEST_REFETCH_ALL, the repair switch that empties the seen-set so a
// crawl re-WRITES stored rows instead of only refreshing liveness (see
// dbStore.ExistingExternalIDs). It carries an adapter fix to already-ingested postings, which
// re-deriving from the database cannot do.
//
// Set it by hand for one run; expect one detail request per stored posting. A failed detail
// request loses nothing (UpsertJob keeps the stored description when the incoming one is
// empty), so a timed-out run can be repeated.
//
// Anything but the two accepted spellings is an error, not a quiet false.
func refetchAllFor(env string) (bool, error) {
	switch env {
	case "":
		return false, nil
	case "1", "true":
		return true, nil
	default:
		return false, fmt.Errorf("INGEST_REFETCH_ALL must be 1 or true when set, got %q", env)
	}
}

// maxRecentDays caps INGEST_RECENT_DAYS; a typo like 365 would pass for a recent run while
// asking a source for a year of postings.
const maxRecentDays = 30

// recentWindowFor reads INGEST_RECENT_DAYS: unset means a full crawl (0), otherwise a positive
// whole number of days up to maxRecentDays. A bad value fails the run rather than starting an
// expensive full crawl where a cheap one was meant.
func recentWindowFor(env string) (time.Duration, error) {
	if env == "" {
		return 0, nil
	}
	days, err := strconv.Atoi(env)
	if err != nil || days < 1 || days > maxRecentDays {
		return 0, fmt.Errorf("INGEST_RECENT_DAYS must be a whole number of days from 1 to %d when set, got %q", maxRecentDays, env)
	}
	return time.Duration(days) * 24 * time.Hour, nil
}
