package handler

import (
	"context"
	"errors"
	"fmt"
	"log"
	"maps"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/api/apigen"
	"github.com/strelov1/freehire/internal/api/ojcp"
	"github.com/strelov1/freehire/internal/application/agentview"
	"github.com/strelov1/freehire/internal/dict/skillvec"
	"github.com/strelov1/freehire/internal/engage/companywatch"
	"github.com/strelov1/freehire/internal/identity/auth"
	"github.com/strelov1/freehire/internal/identity/userprofile"
	"github.com/strelov1/freehire/internal/job/formeremployer"
	"github.com/strelov1/freehire/internal/job/jobview"
	"github.com/strelov1/freehire/internal/platform/cache"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/search/search"
	"github.com/strelov1/freehire/internal/search/suggest"
)

// searchHandlers serves the Meilisearch-backed job search surfaces: the keyword
// search, the agent search (always full descriptions in a selectable format), the
// similar-jobs read, and the facet distribution. search/facets are two narrow
// views of the same client, kept separate so the concerns stay decoupled; both
// nil when Meilisearch is unconfigured (the endpoints then report 503).
type searchHandlers struct {
	search searcher
	// descriptions loads full job descriptions by internal id, to rehydrate the
	// truncated search preview on the agent search endpoint. *db.Queries in
	// production; a fake in tests.
	descriptions jobDescriptions
	queries      *db.Queries
	facets       facetCounter
	// companyNames names the top companies of a company_slug facet request.
	// *db.Queries in production; nil names each company by its slug.
	companyNames companyNamer
	// cache holds facet distributions for facetCacheTTL. Best-effort and optional:
	// a nil cache (search configured but no Redis) simply recomputes every request,
	// the shape every facet test but the caching ones runs under.
	cache cache.Cache
	// userProfile reads the caller's profile skills for the match sort. Nil disables
	// that sort entirely, which is exactly how it degrades everywhere else.
	userProfile profileReader
	// agentViews receives the postings /agent/jobs/search serves; nil records nothing.
	agentViews agentview.Sink
	// appliedCompanies reads the employers a signed-in caller has applied to, for
	// /jobs/search's hide_applied_companies. Nil leaves the param unhonoured, and
	// reported as such.
	appliedCompanies appliedCompanyReader
	// formerEmployers resolves a signed-in caller's experience bank to the companies the
	// "Hiring again" filter narrows to. Nil leaves former_employers unread, so it is
	// reported in meta.ignored_params like any other param nothing reads.
	formerEmployers formerEmployerLookup
	// watchlist reads a signed-in caller's company watchlist, which `watchlist=true`
	// narrows to. Nil leaves the param unread, and reported in meta.ignored_params.
	watchlist watchlistLookup
	// seen reads the jobs a signed-in caller has already opened, for /jobs/search's
	// hide_seen. Nil leaves the param unhonoured, and reported as such.
	seen seenJobs
	// removals reads which of a page's jobs are already closed but still indexed. Nil
	// skips the check, which is how a search without a database behind it runs.
	removals pendingRemovals
}

// pendingRemovals names the jobs, among those given, whose removal from the index is
// queued but not yet applied. *db.Queries satisfies it.
type pendingRemovals interface {
	ListPendingSearchRemovals(ctx context.Context, jobIds []int64) ([]int64, error)
}

// appliedCompanyReader reads the company slugs of the employers a caller has applied
// to. *jobtracking.Service satisfies it.
type appliedCompanyReader interface {
	AppliedCompanySlugs(ctx context.Context, userID int64) ([]string, error)
}

// seenJobs reads the job ids a caller has already opened. It is the same set the feed
// dims a card by — every user_jobs row carries viewed_at, so any interaction is a view —
// capped to the most recently touched. *jobtracking.Service satisfies it.
type seenJobs interface {
	ExcludedJobIDs(ctx context.Context, userID int64) ([]int64, error)
}

// formerEmployerLookup resolves a person's past employers to company slugs.
// *formeremployer.Store satisfies it; tests inject a fake.
type formerEmployerLookup interface {
	Slugs(ctx context.Context, userID int64) ([]string, error)
}

// watchlistLookup reads a person's watchlist as company slugs. *companywatch.Service
// satisfies it; tests inject a fake.
type watchlistLookup interface {
	Slugs(ctx context.Context, userID int64) ([]string, error)
}

// formerEmployersParam is the "Hiring again" filter: only postings at companies the
// signed-in caller has worked at and left, per their experience bank. It is not part of
// the search package's vocabulary because it is not a facet of the posting — it is a fact
// about the CALLER, resolved per request — so the endpoint reads it itself, and declares it
// as its own only when it did (see jobSearchOutcome.own).
const formerEmployersParam = "former_employers"

