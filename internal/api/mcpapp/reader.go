package mcpapp

import "context"

// Reader is everything this transport needs from the rest of the system: the four tool
// answers, already projected. The search handlers satisfy it.
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
}

// NotFoundError marks the one failure the model must be able to act on: the posting or the
// employer simply is not here, and the next useful move is a search rather than a retry.
//
// Unlike ojcpmcp's, it does not carry a wire code. The audience differs: a conforming agent
// branches on an enum, a language model reads the sentence. What both need is that "absent"
// and "broken" stay distinguishable.
type NotFoundError struct{ What string }

func (e NotFoundError) Error() string { return "no " + e.What + " with that identifier" }
