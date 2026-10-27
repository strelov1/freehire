package mcpapp

import "context"

// Reader is everything this transport needs from the rest of the system: the tool answers,
// already projected. search and fetch need nothing of their own — they adapt SearchJobs and
// JobDetail. The search handlers satisfy it.
//
// It is an interface here rather than a concrete dependency for the same reason ojcpmcp's
// is: this package sits above nothing it can construct. handler reads Postgres, Meilisearch
// and the captured apply forms, and a transport that imported it could not be tested
// without all three.
type Reader interface {
	SearchJobs(ctx context.Context, in SearchInput) (SearchJobsResult, error)
	JobDetail(ctx context.Context, slug string) (JobResult, error)
	SearchCompanies(ctx context.Context, in CompanySearchInput) (SearchCompaniesResult, error)
	CompanyDetail(ctx context.Context, slug string) (CompanyResult, error)
	// CheckJobsOpen answers one state per DISTINCT slug it was given, in the order given. The
	// cap and the de-duplication are this package's, applied before the call.
	CheckJobsOpen(ctx context.Context, slugs []string) ([]JobOpenState, error)
	SimilarJobs(ctx context.Context, slug string, limit int) (SimilarJobsResult, error)
	SalaryStats(ctx context.Context, in SalaryInput) (SalaryStatsResult, error)
	MarketStats(ctx context.Context, in SearchInput) (MarketStatsResult, error)
	// FacetCounts is the catalogue's whole facet distribution, unfiltered, keyed by the
	// search core's facet param — what list_filter_values projects one filter of.
	FacetCounts(ctx context.Context) (map[string]map[string]int64, error)
}

// NotFoundError marks the one failure the model must be able to act on: the posting or the
// employer simply is not here, and the next useful move is a search rather than a retry.
//
// Unlike ojcpmcp's, it does not carry a wire code. The audience differs: a conforming agent
// branches on an enum, a language model reads the sentence. What both need is that "absent"
// and "broken" stay distinguishable.
type NotFoundError struct{ What string }

func (e NotFoundError) Error() string { return "no " + e.What + " with that identifier" }