// profileReader loads a caller's profile. *userprofile.Service satisfies it; tests
// inject a fake. Only the skills are read here — the match sort has no opinion on
// location preferences or specializations.
type profileReader interface {
	Get(ctx context.Context, userID int64) (userprofile.Profile, error)
}

func newSearchHandlers(search searcher, facets facetCounter, queries *db.Queries, c cache.Cache, profiles profileReader) *searchHandlers {
	h := &searchHandlers{
		search: search, descriptions: queries, queries: queries, facets: facets, cache: c,
		userProfile: profiles,
	}
	// Assign only when set: a nil *db.Queries in the interface would be non-nil.
	if queries != nil {
		h.companyNames = queries
		h.formerEmployers = formeremployer.NewStore(queries)
		h.watchlist = companywatch.New(queries)
	}
	return h
}

// searchRecordTimeout bounds the fire-and-forget demand write. Short: it is one
// upsert on a small table, and a write still running past this is a database in
// trouble rather than a slow row.
const searchRecordTimeout = 5 * time.Second

func (h *searchHandlers) register(api fiber.Router, mw middleware, contract fiber.Handler) {
	// Literal routes are registered before the /jobs/:slug param route (see
	// Register) so they are not read as slugs.
	api.Get("/jobs/search", mw.optional, contract)
	// Agent search: same query as /jobs/search, but always full descriptions in a
	// selectable format for programmatic consumers. Public, like the other reads.
	api.Get("/agent/jobs/search",
		agentViews(h.agentViews, agentview.SurfaceAgentAPI), contract)
	api.Get("/jobs/facets", contract)
	api.Get("/jobs/:slug/similar", contract)
}

// searcher is the search backend the handler depends on. *search.Client
// satisfies it; tests inject a fake. A nil searcher means search is not
// configured (no MEILI_MASTER_KEY) and search endpoints report 503. Similar jobs
// use the same Search method with mandatory location and specialization filters.
type searcher interface {
	Search(ctx context.Context, p search.SearchParams) (search.SearchResult, error)
}

// The pagination guard the search endpoints apply is maxPageWindow (handler.go), which
// they no longer own: it used to be a `maxSearchWindow` const here, and the Postgres-backed
// lists carried no equivalent at all — the asymmetry the 2026-09-14 deep-offset outage went
// through. It stays decoupled from the index's maxTotalHits (which now only sets how high
// the reported total may count), so the total can read the true filtered count while deep
// offset paging — the expensive part — stays refused.

// searchParams are the query params the search endpoints read themselves rather
// than hand to the filter: the query text, the sort directive and the pagination
// window. search.UnknownParams owns the filter vocabulary and nothing else, so
// each endpoint declares its own transport params beside itself (see
// facetsParams in facets.go, companiesParams in companies.go).
var searchParams = []string{"q", "sort", "order", "limit", "offset"}

// jobSearchParams is searchParams plus q_fields, the field-scoping parameter
// only the two job-search endpoints (not the swipe deck, which shares
// searchParams but never reads q_fields) understand.
var jobSearchParams = slices.Concat(searchParams, []string{"q_fields"})

// hideAppliedCompaniesParam asks /jobs/search to leave out the postings of every
// employer the caller has applied to. It is a per-person preference, not a filter:
// search.FilterFromValues does not read it, so a saved search carrying it narrows
// nothing and its digest ignores it. Only the public /jobs/search reads it; the agent
// search reports it like any other unread param.
const hideAppliedCompaniesParam = "hide_applied_companies"

// hideSeenParam asks /jobs/search to leave out the jobs the caller has already opened.
// It is a per-person preference, not a filter: search.FilterFromValues does not read it,
// so a saved search carrying it narrows nothing and its digest ignores it. Only the
// public /jobs/search reads it; the agent search reports it like any other unread param.
const hideSeenParam = "hide_seen"

// publicJobSearchParams is what /jobs/search itself reads beyond the filter vocabulary.
var publicJobSearchParams = slices.Concat(jobSearchParams, []string{hideSeenParam, hideAppliedCompaniesParam, minSkillMatchParam})

// agentSearchParams is jobSearchParams plus the agent endpoint's own two: the
// response-format selector and the opt-in semantic search.
var agentSearchParams = slices.Concat(jobSearchParams, []string{"description_format", "semantic"})

// ignoredParams reports the query params of this request that neither the filter
// nor the endpoint itself reads. They are echoed in the response meta instead of
// being refused: rejecting them would break saved searches and shared links that
// still carry retired params, while staying silent is what let a mistyped
// `country=it` pass for a search of Italy.
func ignoredParams(vals url.Values, own []string) []search.UnknownParam {
	return search.UnknownParams(vals, own)
}

