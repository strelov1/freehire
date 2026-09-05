// Package socialdigest builds the daily "most viewed postings" list and hands it to
// whatever publishes it. It decides WHICH postings the public sees and WHETHER a day
// is worth publishing at all; it does not know what Discord or LinkedIn look like.
//
// The number it ranks on is job_daily_views.page_uniques, never uniques. uniques
// fuses bot-filtered page opens with unfiltered API reads, and crawlers are most of
// this host's traffic — ranking a public list on it would publish what robots
// fetched as though it were what people liked. See migration 0138 and
// internal/application/viewlog.
package socialdigest

import (
	"context"
	"errors"
	"time"
)

// The editorial constants. These are package constants and not configuration on
// purpose: each one decides what the public sees under our own name, so changing one
// should be a commit somebody reviewed, not an environment variable somebody edited
// over SSH. They are starting values, to be revisited after a week of dry runs.
const (
	// MinPageUniques is the floor a posting must clear to appear at all. Below it the
	// word "popular" is not describing anything.
	MinPageUniques = 10

	// QuarantineDays is how long a published posting stays out of the list. Without it
	// a posting that stays popular for a week is the lead item every day for a week,
	// and the feed reads as a broken bot.
	QuarantineDays = 7

	// MaxPerCompany keeps one employer having a good day from becoming the whole list.
	MaxPerCompany = 2

	// Size is how many postings a full digest carries.
	Size = 10

	// StaleAfterDays is how far behind the freshest view day may fall before the run
	// treats the view pipeline as broken rather than the day as quiet. Three days and
	// not one, because cmd/rollup-views missing a single night is ordinary noise.
	StaleAfterDays = 3

	// CandidateLimit is how many rows the candidate query fetches. The rules below
	// drop rows, so fetching exactly Size would return fewer than Size publishable
	// postings on any day one company had a good morning. Generous on purpose — the
	// query is one indexed day's worth of rows and the cost of over-fetching is
	// nothing next to publishing a short list.
	CandidateLimit = 200
)

// DayLayout is how a digest day is written wherever it is shown to a person — in a
// log line, in an error, and as the -day flag's argument. Exported so cmd/social-digest
// spells it the same way this package does rather than repeating the literal.
const DayLayout = "2006-01-02"

// Posting is one job in the digest — a candidate before the editorial rules run, an
// item of the published list after. One type rather than two: the rules select from
// the set, they do not transform its members.
type Posting struct {
	JobID       int64
	Slug        string
	Title       string
	Company     string
	CompanySlug string
	Location    string
	Remote      bool

	// PageUniques is the distinct visitors who opened this posting's page on the
	// digest's day. The bot-filtered signal; the only one safe to rank on.
	PageUniques int
}

// Digest is a finished list, ready to publish. An empty Items means the day had
// nothing worth publishing — a quiet day, not a failure.
type Digest struct {
	// Day is the day the digest DESCRIBES, not the day it is sent. They always differ:
	// the freshest day in the view rollup is a completed day.
	Day   time.Time
	Items []Posting
}

// Empty reports whether this digest has nothing to say.
func (d Digest) Empty() bool { return len(d.Items) == 0 }

// Publisher delivers a digest to one destination. Implementations own the format;
// they do not decide what goes in the list.
type Publisher interface {
	// Name identifies the channel in the ledger and in logs. It is stored, so it must
	// stay stable across releases — the publish-once check reads it back.
	Name() string

	// Render returns exactly what Publish would send. It exists so a dry run can show
	// the real payload rather than a description of it: the failure a dry run is meant
	// to catch is a post that reads badly, and a summary of a post cannot read badly.
	Render(d Digest) (string, error)

	Publish(ctx context.Context, d Digest) error
}

// Repository is the storage socialdigest needs. An interface rather than *db.Queries
// so the rules above can be tested against a fake without a database — the queries
// themselves are covered by the integration tests that exercise real SQL.
type Repository interface {
	// LatestViewDay reports the freshest day the view rollup holds. The bool is false
	// when there is none, which is a broken pipeline rather than an empty day.
	LatestViewDay(ctx context.Context) (time.Time, bool, error)

	// TopPageViewed returns the day's eligible postings ranked by page views, most
	// first, up to limit.
	TopPageViewed(ctx context.Context, day time.Time, limit int) ([]Posting, error)

	// RecentlyDigested returns the job ids published in any channel in the days
	// [since, before) — the quarantine set. `before` is the digest's own day and is
	// exclusive: a digest must not quarantine itself, or a second channel building the
	// day a first one already published would read back its own list and drop it.
	RecentlyDigested(ctx context.Context, since, before time.Time) (map[int64]bool, error)

	// PublishedForChannel reports whether this day already went out on this channel.
	PublishedForChannel(ctx context.Context, day time.Time, channel string) (bool, error)

	// RecordPublished writes the ledger rows for a channel that has just published.
	RecordPublished(ctx context.Context, day time.Time, channel string, items []Posting) error
}

// Failures the caller must tell apart, because one means "nothing to say today" and
// the others mean "the machinery that feeds me is broken".
var (
	// ErrNoViewData means the view rollup has produced nothing at all. Distinct from a
	// quiet day: a quiet day has rows that no posting clears the floor with.
	ErrNoViewData = errors.New("socialdigest: no view data")

	// ErrStaleViewData means the freshest view day is further behind than
	// StaleAfterDays allows. Publishing a list this old would be worse than publishing
	// nothing, because nobody would notice.
	ErrStaleViewData = errors.New("socialdigest: view data is stale")
)

// ResolveDay picks the day to build a digest for.
//
// latest is the freshest day the view rollup holds and hasData says whether there was
// one at all. The day is DISCOVERED rather than computed from now, because
// cmd/rollup-views reads the rotated access log and whether its freshest complete day
// is yesterday or the day before depends on when logrotate runs on the host — an
// assumption that would fail by silently publishing a stale list.
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

// QuarantineSince is the earliest day whose published postings still block a repeat.
// The window it opens runs up to, but does not include, the digest's own day — see
// Repository.RecentlyDigested.
func QuarantineSince(day time.Time) time.Time {
	return truncateDay(day).AddDate(0, 0, -QuarantineDays)
}

func truncateDay(t time.Time) time.Time {
	y, m, d := t.UTC().Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}
