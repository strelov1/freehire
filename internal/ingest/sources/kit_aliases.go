package sources

import (
	"time"

	"github.com/strelov1/freehire/internal/ingest/sources/kit"
)

// The adapter kernel moved to sources/kit (#4586). These names keep this package's API and the
// adapters still in it compiling unchanged while the families move out one at a time.
// shortcut: one alias per kernel name; delete each when nothing in sources uses it, and the file
// with the last adapter.

type (
	Job                = kit.Job
	BodyTooLargeError  = kit.BodyTooLargeError
	StatusError        = kit.StatusError
	ChallengeError     = kit.ChallengeError
	JSONGetter         = kit.JSONGetter
	JSONResolvedGetter = kit.JSONResolvedGetter
	StreamGetter       = kit.StreamGetter
	XMLGetter          = kit.XMLGetter
	XMLResolvedGetter  = kit.XMLResolvedGetter
	HTMLGetter         = kit.HTMLGetter
	HTMLResolvedGetter = kit.HTMLResolvedGetter
	TextGetter         = kit.TextGetter
	HeaderTextGetter   = kit.HeaderTextGetter
	LargeTextGetter    = kit.LargeTextGetter
	JSONPoster         = kit.JSONPoster
	HeaderJSONGetter   = kit.HeaderJSONGetter
	HeaderJSONPoster   = kit.HeaderJSONPoster
	CookieReader       = kit.CookieReader
	HeaderFormPoster   = kit.HeaderFormPoster
	FormJSONPoster     = kit.FormJSONPoster
	FormTextPoster     = kit.FormTextPoster
	HTTPClient         = kit.HTTPClient
	CompanyEntry       = kit.CompanyEntry
	ApplicationMethod  = kit.ApplicationMethod
	Source             = kit.Source
	StreamingSource    = kit.StreamingSource
	HydratingSource    = kit.HydratingSource
	RecentSource       = kit.RecentSource
	CoverageGated      = kit.CoverageGated
	DetailGate         = kit.DetailGate
	CompanyDescriber   = kit.CompanyDescriber
	RegionScopedBoards = kit.RegionScopedBoards
)

const (
	DefaultSweepGrace    = kit.DefaultSweepGrace
	ApplyOnSite          = kit.ApplyOnSite
	ApplyExternal        = kit.ApplyExternal
	defaultDetailWorkers = kit.DefaultDetailWorkers
)

func NotFuture(t *time.Time) *time.Time      { return kit.NotFuture(t) }
func ParseDate(s string) *time.Time          { return kit.ParseDate(s) }
func ParseEpochMillis(ms int64) *time.Time   { return kit.ParseEpochMillis(ms) }
func ParseRFC3339(s string) *time.Time       { return kit.ParseRFC3339(s) }
func IsRemote(location string) bool          { return kit.IsRemote(location) }
func UnescapeEncodedHTML(s string) string    { return kit.UnescapeEncodedHTML(s) }
func LenientPercentUnescape(s string) string { return kit.LenientPercentUnescape(s) }

var (
	parseRFC3339             = kit.ParseRFC3339
	parseDate                = kit.ParseDate
	parseLayout              = kit.ParseLayout
	parseEpochMillis         = kit.ParseEpochMillis
	parseEpochSeconds        = kit.ParseEpochSeconds
	parsePubDate             = kit.ParsePubDate
	parseRFC3339OrDate       = kit.ParseRFC3339OrDate
	parseSpaceTime           = kit.ParseSpaceTime
	collapseSpace            = kit.CollapseSpace
	countriesFromCodes       = kit.CountriesFromCodes
	countryFromCode          = kit.CountryFromCode
	detailUnreadable         = kit.DetailUnreadable
	firstNonEmpty            = kit.FirstNonEmpty
	firstSubmatch            = kit.FirstSubmatch
	headedSection            = kit.HeadedSection
	isRateLimited            = kit.IsRateLimited
	isRefusedOrTransient     = kit.IsRefusedOrTransient
	isRemote                 = kit.IsRemote
	joinNonEmpty             = kit.JoinNonEmpty
	retryWhile               = kit.RetryWhile
	trimURLSuffix            = kit.TrimURLSuffix
	unreadableDetail         = kit.UnreadableDetail
	workModeFromRemote       = kit.WorkModeFromRemote
	workModeFromRemoteHybrid = kit.WorkModeFromRemoteHybrid
	workplaceTypeMode        = kit.WorkplaceTypeMode
	isSalaryPeriod           = kit.IsSalaryPeriod
	roundSalaryPart          = kit.RoundSalaryPart
	rubMonthlySalary         = kit.RubMonthlySalary
	encodedTagOpener         = kit.EncodedTagOpener
	liveTagOpener            = kit.LiveTagOpener
	sanitizeHTML             = kit.SanitizeHTML
	unescapeEncodedHTML      = kit.UnescapeEncodedHTML
	errListingAbsent         = kit.ErrListingAbsent
)

func fetchDetails[P any](postings []P, workers int, fetch func(P) (Job, bool)) []Job {
	return kit.FetchDetails(postings, workers, fetch)
}
func hydrationLanes[P any](unseen []P, fresh, backlog int, now time.Time) []P {
	return kit.HydrationLanes(unseen, fresh, backlog, now)
}
func fetchDetailsStream[P any](postings []P, workers int, fetch func(P) (Job, bool), emit func(Job)) {
	kit.FetchDetailsStream(postings, workers, fetch, emit)
}
func distinctJoin[T any](items []T, sep string, label func(T) string) string {
	return kit.DistinctJoin(items, sep, label)
}