// mergedJobSearchIgnored combines runJobSearch's own value-level ignored-param
// report — q_fields naming a field outside the searchable vocabulary, and/or any
// filter params the Meilisearch-rejection degrade dropped — with the ordinary
// name-level ignoredParams(vals, own) report, into one sorted, capped list. The same
// merge both job-search endpoints need, named once so neither can apply it
// slightly differently.
func mergedJobSearchIgnored(vals url.Values, own []string, fromSearch []search.UnknownParam) []search.UnknownParam {
	return search.SortAndCap(append(fromSearch, ignoredParams(vals, own)...))
}

// searchSortable is the allowlist of sort params mapped to their index attribute;
// anything else is ignored so a bad param cannot make Meilisearch reject the query.
var searchSortable = map[string]string{
	"created_at": "created_at",
	"posted_at":  "posted_at",
	// The "Most viewed" ordering. This entry must NEVER ship ahead of the live index
	// declaring view_count sortable (see search.facetSettings, and "Adding a sortable
	// attribute" in internal/search/search/AGENTS.md): Meilisearch rejects the entire
	// query for an undeclared sort attribute, and that package maps any Meili error to
	// a 500 — so the wrong deploy order breaks the page rather than ignoring the param.
	"view_count": "view_count",
	"salary_min": "enrichment.salary_min",
	"salary_max": "enrichment.salary_max",
}

// sortMatch is the profile-match sort. Unlike every value in searchSortable it names
// no index attribute: it ranks by cosine against the caller's own skill vector, so it
// is resolved from the request rather than looked up in that table.
const sortMatch = "match"

// matchVector builds the caller's skill vector for ?sort=match, or nil when the sort
// was not asked for or cannot be served.
//
// EVERY "cannot" degrades to the default feed: no session, no profile service wired,
// no profile, no skills, no recognised skills, no weights. A saved search or a shared
// link carrying sort=match must never 400 when opened by someone it cannot be served
// for — the same reason the jobs list ignores unknown filters rather than refusing
// them.
func (h *searchHandlers) matchVectorFor(ctx context.Context, userID int64, vals url.Values) []float32 {
	vector, _ := h.matchProfileFor(ctx, userID, vals)
	return vector
}

// matchProfileFor is matchVectorFor plus the profile skills the vector was built from,
// which the minimum-skill-match scan needs to state each posting's percent.
func (h *searchHandlers) matchProfileFor(ctx context.Context, userID int64, vals url.Values) (vector []float32, skills []string) {
	if vals.Get("sort") != sortMatch || h.userProfile == nil {
		return nil, nil
	}
	// auth.UserID is what mw.optional populates; it reports false for an anonymous
	// caller rather than erroring. requireUserID is the wrong helper here — its 401 is
	// exactly the outcome this must avoid.
	if userID == 0 {
		return nil, nil
	}
	profile, err := h.userProfile.Get(ctx, userID)
	if err != nil || len(profile.Skills) == 0 {
		return nil, nil
	}
	// ProfileVector, never JobVector: the ballast belongs to the stored side only.
	return skillvec.ProfileVector(profile.Skills), profile.Skills
}

// SearchJobs runs a full-text/faceted keyword search over the jobs index. It is
// public (unauthenticated) like the other job reads. Response: {"data": [job
// view...], "meta": {total, limit, offset}} — results carry public_slug and never
// the internal id.
func (h *searchHandlers) SearchJobs(ctx context.Context, _ apigen.SearchJobsParams) (*apigen.JobSearchPageHeaders, error) {
	vals := contractQuery(ctx)
	userID, _ := auth.UserIDFromContext(ctx)
	seen, seenIgnored := h.seenExclusionFor(ctx, userID, vals)
	appliedFrag, appliedIgnored := h.appliedCompanyExclusionFor(ctx, userID, vals)
	match, matchIgnored := h.skillMatchFilterFor(ctx, userID, vals)
	// The answer depends on who asked, under a URL that does not say so.
	private := match != nil ||
		(userID != 0 && !seenIgnored && wantsParam(vals, hideSeenParam)) ||
		(userID != 0 && !appliedIgnored && wantsParam(vals, hideAppliedCompaniesParam))
	out, err := h.runJobSearch(ctx, vals, userID, false, seen, appliedFrag, match)
	if err != nil {
		return nil, err
	}
	res, limit, offset, dropped := out.res, out.limit, out.offset, out.dropped
	if appliedIgnored {
		dropped = append(dropped, search.UnknownParam{Param: hideAppliedCompaniesParam})
	}
	if seenIgnored {
		dropped = append(dropped, search.UnknownParam{Param: hideSeenParam})
	}
	if matchIgnored {
		dropped = append(dropped, search.UnknownParam{Param: minSkillMatchParam})
	}

	views := make([]jobview.Job, len(res.Hits))
	for i, hit := range res.Hits {
		views[i] = hit.Job
	}
	h.attachGhost(ctx, res.Hits, views)
	data, err := marshalRaws(views)
	if err != nil {
		return nil, err
	}

	ignored := mergedJobSearchIgnored(vals, out.own(publicJobSearchParams), dropped)
	meta := apigen.JobSearchMeta{
		Total: res.Total, Limit: int64(limit), Offset: int64(offset),
		IgnoredParams: contractIgnoredParams(ignored),
		IgnoredValues: contractIgnoredValues(ignoredJobValues(vals)),
	}
	if res.TotalIsLowerBound {
		meta.TotalIsLowerBound = apigen.NewOptBool(true)
	}
	return &apigen.JobSearchPageHeaders{
		CacheControl: optString(privateCacheControl(private || out.private)),
		Response:     apigen.JobSearchPage{Data: data, Meta: meta},
	}, nil
}

