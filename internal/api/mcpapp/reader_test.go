package mcpapp

import "context"

// fakeReader is what every test in this package drives the server with, and its existence
// is the point: the Reader must be satisfiable without a database, a network or Fiber, or
// the tools below it cannot be tested at all. The production implementation lives in
// internal/api/handler, which this package must never import — the dependency runs the
// other way.
type fakeReader struct {
	searchJobs      func(context.Context, SearchInput) (SearchJobsResult, error)
	jobDetail       func(context.Context, string) (JobResult, error)
	searchCompanies func(context.Context, CompanySearchInput) (SearchCompaniesResult, error)
	companyDetail   func(context.Context, string) (CompanyResult, error)
}

func (f fakeReader) SearchJobs(ctx context.Context, in SearchInput) (SearchJobsResult, error) {
	return f.searchJobs(ctx, in)
}

func (f fakeReader) JobDetail(ctx context.Context, slug string) (JobResult, error) {
	return f.jobDetail(ctx, slug)
}

func (f fakeReader) SearchCompanies(ctx context.Context, in CompanySearchInput) (SearchCompaniesResult, error) {
	return f.searchCompanies(ctx, in)
}

func (f fakeReader) CompanyDetail(ctx context.Context, slug string) (CompanyResult, error) {
	return f.companyDetail(ctx, slug)
}

// The assertion the file exists for. A method added to Reader without a fake to answer it
// fails here, at compile time, rather than in whichever test happens to call it first.
var _ Reader = fakeReader{}
