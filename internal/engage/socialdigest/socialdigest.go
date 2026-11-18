// Package socialdigest builds the daily "most viewed postings" list and hands it to whatever
// publishes it. It decides which postings the public sees and whether a day is worth
// publishing; it knows nothing of Discord or LinkedIn formats.
//
// It ranks on job_daily_views.page_uniques, never uniques: uniques includes unfiltered API
// reads, mostly crawlers. See internal/application/viewlog.
package socialdigest

import (
	"context"
	"errors"
	"time"
)

// The editorial constants. Package constants, not configuration, on purpose: each decides what
// the public sees under our name, so a change should be a reviewed commit.
const (
	// MinPageUniques is the floor a posting must clear to appear at all. Kept low because Size
	// and MaxPerCompany shape the digest; the floor only keeps unopened postings out of a
	// "most viewed" list. Revisit upward as page traffic grows.
	MinPageUniques = 3

	// QuarantineDays is how long a published posting stays out of the list, so one popular
	// posting does not lead every day.
	QuarantineDays = 7

	// MaxPerCompany keeps one employer having a good day from becoming the whole list.
	MaxPerCompany = 2

	// Size is how many postings a full digest carries.
	Size = 10

	// StaleAfterDays is how far behind the freshest view day may fall before the view pipeline
	// counts as broken, not the day as quiet. Not one, since a single missed
	// cmd/rollup-views night is ordinary.
	StaleAfterDays = 3

	// MinThemeItems is the shortest list a themed day may publish; below it the day falls back
	// to the plain most-viewed list.
	MinThemeItems = 5

	// CandidateLimit is how many rows the candidate query fetches. The rules and themes drop
	// rows and a theme's postings sit anywhere in the ranking, so the pool must reach well past
	// Size.
	CandidateLimit = 1000
)

// DayLayout is how a digest day is written in logs, errors and the -day flag. Exported so
// cmd/social-digest shares it.
const DayLayout = "2006-01-02"

// Posting is one job in the digest, a candidate before the editorial rules run and an item
// after.
type Posting struct {
	JobID       int64
	Slug        string
	Title       string
	Company     string
	CompanySlug string
	Location    string
	Remote      bool
	Seniority   string
	Category    string

	// PageUniques is the distinct visitors who opened this posting's page on the digest's day.
	// Bot-filtered; the only signal safe to rank on.
	PageUniques int
}

// Digest is a finished list, ready to publish. Empty Items is a quiet day, not a failure.
type Digest struct {
	// Day is the day the digest describes, not the day it is sent: it is a completed day, and
	// LatestViewDay excludes the rollup's stub of the day in progress.
	Day   time.Time
	Items []Posting
	// Theme is the top this list actually is (ThemeMostViewed on a thin day). Publishers read
	// their heading from it.
	Theme Theme
}

// Title is the post's heading, one wording for every channel.
func (d Digest) Title() string {
	return d.Theme.Heading + " on freehire — " + d.Day.Format("2 January 2006")
}

// Empty reports whether this digest has nothing to say.
func (d Digest) Empty() bool { return len(d.Items) == 0 }

// Publisher delivers a digest to one destination. Implementations own the format, not the
// contents.
type Publisher interface {
	// Name identifies the channel in the ledger and logs. It is stored: keep it stable.
	Name() string

	// Render returns exactly what Publish would send, so a dry run shows the real payload.
	Render(d Digest) (string, error)

	Publish(ctx context.Context, d Digest) error
}

// Repository is the storage socialdigest needs, an interface so the rules can be tested with a
// fake; the queries are covered by integration tests.
type Repository interface {
	// LatestViewDay reports the freshest completed day the view rollup holds. The bool is false
	// when there is none: a broken pipeline, not an empty day.
	//
	// Completed, not merely freshest: cmd/rollup-views leaves a stub of the day in progress,
	// and a digest built from it would report a quiet day. See the query's comment.
	LatestViewDay(ctx context.Context) (time.Time, bool, error)

	// TopPageViewed returns the day's eligible postings ranked by page views, most
	// first, up to limit.
	TopPageViewed(ctx context.Context, day time.Time, limit int) ([]Posting, error)

	// RecentlyDigested returns the job ids published in any channel in [since, before), the
	// quarantine set. `before` is the digest's own day and exclusive: otherwise a second
	// channel building the day would read back the first channel's list and drop it.
	RecentlyDigested(ctx context.Context, since, before time.Time) (map[int64]bool, error)

	// PublishedForChannel reports whether this day already went out on this channel.
	PublishedForChannel(ctx context.Context, day time.Time, channel string) (bool, error)

	// RecordPublished writes the ledger rows for a channel that has just published.
	RecordPublished(ctx context.Context, day time.Time, channel string, items []Posting) error
}

// Failures the caller must tell apart: a quiet day versus a broken feeding pipeline.
var (
	// ErrNoViewData means the view rollup has produced nothing at all, unlike a quiet day.
	ErrNoViewData = errors.New("socialdigest: no view data")

	// ErrStaleViewData means the freshest view day is further behind than StaleAfterDays
	// allows. A list this old is worse than none, because nobody would notice.
	ErrStaleViewData = errors.New("socialdigest: view data is stale")

	// ErrThinViewData means no posting on the day clears MinPageUniques, before quarantine.
	// That is an unfinished rollup (the digest ran before cmd/rollup-views reached the day),
	// not a quiet day; exiting 0 there would lose the post silently.
	ErrThinViewData = errors.New("socialdigest: no posting clears the view floor, so the rollup has not reached this day")
)

// ResolveDay picks the day to build a digest for. latest is the freshest day the view rollup
// holds and hasData says whether there is one. The day is discovered, not computed from now:
// whether the freshest complete day is yesterday or the day before depends on when logrotate
// runs.
func ResolveDay(latest time.Time, hasData bool, now time.Time) (time.Time, error) {
	if !hasData {
		return time.Time{}, ErrNoViewData
	}
	latest = truncateDay(latest)
	if truncateDay(now.UTC()).Sub(latest) > StaleAfterDays*24*time.Hour {
		return time.Time{}, ErrStaleViewData
	}
	return latest, nil
}

// QuarantineSince is the earliest day whose published postings still block a repeat. The window
// excludes the digest's own day; see Repository.RecentlyDigested.
func QuarantineSince(day time.Time) time.Time {
	return truncateDay(day).AddDate(0, 0, -QuarantineDays)
}

func truncateDay(t time.Time) time.Time {
	y, m, d := t.UTC().Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}