// privateCacheControl is the header of an answer that depends on who asked, or "" for one
// that does not.
func privateCacheControl(private bool) string {
	if private {
		return "private, no-store"
	}
	return ""
}

// wantsParam reads a boolean query param the way Fiber's QueryBool does: the first value,
// false when it is absent or not a boolean.
func wantsParam(vals url.Values, key string) bool {
	want, err := strconv.ParseBool(vals.Get(key))
	return err == nil && want
}

// appliedCompanyExclusionFor resolves ?hide_applied_companies=true into the filter
// fragment that keeps the caller's applied-to employers out of this search, and
// reports whether the param has to be named in meta.ignored_params instead.
//
// The exclusion runs inside the index query rather than over the returned page, so
// meta.total and every later page stay honest: dropping hits after the fact would
// shrink a page below its limit and leave the total counting what the caller never
// sees.
//
// It is honoured for a signed-in caller only, because the set is that account's own
// applications. An anonymous caller, an unparseable value, or a record that could not
// be read all fall back to the unhidden search and say so, rather than refusing a
// shared link or quietly widening. A caller with no applications gets "" — nothing to
// exclude, and nothing ignored either.
func (h *searchHandlers) appliedCompanyExclusionFor(ctx context.Context, userID int64, vals url.Values) (frag string, ignored bool) {
	raw := vals.Get(hideAppliedCompaniesParam)
	if !vals.Has(hideAppliedCompaniesParam) {
		return "", false
	}
	want, err := strconv.ParseBool(raw)
	if err != nil {
		return "", true
	}
	if !want {
		return "", false
	}
	if userID == 0 || h.appliedCompanies == nil {
		return "", true
	}
	slugs, err := h.appliedCompanies.AppliedCompanySlugs(ctx, userID)
	if err != nil {
		log.Printf("search: hide_applied_companies: read applied companies of user %d: %v", userID, err)
		return "", true
	}
	// The answer now depends on who asked, under a URL that does not say so.
	if len(slugs) == 0 {
		return "", false
	}
	return search.NotInStrings("company_slug", slugs), false
}

// seenExclusionFor resolves ?hide_seen=true into the job ids to keep out of this search,
// and reports whether the param has to be named in meta.ignored_params instead.
//
// The exclusion happens inside the index query rather than over the returned page, so
// meta.total and every later page stay honest: hiding hits after the fact would shrink
// a page below its limit and leave the total counting what the caller never sees.
//
// It is honoured for a signed-in caller only, because "seen" is the server's record of
// what that account opened — the same record the feed dims cards by. An anonymous
// caller, an unparseable value, or a record that could not be read all fall back to the
// unhidden search and say so, rather than refusing a shared link or quietly widening.
func (h *searchHandlers) seenExclusionFor(ctx context.Context, userID int64, vals url.Values) (ids []int64, ignored bool) {
	if !vals.Has(hideSeenParam) {
		return nil, false
	}
	// A present but empty `?hide_seen=` is unreadable like any other non-boolean.
	want, err := strconv.ParseBool(vals.Get(hideSeenParam))
	if err != nil {
		return nil, true
	}
	if !want {
		return nil, false
	}
	if userID == 0 || h.seen == nil {
		return nil, true
	}
	ids, err = h.seen.ExcludedJobIDs(ctx, userID)
	if err != nil {
		log.Printf("search: hide_seen: read seen jobs of user %d: %v", userID, err)
		return nil, true
	}
	// The answer now depends on who asked, under a URL that does not say so.
	return ids, false
}

// jobSearchOutcome is what runJobSearch hands back: the raw hits, the applied
// limit/offset, every value-level ignored-param report gathered while building the
// query, and — only when the caller asked for semantic search and it could apply —
// that it was answered by keywords instead (ojcp.SemanticUnavailable).
type jobSearchOutcome struct {
	res      search.SearchResult
	limit    int
	offset   int
	dropped  []search.UnknownParam
	semantic string
	// formerEmployers reports that the "Hiring again" filter was applied, which makes
	// former_employers one of the endpoint's own params rather than an ignored one.
	formerEmployers bool
	// watchlist is the same for `watchlist=true`.
	watchlist bool
	// private is set by runJobSearch when either of those was applied: the answer is
	// one person's own.
	private bool
}

// own is the endpoint's own params for this request: base, plus former_employers and
// watchlist when the filter read them. Anonymous, they were not read, and the report must
// say so — the answer then is the whole catalogue, which a caller who asked for their former
// employers or their watchlist must not mistake for them.
func (o jobSearchOutcome) own(base []string) []string {
	out := slices.Clip(base)
	if o.formerEmployers {
		out = append(out, formerEmployersParam)
	}
	if o.watchlist {
		out = append(out, search.WatchlistParam)
	}
	return out
}

// formerEmployerSlugsFor resolves the "Hiring again" filter for this request. applied is false
// when the param is absent or false, or when nothing can read it — no session, no lookup
// wired — and the caller then searches as if it were absent (and reports it, see own).
// Applied with no slugs is a real answer: nothing banked resolves to a hiring company.
func (h *searchHandlers) formerEmployerSlugsFor(ctx context.Context, userID int64, vals url.Values) (slugs []string, applied bool, err error) {
	if h.formerEmployers == nil || vals.Get(formerEmployersParam) != "true" {
		return nil, false, nil
	}
	if userID == 0 {
		return nil, false, nil
	}
	// The answer is this person's own history; no shared cache may keep it.
	slugs, err = h.formerEmployers.Slugs(ctx, userID)
	if err != nil {
		return nil, false, err
	}
	return slugs, true, nil
}

// watchlistSlugsFor resolves `watchlist=true` for this request, with formerEmployerSlugs's
// contract: applied is false when the param is absent, anonymous or unreadable, and applied
// with no slugs is a real answer — an empty list — that narrows to nothing.
func (h *searchHandlers) watchlistSlugsFor(ctx context.Context, userID int64, vals url.Values) (slugs []string, applied bool, err error) {
	if h.watchlist == nil || !search.WantsWatchlist(vals) || userID == 0 {
		return nil, false, nil
	}
	slugs, err = h.watchlist.Slugs(ctx, userID)
	if err != nil {
		return nil, false, err
	}
	return slugs, true, nil
}

// personalCompanies combines the two per-caller company sets a search can be narrowed to:
// the companies the caller left ("Hiring again") and the ones they watch. Asked for both,
// a posting must satisfy both, so the set is their intersection. applied is false when
// neither was asked for.
func personalCompanies(former []string, hasFormer bool, watch []string, hasWatch bool) (slugs []string, applied bool) {
	switch {
	case hasFormer && hasWatch:
		return slices.DeleteFunc(slices.Clone(former), func(s string) bool { return !slices.Contains(watch, s) }), true
	case hasFormer:
		return former, true
	case hasWatch:
		return watch, true
	}
	return nil, false
}

// runJobSearch performs the request handling shared by both job-search endpoints:
// the availability check, the pagination-window guard, and the index query. It is
// the single place the query is built, so the public and agent search endpoints
// cannot drift. The availability and deep-pagination guards return a fiber *Error
// the caller can return directly; on success `dropped` carries q_fields naming a
// field outside the searchable vocabulary, plus any filter params dropped by the
// degrade below (nil in the ordinary case), for the caller to merge into its own
// ignoredParams(vals, own) report.
//
// wantSemantic is the caller's semantic flag, which is never honoured (see
// semanticOutcome); one with nothing to apply to is reported as a dropped param.
// exclude names job ids to keep out of the result (hide_seen); nil excludes nothing. personal is the applied-company
// exclusion fragment; an empty fragment leaves the result unchanged. match, when set, keeps
// only the postings at or above the caller's minimum skill match (see skillMatchFilter).
func (h *searchHandlers) runJobSearch(ctx context.Context, vals url.Values, userID int64, wantSemantic bool, exclude []int64, personal string, match *skillMatchFilter) (jobSearchOutcome, error) {
	if h.search == nil {
		return jobSearchOutcome{}, fiber.NewError(fiber.StatusServiceUnavailable, "search is not available")
	}

	limit, offset, err := contractPageWindowed(ctx, defaultLimit, maxLimit)
	if err != nil {
		return jobSearchOutcome{}, err
	}

	// QueryBool historically accepts the alternate strconv boolean spellings.
	formerVals := vals
	if wantsParam(vals, formerEmployersParam) {
		formerVals = maps.Clone(vals)
		formerVals.Set(formerEmployersParam, "true")
	}
	former, formerApplied, err := h.formerEmployerSlugsFor(ctx, userID, formerVals)
	if err != nil {
		return jobSearchOutcome{}, err
	}
	// Only the spelling search.WantsWatchlist reads, the one Narrows and cmd/notify read too.
	watch, watchApplied, err := h.watchlistSlugsFor(ctx, userID, vals)
	if err != nil {
		return jobSearchOutcome{}, err
	}
	out, err := h.executeJobSearch(ctx, vals, jobSearchInput{limit: limit, offset: offset, former: former, hasFormer: formerApplied, watch: watch, hasWatch: watchApplied, vector: h.matchVectorFor(ctx, userID, vals), exclude: exclude, personal: personal, wantSemantic: wantSemantic, match: match})
	// Both are one person's own sets; no shared cache may keep the answer.
	out.private = formerApplied || watchApplied
	return out, err
}

type jobSearchInput struct {
	limit, offset        int
	former               []string
	hasFormer            bool
	watch                []string
	hasWatch             bool
	vector               []float32
	exclude              []int64
	personal             string
	wantSemantic, strict bool
	match                *skillMatchFilter
}

func (h *searchHandlers) executeJobSearch(ctx context.Context, vals url.Values, in jobSearchInput) (jobSearchOutcome, error) {
	limit, offset := in.limit, in.offset
	vector, exclude, personal, wantSemantic, strict := in.vector, in.exclude, in.personal, in.wantSemantic, in.strict

	companies, companiesApplied := personalCompanies(in.former, in.hasFormer, in.watch, in.hasWatch)
	if companiesApplied && len(companies) == 0 {
		// Narrow to nothing rather than drop the filter: the set the caller asked for is
		// empty, and searching without it would answer with strangers.
		return jobSearchOutcome{limit: limit, offset: offset, formerEmployers: in.hasFormer, watchlist: in.hasWatch}, nil
	}

	sort := searchSortValues(vals, vector != nil)
	base := search.FilterFromValues(vals)
	// companyFilter is the personal company group alone, kept for the degrade below.
	filter, companyFilter := base, any(nil)
	if companiesApplied {
		filter = search.AndInStrings(base, "company_slug", companies)
		companyFilter = search.AndInStrings(nil, "company_slug", companies)
	}
	qFields, qFieldsIgnored := search.QFieldsFromValues(vals)
	params := search.SearchParams{
		Query:   vals.Get("q"),
		Filter:  withExcludedIDs(withFilterGroup(filter, personal), exclude),
		Sort:    sort,
		Vector:  vector,
		QFields: qFields,
		Limit:   limit,
		Offset:  offset,
	}

	if strict && len(qFieldsIgnored) > 0 {
		return jobSearchOutcome{}, fmt.Errorf("search fields cannot be enforced")
	}
	dropped := qFieldsIgnored
	var (
		res     search.SearchResult
		outcome semanticOutcome
		err     error
	)
	if in.match != nil {
		res, err = in.match.scan(ctx, h.search, params)
	} else {
		res, err = h.search.Search(ctx, params)
		outcome = semanticOutcomeFor(params, wantSemantic)
	}
	if outcome == semanticDropped {
		dropped = append(dropped, search.UnknownParam{Param: "semantic"})
	}

	if err != nil {
		// Degrade, not fail, when the filter itself is why Meilisearch refused the
		// query — the deploy window a filterable attribute is declared in code
		// before the live index catches up (AGENTS.md's "Adding a filterable
		// attribute") is the realistic trigger, not a malformed client value
		// (every facet/scalar value is already escaped before it reaches a filter
		// expression — see search.quote). base == nil means there is nothing to
		// blame — retrying an unchanged query would just fail again identically.
		// The personal company group ("Hiring again", the watchlist) is not blamed and survives the retry: company_slug
		// has been filterable since the index existed, and dropping it would answer a
		// request for former employers with the whole catalogue.
		if strict || base == nil || !errors.Is(err, search.ErrBadQuery) {
			// RenderError renders a generic 500 (or 400 for ErrBadQuery); returning
			// the error keeps the failure cause visible to logging instead of
			// swallowing it.
			return jobSearchOutcome{}, err
		}
		// Both personal exclusions survive the retry: id and company_slug have always
		// been filterable, so an undeclared facet cannot justify dropping either.
		retry := search.SearchParams{
			Query: vals.Get("q"), Filter: withExcludedIDs(withFilterGroup(companyFilter, personal), exclude),
			Sort: sort, Vector: vector, QFields: qFields, Limit: limit, Offset: offset,
		}
		if in.match != nil {
			res, err = in.match.scan(ctx, h.search, retry)
		} else {
			res, err = h.search.Search(ctx, retry)
		}
		if err != nil {
			return jobSearchOutcome{}, err
		}
		// Which single param Meilisearch minded is not knowable from its own
		// error, so every active filter param is reported rather than a guess.
		dropped = append(dropped, search.ActiveFilterParams(vals)...)
	}

	res = h.dropPendingRemovals(ctx, res)
	h.recordQuery(vals.Get("q"))
	return jobSearchOutcome{
		res: res, limit: limit, offset: offset, dropped: dropped,
		semantic:        outcome.wire(ojcp.SemanticUnavailable),
		formerEmployers: in.hasFormer,
		watchlist:       in.hasWatch,
	}, nil
}

// pendingRemovalsTimeout bounds the lookup dropPendingRemovals makes. It is a primary-key
// read over one page of ids, so a healthy answer takes milliseconds.
const pendingRemovalsTimeout = 300 * time.Millisecond

// dropPendingRemovals takes out of a page the postings that have closed but are still in
// the index. The drain applies a closure asynchronously, and a facet rebuild pauses it for
// hours, so the queue it works from (search_delete_outbox) is exactly the set the index
// still shows wrongly. Asking it about the page's own ids is a primary-key lookup; asking the
// index to exclude the whole queue is not possible, it can hold a six-figure number of ids.
//
// This is the one place a hit is dropped after the query, which the hide_applied_companies
// note above avoids because it shrinks the page and leaves the total counting what the
// caller never sees. Here the dropped hit is by definition not a result, so the total is
// lowered by the same count; what stays over-counted is the part of the backlog on later
// pages, and it is gone once the drain catches up.
//
// Best-effort: a failed lookup logs and serves the page as the index has it, as the search
// did before this check existed.
func (h *searchHandlers) dropPendingRemovals(ctx context.Context, res search.SearchResult) search.SearchResult {
	if h.removals == nil || len(res.Hits) == 0 {
		return res
	}
	ids := make([]int64, len(res.Hits))
	for i, hit := range res.Hits {
		ids[i] = hit.ID
	}
	// An optional check must not hold the search behind a saturated pool or a stalled query.
	ctx, cancel := context.WithTimeout(ctx, pendingRemovalsTimeout)
	defer cancel()
	gone, err := h.removals.ListPendingSearchRemovals(ctx, ids)
	if err != nil {
		log.Printf("search: read pending removals: %v", err)
		return res
	}
	if len(gone) == 0 {
		return res
	}
	closed := make(map[int64]struct{}, len(gone))
	for _, id := range gone {
		closed[id] = struct{}{}
	}
	kept := make([]search.JobDocument, 0, len(res.Hits))
	for _, hit := range res.Hits {
		if _, ok := closed[hit.ID]; !ok {
			kept = append(kept, hit)
		}
	}
	res.Total = max(res.Total-int64(len(res.Hits)-len(kept)), 0)
	res.Hits = kept
	return res
}

// recordQuery notes what the catalogue was asked for, so the suggestion dictionary can
// rank by demand rather than only by supply.
//
// Three properties, all deliberate:
//
//   - It CANNOT fail the search. A write error is logged and discarded: the result is
//     what the visitor asked for and this is a by-product.
//   - It CANNOT delay the response. It runs on its own goroutine with its own bounded
//     context, because c.Context() is released when the handler returns.
//   - It identifies nobody. The row is the phrase and a counter; no user, no session,
//     no address — and `Recordable` refuses text that is not a search phrase at all,
//     so a pasted email address or job description never reaches the table.
//
// The key is the same normalisation the builder applies to mined titles, so a typed
// query and the title it names land on one row.
func (h *searchHandlers) recordQuery(raw string) {
	q := demandKey(raw)
	if !suggest.Recordable(q) || h.queries == nil {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), searchRecordTimeout)
		defer cancel()
		if err := h.queries.RecordSearchQuery(ctx, q); err != nil {
			log.Printf("search: record query: %v", err)
		}
	}()
}

// demandKey normalises a raw `q` into the same key suggest.Title gives a mined
// posting title. A title suggestion's click sends its query wrapped in a matching
// pair of `"` (see the search-suggestions spec) — that wrapper is Meilisearch's own
// quoting syntax, not part of the phrase, so it is stripped before normalising.
// Anything else — no quotes, or a quote at only one end — is not that shape and is
// left alone: guessing would rewrite text nobody asked to rewrite.
func demandKey(raw string) string {
	if len(raw) >= 2 && strings.HasPrefix(raw, `"`) && strings.HasSuffix(raw, `"`) {
		raw = raw[1 : len(raw)-1]
	}
	return suggest.Title(raw)
}

// defaultSort is the order when the caller names none: an empty query has no relevance to
// rank by, so the freshest first; a text query keeps relevance order (nil).
func defaultSort(query string) []string {
	if query == "" {
		return []string{"posted_at:desc"}
	}
	return nil
}

// searchSortValues builds the Meilisearch sort directive from ?sort=<field>&order=<dir>.
// Without a valid sort param, a no-text browse defaults to the freshest postings
// first (posted_at desc) — relevance is meaningless for an empty query — while a
// text query keeps relevance order (nil).
//
// ranked reports that the request carries a match vector. Meilisearch lets an explicit
// sort directive take precedence over vector ranking, so returning one here would
// silently discard the match order the caller asked for — not error, just quietly
// serve something else.
func searchSortValues(vals url.Values, ranked bool) []string {
	if ranked {
		return nil
	}
	attr, ok := searchSortable[vals.Get("sort")]
	if !ok {
		return defaultSort(vals.Get("q"))
	}
	order := vals.Get("order")
	if order != "asc" && order != "desc" {
		order = "desc"
	}
	return []string{attr + ":" + order}
}

// buildSearchFilter turns the request's facet query params into a Meilisearch
// filter by delegating to the shared, pure search.FilterFromValues — the same
// translation the notification matcher applies to a saved search's stored query,
// so the two cannot drift. Returns nil when no facet is set.
func buildSearchFilter(vals url.Values) any {
	return search.FilterFromValues(vals)
}

// attachGhost attaches the ghost signal to a page of search hits.
//
// Search is the surface where job cards are actually browsed, so leaving it out
// would mean the badge existed almost nowhere. It costs two extra reads per page:
// the outcome evidence (sparse — only jobs anyone reported or applied to) and the
// absence stamps.
//
// The stamps have to come from Postgres because they cannot be in the index:
// cmd/reindex pushes only documents whose content_hash moved, and no adapter writes
// ats_absent_at, so the column would never reach Meilisearch on its own. That is the
// same trap is_tech already fell into. The reality class, by contrast, IS on the
// document (search.FromJob promotes it), so it is read from the hit rather than
// recomputed.
//
// Best-effort: a failed lookup leaves the signal off the page rather than failing
// the search — but not silently. These stamps are a structural tier of this path,
// so a persistent failure here is the badge disappearing from every search result
// while the endpoint keeps answering 200; without the log there is nothing to see
// it by (see logGhostLookup for why cancellation is told apart on the context).
func (h *searchHandlers) attachGhost(ctx context.Context, hits []search.JobDocument, views []jobview.Job) {
	if len(hits) == 0 || h.queries == nil {
		return
	}
	ids := make([]int64, len(hits))
	for i, hit := range hits {
		ids[i] = hit.ID
	}

	stampRows, err := h.queries.ListJobGhostStamps(ctx, ids)
	if err != nil {
		logGhostLookup(ctx, "absence stamps", len(ids), err)
		return
	}
	stamps := make(map[int64]db.ListJobGhostStampsRow, len(stampRows))
	for _, r := range stampRows {
		stamps[r.ID] = r
	}
	evidence := ghostEvidenceFor(ctx, h.queries, ids)

	now := time.Now()
	for i, hit := range hits {
		realityClass := ""
		if hit.Reality != nil {
			realityClass = hit.Reality.Class
		}
		row := stamps[hit.ID]
		views[i].Ghost = jobview.ClassifyGhost(jobview.GhostInput{
			Now: now,
			// Read from Postgres, not from the hit: a sweep-closed job stays in the
			// index until a reindex, whose timer is disabled, so the index cannot be
			// trusted to say a posting is still up. Without this a warning would
			// appear on postings already taken down.
			Closed:       row.ClosedAt.Valid,
			RealityClass: realityClass,
			ATSAbsentAt:  row.AtsAbsentAt.Time,
			HasATSAbsent: row.AtsAbsentAt.Valid,
			Evidence:     evidence[hit.ID],
		})
	}
}

// withFilterGroup ANDs one more fragment onto buildSearchFilter's result — either a
// [][]string of facet groups or nil. An empty fragment leaves the filter untouched.
func withFilterGroup(base any, frag string) any {
	if frag == "" {
		return base
	}
	groups, _ := base.([][]string) // nil base → nil groups; append allocates
	return append(groups, []string{frag})
}
